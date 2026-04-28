package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit <target>",
	Short: "Audit a target against agent-first-cli principles",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), args[0])
		return nil
	},
}
