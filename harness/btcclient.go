package harness

import (
	"fmt"

	"github.com/babylonlabs-io/babylon-benchmark/config"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
)

type BTCClient struct {
	client *rpcclient.Client
	config config.BTCConfig
}

func defaultConfig() *Config {
	cfg := DefaultConfig()
	cfg.BTC.NetParams = regtestParams.Name
	cfg.BTC.Endpoint = "127.0.0.1:18443"
	cfg.BTC.WalletPassword = "pass"
	cfg.BTC.Username = "user"
	cfg.BTC.Password = "pass"
	cfg.BTC.ZmqSeqEndpoint = config.DefaultZmqSeqEndpoint

	return cfg
}

func NewBTCClient(cfg config.BTCConfig) (*BTCClient, error) {
	client := &BTCClient{
		config: cfg,
	}
	return client, nil
}

func (c *BTCClient) Setup(cfg config.Config) error {
	c.config = c.ConvertToBTCConfig(cfg)
	rpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         rpcHostURL(c.config.Endpoint, c.config.WalletName),
		User:         c.config.Username,
		Pass:         c.config.Password,
		DisableTLS:   true,
		HTTPPostMode: true,
	}, nil)
	if err != nil {
		return err
	}

	c.client = rpcClient

	return nil
}

func rpcHostURL(host, walletName string) string {
	if len(walletName) > 0 {
		return host + "/wallet/" + walletName
	}

	return host
}

func (c *BTCClient) Stop() {
	if c.client != nil {
		c.client.Shutdown()
	}
}

func (c *BTCClient) ConvertToBTCConfig(cfg config.Config) config.BTCConfig {
	btcCfg := config.BTCConfig{}

	if cfg.BTCRPC != "" {
		btcCfg.Endpoint = cfg.BTCRPC
	}
	if cfg.WalletName != "" {
		btcCfg.WalletName = cfg.WalletName
	}
	if cfg.WalletPassphrase != "" {
		btcCfg.WalletPassword = cfg.WalletPassphrase
	}
	if cfg.BTCUser != "" {
		btcCfg.Username = cfg.BTCUser
	}
	if cfg.BTCPass != "" {
		btcCfg.Password = cfg.BTCPass
	}

	return btcCfg
}

func (c *BTCClient) AtomicFundSignSendStakingTx(stakingOutput *wire.TxOut) (*wire.MsgTx, *chainhash.Hash, error) {
	// 	1 sat/vB
	// = 1 sat/vB × 1000 vB/kvB
	// = 1000 sat/kvB × 1/100'000'000 ₿/sat
	// = 103 × 10-8 ₿/kvB
	// = 10-5 ₿/kvB
	// = 1 / 100'000 ₿/kvB

	feeRate := float64(0.00002)
	pos := 1

	err := c.client.WalletPassphrase("pass", 60)
	if err != nil {
		return nil, nil, err
	}

	tx := wire.NewMsgTx(2)
	tx.AddTxOut(stakingOutput)

	lock := true
	rawTxResult, err := c.client.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{
		FeeRate:        &feeRate,
		ChangePosition: &pos,
		LockUnspents:   &lock,
	}, nil)
	if err != nil {
		return nil, nil, err
	}

	signed, all, err := c.client.SignRawTransactionWithWallet(rawTxResult.Transaction)
	if err != nil {
		return nil, nil, err
	}
	if !all {
		return nil, nil, fmt.Errorf("all inputs need to be signed %s", rawTxResult.Transaction.TxID())
	}

	txHash, err := c.client.SendRawTransaction(signed, true)
	if err != nil {
		return nil, nil, err
	}

	return rawTxResult.Transaction, txHash, nil
}
