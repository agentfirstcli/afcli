package cli

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"time"

	"github.com/agentfirstcli/afcli/internal/audit"
	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
	"github.com/spf13/cobra"
)

// AfcliVersion is the binary's reported version. Hard-coded for S01;
// populated from build flags in a later milestone. Exported so the
// help-schema renderer (and any future introspection surface) can
// reference it without inviting a circular import.
const AfcliVersion = "0.0.0-dev"

// debugSleep simulates a slow audit so the signal-interrupt integration
// test can SIGINT a running invocation. Hidden — not part of the public
// CLI surface. Real audit work in later slices replaces this stub.
var debugSleep time.Duration

// descriptorPath is the value of --descriptor on auditCmd. Empty means
// "no descriptor" — Engine.Run gets nil and runs every principle at its
// declared severity. Populated, the file is loaded and validated before
// any probe runs; parse failures short-circuit to a DESCRIPTOR_INVALID /
// DESCRIPTOR_NOT_FOUND envelope without contaminating findings.
var descriptorPath string

// probeEnabled mirrors --probe. Default off keeps the audit byte-
// identical to S04 (only --help and --afcli-bogus-flag exec'd).
// T03 wires this through Engine.ProbeEnabled to drive the
// descriptor-authorized behavioral-capture pass.
var probeEnabled bool

// probeTimeout mirrors --probe-timeout. It bounds every probe the engine
// runs (--help, --afcli-bogus-flag, and behavioral probes), defaulting
// to 5s so a hanging target cannot wedge the audit.
var probeTimeout time.Duration

// errInterrupted is returned by audit's RunE after it has already written
// a partial report to stdout in response to SIGINT/SIGTERM. Execute()
// recognises this sentinel and exits 130 without re-rendering.
var errInterrupted = errors.New("audit interrupted")

var auditCmd = &cobra.Command{
	Use:   "audit <target>",
	Short: "Audit a target against agent-first-cli principles",
	// --help-schema short-circuits in PersistentPreRunE, but Cobra runs
	// ValidateArgs before PersistentPreRunE — so we have to skip the
	// arg-count check ourselves when the user is asking for the schema.
	Args: func(cmd *cobra.Command, args []string) error {
		if helpSchema {
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]

		resolved, err := exec.LookPath(target)
		if err != nil {
			return classifyResolveError(target, err)
		}

		var d *descriptor.Descriptor
		if descriptorPath != "" {
			var dErr error
			d, dErr = descriptor.Load(descriptorPath)
			if dErr != nil {
				return classifyDescriptorError(descriptorPath, dErr)
			}
		}

		ctx, cleanup := InstallSignalHandler(cmd.Context())
		defer cleanup()

		started := time.Now().UTC()
		r := &report.Report{
			ManifestVersion: manifest.Embedded.Version,
			AfcliVersion:    AfcliVersion,
			Target:          resolved,
			StartedAt:       started.Format(time.RFC3339Nano),
			DurationMs:      0,
			Findings:        []report.Finding{},
		}

		opts := report.RenderOptions{Deterministic: DeterministicFromContext(cmd.Context())}
		if opts.Deterministic {
			r.StartedAt = ""
		}

		eng := audit.DefaultEngine()
		eng.ProbeEnabled = probeEnabled
		eng.ProbeTimeout = probeTimeout
		eng.Run(ctx, resolved, r, d)

		if debugSleep > 0 {
			select {
			case <-time.After(debugSleep):
			case <-ctx.Done():
			}
		}

		if ctx.Err() != nil {
			r.Interrupted = true
			markUnfinishedAsSkipped(r)
			if rerr := renderReport(cmd.OutOrStdout(), r, opts, outputFormat); rerr != nil {
				return rerr
			}
			return errInterrupted
		}

		return renderReport(cmd.OutOrStdout(), r, opts, outputFormat)
	},
}

func init() {
	auditCmd.Flags().DurationVar(&debugSleep, "debug-sleep", 0, "internal: sleep N before finalizing the audit (used by signal tests)")
	_ = auditCmd.Flags().MarkHidden("debug-sleep")
	auditCmd.Flags().StringVar(&descriptorPath, "descriptor", "", "path to afcli.yaml descriptor (skip/relax + S05 probe authorization)")
	auditCmd.Flags().BoolVar(&probeEnabled, "probe", false, "invoke descriptor.commands.safe[] argv (default off; default-off path is byte-identical to S04)")
	auditCmd.Flags().DurationVar(&probeTimeout, "probe-timeout", 5*time.Second, "per-probe timeout (default 5s; affects --help, --afcli-bogus-flag, and behavioral probes)")
}

// markUnfinishedAsSkipped flips any non-terminal Finding to Status=skip
// after a signal-driven cancellation. S01 ships zero principles, so this
// is a no-op today — plumbed now so S05's probe layer can populate
// in-progress findings and rely on the same finalization path.
func markUnfinishedAsSkipped(r *report.Report) {
	for i := range r.Findings {
		switch r.Findings[i].Status {
		case report.StatusPass, report.StatusFail, report.StatusSkip, report.StatusReview:
			// Terminal — leave as-is.
		default:
			r.Findings[i].Status = report.StatusSkip
		}
	}
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
