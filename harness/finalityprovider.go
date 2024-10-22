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
	"sync"
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
	wg                *sync.WaitGroup
	logger            *zap.Logger
	localEOTS         *eotsmanager.LocalEOTSManager
	blockInfoChan     chan *BlockInfo

	quit       chan struct{}
	passphrase string
	fpCount    int
	homeDir    string
	eotsDb     string
	keyDir     string
}
type FinalityProviderInstance struct {
	btcPk               *bbntypes.BIP340PubKey
	proofStore          *lib.PubRandProofStore
	fpAddr              sdk.AccAddress
	pop                 *bstypes.ProofOfPossessionBTC
	client              *SenderWithBabylonClient
	lastProcessedHeight uint64
}

func NewFinalityProviderManager(
	tm *TestManager,
	client *SenderWithBabylonClient,
	logger *zap.Logger,
	fpCount int,
	homeDir string,
	eotsDir string,
	keyDir string,
) *FinalityProviderManager {
	return &FinalityProviderManager{
		tm:            tm,
		client:        client,
		wg:            &sync.WaitGroup{},
		logger:        logger,
		fpCount:       fpCount,
		quit:          make(chan struct{}),
		blockInfoChan: make(chan *BlockInfo, 1000),
		homeDir:       homeDir,
		eotsDb:        eotsDir,
		keyDir:        keyDir,
	}
}

func (fpm *FinalityProviderManager) Start(ctx context.Context) {
	go fpm.submitFinalitySigForever(ctx)
}

// Initialize creates finality provider instances and EOTS manager
func (fpm *FinalityProviderManager) Initialize(ctx context.Context) error {
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
			btcPk:      btcPk,
			proofStore: lib.NewPubRandProofStore(),
			pop:        pop,
			fpAddr:     finalitySender.BabylonAddress,
			client:     finalitySender,
		}

		if _, err = fpis[i].register(ctx, finalitySender.BabylonAddress.String(), btcPk, pop); err != nil {
			return err
		}
	}

	fpm.finalityProviders = fpis
	fpm.localEOTS = eots

	fmt.Printf("🎲: starting to commit randomness\n")
	if err := fpm.commitRandomness(ctx); err != nil {
		return err
	}

	fmt.Printf("⌛: waiting checkpoint to be finalized\n")
	if err := fpm.waitUntilFinalized(ctx, res.CurrentEpoch); err != nil {
		return err
	}

	return nil
}

func (fpm *FinalityProviderManager) submitFinalitySigForever(ctx context.Context) {
	commitRandTicker := time.NewTicker(10 * time.Second)
	defer commitRandTicker.Stop()

	height := uint64(1)
	for {
		select {
		case <-commitRandTicker.C:
			//tipBlock, err := fpm.blockWithRetry(ctx, height)
			tipBlock, err := fpm.getLatestBlockWithRetry(ctx)

			if err != nil {
				fmt.Printf("err %v\n", err)
				continue
			}
			for _, fp := range fpm.finalityProviders {
				hasVp, err := fp.hasVotingPower(ctx, tipBlock)
				if err != nil {
					fmt.Printf("err getting voting power %v\n", err)
				}
				if !hasVp {
					continue
				}

				if err = fpm.submitFinalitySignature(ctx, tipBlock, fp); err != nil {
					fmt.Printf("err submitting fin signature %v\n", err)
				}
			}
			height++
		case <-ctx.Done():
			return
		}
	}
}

func (fpm *FinalityProviderManager) commitRandomness(ctx context.Context) error {
	startHeight := uint64(1) // todo(lazar): configure
	npr := uint32(1000)
	for _, fp := range fpm.finalityProviders {
		pubRandList, err := fpm.getPubRandList(startHeight, npr, *fp.btcPk)
		if err != nil {
			return err
		}
		numPubRand := uint64(len(pubRandList))
		commitment, proofList := GetPubRandCommitAndProofs(pubRandList)

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
		return err
	}

	return nil
}

func (fpm *FinalityProviderManager) queryLastCommittedPublicRand(fpPk *btcec.PublicKey, count uint64) (map[uint64]*finalitytypes.PubRandCommitResponse, error) {
	fpBtcPk := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk)

	pagination := &sdkquery.PageRequest{
		Limit:   count,
		Reverse: true,
	}

	res, err := fpm.client.QueryClient.ListPubRandCommit(fpBtcPk.MarshalHex(), pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query committed public randomness: %w", err)
	}

	return res.PubRandCommitMap, nil
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

func (fpm *FinalityProviderManager) queryCometBestBlock(ctx context.Context) (*BlockInfo, error) {
	innerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	// this will return 20 items at max in the descending order (highest first)
	chainInfo, err := fpm.client.RPCClient.BlockchainInfo(innerCtx, 0, 0)
	defer cancel()

	if err != nil {
		return nil, err
	}

	headerHeightInt64 := chainInfo.BlockMetas[0].Header.Height
	if headerHeightInt64 < 0 {
		return nil, fmt.Errorf("block height %v should be positive", headerHeightInt64)
	}
	// Returning response directly, if header with specified number did not exist
	// at request will contain nil header
	return &BlockInfo{
		Height: uint64(headerHeightInt64),
		Hash:   chainInfo.BlockMetas[0].Header.AppHash,
	}, nil
}

func (fpm *FinalityProviderManager) queryBestBlock(ctx context.Context) (*BlockInfo, error) {
	blocks, err := fpm.queryLatestBlocks(nil, 1, finalitytypes.QueriedBlockStatus_ANY, true)
	if err != nil || len(blocks) != 1 {
		return fpm.queryCometBestBlock(ctx)
	}

	return blocks[0], nil
}

func (fpm *FinalityProviderManager) getLatestBlockWithRetry(ctx context.Context) (*BlockInfo, error) {
	var (
		latestBlock *BlockInfo
		err         error
	)

	if err := retry.Do(func() error {
		latestBlock, err = fpm.queryBestBlock(ctx)
		if err != nil {
			return err
		}
		return nil
	}, RtyAtt, RtyDel, RtyErr); err != nil {
		return nil, err
	}

	return latestBlock, nil
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

func GetPubRandCommitAndProofs(pubRandList []*btcec.FieldVal) ([]byte, []*merkle.Proof) {
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

func (fpm *FinalityProviderManager) submitFinalitySignature(ctx context.Context, b *BlockInfo, fpi *FinalityProviderInstance) error {
	sig, err := fpm.signFinalitySig(b, fpi.btcPk)
	if err != nil {
		return err
	}

	prList, err := fpm.getPubRandList(b.Height, 1, *fpi.btcPk)
	if err != nil {
		return err
	}

	pubRand := prList[0]

	proofBytes, err := fpi.proofStore.GetPubRandProof(pubRand)
	if err != nil {
		return fmt.Errorf(
			"failed to get inclusion proof of public randomness %s for FP %s for block %d: %w",
			pubRand.String(),
			fpi.btcPk.MarshalHex(),
			b.Height,
			err,
		)
	}

	err = fpi.SubmitFinalitySig(ctx, fpi.btcPk.MustToBTCPK(), b, pubRand, proofBytes, sig.ToModNScalar())
	if err != nil {
		return err
	}

	fpi.lastProcessedHeight = b.Height

	fmt.Printf("✍️: fp voted %s for block %d\n", fpi.btcPk.MarshalHex(), b.Height)

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
	block *BlockInfo,
	pubRand *btcec.FieldVal,
	proof []byte, // TODO: have a type for proof
	sig *btcec.ModNScalar,
) error {
	cmtProof := cmtcrypto.Proof{}
	if err := cmtProof.Unmarshal(proof); err != nil {
		return err
	}

	msg := &finalitytypes.MsgAddFinalitySig{
		Signer:       fpi.client.BabylonAddress.String(),
		FpBtcPk:      bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
		BlockHeight:  block.Height,
		PubRand:      bbntypes.NewSchnorrPubRandFromFieldVal(pubRand),
		Proof:        &cmtProof,
		BlockAppHash: block.Hash,
		FinalitySig:  bbntypes.NewSchnorrEOTSSigFromModNScalar(sig),
	}

	_, err := fpi.client.SendMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
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

	}, 120*time.Second, eventuallyPollTime, "err waiting for ckpt to be finalized")

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
		fmt.Printf("🙁: the fp has no voting power %s block: %d\n", fpi.btcPk.MarshalHex(), b.Height)
		return false, nil
	}

	return true, nil
}
