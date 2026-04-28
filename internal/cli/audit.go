package cli

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"time"

	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/report"
	"github.com/spf13/cobra"
)

// afcliVersion is the binary's reported version. Hard-coded for S01;
// populated from build flags in a later milestone.
const afcliVersion = "0.0.0-dev"

// manifestVersionPlaceholder fills the contract-required manifest_version
// field until the embedded manifest lands in S02. The schema requires a
// non-empty string here, so a placeholder keeps stderr/stdout reports
// schema-valid even when no real manifest is yet bound to the binary.
const manifestVersionPlaceholder = "v0-placeholder"

var auditCmd = &cobra.Command{
	Use:           "audit <target>",
	Short:         "Audit a target against agent-first-cli principles",
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]

		resolved, err := exec.LookPath(target)
		if err != nil {
			return classifyResolveError(target, err)
		}

		started := time.Now().UTC()
		r := &report.Report{
			ManifestVersion: manifestVersionPlaceholder,
			AfcliVersion:    afcliVersion,
			Target:          resolved,
			StartedAt:       started.Format(time.RFC3339Nano),
			DurationMs:      0,
			Findings:        []report.Finding{},
		}

		opts := report.RenderOptions{Deterministic: DeterministicFromContext(cmd.Context())}
		if opts.Deterministic {
			r.StartedAt = ""
		}
		return renderReport(cmd.OutOrStdout(), r, opts, outputFormat)
	},
}

// classifyResolveError distinguishes a missing target from an existing
// non-executable file. exec.LookPath collapses both into a generic error,
// so we re-stat to surface the agent-useful TARGET_NOT_EXECUTABLE code
// when the file is present but lacks the executable bit.
func classifyResolveError(target string, lookErr error) *auditError {
	if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() && info.Mode()&0o111 == 0 {
		return newAuditError(
			report.CodeTargetNotExecutable,
			"target exists but is not executable",
			"chmod +x the file or pass an executable target",
			target,
			map[string]any{"target": target, "resolved": target},
			exit.CouldNotAudit,
		)
	}
	hint := "check the spelling and that the target exists on $PATH or as a direct path"
	if errors.Is(lookErr, fs.ErrNotExist) {
		hint = "no such file: verify the path or PATH lookup"
	}
	return newAuditError(
		report.CodeTargetNotFound,
		"target not found",
		hint,
		target,
		map[string]any{"target": target},
		exit.CouldNotAudit,
	)
}
