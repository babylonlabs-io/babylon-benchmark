package harness

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/avast/retry-go/v4"
	"math/rand"
	"time"

	sdkmath "cosmossdk.io/math"
	bbn "github.com/babylonlabs-io/babylon/app"
	"github.com/babylonlabs-io/babylon/client/config"
	bncfg "github.com/babylonlabs-io/babylon/client/config"
	"github.com/babylonlabs-io/babylon/client/query"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/wire"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"
)

var (
	r         = rand.New(rand.NewSource(time.Now().Unix()))
	RtyAttNum = uint(5)
	RtyAtt    = retry.Attempts(RtyAttNum)
	RtyDel    = retry.Delay(time.Millisecond * 400)
	RtyErr    = retry.LastErrorOnly(true)
)

type Client struct {
	*query.QueryClient

	provider *cosmos.CosmosProvider
	timeout  time.Duration
	logger   *zap.Logger
	cfg      *config.BabylonConfig
}

func New(
	ctx context.Context, cfg *config.BabylonConfig, logger *zap.Logger) (*Client, error) {
	var (
		zapLogger *zap.Logger
		err       error
	)

	// ensure cfg is valid
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// use the existing logger or create a new one if not given
	zapLogger = logger
	if zapLogger == nil {
		zapLogger = zap.NewNop()
	}

	provider, err := cfg.ToCosmosProviderConfig().NewProvider(
		zapLogger,
		"", // TODO: set home path
		true,
		"babylon",
	)
	if err != nil {
		return nil, err
	}

	cp := provider.(*cosmos.CosmosProvider)
	cp.PCfg.KeyDirectory = cfg.KeyDirectory

	// Create tmp Babylon app to retrieve and register codecs
	// Need to override this manually as otherwise option from config is ignored
	encCfg := bbn.GetEncodingConfig()
	cp.Cdc = cosmos.Codec{
		InterfaceRegistry: encCfg.InterfaceRegistry,
		Marshaler:         encCfg.Codec,
		TxConfig:          encCfg.TxConfig,
		Amino:             encCfg.Amino,
	}

	// initialise Cosmos provider
	// NOTE: this will create a RPC client. The RPC client will be used for
	// submitting txs and making ad hoc queries. It won't create WebSocket
	// connection with Babylon node
	err = cp.Init(ctx)
	if err != nil {
		return nil, err
	}

	// create a queryClient so that the Client inherits all query functions
	// TODO: merge this RPC client with the one in `cp` after Cosmos side
	// finishes the migration to new RPC client
	// see https://github.com/strangelove-ventures/cometbft-client
	c, err := rpchttp.NewWithTimeout(cp.PCfg.RPCAddr, "/websocket", uint(cfg.Timeout.Seconds()))
	if err != nil {
		return nil, err
	}
	queryClient, err := query.NewWithClient(c, cfg.Timeout)
	if err != nil {
		return nil, err
	}

	return &Client{
		queryClient,
		cp,
		cfg.Timeout,
		zapLogger,
		cfg,
	}, nil
}

type SenderWithBabylonClient struct {
	*Client
	PrvKey         cryptotypes.PrivKey
	PubKey         cryptotypes.PubKey
	BabylonAddress sdk.AccAddress
}

func NewSenderWithBabylonClient(
	ctx context.Context,
	keyName string,
	rpcaddr string,
	grpcaddr string) (*SenderWithBabylonClient, error) {

	cfg := bncfg.DefaultBabylonConfig()
	cfg.Key = keyName
	cfg.ChainID = chainId
	cfg.KeyringBackend = "memory"
	cfg.RPCAddr = rpcaddr
	cfg.GRPCAddr = grpcaddr
	cfg.GasAdjustment = 3.0

	cl, err := New(ctx, &cfg, zap.NewNop())
	if err != nil {
		return nil, err
	}

	prvKey, pubKey, address := testdata.KeyTestPubAddr()

	err = cl.provider.Keybase.ImportPrivKeyHex(
		keyName,
		hex.EncodeToString(prvKey.Bytes()),
		"secp256k1",
	)
	if err != nil {
		return nil, err
	}

	return &SenderWithBabylonClient{
		Client:         cl,
		PrvKey:         prvKey,
		PubKey:         pubKey,
		BabylonAddress: address,
	}, nil
}

func (s *SenderWithBabylonClient) SendMsgs(ctx context.Context, msgs []sdk.Msg) (*pv.RelayerTxResponse, error) {
	relayerMsgs := ToProviderMsgs(msgs)
	resp, success, err := s.provider.SendMessages(ctx, relayerMsgs, "")

	if err != nil {
		return nil, err
	}

	if !success {
		return resp, fmt.Errorf("message send but failed to execute")
	}

	return resp, nil
}

func ToProviderMsgs(msgs []sdk.Msg) []pv.RelayerMessage {
	var relayerMsgs []pv.RelayerMessage
	for _, m := range msgs {
		relayerMsgs = append(relayerMsgs, cosmos.NewCosmosMessage(m, func(signer string) {}))
	}
	return relayerMsgs
}

func (s *SenderWithBabylonClient) InsertBTCHeadersToBabylon(ctx context.Context, headers []*wire.BlockHeader) (*pv.RelayerTxResponse, error) {
	var headersBytes []bbntypes.BTCHeaderBytes

	for _, h := range headers {
		headersBytes = append(headersBytes, bbntypes.NewBTCHeaderBytesFromBlockHeader(h))
	}

	msg := btclctypes.MsgInsertHeaders{
		Headers: headersBytes,
		Signer:  s.BabylonAddress.String(),
	}

	return s.SendMsgs(ctx, []sdk.Msg{&msg})
}

func (s *SenderWithBabylonClient) CreateFinalityProvider(ctx context.Context) (*pv.RelayerTxResponse, *bstypes.FinalityProvider, error) {
	var err error
	signerAddr := s.BabylonAddress.String()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	fpSK, _, err := datagen.GenRandomBTCKeyPair(r)
	if err != nil {
		return nil, nil, err
	}
	btcFp, err := datagen.GenRandomFinalityProviderWithBTCBabylonSKs(r, fpSK, addr)
	if err != nil {
		return nil, nil, err
	}

	commission := sdkmath.LegacyZeroDec()
	msgNewVal := &bstypes.MsgCreateFinalityProvider{
		Addr:        signerAddr,
		Description: &stakingtypes.Description{Moniker: datagen.GenRandomHexStr(r, 10)},
		Commission:  &commission,
		BtcPk:       btcFp.BtcPk,
		Pop:         btcFp.Pop,
	}
	resp, err := s.SendMsgs(ctx, []sdk.Msg{msgNewVal})

	if err != nil {
		return resp, nil, err
	}

	return resp, btcFp, nil
}

func senders(stakers []*BTCStaker) []*SenderWithBabylonClient {
	var sends []*SenderWithBabylonClient

	for _, staker := range stakers {
		stakerCp := staker
		sends = append(sends, stakerCp.client)
	}
	return sends
}
