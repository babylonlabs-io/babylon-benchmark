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
	BTCGRPC                string
	UseRemote              bool
	BTCPass                string
	BTCUser                string
	HomeDir                string
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

	if c.UseRemote {
		if c.BTCGRPC == "" {
			return fmt.Errorf("btcrpc should not be empty")
		}

		if c.BTCGRPC == "" {
			return fmt.Errorf("btcgrpc should not be empty")
		}
	}

	return nil
}
