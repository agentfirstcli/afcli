package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	outputFormat  string
	deterministic bool
)

// ctxKey is unexported so callers cannot collide with cli's context values.
type ctxKey int

const (
	deterministicKey ctxKey = iota
)

// WithDeterministic returns a child context carrying the effective
// deterministic-output decision. Renderers and the audit pipeline read
// it via DeterministicFromContext.
func WithDeterministic(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, deterministicKey, v)
}

// DeterministicFromContext reports whether deterministic-output mode is
// active for the current command. Defaults to false.
func DeterministicFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if v, ok := ctx.Value(deterministicKey).(bool); ok {
		return v
	}
	return false
}

// resolveDeterministic merges the --deterministic flag with the
// AFCLI_DETERMINISTIC env var. Precedence: an explicit --deterministic=true
// wins; otherwise AFCLI_DETERMINISTIC=1 enables it.
func resolveDeterministic(flag bool) bool {
	if flag {
		return true
	}
	return os.Getenv("AFCLI_DETERMINISTIC") == "1"
}

var rootCmd = &cobra.Command{
	Use:   "afcli",
	Short: "Agent-first CLI auditor",
	Long:  "afcli audits binaries and projects against agent-first-cli design principles.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		cmd.SetContext(WithDeterministic(ctx, resolveDeterministic(deterministic)))
		return nil
	},
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
