package harness

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"
	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/keyring"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/cometbft/cometbft/crypto/tmhash"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"
	"strings"
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
	finalityProviders []FinalityProviderInstance
	wg                *sync.WaitGroup
	logger            *zap.Logger
	localEOTS         *eotsmanager.LocalEOTSManager

	quit       chan struct{}
	passphrase string
	fpCount    int
	homeDir    string
	eotsDb     string
	keyDir     string
}
type FinalityProviderInstance struct {
	btcPk      *bbntypes.BIP340PubKey
	proofStore *lib.PubRandProofStore
	fpAddr     sdk.AccAddress
	pop        *bstypes.ProofOfPossessionBTC
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
		tm:      tm,
		client:  client,
		wg:      &sync.WaitGroup{},
		logger:  logger,
		fpCount: fpCount,
		quit:    make(chan struct{}),
		homeDir: homeDir,
		eotsDb:  eotsDir,
		keyDir:  keyDir,
	}
}

func (fpm *FinalityProviderManager) Start(ctx context.Context) {
	fpm.wg.Add(1)
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

	fpis := make([]FinalityProviderInstance, fpm.fpCount)

	input := strings.NewReader("")
	kr, err := keyring.CreateKeyring(
		fpm.keyDir,
		chainId,
		"memory",
		input,
	)

	for i := 0; i < fpm.fpCount; i++ {
		keyName := lib.GenRandomHexStr(r, 10)
		fpPK, err := eots.CreateKey(keyName, fpm.passphrase, "") // todo(lazar): hdpath?
		if err != nil {
			return err
		}
		btcPk, err := bbntypes.NewBIP340PubKey(fpPK)
		if err != nil {
			return err
		}
		_, err = keyring.NewChainKeyringControllerWithKeyring(kr, keyName, input)
		if err != nil {
			return err
		}
		//keyInfo, err := kc.CreateChainKey(fpm.passphrase, "", "")
		//if err != nil {
		//	return err
		//}
		fpRecord, err := eots.KeyRecord(btcPk.MustMarshal(), fpm.passphrase)
		if err != nil {
			return err
		}
		//pop, err := kc.CreatePop(fpm.client.BabylonAddress, fpRecord.PrivKey) // todo(lazar): check the fp addr?
		//if err != nil {
		//	return err
		//}
		pop, err := bstypes.NewPoPBTC(fpm.client.BabylonAddress, fpRecord.PrivKey)
		if err != nil {
			return err
		}
		fpis[i] = FinalityProviderInstance{
			btcPk:      btcPk,
			proofStore: lib.NewPubRandProofStore(),
			pop:        pop,
			fpAddr:     fpm.client.BabylonAddress,
		}

		if _, err = fpm.register(ctx, btcPk, pop); err != nil {
			return err
		}
	}

	fpm.finalityProviders = fpis
	fpm.localEOTS = eots

	return nil
}

func (fpm *FinalityProviderManager) commitRandomnessForever(ctx context.Context) {
	commitRandTicker := time.NewTicker(10 * time.Second)
	defer commitRandTicker.Stop()

	for {
		select {
		case <-commitRandTicker.C:
			tipBlock, err := fpm.getLatestBlockWithRetry(ctx)
			if err != nil {

				continue
			}
			_ = tipBlock

		case <-ctx.Done():
			fmt.Printf("the randomness commitment loop is closing")
			return
		}
	}
}

func (fpm *FinalityProviderManager) register(
	ctx context.Context, fpPk *bbntypes.BIP340PubKey, pop *bstypes.ProofOfPossessionBTC) (*pv.RelayerTxResponse, error) {
	signerAddr := fpm.client.BabylonAddress.String()

	commission := sdkmath.LegacyZeroDec()
	msgNewVal := &bstypes.MsgCreateFinalityProvider{
		Addr:        signerAddr,
		Description: &stakingtypes.Description{Moniker: lib.GenRandomHexStr(r, 10)},
		Commission:  &commission,
		BtcPk:       fpPk,
		Pop:         pop,
	}
	resp, err := fpm.client.SendMsgs(ctx, []sdk.Msg{msgNewVal})

	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, fmt.Errorf("resp from send msg is nil for fp %s", fpPk.MustMarshal())
	}

	return resp, nil
}

func (fpm *FinalityProviderManager) commitPubRandList(
	ctx context.Context,
	fpPk *btcec.PublicKey,
	startHeight uint64,
	numPubRand uint64,
	commitment []byte,
	sig *schnorr.Signature) error {
	msg := &finalitytypes.MsgCommitPubRandList{
		Signer:      fpm.client.BabylonAddress.String(),
		FpBtcPk:     bbntypes.NewBIP340PubKeyFromBTCPK(fpPk),
		StartHeight: startHeight,
		NumPubRand:  numPubRand,
		Commitment:  commitment,
		Sig:         bbntypes.NewBIP340SignatureFromBTCSig(sig),
	}

	resp, err := fpm.client.SendMsgs(ctx, []sdk.Msg{msg})
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
