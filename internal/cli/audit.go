package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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

// badgeEnabled mirrors --badge. Default off keeps the audit byte-identical
// to S03 (no docs/badge.* files written, no MkdirAll side effect).
// Only the success path writes — the Interrupted path leaves the badge
// untouched because a partial report would mint a misleading score.
var badgeEnabled bool

// badgeOut mirrors --badge-out. Relative paths resolve against the
// current working directory; the default "docs" matches the README's
// `<img src="docs/badge.svg">` reference so dogfood and downstream
// consumers see the same path.
var badgeOut string

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
		if helpSchema || versionFlag {
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
		if badgeEnabled {
			if berr := writeBadgeArtefacts(badgeOut, r, opts); berr != nil {
				return berr
			}
		}
		finalReport = r
		finalThreshold = threshold
		finalNeverFail = neverFail
		return nil
	},
}

// writeBadgeArtefacts emits docs/badge.svg + docs/badge.json (or whatever
// --badge-out resolves to) for r. Only invoked on the clean rendering
// path: an envelope error or signal interrupt skips this entirely so the
// badge is never written for a partial or could-not-audit report.
//
// Disk-write failures surface as INTERNAL-coded *auditError so Execute()
// renders the envelope to stderr and exits 4 (exit.Internal). The audit's
// stdout JSON has already been written by renderReport — we never touch
// stdout from here. details.path points at the offending file or directory
// so an operator can see what failed without re-running with --probe.
func writeBadgeArtefacts(dir string, r *report.Report, opts report.RenderOptions) *auditError {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return newAuditError(
			report.CodeInternal,
			fmt.Sprintf("could not create badge output directory: %v", err),
			"check that the parent directory exists and is writable, or pass --badge-out to a writable path",
			"",
			map[string]any{"path": dir, "os": err.Error()},
			exit.Internal,
		)
	}

	svgPath := filepath.Join(dir, "badge.svg")
	if err := writeBadgeFile(svgPath, func(f *os.File) error {
		return report.RenderBadgeSVG(f, r, opts)
	}); err != nil {
		return err
	}

	jsonPath := filepath.Join(dir, "badge.json")
	if err := writeBadgeFile(jsonPath, func(f *os.File) error {
		return report.RenderBadgeJSON(f, r, opts)
	}); err != nil {
		return err
	}
	return nil
}

// writeBadgeFile opens path with O_WRONLY|O_CREATE|O_TRUNC|0o644, calls
// render(f), and ensures the file is closed before returning. Any open,
// render, or close error becomes an INTERNAL *auditError carrying the
// offending path in details.path.
func writeBadgeFile(path string, render func(*os.File) error) *auditError {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return newAuditError(
			report.CodeInternal,
			fmt.Sprintf("could not open badge file: %v", err),
			"check that the directory is writable",
			"",
			map[string]any{"path": path, "os": err.Error()},
			exit.Internal,
		)
	}
	if rerr := render(f); rerr != nil {
		_ = f.Close()
		return newAuditError(
			report.CodeInternal,
			fmt.Sprintf("could not render badge file: %v", rerr),
			"this is an afcli bug — please file an issue with the failing target",
			"",
			map[string]any{"path": path, "os": rerr.Error()},
			exit.Internal,
		)
	}
	if cerr := f.Close(); cerr != nil {
		return newAuditError(
			report.CodeInternal,
			fmt.Sprintf("could not close badge file: %v", cerr),
			"check that the filesystem is healthy and not full",
			"",
			map[string]any{"path": path, "os": cerr.Error()},
			exit.Internal,
		)
	}
	return nil
}

func init() {
	auditCmd.Flags().DurationVar(&debugSleep, "debug-sleep", 0, "internal: sleep N before finalizing the audit (used by signal tests)")
	_ = auditCmd.Flags().MarkHidden("debug-sleep")
	auditCmd.Flags().StringVar(&descriptorPath, "descriptor", "", "path to afcli.yaml descriptor (skip/relax + S05 probe authorization)")
	auditCmd.Flags().BoolVar(&probeEnabled, "probe", false, "invoke descriptor.commands.safe[] argv (default off; default-off path is byte-identical to S04)")
	auditCmd.Flags().DurationVar(&probeTimeout, "probe-timeout", 5*time.Second, "per-probe timeout (default 5s; affects --help, --afcli-bogus-flag, and behavioral probes)")
	auditCmd.Flags().StringVar(&failOnSeverity, "fail-on", "high", "severity threshold for exit 1: low|medium|high|critical|never")
	auditCmd.Flags().BoolVar(&badgeEnabled, "badge", false, "emit docs/badge.svg + docs/badge.json after a clean audit (default off)")
	auditCmd.Flags().StringVar(&badgeOut, "badge-out", "docs", "directory for badge artefacts when --badge is set; relative paths resolve against cwd")
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
