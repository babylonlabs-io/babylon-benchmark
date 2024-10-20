package harness

import (
	"context"
	"encoding/hex"
	"fmt"
	bbn "github.com/babylonlabs-io/babylon/app"
	"github.com/babylonlabs-io/babylon/client/config"
	bncfg "github.com/babylonlabs-io/babylon/client/config"
	"github.com/babylonlabs-io/babylon/client/query"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"
	"time"
)

type Client struct {
	*query.QueryClient

	provider *cosmos.CosmosProvider
	timeout  time.Duration
	logger   *zap.Logger
	cfg      *config.BabylonConfig
}

func New(
	cfg *config.BabylonConfig, logger *zap.Logger) (*Client, error) {
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
	err = cp.Init(context.Background())
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

	cl, err := New(&cfg, zap.NewNop())
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
