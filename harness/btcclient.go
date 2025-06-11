package harness

import (
	benchcfg "github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/btcsuite/btcd/rpcclient"
)

type BTCConfig struct {
	RPCAddr    string
	GRPCAddr   string
	BtcAddress string
	PubKey     string
	PrivKey    string
	Username   string
	Password   string
}

type BTCClient struct {
	*rpcclient.Client
	BTCConfig
}

func NewBTCClient(runCfg benchcfg.Config) (*BTCClient, error) {
	client := &BTCClient{
		BTCConfig: BTCConfig{
			RPCAddr:  runCfg.BabylonRPC,
			GRPCAddr: runCfg.BabylonGRPC,
			Username: runCfg.BTCUser,
			Password: runCfg.BTCPass,
		},
	}
	return client, nil
}

func (c *BTCClient) Start(runCfg benchcfg.Config) error {
	rpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 c.RPCAddr,
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

	if err := c.importKey(runCfg.PathToKeyExport); err != nil {
		return err
	}

	c.Client = rpcClient
	return nil
}

func (cfg *BTCConfig) importKey(path string) error {
	keys, err := LoadKeys(path)
	if err != nil {
		return err
	}

	cfg.PubKey = keys.BitcoinKey.PrivKey
	cfg.PrivKey = keys.BabylonKey.PubKey
	cfg.BtcAddress = keys.BabylonKey.Address

	return nil
}

func (c *BTCClient) Stop() {
	if c.Client != nil {
		c.Client.Shutdown()
	}
}
