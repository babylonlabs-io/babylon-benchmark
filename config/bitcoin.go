package config

import (
	"github.com/babylonlabs-io/babylon/types"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
)

// BTCConfig defines configuration for the Bitcoin client
type BTCConfig struct {
	Endpoint          string                `mapstructure:"endpoint"`
	WalletPassword    string                `mapstructure:"wallet-password"`
	WalletName        string                `mapstructure:"wallet-name"`
	WalletLockTime    int64                 `mapstructure:"wallet-lock-time"` // time duration in which the wallet remains unlocked, in seconds
	TxFeeMin          chainfee.SatPerKVByte `mapstructure:"tx-fee-min"`       // minimum tx fee, sat/kvb
	TxFeeMax          chainfee.SatPerKVByte `mapstructure:"tx-fee-max"`       // maximum tx fee, sat/kvb
	DefaultFee        chainfee.SatPerKVByte `mapstructure:"default-fee"`      // default BTC tx fee in case estimation fails, sat/kvb
	EstimateMode      string                `mapstructure:"estimate-mode"`    // the BTC tx fee estimate mode, which is only used by bitcoind, must be either ECONOMICAL or CONSERVATIVE
	TargetBlockNum    int64                 `mapstructure:"target-block-num"` // this implies how soon the tx is estimated to be included in a block, e.g., 1 means the tx is estimated to be included in the next block
	NetParams         string                `mapstructure:"net-params"`
	Username          string                `mapstructure:"username"`
	Password          string                `mapstructure:"password"`
	ReconnectAttempts int                   `mapstructure:"reconnect-attempts"`
	ZmqSeqEndpoint    string                `mapstructure:"zmq-seq-endpoint"`
	ZmqBlockEndpoint  string                `mapstructure:"zmq-block-endpoint"`
	ZmqTxEndpoint     string                `mapstructure:"zmq-tx-endpoint"`
}

const (
	DefaultRpcBtcNodeHost      = "127.0.01:18556"
	DefaultBtcNodeRpcUser      = "rpcuser"
	DefaultBtcNodeRpcPass      = "rpcpass"
	DefaultBtcNodeEstimateMode = "CONSERVATIVE"
	DefaultZmqSeqEndpoint      = "tcp://127.0.0.1:28333"
	DefaultZmqBlockEndpoint    = "tcp://127.0.0.1:29001"
	DefaultZmqTxEndpoint       = "tcp://127.0.0.1:29002"
)

func DefaultBTCConfig() BTCConfig {
	return BTCConfig{
		Endpoint:          DefaultRpcBtcNodeHost,
		WalletPassword:    "walletpass",
		WalletName:        "default",
		WalletLockTime:    10,
		TxFeeMax:          chainfee.SatPerKVByte(20 * 1000), // 20,000sat/kvb = 20sat/vbyte
		TxFeeMin:          chainfee.SatPerKVByte(1 * 1000),  // 1,000sat/kvb = 1sat/vbyte
		DefaultFee:        chainfee.SatPerKVByte(1 * 1000),  // 1,000sat/kvb = 1sat/vbyte
		EstimateMode:      DefaultBtcNodeEstimateMode,
		TargetBlockNum:    1,
		NetParams:         string(types.BtcSimnet),
		Username:          DefaultBtcNodeRpcUser,
		Password:          DefaultBtcNodeRpcPass,
		ReconnectAttempts: 3,
		ZmqSeqEndpoint:    DefaultZmqSeqEndpoint,
		ZmqBlockEndpoint:  DefaultZmqBlockEndpoint,
		ZmqTxEndpoint:     DefaultZmqTxEndpoint,
	}
}
