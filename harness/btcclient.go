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
	TestRpcClient *rpcclient.Client
	BTCConfig
}

func NewBTCClient(runCfg benchcfg.Config) (*BTCClient, error) {
	client := &BTCClient{
		BTCConfig: BTCConfig{
			RPCAddr:  runCfg.BTCRPC,
			GRPCAddr: runCfg.BTCGRPC,
			Username: runCfg.BTCUser,
			Password: runCfg.BTCPass,
		},
	}

	rpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 client.RPCAddr,
		User:                 client.Username,
		Pass:                 client.Password,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		HTTPPostMode:         true,
	}, nil)
	if err != nil {
		return nil, err
	}

	client.ImportKey(runCfg.HomeDir)

	client.TestRpcClient = rpcClient
	return client, nil
}

func (cfg *BTCConfig) ImportKey(path string) error {
	keys, err := LoadKeys(path)
	if err != nil {
		return err
	}

	cfg.PubKey = keys.BitcoinKey.PrivKey
	cfg.PrivKey = keys.BabylonKey.PubKey
	cfg.BtcAddress = keys.BabylonKey.Address

	
	return nil
}
