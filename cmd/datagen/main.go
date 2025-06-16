package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/babylonlabs-io/babylon-benchmark/cmd/datagen/cmd"
	"github.com/spf13/cobra"
)

// NewRootCmd creates a new root command for fpd. It is called once in the main function.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "dgd",
		Short:         "dgd - data generation daemon.",
		Long:          `dgd is tool to populate data babylon node.`,
		SilenceErrors: false,
	}

	return rootCmd
}

func main() {
	rootCmd := NewRootCmd()
	rootCmd.AddCommand(
		cmd.CommandVersion(),
		cmd.CommandGenerate(),
		cmd.CommandGenerateAndSaveKey(),
		cmd.CommandGenerateRemote(),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "⚠️: There was an error while executing dgd CLI '%s'\n", err)
		os.Exit(1)
	}
}
