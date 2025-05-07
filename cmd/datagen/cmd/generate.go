package cmd

import (
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/babylonlabs-io/babylon-benchmark/harness"
	"github.com/spf13/cobra"
)

const (
	totalFpFlag          = "total-fps"
	totalStakersFlag     = "total-stakers"
	babylonPathFlag      = "babylon-path"
	totalDelegationsFlag = "total-delegations"
	numPubRandFlag       = "num-public-randomness"
	iavlDisabledFastnode = "iavl-disabled-fastnode"
	iavlCacheSize        = "iavl-cache-size"
	numMatureOutputsFlag = "num-mature-outputs"
)

// CommandGenerate generates data
func CommandGenerate() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generates delegations with configurable finality providers, stakers, and total delegations",
		Example: `dgd generate --babylon-path /path/to/babylon --total-fps 5 --total-stakers 150 --total-delegations 500`,
		Args:    cobra.NoArgs,
		RunE:    cmdGenerate,
	}

	f := cmd.Flags()
	f.String(babylonPathFlag, "", "Path to which babylond docker will mount (optional, tmp dir will be used as default)")
	f.Int(totalFpFlag, 3, "Number of finality providers to run (optional)")
	f.Int(totalDelegationsFlag, 0, "Number of delegations to run this cmd, after it we will exit. (optional, 0 for indefinite)")
	f.Int(totalStakersFlag, 100, "Number of stakers to run (optional)")
	f.Uint32(numPubRandFlag, 150_000, "Number of pub randomness to commit, should be a high value (optional)")
	f.Bool(iavlDisabledFastnode, true, "IAVL disabled fast node (additional fast node cache) (optional)")
	f.Uint(iavlCacheSize, 0, "IAVL cache size, note cache too big can cause OOM, 100k -> ~20 GB of RAM (optional)")
	f.Uint32(numMatureOutputsFlag, 6000, "Number of blocks to be mined")

	return cmd
}

func cmdGenerate(cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()
	babylonPath, err := flags.GetString(babylonPathFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", babylonPathFlag, err)
	}

	totalFps, err := flags.GetInt(totalFpFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", totalFpFlag, err)
	}

	totalDelegations, err := flags.GetInt(totalDelegationsFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", totalDelegationsFlag, err)
	}

	totalStakers, err := flags.GetInt(totalStakersFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", totalStakersFlag, err)
	}

	numPubRand, err := flags.GetUint32(numPubRandFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", numPubRandFlag, err)
	}

	iavlCache, err := flags.GetUint(iavlCacheSize)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", iavlCacheSize, err)
	}

	disabledFastnode, err := flags.GetBool(iavlDisabledFastnode)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", iavlDisabledFastnode, err)
	}

	numMatureOutputs, err := flags.GetUint32(numMatureOutputsFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", numMatureOutputsFlag, err)
	}

	cfg := config.Config{
		NumPubRand:             numPubRand,
		TotalStakers:           totalStakers,
		TotalFinalityProviders: totalFps,
		TotalDelegations:       totalDelegations,
		BabylonPath:            babylonPath,
		IavlCacheSize:          iavlCache,
		IavlDisableFastnode:    disabledFastnode,
		NumMatureOutputs:       numMatureOutputs,
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	return harness.Run(cmd.Context(), cfg)
}
