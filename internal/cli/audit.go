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
	"github.com/agentfirstcli/afcli/internal/version"
	"github.com/spf13/cobra"
)

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

// failOnSeverity mirrors --fail-on. Validated by parseFailOn before the
// engine runs; bogus values short-circuit to a USAGE envelope at exit 2.
var failOnSeverity string

// finalReport / finalThreshold / finalNeverFail communicate the audit
// outcome from auditCmd.RunE up to Execute() so the report-aware exit
// path can call exit.MapFromReport without coupling Execute() to Cobra
// internals. RunE writes them only on the clean rendering path; the
// Interrupted path leaves them nil and errInterrupted owns exit 130.
var (
	finalReport    *report.Report
	finalThreshold report.Severity
	finalNeverFail bool
)

// errInterrupted is returned by audit's RunE after it has already written
// a partial report to stdout in response to SIGINT/SIGTERM. Execute()
// recognises this sentinel and exits 130 without re-rendering.
var errInterrupted = errors.New("audit interrupted")

// parseFailOn validates the --fail-on value and returns the corresponding
// report.Severity (or isNever=true for "never"). Bogus input becomes a
// USAGE-coded *auditError so Execute() renders the documented envelope at
// exit 2 instead of letting MapFromReport silently treat unknown ranks
// as zero (footgun called out in the slice research).
func parseFailOn(s string) (report.Severity, bool, *auditError) {
	switch s {
	case "low":
		return report.SeverityLow, false, nil
	case "medium":
		return report.SeverityMedium, false, nil
	case "high":
		return report.SeverityHigh, false, nil
	case "critical":
		return report.SeverityCritical, false, nil
	case "never":
		return "", true, nil
	}
	return "", false, newAuditError(
		report.CodeUsage,
		"invalid --fail-on value: "+s,
		"allowed values: low, medium, high, critical, never",
		"",
		map[string]any{"flag": "fail-on", "value": s, "allowed": []string{"low", "medium", "high", "critical", "never"}},
		exit.Usage,
	)
}

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

		threshold, neverFail, ferr := parseFailOn(failOnSeverity)
		if ferr != nil {
			return ferr
		}

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
			AfcliVersion:    version.Version,
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

		if rerr := renderReport(cmd.OutOrStdout(), r, opts, outputFormat); rerr != nil {
			return rerr
		}
		finalReport = r
		finalThreshold = threshold
		finalNeverFail = neverFail
		return nil
	},
}

func init() {
	auditCmd.Flags().DurationVar(&debugSleep, "debug-sleep", 0, "internal: sleep N before finalizing the audit (used by signal tests)")
	_ = auditCmd.Flags().MarkHidden("debug-sleep")
	auditCmd.Flags().StringVar(&descriptorPath, "descriptor", "", "path to afcli.yaml descriptor (skip/relax + S05 probe authorization)")
	auditCmd.Flags().BoolVar(&probeEnabled, "probe", false, "invoke descriptor.commands.safe[] argv (default off; default-off path is byte-identical to S04)")
	auditCmd.Flags().DurationVar(&probeTimeout, "probe-timeout", 5*time.Second, "per-probe timeout (default 5s; affects --help, --afcli-bogus-flag, and behavioral probes)")
	auditCmd.Flags().StringVar(&failOnSeverity, "fail-on", "high", "severity threshold for exit 1: low|medium|high|critical|never")
}

// markUnfinishedAsSkipped finalizes a partial report after signal-driven
// cancellation. Two-step process: (1) flip any non-terminal Finding to
// Status=skip (preserves pre-S05 callers that may still emit
// in-progress sentinel statuses); (2) delegate to
// audit.AppendUnfinishedAsSkipped so any principle missing from
// r.Findings entirely (the engine broke its loop early on ctx.Err()) is
// synthesized in manifest order. Post-call invariant: len(r.Findings)
// equals the manifest principle count (16 today), every status is terminal.
func markUnfinishedAsSkipped(r *report.Report) {
	for i := range r.Findings {
		switch r.Findings[i].Status {
		case report.StatusPass, report.StatusFail, report.StatusSkip, report.StatusReview:
			// Terminal — leave as-is.
		default:
			r.Findings[i].Status = report.StatusSkip
		}
	}
	audit.AppendUnfinishedAsSkipped(r)
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
