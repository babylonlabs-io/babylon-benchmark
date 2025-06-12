package harness

import (
	"fmt"
	"strings"

	"github.com/babylonlabs-io/babylon-benchmark/config"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/rpcclient"
)

type BTCClient struct {
	client *rpcclient.Client
	config config.BTCConfig
}

func NewBTCClient(runCfg config.Config) (*BTCClient, error) {
	rpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 rpcHostURL(runCfg.BTCRPC, runCfg.WalletName),
		User:                 runCfg.BTCUser,
		Pass:                 runCfg.BTCPass,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		HTTPPostMode:         true,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}

	_, err = rpcClient.CreateWallet(
		runCfg.WalletName,
		rpcclient.WithCreateWalletPassphrase(runCfg.WalletPassphrase),
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	client := &BTCClient{
		client: rpcClient,
		config: config.BTCConfig{
			Endpoint:   runCfg.BTCRPC,
			Username:   runCfg.BTCUser,
			Password:   runCfg.BTCPass,
			WalletName: runCfg.WalletName,
		},
	}
	return client, nil
}

func (c *BTCClient) Start(runCfg config.Config) error {
	rpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 rpcHostURL(c.config.Endpoint, c.config.WalletName),
		User:                 c.config.Username,
		Pass:                 c.config.Password,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		HTTPPostMode:         true,
	}, nil)
	if err != nil {
		return err
	}

	c.client = rpcClient

	if err := c.importKey(runCfg.Keys); err != nil {
		return err
	}

	return nil
}

func rpcHostURL(host, walletName string) string {
	if len(walletName) > 0 {
		return host + "/wallet/" + walletName
	}

	return host
}

func (c *BTCClient) importKey(path string) error {
	keys, err := LoadKeys(path)
	if err != nil {
		return err
	}

	wif, err := btcutil.DecodeWIF(keys.BitcoinKey.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to decode WIF: %w", err)
	}

	if err := c.client.ImportPrivKey(wif); err != nil {
		return fmt.Errorf("failed to import private key: %w", err)
	}

	c.config.PrivateKey = keys.BitcoinKey.PrivKey
	c.config.Address = keys.BitcoinKey.Address

	return nil
}

func (c *BTCClient) Stop() {
	if c.client != nil {
		c.client.Shutdown()
	}
}
