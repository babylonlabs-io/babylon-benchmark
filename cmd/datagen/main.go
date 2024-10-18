package main

import (
	"context"
	"fmt"
	"github.com/babylonlabs-io/babylon-benchmark/cmd/datagen/cmd"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"syscall"
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
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	err := rootCmd.ExecuteContext(ctx)
	cancel()

	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "⚠️: There was an error while executing dgd CLI '%s'", err)
		os.Exit(1)
	}
}