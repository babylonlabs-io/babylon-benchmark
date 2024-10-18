package cmd

import (
	"github.com/spf13/cobra"
	"strings"
)

// CommandVersion prints cmd version
func CommandVersion() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "version",
		Short:   "Print version of this binary.",
		Example: `dgd version`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			// todo(lazar): real version command

			version := "v0.0.1"
			commit := "todo"
			timestamp := "todo"

			var sb strings.Builder
			_, _ = sb.WriteString("Version       " + version)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Commit    " + commit)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Timestamp " + timestamp)
			_, _ = sb.WriteString("\n")

			cmd.Printf(sb.String())
		},
	}
	return cmd
}
