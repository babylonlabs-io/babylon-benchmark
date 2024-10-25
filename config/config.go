package config

import "fmt"

type Config struct {
	TotalStakers           int
	TotalFinalityProviders int
	TotalDelegations       int
	BabylonPath            string
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

	return nil
}
