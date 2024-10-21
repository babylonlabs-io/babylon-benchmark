package cmd

import (
	"github.com/babylonlabs-io/babylon-benchmark/harness"
	"github.com/spf13/cobra"
)

// CommandGenerate generates data
func CommandGenerate() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generates data #TBD all info.", // todo(lazar): more info here, args, config etc
		Example: `dgd generate`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return harness.Run(cmd.Context())
		},
	}
	return cmd
}
