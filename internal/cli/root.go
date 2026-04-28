package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	outputFormat  string
	deterministic bool
)

var rootCmd = &cobra.Command{
	Use:   "afcli",
	Short: "Agent-first CLI auditor",
	Long:  "afcli audits binaries and projects against agent-first-cli design principles.",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "text", "output format: json, text, or markdown")
	rootCmd.PersistentFlags().BoolVar(&deterministic, "deterministic", false, "produce deterministic output (zero timestamps/durations, relative paths, stable sort)")
	rootCmd.AddCommand(auditCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
