package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	stdtime "time"

	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/report"
	"github.com/agentfirstcli/afcli/internal/version"
	"github.com/spf13/cobra"
)

var (
	outputFormat  string
	deterministic bool
	quietFlag     bool
	helpSchema    bool
	versionFlag   bool
)

// ctxKey is unexported so callers cannot collide with cli's context values.
type ctxKey int

const (
	deterministicKey ctxKey = iota
	quietKey
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

// WithQuiet returns a child context carrying the effective --quiet
// decision. The text and markdown renderers read it via
// QuietFromContext to drop pass/skip findings and the header line.
func WithQuiet(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, quietKey, v)
}

// QuietFromContext reports whether quiet mode is active. Defaults to false.
func QuietFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if v, ok := ctx.Value(quietKey).(bool); ok {
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
	Use:   "afcli",
	Short: "Agent-first CLI auditor",
	Long: `afcli audits binaries and projects against agent-first-cli design principles.

EXIT STATUS
  0   OK — audit ran cleanly and no finding met the fail-on threshold
  1   findings at or above the fail-on threshold (default: high)
  2   usage error — unknown flag, bad arg count, or malformed descriptor
  3   could not audit — target missing, not executable, or probe denied
  4   internal error — unexpected failure inside afcli
  130 interrupted (SIGINT/SIGTERM); a partial report is written to stdout

PROGRESS & SILENCE
  --quiet, -q  suppress passes and the header in text/markdown output;
               --output json is unaffected (machine consumers always get
               the full 16-finding envelope).
  NO_COLOR     the text and markdown renderers emit no ANSI escapes, so
               the NO_COLOR convention (https://no-color.org) is honored
               by construction. Set NO_COLOR=1 to be explicit; afcli's
               output is unchanged either way.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ctx = WithDeterministic(ctx, resolveDeterministic(deterministic))
		ctx = WithQuiet(ctx, quietFlag)
		cmd.SetContext(ctx)
		if versionFlag {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), version.String()); err != nil {
				return err
			}
			return errVersion
		}
		if helpSchema {
			hs := BuildHelpSchema(cmd)
			if err := RenderHelpSchema(cmd.OutOrStdout(), hs); err != nil {
				return err
			}
			return errHelpSchema
		}
		return nil
	},
	// RunE makes rootCmd Runnable so PersistentPreRunE fires when the user
	// invokes `afcli --help-schema` directly. Without a RunE, Cobra's
	// execute() returns flag.ErrHelp before any pre-run hook runs and the
	// help-schema sentinel never gets a chance to short-circuit.
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "json", "output format: json, text, or markdown")
	rootCmd.PersistentFlags().BoolVar(&deterministic, "deterministic", false, "produce deterministic output (zero timestamps/durations, relative paths, stable sort)")
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "suppress passes and the header in text/markdown output (--output json is unaffected)")
	rootCmd.PersistentFlags().BoolVar(&helpSchema, "help-schema", false, "emit machine-parseable help schema as JSON and exit")
	rootCmd.PersistentFlags().BoolVar(&versionFlag, "version", false, "print version information and exit")
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(manifestCmd)
	rootCmd.AddCommand(versionCmd)
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
		// Report-aware exit: an audit on the clean rendering path
		// stashes its final report + threshold so we can route through
		// exit.MapFromReport. Other clean runs (manifest list, init,
		// help, --help-schema short-circuit) leave finalReport nil and
		// fall through to exit.OK.
		if finalReport != nil {
			if finalNeverFail {
				os.Exit(exit.OK)
			}
			os.Exit(exit.MapFromReport(finalReport, finalThreshold))
		}
		os.Exit(exit.OK)
	}

	// Help-schema short-circuit: PersistentPreRunE already wrote the JSON
	// document to stdout; the sentinel here just guarantees a clean 0 exit
	// without invoking any subcommand's RunE.
	if errors.Is(err, errHelpSchema) {
		os.Exit(exit.OK)
	}

	// --version short-circuit: PersistentPreRunE wrote the version line
	// before any subcommand RunE could run. Same shape as errHelpSchema.
	if errors.Is(err, errVersion) {
		os.Exit(exit.OK)
	}

	// Interrupt path: the audit handler has already written a partial
	// report to stdout; we just need to exit with the dedicated 130 code
	// instead of letting Cobra's error path render a usage envelope.
	if errors.Is(err, errInterrupted) {
		os.Exit(exit.Interrupted)
	}

	opts := report.RenderOptions{
		Deterministic: resolveDeterministic(deterministic),
		Quiet:         quietFlag,
	}

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
