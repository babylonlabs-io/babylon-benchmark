package harness

import (
	"bytes"
	"context"
	"cosmossdk.io/errors"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/container"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/txscript"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"

	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbn "github.com/babylonlabs-io/babylon/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

var (
	submitterAddrStr = "bbn1eppc73j56382wjn6nnq3quu5eye4pmm087xfdh" //nolint:unused
	babylonTag       = []byte{1, 2, 3, 4}                           //nolint:unused
	babylonTagHex    = hex.EncodeToString(babylonTag)               //nolint:unused

	eventuallyWaitTimeOut = 40 * time.Second
	eventuallyPollTime    = 1 * time.Second
	regtestParams         = &chaincfg.RegressionNetParams
	defaultEpochInterval  = uint(400) //nolint:unused
)

const (
	chainId = "chain-test"
)

func defaultVigilanteConfig() *config.Config {
	defaultConfig := config.DefaultConfig()
	defaultConfig.BTC.NetParams = regtestParams.Name
	defaultConfig.BTC.Endpoint = "127.0.0.1:18443"
	// Config setting necessary to connect btcwallet daemon
	defaultConfig.BTC.WalletPassword = "pass"
	defaultConfig.BTC.Username = "user"
	defaultConfig.BTC.Password = "pass"
	defaultConfig.BTC.ZmqSeqEndpoint = config.DefaultZmqSeqEndpoint

	return defaultConfig
}

type TestManager struct {
	TestRpcClient   *rpcclient.Client
	BitcoindHandler *BitcoindTestHandler
	BabylonClient   *bbnclient.Client
	BTCClient       *btcclient.Client
	Config          *config.Config
	WalletPrivKey   *btcec.PrivateKey
	manger          *container.Manager
	mu              sync.Mutex
}

func initBTCClientWithSubscriber(ctx context.Context, cfg *config.Config) *btcclient.Client {
	client, err := btcclient.NewWallet(cfg, zap.NewNop())
	if err != nil {
		panic(err)
	}

	// let's wait until chain rpc becomes available
	// poll time is increase here to avoid spamming the rpc server
	var innerErr error
	err = lib.Eventually(ctx, func() bool {
		if _, err := client.GetBlockCount(); err != nil {
			innerErr = err
			return false
		}

		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime, "err waiting init btc client")

	if err != nil {
		panic(fmt.Errorf("err %v inner: %v", err, innerErr))
	}

	return client
}

// StartManager creates a test manager
// NOTE: uses btc client with zmq
func StartManager(ctx context.Context, numMatureOutputsInWallet uint32, epochInterval uint) (*TestManager, error) {
	manager, err := container.NewManager()
	if err != nil {
		return nil, err
	}

	btcHandler := NewBitcoindHandler(manager)
	bitcoind, err := btcHandler.Start(ctx, "bitcoind")
	if err != nil {
		return nil, err
	}

	passphrase := "pass"
	_ = btcHandler.CreateWallet(ctx, "default", passphrase)

	cfg := defaultVigilanteConfig()

	cfg.BTC.Endpoint = fmt.Sprintf("127.0.0.1:%s", bitcoind.GetPort("18443/tcp"))

	testRpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 cfg.BTC.Endpoint,
		User:                 cfg.BTC.Username,
		Pass:                 cfg.BTC.Password,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		HTTPPostMode:         true,
	}, nil)
	if err != nil {
		return nil, err
	}

	err = testRpcClient.WalletPassphrase(passphrase, 600)
	if err != nil {
		return nil, err
	}

	walletPrivKey, err := importPrivateKey(ctx, btcHandler)
	if err != nil {
		return nil, err
	}
	blocksResponse := btcHandler.GenerateBlocks(ctx, int(numMatureOutputsInWallet))

	btcClient := initBTCClientWithSubscriber(ctx, cfg)

	var buff bytes.Buffer
	err = regtestParams.GenesisBlock.Header.Serialize(&buff)
	if err != nil {
		return nil, err
	}
	baseHeaderHex := hex.EncodeToString(buff.Bytes())

	minerAddressDecoded, err := btcutil.DecodeAddress(blocksResponse.Address, regtestParams)
	if err != nil {
		return nil, err
	}

	pkScript, err := txscript.PayToAddrScript(minerAddressDecoded)
	if err != nil {
		return nil, err
	}

	// start Babylon node

	tmpDir, err := tempDir()
	if err != nil {
		return nil, err
	}

	babylond, err := manager.RunBabylondResource("babylon-master", tmpDir, baseHeaderHex, hex.EncodeToString(pkScript), epochInterval)
	if err != nil {
		return nil, err
	}

	// create Babylon client
	cfg.Babylon.KeyDirectory = filepath.Join(tmpDir, "node0", "babylond")
	cfg.Babylon.Key = "test-spending-key" // keyring to bbn node
	cfg.Babylon.GasAdjustment = 3.0

	// update port with the dynamically allocated one from docker
	cfg.Babylon.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.Babylon.GRPCAddr = fmt.Sprintf("https://localhost:%s", babylond.GetPort("9090/tcp"))

	babylonClient, err := bbnclient.New(&cfg.Babylon, nil)
	if err != nil {
		return nil, err
	}

	// wait until Babylon is ready
	err = lib.Eventually(ctx, func() bool {
		_, err := babylonClient.CurrentEpoch()
		if err != nil {
			return false
		}
		//log.Infof("Babylon is ready: %v", resp)
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime, "err waiting current epoch")

	if err != nil {
		return nil, err
	}
	return &TestManager{
		TestRpcClient:   testRpcClient,
		BabylonClient:   babylonClient,
		BitcoindHandler: btcHandler,
		BTCClient:       btcClient,
		Config:          cfg,
		WalletPrivKey:   walletPrivKey,
		manger:          manager,
	}, nil
}

func (tm *TestManager) Stop() {
	if tm.BabylonClient.IsRunning() {
		err := tm.BabylonClient.Stop()
		fmt.Printf("err stopping client %v", err)
	}
	if err := tm.manger.ClearResources(); err != nil {
		fmt.Printf("err clearning docker resource %v", err)
	}
}

// mineBlock mines a single block
func (tm *TestManager) mineBlock(ctx context.Context) *wire.MsgBlock {
	resp := tm.BitcoindHandler.GenerateBlocks(ctx, 1)

	hash, err := chainhash.NewHashFromStr(resp.Blocks[0])
	if err != nil {
		panic(err)
	}

	header, err := tm.TestRpcClient.GetBlock(hash)
	if err != nil {
		panic(err)
	}

	return header
}

func (tm *TestManager) MustGetBabylonSigner() string {
	return tm.BabylonClient.MustGetAddr()
}

// RetrieveTransactionFromMempool fetches transactions from the mempool for the given hashes
func (tm *TestManager) RetrieveTransactionFromMempool(t *testing.T, hashes []*chainhash.Hash) []*btcutil.Tx {
	var txs []*btcutil.Tx
	for _, txHash := range hashes {
		tx, err := tm.BTCClient.GetRawTransaction(txHash)
		require.NoError(t, err)
		txs = append(txs, tx)
	}

	return txs
}

func (tm *TestManager) InsertBTCHeadersToBabylon(ctx context.Context, headers []*wire.BlockHeader) (*pv.RelayerTxResponse, error) {
	var headersBytes []bbn.BTCHeaderBytes

	for _, h := range headers {
		headersBytes = append(headersBytes, bbn.NewBTCHeaderBytesFromBlockHeader(h))
	}

	msg := btclctypes.MsgInsertHeaders{
		Headers: headersBytes,
		Signer:  tm.MustGetBabylonSigner(),
	}

	return tm.BabylonClient.InsertHeaders(ctx, &msg)
}

func (tm *TestManager) CatchUpBTCLightClient(ctx context.Context) error {
	btcHeight, err := tm.TestRpcClient.GetBlockCount()
	if err != nil {
		return err
	}

	tipResp, err := tm.BabylonClient.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	btclcHeight := tipResp.Header.Height

	var headers []*wire.BlockHeader
	for i := int(btclcHeight + 1); i <= int(btcHeight); i++ {
		hash, err := tm.TestRpcClient.GetBlockHash(int64(i))
		if err != nil {
			return err
		}
		header, err := tm.TestRpcClient.GetBlockHeader(hash)
		if err != nil {
			return err
		}
		headers = append(headers, header)
	}

	_, err = tm.InsertBTCHeadersToBabylon(ctx, headers)
	if err != nil {
		return err
	}

	return nil
}

func importPrivateKey(ctx context.Context, btcHandler *BitcoindTestHandler) (*btcec.PrivateKey, error) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	wif, err := btcutil.NewWIF(privKey, regtestParams, true)
	if err != nil {
		return nil, err
	}

	// "combo" allows us to import a key and handle multiple types of btc scripts with a single descriptor command.
	descriptor := fmt.Sprintf("combo(%s)", wif.String())

	// Create the JSON descriptor object.
	descJSON, err := json.Marshal([]map[string]interface{}{
		{
			"desc":      descriptor,
			"active":    true,
			"timestamp": "now", // tells Bitcoind to start scanning from the current blockchain height
			"label":     "test key",
		},
	})

	if err != nil {
		return nil, err
	}

	btcHandler.ImportDescriptors(ctx, string(descJSON))

	return privKey, nil
}

func tempDir() (string, error) {
	tempPath, err := os.MkdirTemp(os.TempDir(), "babylon-test-*")
	if err != nil {
		return "", err
	}

	if err = os.Chmod(tempPath, 0777); err != nil {
		return "", err
	}

	// todo(lazar): this
	//t.Cleanup(func() {
	//	_ = os.RemoveAll(tempPath)
	//})

	return tempPath, err
}

func (tm *TestManager) AtomicFundSignSendStakingTx(stakingOutput *wire.TxOut) (*wire.MsgTx, *chainhash.Hash, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	// 	1 sat/vB
	// = 1 sat/vB × 1000 vB/kvB
	// = 1000 sat/kvB × 1/100'000'000 ₿/sat
	// = 103 × 10-8 ₿/kvB
	// = 10-5 ₿/kvB
	// = 1 / 100'000 ₿/kvB

	feeRate := float64(0.00002)
	pos := 1
	// isWitness := true

	err := tm.TestRpcClient.WalletPassphrase("pass", 60)
	if err != nil {
		return nil, nil, err
	}

	tx := wire.NewMsgTx(2)
	tx.AddTxOut(stakingOutput)

	rawTxResult, err := tm.TestRpcClient.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{
		FeeRate:        &feeRate,
		ChangePosition: &pos,
	}, nil)
	if err != nil {
		return nil, nil, err
	}

	signed, all, err := tm.TestRpcClient.SignRawTransactionWithWallet(rawTxResult.Transaction)
	if err != nil {
		return nil, nil, err
	}
	if !all {
		return nil, nil, fmt.Errorf("all inputs need to be signed %s", rawTxResult.Transaction.TxID())
	}

	txHash, err := tm.TestRpcClient.SendRawTransaction(signed, true)
	if err != nil {
		return nil, nil, err
	}

	return rawTxResult.Transaction, txHash, nil
}

func (tm *TestManager) fundAllParties(
	ctx context.Context,
	senders []*SenderWithBabylonClient,
) error {

	fundingAccount := tm.BabylonClient.MustGetAddr()
	fundingAddress := sdk.MustAccAddressFromBech32(fundingAccount)

	var msgs []sdk.Msg

	for _, sender := range senders {
		msg := banktypes.NewMsgSend(fundingAddress, sender.BabylonAddress, types.NewCoins(types.NewInt64Coin("ubbn", 100000000)))
		msgs = append(msgs, msg)
	}

	resp, err := tm.BabylonClient.ReliablySendMsgs(
		ctx,
		msgs,
		[]*errors.Error{},
		[]*errors.Error{},
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("resp fund parties empty")
	}

	return nil
}
