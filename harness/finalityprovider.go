package harness

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
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
	localEOTS   *eotsmanager.LocalEOTSManager
	passphrase  string
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
	fpm.submitFinalitySigForever(ctx)
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

	res, err := fpm.tm.BabylonClientNode0.CurrentEpoch()
	if err != nil {
		return err
	}

	for i := 0; i < fpm.fpCount; i++ {
		keyName := lib.GenRandomHexStr(r, 10)

		finalitySender, err := NewSenderWithBabylonClient(ctx, keyName, fpm.tm.Config.Babylon0.RPCAddr, fpm.tm.Config.Babylon0.GRPCAddr)
		if err != nil {
			return err
		}

		if err := fpm.tm.fundAllParties(ctx, []*SenderWithBabylonClient{finalitySender}); err != nil {
			return err
		}

		fpPK, err := eots.CreateKey(keyName) // todo(lazar): hdpath?
		if err != nil {
			return err
		}
		btcPk, err := bbntypes.NewBIP340PubKey(fpPK)
		if err != nil {
			return err
		}

		fpRecord, err := eots.KeyRecord(btcPk.MustMarshal())
		if err != nil {
			return err
		}

		pop, err := datagen.NewPoPBTC(finalitySender.BabylonAddress, fpRecord.PrivKey)
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
			localEOTS:   eots,
			passphrase:  fpm.passphrase,
		}

		if err = fpis[i].register(ctx, finalitySender.BabylonAddress.String(), btcPk, pop); err != nil {
			return err
		}
	}

	fpm.finalityProviders = fpis

	fmt.Printf("🎲 Starting to commit randomness\n")
	for _, fpi := range fpm.finalityProviders {
		if err := fpi.commitRandomness(ctx, numPubRand); err != nil {
			return err
		}
	}

	fmt.Printf("⌛ Waiting checkpoint epoch %d to be finalized\n", res.CurrentEpoch)
	if err := fpm.waitUntilFinalized(ctx, res.CurrentEpoch); err != nil {
		return err
	}

	return nil
}

func (fpm *FinalityProviderManager) submitFinalitySigForever(ctx context.Context) {
	for _, fpi := range fpm.finalityProviders {
		fpi.start(ctx)
	}
}

func (fpi *FinalityProviderInstance) commitRandomness(ctx context.Context, npr uint32) error {
	startHeight := uint64(1) // todo(lazar): configure
	pubRandList, err := fpi.getPubRandList(startHeight, npr, *fpi.btcPk)
	if err != nil {
		return err
	}
	numPubRand := uint64(len(pubRandList))
	commitment, proofList := getPubRandCommitAndProofs(pubRandList)

	if err := fpi.proofStore.AddPubRandProofList(pubRandList, proofList); err != nil {
		return err
	}

	schnorrSig, err := fpi.signPubRandCommit(*fpi.btcPk, startHeight, numPubRand, commitment)
	if err != nil {
		return err
	}

	err = fpi.commitPubRandList(ctx, fpi.btcPk.MustToBTCPK(), startHeight, numPubRand, commitment, schnorrSig)
	if err != nil {
		return err
	}

	return nil
}

func (fpi *FinalityProviderInstance) register(
	ctx context.Context, signerAddr string, fpPk *bbntypes.BIP340PubKey, pop *bstypes.ProofOfPossessionBTC) error {
	zero := sdkmath.LegacyZeroDec()
	commission := bstypes.NewCommissionRates(zero, zero, zero)
	msgNewVal := &bstypes.MsgCreateFinalityProvider{
		Addr:        signerAddr,
		Description: &stakingtypes.Description{Moniker: lib.GenRandomHexStr(r, 10)},
		Commission:  commission,
		BtcPk:       fpPk,
		Pop:         pop,
	}

	if err := fpi.client.SendMsgs(ctx, []sdk.Msg{msgNewVal}); err != nil {
		return err
	}

	return nil
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

	if err := fpi.client.SendMsgs(ctx, []sdk.Msg{msg}); err != nil {
		return err
	}

	return nil
}

func (fpi *FinalityProviderInstance) queryLatestBlocks(startKey []byte, count uint64, status finalitytypes.QueriedBlockStatus, reverse bool) ([]*BlockInfo, error) {
	var blocks []*BlockInfo
	pagination := &sdkquery.PageRequest{
		Limit:   count,
		Reverse: reverse,
		Key:     startKey,
	}

	res, err := fpi.client.QueryClient.ListBlocks(status, pagination)
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

func (fpi *FinalityProviderInstance) getPubRandList(startHeight uint64, numPubRand uint32, fpPk bbntypes.BIP340PubKey) ([]*btcec.FieldVal, error) {
	pubRandList, err := fpi.localEOTS.CreateRandomnessPairList(
		fpPk.MustMarshal(),
		[]byte(chainId),
		startHeight,
		numPubRand,
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

func (fpi *FinalityProviderInstance) signPubRandCommit(fpPk bbntypes.BIP340PubKey, startHeight uint64, numPubRand uint64, commitment []byte) (*schnorr.Signature, error) {
	hash, err := getHashToSignForCommitPubRand(startHeight, numPubRand, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the commit public randomness message: %w", err)
	}

	// sign the message hash using the finality-provider's BTC private key
	return fpi.localEOTS.SignSchnorrSig(fpPk.MustMarshal(), hash)
}

func getMsgToSignForVote(blockHeight uint64, blockHash []byte) []byte {
	return append(sdk.Uint64ToBigEndian(blockHeight), blockHash...)
}

func (fpi *FinalityProviderInstance) submitFinalitySignature(ctx context.Context, b []*BlockInfo) error {
	prList, err := fpi.getPubRandList(b[0].Height, uint32(len(b)), *fpi.btcPk)
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
		eotsSig, err := fpi.signFinalitySig(block, fpi.btcPk)
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

func (fpi *FinalityProviderInstance) signFinalitySig(b *BlockInfo, btcPk *bbntypes.BIP340PubKey) (*bbntypes.SchnorrEOTSSig, error) {
	// build proper finality signature request
	msgToSign := getMsgToSignForVote(b.Height, b.Hash)
	sig, err := fpi.localEOTS.SignEOTS(btcPk.MustMarshal(), []byte(chainId), msgToSign, b.Height)
	if err != nil {
		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	return bbntypes.NewSchnorrEOTSSigFromModNScalar(sig), nil
}

// SubmitFinalitySig submits the finality signature via a MsgAddVote to Babylon0
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

	if err := fpi.client.SendMsgs(ctx, msgs); err != nil {
		return err
	}

	return nil
}

func (fpm *FinalityProviderManager) waitUntilFinalized(ctx context.Context, epoch uint64) error {
	err := lib.Eventually(ctx, func() bool {
		lastFinalizedCkpt, err := fpm.tm.BabylonClientNode0.LatestEpochFromStatus(ckpttypes.Finalized)
		if err != nil {
			return false
		}

		fmt.Printf("⌛ Waiting for finalized checkpoint for epoch %d current: %v\n", epoch, lastFinalizedCkpt.RawCheckpoint.EpochNum)

		return epoch <= lastFinalizedCkpt.RawCheckpoint.EpochNum

	}, 360*time.Second, eventuallyPollTime, "🚫: err waiting for ckpt to be finalized")

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

func (fpm *FinalityProviderManager) getRandomChunk(chunkSize int) []*btcec.PublicKey {
	if len(fpm.finalityProviders) == 0 {
		return []*btcec.PublicKey{}
	}

	if chunkSize >= len(fpm.finalityProviders) || len(fpm.finalityProviders) == 1 {
		return fpiToBtcPks(fpm.finalityProviders)
	}

	r.Seed(time.Now().UnixNano())

	startIndex := r.Intn(len(fpm.finalityProviders) - chunkSize + 1)

	chunk := fpm.finalityProviders[startIndex : startIndex+chunkSize]

	return fpiToBtcPks(chunk)
}

func fpiToBtcPks(fpi []*FinalityProviderInstance) []*btcec.PublicKey {
	fpPKs := make([]*btcec.PublicKey, 0, len(fpi))

	for _, fp := range fpi {
		fpPKs = append(fpPKs, fp.btcPk.MustToBTCPK())
	}

	return fpPKs
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
			fmt.Printf("🚫 Err: %s\n", err)
			return false
		}
		height = res.Height
		return height > 0
	}, 320*time.Second, eventuallyPollTime)

	if err != nil {
		return 0, fmt.Errorf("err getting activated height err:%v\n", err)
	}

	return height, nil
}

func (fpi *FinalityProviderInstance) getEarliestNonFinalizedBlocks(count uint64) ([]*BlockInfo, error) {
	blocks, err := fpi.queryLatestBlocks(nil, count, finalitytypes.QueriedBlockStatus_NON_FINALIZED, false)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}

	return blocks, nil
}

func (fpi *FinalityProviderInstance) countNonFinalized() (int, error) {
	resp, err := fpi.client.ListBlocks(finalitytypes.QueriedBlockStatus_NON_FINALIZED, nil)

	if err != nil {
		return 0, err
	}

	return len(resp.Blocks), nil
}

func (fpi *FinalityProviderInstance) start(ctx context.Context) {
	go fpi.submitFinalitySigForever(ctx)
}

func (fpi *FinalityProviderInstance) submitFinalitySigForever(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		time.Sleep(100 * time.Millisecond)
		countNonFinalized, err := fpi.countNonFinalized()
		if err != nil {
			fmt.Printf("🚫 Err %v\n", err)
			continue
		}

		tipBlocks, err := fpi.getEarliestNonFinalizedBlocks(uint64(countNonFinalized))
		if err != nil {
			fmt.Printf("🚫 Err %v\n", err)
			continue
		}

		if tipBlocks == nil {
			continue
		}

		var blocks []*BlockInfo
		for _, b := range tipBlocks {
			hasVp, err := fpi.hasVotingPower(ctx, b)
			if err != nil {
				fmt.Printf("🚫 Err getting voting power %v\n", err)
			}

			if !hasVp {
				time.Sleep(500 * time.Millisecond)
				continue
			}

			_, voted := fpi.votedBlocks[b.Height]
			if voted {
				continue
			}

			blocks = append(blocks, b)
		}

		if len(blocks) == 0 {
			continue
		}

		if err = fpi.submitFinalitySignature(ctx, blocks); err != nil {
			fmt.Printf("🚫 Err submitting fin signature: %v\n", err)
		}
	}
}
