package cli

import (
	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/spf13/cobra"
)

var manifestCmd = &cobra.Command{
	Use:           "manifest",
	Short:         "Inspect the embedded agent-first-cli manifest",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var manifestListCmd = &cobra.Command{
	Use:           "list",
	Short:         "List all 16 principles from the embedded manifest",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return manifest.Render(
			cmd.OutOrStdout(),
			manifest.Embedded,
			outputFormat,
			DeterministicFromContext(cmd.Context()),
		)
	},
}

func init() {
	manifestCmd.AddCommand(manifestListCmd)
}
