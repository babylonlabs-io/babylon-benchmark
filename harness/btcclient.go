package harness

import (
	"fmt"
	"strings"

	"github.com/babylonlabs-io/babylon-benchmark/config"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/rpcclient"
)

type BTCClient struct {
	*rpcclient.Client
	config.BTCConfig
}

func NewBTCClient(runCfg config.Config) (*BTCClient, error) {
	client := &BTCClient{
		Client: &rpcclient.Client{},
		BTCConfig: config.BTCConfig{
			Endpoint: runCfg.BTCRPC,
			Username: runCfg.BTCUser,
			Password: runCfg.BTCPass,
		},
	}
	return client, nil
}

func (c *BTCClient) Start(runCfg config.Config) error {
	rpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 rpcHostURL(c.Endpoint, c.WalletName),
		User:                 c.Username,
		Pass:                 c.Password,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		HTTPPostMode:         true,
	}, nil)
	if err != nil {
		return err
	}

	c.Client = rpcClient
	c.WalletName = "test"

	if err := c.CreateWallet(c.WalletName); err != nil {
		return fmt.Errorf("failed to create/load wallet: %w", err)
	}

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

func (c *BTCClient) CreateWallet(walletName string) error {
	_, err := c.Client.GetWalletInfo()
	if err == nil {
		return nil
	}

	_, err = c.Client.CreateWallet(
		walletName,
		rpcclient.WithCreateWalletPassphrase(c.Password),
	)

	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			_, err = c.Client.LoadWallet(walletName)
			if err != nil {
				return fmt.Errorf("failed to load existing wallet: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create wallet: %w", err)
		}
	}

	err = c.Client.WalletPassphrase(c.Password, 60)
	if err != nil {
		return fmt.Errorf("failed to unlock wallet: %w", err)
	}

	return nil
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

	if err := c.Client.ImportPrivKey(wif); err != nil {
		return fmt.Errorf("failed to import private key: %w", err)
	}

	c.PrivateKey = keys.BitcoinKey.PrivKey
	c.Address = keys.BitcoinKey.Address

	return nil
}

func (c *BTCClient) Stop() {
	if c.Client != nil {
		c.Client.Shutdown()
	}
}
