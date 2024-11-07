package harness

import (
	"bytes"
	"context"
	"cosmossdk.io/errors"
	"encoding/hex"
	"encoding/json"
	"fmt"
	benchcfg "github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/babylonlabs-io/babylon-benchmark/container"
	"github.com/babylonlabs-io/babylon-benchmark/lib"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbncfg "github.com/babylonlabs-io/babylon/client/config"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"os"
	"path/filepath"
	"time"
)

var (
	babylonTag            = []byte{1, 2, 3, 4} //nolint:unused
	eventuallyWaitTimeOut = 40 * time.Second
	eventuallyPollTime    = 1 * time.Second
	regtestParams         = &chaincfg.RegressionNetParams
)

const (
	chainId = "chain-test"
)

type Config struct {
	BTC      benchcfg.BTCConfig   `mapstructure:"btc"`
	Babylon0 bbncfg.BabylonConfig `mapstructure:"babylon"`
	Babylon1 bbncfg.BabylonConfig `mapstructure:"babylon"`
}

func DefaultConfig() *Config {
	return &Config{
		BTC:      benchcfg.DefaultBTCConfig(),
		Babylon0: bbncfg.DefaultBabylonConfig(),
		Babylon1: bbncfg.DefaultBabylonConfig(),
	}
}

func defaultConfig() *Config {
	cfg := DefaultConfig()
	cfg.BTC.NetParams = regtestParams.Name
	cfg.BTC.Endpoint = "127.0.0.1:18443"
	cfg.BTC.WalletPassword = "pass"
	cfg.BTC.Username = "user"
	cfg.BTC.Password = "pass"
	cfg.BTC.ZmqSeqEndpoint = benchcfg.DefaultZmqSeqEndpoint

	return cfg
}

type TestManager struct {
	TestRpcClient      *rpcclient.Client
	BitcoindHandler    *BitcoindTestHandler
	BabylonClientNode0 *bbnclient.Client
	BabylonClientNode1 *bbnclient.Client
	Config             *Config
	WalletPrivKey      *btcec.PrivateKey
	manger             *container.Manager
	babylonDir         string
	benchConfig        benchcfg.Config
}

// StartManager creates a test manager
func StartManager(ctx context.Context, outputsInWallet uint32, epochInterval uint, runCfg benchcfg.Config) (*TestManager, error) {
	manager, err := container.NewManager()
	if err != nil {
		return nil, err
	}

	btcHandler := NewBitcoindHandler(manager)
	bitcoind, err := btcHandler.Start(ctx)
	if err != nil {
		return nil, err
	}

	passphrase := "pass"
	_ = btcHandler.CreateWallet(ctx, "default", passphrase)

	cfg := defaultConfig()

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

	if err = testRpcClient.WalletPassphrase(passphrase, 600); err != nil {
		return nil, err
	}

	walletPrivKey, err := importPrivateKey(ctx, btcHandler)
	if err != nil {
		return nil, err
	}
	blocksResponse := btcHandler.GenerateBlocks(ctx, int(outputsInWallet))

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

	// start Babylon0 node
	babylonDir, err := tempDir()
	if err != nil {
		return nil, err
	}

	if runCfg.BabylonPath != "" {
		babylonDir = runCfg.BabylonPath // override with cfg
	}

	babylond, babylondNode1, err := manager.RunBabylondResource(babylonDir, baseHeaderHex, hex.EncodeToString(pkScript), epochInterval)
	if err != nil {
		return nil, err
	}

	_ = babylondNode1

	// create a Babylon0 client
	cfg.Babylon0.KeyDirectory = filepath.Join(babylonDir, "node0", "babylond")
	cfg.Babylon0.Key = "test-spending-key" // keyring to bbn node
	cfg.Babylon0.GasAdjustment = 3.0

	// update port with the dynamically allocated one from docker
	cfg.Babylon0.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.Babylon0.GRPCAddr = fmt.Sprintf("https://localhost:%s", babylond.GetPort("9090/tcp"))

	// create a Babylon1 client
	cfg.Babylon1.KeyDirectory = filepath.Join(babylonDir, "node0", "babylond")
	cfg.Babylon1.Key = "test-spending-key" // keyring to bbn node
	cfg.Babylon1.GasAdjustment = 3.0

	// update port with the dynamically allocated one from docker
	cfg.Babylon1.RPCAddr = fmt.Sprintf("http://localhost:%s", babylondNode1.GetPort("26657/tcp"))
	cfg.Babylon1.GRPCAddr = fmt.Sprintf("https://localhost:%s", babylondNode1.GetPort("9090/tcp"))

	babylonClientNode0, err := bbnclient.New(&cfg.Babylon0, nil)
	if err != nil {
		return nil, err
	}

	babylonClientNode1, err := bbnclient.New(&cfg.Babylon1, nil)
	if err != nil {
		return nil, err
	}

	// wait until Babylon0 is ready
	err = lib.Eventually(ctx, func() bool {
		_, err := babylonClientNode0.CurrentEpoch()

		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime, "err waiting current epoch")

	// wait until Babylon1 is ready
	err = lib.Eventually(ctx, func() bool {
		_, err := babylonClientNode1.CurrentEpoch()

		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime, "err waiting current epoch")

	if err != nil {
		return nil, err
	}
	return &TestManager{
		TestRpcClient:      testRpcClient,
		BabylonClientNode0: babylonClientNode0,
		BabylonClientNode1: babylonClientNode1,
		BitcoindHandler:    btcHandler,
		Config:             cfg,
		WalletPrivKey:      walletPrivKey,
		manger:             manager,
		babylonDir:         babylonDir,
		benchConfig:        runCfg,
	}, nil
}

func (tm *TestManager) Stop() {
	if tm.BabylonClientNode0.IsRunning() {
		err := tm.BabylonClientNode0.Stop()
		fmt.Printf("🚫 Rrr stopping client %v\n", err)

		err = tm.BabylonClientNode0.Stop()
		fmt.Printf("🚫 Rrr stopping client %v\n", err)
	}

	if err := tm.manger.ClearResources(); err != nil {
		fmt.Printf("🚫 Err clearning docker resource %v\n", err)
	}

	tm.BitcoindHandler.Stop()

	if tm.benchConfig.BabylonPath == "" {
		cleanupDir(tm.babylonDir) // don't cleanup babylon if user specified a path
	}
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

	return tempPath, err
}

func cleanupDir(path string) {
	_ = os.RemoveAll(path)
}

func (tm *TestManager) AtomicFundSignSendStakingTx(stakingOutput *wire.TxOut) (*wire.MsgTx, *chainhash.Hash, error) {
	// 	1 sat/vB
	// = 1 sat/vB × 1000 vB/kvB
	// = 1000 sat/kvB × 1/100'000'000 ₿/sat
	// = 103 × 10-8 ₿/kvB
	// = 10-5 ₿/kvB
	// = 1 / 100'000 ₿/kvB

	feeRate := float64(0.00002)
	pos := 1

	err := tm.TestRpcClient.WalletPassphrase("pass", 60)
	if err != nil {
		return nil, nil, err
	}

	tx := wire.NewMsgTx(2)
	tx.AddTxOut(stakingOutput)

	// todo(lazar): investigate if we can push this more, currently max ~50txs/block
	lock := true
	rawTxResult, err := tm.TestRpcClient.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{
		FeeRate:        &feeRate,
		ChangePosition: &pos,
		LockUnspents:   &lock,
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

	fundingAccount := tm.BabylonClientNode0.MustGetAddr()
	fundingAddress := sdk.MustAccAddressFromBech32(fundingAccount)

	var msgs []sdk.Msg

	for _, sender := range senders {
		msg := banktypes.NewMsgSend(fundingAddress, sender.BabylonAddress, types.NewCoins(types.NewInt64Coin("ubbn", 100000000)))
		msgs = append(msgs, msg)
	}

	resp, err := tm.BabylonClientNode0.ReliablySendMsgs(
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

func (tm *TestManager) listBlocksForever(ctx context.Context) {
	lt := time.NewTicker(5 * time.Second)
	defer lt.Stop()

	for {
		select {

		case <-ctx.Done():
			return
		case <-lt.C:
			resp, err := tm.BabylonClientNode0.ListBlocks(finalitytypes.QueriedBlockStatus_NON_FINALIZED, nil)
			if err != nil {
				fmt.Printf("🚫 Failed to list blocks: %v\n", err)
				continue
			}

			if len(resp.Blocks) == 0 {
				continue
			}

			fmt.Printf("🔎 Found %d non-finalized block(s). Next block to finalize: %d\n", len(resp.Blocks), resp.Blocks[0].Height)
		}
	}
}
