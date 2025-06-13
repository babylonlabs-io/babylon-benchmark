package harness

import (
	"github.com/babylonlabs-io/babylon-benchmark/config"

	"github.com/btcsuite/btcd/rpcclient"
)

type BTCClient struct {
	client *rpcclient.Client
	config config.BTCConfig
}

func NewBTCClient(cfg config.BTCConfig) (*BTCClient, error) {
	client := &BTCClient{
		config: cfg,
	}
	return client, nil
}

func (c *BTCClient) Start() error {
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
	return config.BTCConfig{
		Endpoint:       cfg.BTCRPC,
		WalletName:     cfg.WalletName,
		WalletPassword: cfg.WalletPassphrase,
		Username:       cfg.BTCUser,
		Password:       cfg.BTCPass,
	}
}
