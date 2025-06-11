package config

import "fmt"

type Config struct {
	NumPubRand             uint32
	NumMatureOutputs       uint32
	TotalStakers           int
	TotalFinalityProviders int
	TotalDelegations       int
	BabylonPath            string
	IavlDisableFastnode    bool
	IavlCacheSize          uint
	BTCRPC                 string
	BBNRPC                 string
	BBNGRPC                string
	BTCGRPC                string
	BTCPass                string
	BTCUser                string
	Keys                   string
}

func (c *Config) Validate() error {
	if c.TotalStakers > 1000 {
		return fmt.Errorf("max number of stakers is 1000")
	}

	if c.TotalFinalityProviders > 80 {
		return fmt.Errorf("max number of finality providers is 80")
	}

	if c.TotalDelegations < 0 || c.TotalDelegations > 10_000_000 {
		return fmt.Errorf("max number of total delegations has to be between [0, 10m]")
	}

	if c.NumPubRand > 10_000_000 {
		return fmt.Errorf("max numbe for pub randomness 10m")
	}

	if c.NumMatureOutputs <= 0 {
		return fmt.Errorf("num mature outputs should be greater than 0")
	}

	return nil
}

func (c *Config) ValidateRemote() error {
	if c.BTCGRPC == "" {
		return fmt.Errorf("btcrpc should not be empty")
	}

	if c.BTCGRPC == "" {
		return fmt.Errorf("btcgrpc should not be empty")
	}

	if c.BTCPass == "" {
		return fmt.Errorf("btcpass should not be empty")
	}

	if c.BTCUser == "" {
		return fmt.Errorf("btcuser should not be empty")
	}

	if c.Keys == "" {
		return fmt.Errorf("keys should not be empty")
	}

	return nil
}
