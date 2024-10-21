package cmd

import (
	"github.com/babylonlabs-io/babylon-benchmark/lib/versioninfo"
	"github.com/spf13/cobra"
	"strings"
)

// CommandVersion prints cmd version
func CommandVersion() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "version",
		Short:   "Prints version.",
		Aliases: []string{"v"},
		Example: `dgd version`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			version := versioninfo.Version()
			commit, ts := versioninfo.CommitInfo()

			if version == "" {
				version = "main"
			}

			var sb strings.Builder
			_, _ = sb.WriteString("Version:       " + version)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Commit:    " + commit)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Timestamp: " + ts)
			_, _ = sb.WriteString("\n")

			cmd.Printf(sb.String())
		},
	}
	return cmd
}
