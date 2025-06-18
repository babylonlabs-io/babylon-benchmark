package harness

import (
	"github.com/babylonlabs-io/babylon-benchmark/config"

	"github.com/btcsuite/btcd/rpcclient"
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
	c.ConvertToBTCConfig(cfg)
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
