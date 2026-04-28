package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/report"
	"github.com/spf13/cobra"
)

// initOutPath mirrors --out on initCmd. Default ./afcli.yaml. We use --out
// rather than --output because the persistent rootCmd flag --output (json/
// text/markdown) would shadow a per-command --output and silently swallow
// the path the user provided.
var initOutPath string

// initForce mirrors --force on initCmd. When false, an existing file at
// the resolved out-path triggers INIT_FILE_EXISTS / exit 3 instead of an
// overwrite.
var initForce bool

// initTemplate is the literal YAML body written by `afcli init`. It is
// NOT produced via yaml.Marshal: the omitempty tags on Descriptor.Commands,
// Env, SkipPrinciples and RelaxPrinciples would suppress the explicit-empty
// keys this template needs to surface as scaffolding hints. The "<TARGET>"
// token is replaced via strconv.Quote so quotes/backslashes in the user-
// supplied target name cannot break out of the YAML string.
const initTemplate = `# afcli.yaml — agent-first-cli descriptor scaffold.
# See https://agentfirstcli.com/descriptor for the full schema.
# This file is the contract afcli reads when auditing this target; edit it
# in place and re-run ` + "`afcli audit`" + ` to apply your changes.

# format_version locks the descriptor wire shape. This build supports "1".
format_version: "1"

# target is the name of the CLI tool this descriptor describes. It is
# advisory only — the actual audit target comes from afcli's positional arg.
target: "<TARGET>"

# commands lists invocations afcli is permitted to run during the
# behavioral-capture pass (--probe). Leave empty to keep probing
# constrained to --help / --afcli-bogus-flag.
commands:
  # safe[]: read-only invocations afcli may run unattended (e.g. "status",
  # "list", "--version"). Every entry is shell-split with shlex semantics.
  safe: []
  # destructive[]: invocations that mutate state. Reserved for future
  # opt-in probes; currently parsed but never executed.
  destructive: []

# env: extra environment variables passed to every probe. Use this for
# CI-friendly defaults like NO_COLOR=1 or CLICOLOR=0.
env: {}

# skip_principles: principle ids (P1..P16) afcli should omit from this
# audit entirely. Findings are not produced for skipped principles.
skip_principles: []

# relax_principles: per-principle severity overrides. The key is a
# principle id; the value is one of low, medium, high, critical.
relax_principles: {}
`

var initCmd = &cobra.Command{
	Use:   "init <target>",
	Short: "Write a starter afcli.yaml descriptor for the named target",
	Long: "Generate a comment-rich afcli.yaml scaffold pre-populated with the\n" +
		"target name, format_version, and explicit-empty commands/env/skip/\n" +
		"relax sections. Refuses to overwrite an existing file unless --force\n" +
		"is set; uses the INIT_FILE_EXISTS error code on refusal.",
	Args: func(cmd *cobra.Command, args []string) error {
		if helpSchema || versionFlag {
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		path := initOutPath
		if path == "" {
			path = "./afcli.yaml"
		}

		if _, err := os.Stat(path); err == nil && !initForce {
			return newAuditError(
				report.CodeInitFileExists,
				fmt.Sprintf("file already exists: %s", path),
				"pass --force to overwrite",
				path,
				map[string]any{"path": path},
				exit.CouldNotAudit,
			)
		}

		body := strings.Replace(initTemplate, `"<TARGET>"`, strconv.Quote(target), 1)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return newAuditError(
				report.CodeInternal,
				fmt.Sprintf("could not write descriptor: %v", err),
				"check that the parent directory exists and is writable",
				path,
				map[string]any{"path": path, "os": err.Error()},
				exit.Internal,
			)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initOutPath, "out", "./afcli.yaml", "path to write the descriptor (must not exist unless --force)")
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite the destination file if it already exists")
}
