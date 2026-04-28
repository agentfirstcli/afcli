package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	stdtime "time"

	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/report"
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

// startedAt returns an RFC3339Nano timestamp for envelope/report headers,
// or "" when deterministic mode is requested. Called from the envelope
// renderer where the audit hasn't actually started — Execute() reaches
// this code path after Cobra parsing fails.
func startedAt(opts report.RenderOptions) string {
	if opts.Deterministic {
		return ""
	}
	return stdtime.Now().UTC().Format(stdtime.RFC3339Nano)
}

var rootCmd = &cobra.Command{
	Use:           "afcli",
	Short:         "Agent-first CLI auditor",
	Long:          "afcli audits binaries and projects against agent-first-cli design principles.",
	SilenceUsage:  true,
	SilenceErrors: true,
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
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "json", "output format: json, text, or markdown")
	rootCmd.PersistentFlags().BoolVar(&deterministic, "deterministic", false, "produce deterministic output (zero timestamps/durations, relative paths, stable sort)")
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(manifestCmd)
}

// Execute runs the root Cobra command and translates its outcome into a
// process exit code. Successful runs exit 0. Audit failures (target
// missing, target not executable) carry an *auditError and are rendered
// to stderr in the user-selected format with the carried exit code.
// Any other Cobra error — unknown flags, bad arg counts, unknown
// subcommands — is rendered as a USAGE envelope and exits with code 2.
func Execute() {
	err := rootCmd.Execute()
	if err == nil {
		os.Exit(exit.OK)
	}

	// Interrupt path: the audit handler has already written a partial
	// report to stdout; we just need to exit with the dedicated 130 code
	// instead of letting Cobra's error path render a usage envelope.
	if errors.Is(err, errInterrupted) {
		os.Exit(exit.Interrupted)
	}

	opts := report.RenderOptions{Deterministic: resolveDeterministic(deterministic)}

	var ae *auditError
	if errors.As(err, &ae) {
		if rerr := renderEnvelope(os.Stderr, ae.envelope, ae.target, opts, outputFormat); rerr != nil {
			fmt.Fprintln(os.Stderr, rerr)
		}
		os.Exit(ae.exitCode)
	}

	env := usageEnvelope(err)
	if rerr := renderEnvelope(os.Stderr, env, "", opts, outputFormat); rerr != nil {
		fmt.Fprintln(os.Stderr, rerr)
	}
	os.Exit(exit.Usage)
}
