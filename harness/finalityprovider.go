package harness

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"
	"time"
)

type BlockInfo struct {
	Height    uint64
	Hash      []byte
	Finalized bool
}

type FinalityProviderManager struct {
	tm                *TestManager
	client            *SenderWithBabylonClient
	finalityProviders []*FinalityProviderInstance
	logger            *zap.Logger
	localEOTS         *eotsmanager.LocalEOTSManager
	passphrase        string
	fpCount           int
	homeDir           string
	eotsDb            string
}
type FinalityProviderInstance struct {
	btcPk       *bbntypes.BIP340PubKey
	proofStore  *lib.PubRandProofStore
	fpAddr      sdk.AccAddress
	pop         *bstypes.ProofOfPossessionBTC
	client      *SenderWithBabylonClient
	votedBlocks map[uint64]struct{}
}

func NewFinalityProviderManager(
	tm *TestManager,
	client *SenderWithBabylonClient,
	logger *zap.Logger,
	fpCount int,
	homeDir string,
	eotsDir string,
) *FinalityProviderManager {
	return &FinalityProviderManager{
		tm:      tm,
		client:  client,
		logger:  logger,
		fpCount: fpCount,
		homeDir: homeDir,
		eotsDb:  eotsDir,
	}
}

func (fpm *FinalityProviderManager) Start(ctx context.Context) {
	go fpm.submitFinalitySigForever(ctx)
	go fpm.queryFinalizedBlockForever(ctx)
}

// Initialize creates finality provider instances and EOTS manager
func (fpm *FinalityProviderManager) Initialize(ctx context.Context, numPubRand uint32) error {
	db, err := lib.NewBackend(fpm.eotsDb)
	if err != nil {
		return err
	}
	eots, err := eotsmanager.NewLocalEOTSManager(fpm.homeDir, "memory", db, fpm.logger)
	if err != nil {
		return err
	}

	fpis := make([]*FinalityProviderInstance, fpm.fpCount)

	res, err := fpm.tm.BabylonClient.CurrentEpoch()
	if err != nil {
		return err
	}

	for i := 0; i < fpm.fpCount; i++ {
		keyName := lib.GenRandomHexStr(r, 10)

		finalitySender, err := NewSenderWithBabylonClient(ctx, keyName, fpm.tm.Config.Babylon.RPCAddr, fpm.tm.Config.Babylon.GRPCAddr)
		if err != nil {
			return err
		}

		if err := fpm.tm.fundAllParties(ctx, []*SenderWithBabylonClient{finalitySender}); err != nil {
			return err
		}

		fpPK, err := eots.CreateKey(keyName, fpm.passphrase, "") // todo(lazar): hdpath?
		if err != nil {
			return err
		}
		btcPk, err := bbntypes.NewBIP340PubKey(fpPK)
		if err != nil {
			return err
		}

		fpRecord, err := eots.KeyRecord(btcPk.MustMarshal(), fpm.passphrase)
		if err != nil {
			return err
		}

		pop, err := bstypes.NewPoPBTC(finalitySender.BabylonAddress, fpRecord.PrivKey)
		if err != nil {
			return err
		}
		fpis[i] = &FinalityProviderInstance{
			btcPk:       btcPk,
			proofStore:  lib.NewPubRandProofStore(),
			pop:         pop,
			fpAddr:      finalitySender.BabylonAddress,
			client:      finalitySender,
			votedBlocks: make(map[uint64]struct{}),
		}

		if _, err = fpis[i].register(ctx, finalitySender.BabylonAddress.String(), btcPk, pop); err != nil {
			return err
		}
	}

	fpm.finalityProviders = fpis
	fpm.localEOTS = eots

	fmt.Printf("🎲 Starting to commit randomness\n")
	if err := fpm.commitRandomness(ctx, numPubRand); err != nil {
		return err
	}

	fmt.Printf("⌛ Waiting checkpoint to be finalized\n")
	if err := fpm.waitUntilFinalized(ctx, res.CurrentEpoch); err != nil {
		return err
	}

	return nil
}

func (fpm *FinalityProviderManager) submitFinalitySigForever(ctx context.Context) {
	commitRandTicker := time.NewTicker(3 * time.Second)
	defer commitRandTicker.Stop()

	for {
		select {
		case <-commitRandTicker.C:

			countNonFinalized, err := fpm.countNonFinalized()
			if err != nil {
				fmt.Printf("🚫 Err %v\n", err)
				continue
			}
			tipBlocks, err := fpm.getEarliestNonFinalizedBlocks(uint64(countNonFinalized))

			if err != nil {
				fmt.Printf("🚫 Err %v\n", err)
				continue
			}

			if tipBlocks == nil {
				continue
			}

			for _, fp := range fpm.finalityProviders {
				go func() {
					var blocks []*BlockInfo
					for _, b := range tipBlocks {
						hasVp, err := fp.hasVotingPower(ctx, b)
						if err != nil {
							fmt.Printf("🚫 Err getting voting power %v\n", err)
						}

						if !hasVp {
							continue
						}

						_, voted := fp.votedBlocks[b.Height]
						if voted {
							continue
						}

						blocks = append(blocks, b)
					}

					if len(blocks) == 0 {
						return
					}

					if err = fpm.submitFinalitySignature(ctx, blocks, fp); err != nil {
						fmt.Printf("🚫 Err submitting fin signature: %v\n", err)
					}
				}()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (fpm *FinalityProviderManager) commitRandomness(ctx context.Context, numPubRand uint32) error {
	startHeight := uint64(1) // todo(lazar): configure
	npr := numPubRand
	for _, fp := range fpm.finalityProviders {
		pubRandList, err := fpm.getPubRandList(startHeight, npr, *fp.btcPk)
		if err != nil {
			return err
		}
		numPubRand := uint64(len(pubRandList))
		commitment, proofList := getPubRandCommitAndProofs(pubRandList)

		if err := fp.proofStore.AddPubRandProofList(pubRandList, proofList); err != nil {
			return err
		}

		schnorrSig, err := fpm.signPubRandCommit(*fp.btcPk, startHeight, numPubRand, commitment)
		if err != nil {
			return err
		}

		err = fp.commitPubRandList(ctx, fp.btcPk.MustToBTCPK(), startHeight, numPubRand, commitment, schnorrSig)
		if err != nil {
			return err
		}

	}

	return nil
}

func (fpi *FinalityProviderInstance) register(
	ctx context.Context, signerAddr string, fpPk *bbntypes.BIP340PubKey, pop *bstypes.ProofOfPossessionBTC) (*pv.RelayerTxResponse, error) {

	commission := sdkmath.LegacyZeroDec()
	msgNewVal := &bstypes.MsgCreateFinalityProvider{
		Addr:        signerAddr,
		Description: &stakingtypes.Description{Moniker: lib.GenRandomHexStr(r, 10)},
		Commission:  &commission,
		BtcPk:       fpPk,
		Pop:         pop,
	}
	resp, err := fpi.client.SendMsgs(ctx, []sdk.Msg{msgNewVal})

	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, fmt.Errorf("resp from send msg is nil for fp %s", fpPk.MustMarshal())
	}

	return resp, nil
}

func (fpi *FinalityProviderInstance) commitPubRandList(
	ctx context.Context,
	fpPk *btcec.PublicKey,
	startHeight uint64,
	numPubRand uint64,
	commitment []byte,
	sig *schnorr.Signature) error {
	msg := &finalitytypes.MsgCommitPubRandList{
		Signer:      fpi.client.BabylonAddress.String(),
		FpBtcPk:     bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  commitment,
		Sig:         bbntypes.NewBIP340SignatureFromBTCSig(sig),
	}

	resp, err := fpi.client.SendMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
		return err
	}

	if resp == nil {
		return fmt.Errorf("🚫: resp empty when commiting pub ran")
	}

	return nil
}

func (fpm *FinalityProviderManager) queryLatestBlocks(startKey []byte, count uint64, status finalitytypes.QueriedBlockStatus, reverse bool) ([]*BlockInfo, error) {
	var blocks []*BlockInfo
	pagination := &sdkquery.PageRequest{
		Limit:   count,
		Reverse: reverse,
		Key:     startKey,
	}

	res, err := fpm.client.QueryClient.ListBlocks(status, pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query finalized blocks: %v", err)
	}

	for _, b := range res.Blocks {
		ib := &BlockInfo{
			Height: b.Height,
			Hash:   b.AppHash,
		}
		blocks = append(blocks, ib)
	}

	return blocks, nil
}

func (fpm *FinalityProviderManager) blockWithRetry(ctx context.Context, height uint64) (*BlockInfo, error) {
	var (
		block *BlockInfo
	)
	if err := retry.Do(func() error {
		res, err := fpm.client.QueryClient.Block(height)
		if err != nil {
			return err
		}
		block = &BlockInfo{
			Height:    height,
			Hash:      res.Block.AppHash,
			Finalized: res.Block.Finalized,
		}
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.Context(ctx)); err != nil {
		return nil, err
	}

	return block, nil
}

func (fpm *FinalityProviderManager) getPubRandList(startHeight uint64, numPubRand uint32, fpPk bbntypes.BIP340PubKey) ([]*btcec.FieldVal, error) {
	pubRandList, err := fpm.localEOTS.CreateRandomnessPairList(
		fpPk.MustMarshal(),
		[]byte(chainId),
		startHeight,
		numPubRand,
		fpm.passphrase,
	)
	if err != nil {
		return nil, err
	}

	return pubRandList, nil
}

func getPubRandCommitAndProofs(pubRandList []*btcec.FieldVal) ([]byte, []*merkle.Proof) {
	prBytesList := make([][]byte, 0, len(pubRandList))
	for _, pr := range pubRandList {
		prBytesList = append(prBytesList, bbntypes.NewSchnorrPubRandFromFieldVal(pr).MustMarshal())
	}

	return merkle.ProofsFromByteSlices(prBytesList)
}

func getHashToSignForCommitPubRand(startHeight uint64, numPubRand uint64, commitment []byte) ([]byte, error) {
	hasher := tmhash.New()
	if _, err := hasher.Write(sdk.Uint64ToBigEndian(startHeight)); err != nil {
		return nil, err
	}
	if _, err := hasher.Write(sdk.Uint64ToBigEndian(numPubRand)); err != nil {
		return nil, err
	}
	if _, err := hasher.Write(commitment); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

func (fpm *FinalityProviderManager) signPubRandCommit(fpPk bbntypes.BIP340PubKey, startHeight uint64, numPubRand uint64, commitment []byte) (*schnorr.Signature, error) {
	hash, err := getHashToSignForCommitPubRand(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
	}

	// sign the message hash using the finality-provider's BTC private key
	return fpm.localEOTS.SignSchnorrSig(fpPk.MustMarshal(), hash, fpm.passphrase)
}

func getMsgToSignForVote(blockHeight uint64, blockHash []byte) []byte {
	return append(sdk.Uint64ToBigEndian(blockHeight), blockHash...)
}

func (fpm *FinalityProviderManager) submitFinalitySignature(ctx context.Context, b []*BlockInfo, fpi *FinalityProviderInstance) error {
	prList, err := fpm.getPubRandList(b[0].Height, uint32(len(b)), *fpi.btcPk)
	if err != nil {
		return err
	}

	proofBytes, err := fpi.proofStore.GetPubRandProofList(prList)
	if err != nil {
		return fmt.Errorf(
			"failed to get inclusion proof of public randomness for FP %s for block range [%d-%d]: %w",
			fpi.btcPk.MarshalHex(),
			b[0].Height,
			b[len(b)-1].Height,
			err,
		)
	}

	sigList := make([]*btcec.ModNScalar, 0, len(b))
	for _, block := range b {
		eotsSig, err := fpm.signFinalitySig(block, fpi.btcPk)
		if err != nil {
			return err
		}
		sigList = append(sigList, eotsSig.ToModNScalar())
		fpi.votedBlocks[block.Height] = struct{}{} // we assume that all submitted votes to bbn will be accepted
	}

	if err = fpi.SubmitFinalitySig(ctx, fpi.btcPk.MustToBTCPK(), b, prList, proofBytes, sigList); err != nil {
		return err
	}

	fmt.Printf("✍️ Fp %s, voted for block range [%d-%d]\n", fpi.btcPk.MarshalHex(), b[0].Height, b[len(b)-1].Height)

	return nil
}

func (fpm *FinalityProviderManager) signFinalitySig(b *BlockInfo, btcPk *bbntypes.BIP340PubKey) (*bbntypes.SchnorrEOTSSig, error) {
	// build proper finality signature request
	msgToSign := getMsgToSignForVote(b.Height, b.Hash)
	sig, err := fpm.localEOTS.SignEOTS(btcPk.MustMarshal(), []byte(chainId), msgToSign, b.Height, fpm.passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
}

// SubmitFinalitySig submits the finality signature via a MsgAddVote to Babylon
func (fpi *FinalityProviderInstance) SubmitFinalitySig(
	ctx context.Context,
	fpPk *btcec.PublicKey,
	blocks []*BlockInfo,
	pubRandList []*btcec.FieldVal,
	proofList [][]byte,
	sigs []*btcec.ModNScalar,
) error {
	if len(blocks) != len(sigs) {
		return fmt.Errorf("the number of blocks %v should match the number of finality signatures %v", len(blocks), len(sigs))
	}

	msgs := make([]sdk.Msg, 0, len(blocks))
	for i, block := range blocks {
		cmtProof := cmtcrypto.Proof{}
		if err := cmtProof.Unmarshal(proofList[i]); err != nil {
			return err
		}

		msg := &finalitytypes.MsgAddFinalitySig{
			Signer:       fpi.client.BabylonAddress.String(),
			FpBtcPk:      bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
			BlockHeight:  block.Height,
			PubRand:      bbntypes.NewSchnorrPubRandFromFieldVal(pubRandList[i]),
			Proof:        &cmtProof,
			BlockAppHash: block.Hash,
			FinalitySig:  bbntypes.NewSchnorrEOTSSigFromModNScalar(sigs[i]),
		}

		msgs = append(msgs, msg)
	}

	if _, err := fpi.client.SendMsgs(ctx, msgs); err != nil {
		return err
	}

	return nil
}

func (fpm *FinalityProviderManager) waitUntilFinalized(ctx context.Context, epoch uint64) error {
	err := lib.Eventually(ctx, func() bool {
		lastFinalizedCkpt, err := fpm.tm.BabylonClient.LatestEpochFromStatus(ckpttypes.Finalized)
		if err != nil {
			return false
		}
		return epoch <= lastFinalizedCkpt.RawCheckpoint.EpochNum

	}, 120*time.Second, eventuallyPollTime, "🚫: err waiting for ckpt to be finalized")

	return err
}

func (fpi *FinalityProviderInstance) getVotingPowerWithRetry(ctx context.Context, height uint64) (uint64, error) {
	var power uint64

	if err := retry.Do(func() error {
		res, err := fpi.client.FinalityProviderPowerAtHeight(fpi.btcPk.MarshalHex(), height)
		if err != nil {
			return err
		}

		power = res.VotingPower
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.Context(ctx)); err != nil {
		return 0, err
	}

	return power, nil
}

func (fpi *FinalityProviderInstance) hasVotingPower(ctx context.Context, b *BlockInfo) (bool, error) {
	power, err := fpi.getVotingPowerWithRetry(ctx, b.Height)
	if err != nil {
		return false, err
	}
	if power == 0 {
		fmt.Printf("🙁 Fp %s, has no voting power block: %d\n", fpi.btcPk.MarshalHex(), b.Height)
		return false, nil
	}

	return true, nil
}

func (fpm *FinalityProviderManager) randomFp() *FinalityProviderInstance {
	randomIndex := r.Intn(len(fpm.finalityProviders))

	return fpm.finalityProviders[randomIndex]
}

func (fpm *FinalityProviderManager) queryFinalizedBlockForever(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	fmt.Printf("⌛ Waiting for activation\n")
	height, err := fpm.waitForActivation(ctx)
	if err != nil {
		fmt.Printf("🚫 Err %v\n", err)
		return
	}

	fmt.Printf("🔋 Activated height %d\n", height)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			block, err := fpm.blockWithRetry(ctx, height)
			if err != nil {
				fmt.Printf("🚫 Err getting block from bbn at height %d err: %v\n", height, err)
				continue
			}

			fmt.Printf("💡 Got block: %d, finalized: %v\n", block.Height, block.Finalized)
			height = block.Height + 1
		}
	}
}

func (fpm *FinalityProviderManager) waitForActivation(ctx context.Context) (uint64, error) {
	var height uint64

	err := lib.Eventually(ctx, func() bool {
		res, err := fpm.client.ActivatedHeight()
		if err != nil {
			return false
		}
		height = res.Height
		return height > 0
	}, 120*time.Second, eventuallyPollTime)

	if err != nil {
		return 0, fmt.Errorf("err getting activated height err:%v\n", err)
	}

	return height, nil
}

func (fpm *FinalityProviderManager) getEarliestNonFinalizedBlocks(count uint64) ([]*BlockInfo, error) {
	blocks, err := fpm.queryLatestBlocks(nil, count, finalitytypes.QueriedBlockStatus_NON_FINALIZED, false)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}

	return blocks, nil
}

func (fpm *FinalityProviderManager) countNonFinalized() (int, error) {
	resp, err := fpm.tm.BabylonClient.ListBlocks(finalitytypes.QueriedBlockStatus_NON_FINALIZED, nil)

	if err != nil {
		return 0, err
	}

	return len(resp.Blocks), nil
}
