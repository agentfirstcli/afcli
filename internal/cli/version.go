package cli

import (
	"errors"
	"fmt"

	"github.com/agentfirstcli/afcli/internal/version"
	"github.com/spf13/cobra"
)

// errVersion is returned by PersistentPreRunE after rendering the
// --version line to stdout. Execute() recognises this sentinel and
// exits 0 without re-rendering or running any subcommand. Mirrors the
// errHelpSchema sentinel pattern.
var errVersion = errors.New("version emitted")

var versionCmd = &cobra.Command{
	Use:           "version",
	Short:         "Print version information and exit",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), version.String())
		return err
	},
}
