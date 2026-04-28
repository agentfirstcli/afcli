// Package audit implements the static check engine that populates Report.Findings.
package audit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
)

const (
	defaultProbeTimeout = 5 * time.Second
	bogusFlagArg        = "--afcli-bogus-flag"
)

// Check is the per-principle classification function. Implementations read
// env.Help / env.Bogus and return a fully-populated Finding (PrincipleID,
// Title, Category, Status, Kind, Severity, Evidence, Recommendation, Hint).
type Check func(ctx context.Context, env *CheckEnv) report.Finding

// CheckEnv is the read-only environment passed to every Check. Probes are
// captured once per audit and shared across all 16 checks. Behavioral is
// the descriptor-authorized capture pass populated by Engine.Run when
// ProbeEnabled && d != nil; it is nil otherwise. Iteration order
// matches descriptor.Commands.Safe[] declaration order — never sorted.
type CheckEnv struct {
	Target     string
	Principle  manifest.Principle
	Help       *Capture
	Bogus      *Capture
	Behavioral []BehavioralCapture
}

// Engine is the static check engine. Construct via DefaultEngine for
// production use; tests build instances directly to inject a fake Probe
// or override the Registry.
//
// ProbeEnabled gates the S05 behavioral-capture pass — when false (the
// default), Engine.Run continues to execute only the byte-identical S04
// surface (--help and --afcli-bogus-flag). T03 wires the actual
// descriptor-authorized probe pass behind this knob; this slice plants
// the field so the CLI flag and engine plumbing land before semantics
// change.
type Engine struct {
	Registry     map[string]Check
	ProbeTimeout time.Duration
	ProbeEnabled bool
	Probe        func(ctx context.Context, target string, args []string, timeout time.Duration, extraEnv map[string]string) *Capture
}

// DefaultEngine returns an Engine wired with the S03 + S06-T02 check
// registry. The five real S03 checks (P6/P7/P14/P15/P16) are registered
// alongside five S06 review-only checks (P2/P5/P9/P11/P12). The remaining
// 6 principles fall through to stubCheck via Engine.Run until S06-T03
// closes them.
func DefaultEngine() *Engine {
	return &Engine{
		Registry: map[string]Check{
			"P2":  checkP2,
			"P5":  checkP5,
			"P6":  checkP6,
			"P7":  checkP7,
			"P9":  checkP9,
			"P11": checkP11,
			"P12": checkP12,
			"P14": checkP14,
			"P15": checkP15,
			"P16": checkP16,
		},
		ProbeTimeout: defaultProbeTimeout,
		Probe:        runProbe,
	}
}

// Run iterates every principle in manifest.Embedded.Principles, executes
// each registered Check (or stubCheck) inside safeRun, and appends each
// finding to r.Findings. Probes are captured ONCE before any check runs
// (they're cheap and S05 reuses them regardless of descriptor policy).
// When d != nil, descriptor.ShouldSkip short-circuits a principle to a
// synthetic skip finding before any check runs, and descriptor.Apply caps
// each post-check finding's severity. Engine never sorts — normalizeReport
// in the renderer is the single sort site (MEM007).
func (e *Engine) Run(ctx context.Context, target string, r *report.Report, d *descriptor.Descriptor) {
	help := e.Probe(ctx, target, []string{"--help"}, e.ProbeTimeout, nil)
	bogus := e.Probe(ctx, target, []string{bogusFlagArg}, e.ProbeTimeout, nil)

	var behavioral []BehavioralCapture
	if e.ProbeEnabled && d != nil {
		for _, entry := range d.Commands.Safe {
			argv := strings.Fields(entry)
			if len(argv) == 0 {
				continue
			}
			bc := BehavioralCapture{Cmd: entry, Argv: argv}
			if err := authorizeProbe(d, argv); err != nil {
				bc.Capture = &Capture{Args: argv, Err: err}
				behavioral = append(behavioral, bc)
				continue
			}
			bc.Capture = e.Probe(ctx, target, argv, e.ProbeTimeout, d.Env)
			behavioral = append(behavioral, bc)
		}
	}

	for _, p := range manifest.Embedded.Principles {
		// Cancellation invariant (R012): break the principle loop the
		// moment ctx is done so SIGINT mid-audit does not run further
		// checks. The CLI's finalizer fills the unfinished tail via
		// AppendUnfinishedAsSkipped so r.Findings still totals 16.
		if ctx.Err() != nil {
			break
		}
		env := &CheckEnv{Target: target, Principle: p, Help: help, Bogus: bogus, Behavioral: behavioral}
		if descriptor.ShouldSkip(d, p.PrincipleID()) {
			f := baseFinding(env)
			f.Status = report.StatusSkip
			f.Kind = report.KindRequiresReview
			f.Evidence = "skipped per descriptor"
			f.Recommendation = "remove from skip_principles to re-enable this check"
			r.Findings = append(r.Findings, f)
			continue
		}
		check, ok := e.Registry[p.PrincipleID()]
		if !ok {
			check = stubCheck
		}
		f := e.safeRun(ctx, env, p, check)
		descriptor.Apply(d, &f)
		r.Findings = append(r.Findings, f)
	}

	// Probe-evidence aggregator: only runs when the principle loop
	// completed without cancellation. Concentrates probe timeout / denial
	// signal into the existing P3 finding so an agent reading the JSON
	// report sees one review entry per failed probe instead of N
	// scattered envelopes (R008 isolation: 16 findings, never more).
	// Skip-by-policy on P3 wins — the descriptor's authority is preserved.
	if ctx.Err() == nil {
		decorateP3WithProbeEvidence(r, behavioral)
	}
}

// decorateP3WithProbeEvidence walks behavioral captures and, if any have
// non-nil Capture.Err, replaces the P3 finding with a review entry whose
// evidence concatenates one line per failing capture. The replacement is
// suppressed when the existing P3 finding is already StatusSkip — a
// descriptor skip overrides aggregator decoration. Severity is preserved
// via baseFinding so the manifest table value survives.
func decorateP3WithProbeEvidence(r *report.Report, behavioral []BehavioralCapture) {
	if len(behavioral) == 0 {
		return
	}
	var failing []BehavioralCapture
	for _, bc := range behavioral {
		if bc.Capture != nil && bc.Capture.Err != nil {
			failing = append(failing, bc)
		}
	}
	if len(failing) == 0 {
		return
	}

	idx := -1
	for i := range r.Findings {
		if r.Findings[i].PrincipleID == "P3" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	if r.Findings[idx].Status == report.StatusSkip {
		return
	}

	var p3 manifest.Principle
	for _, p := range manifest.Embedded.Principles {
		if p.PrincipleID() == "P3" {
			p3 = p
			break
		}
	}
	env := &CheckEnv{Target: r.Target, Principle: p3}
	f := baseFinding(env)
	f.Status = report.StatusReview
	f.Kind = report.KindRequiresReview

	var lines []string
	for _, bc := range failing {
		var ae *AuthError
		switch {
		case IsProbeTimeout(bc.Capture.Err):
			lines = append(lines, fmt.Sprintf("probe timeout: %s exceeded %dms", bc.Cmd, bc.Capture.Duration.Milliseconds()))
		case errors.As(bc.Capture.Err, &ae):
			lines = append(lines, fmt.Sprintf("probe denied: %s: %s", bc.Cmd, ae.Reason))
		default:
			lines = append(lines, fmt.Sprintf("probe failed: %s: %v", bc.Cmd, bc.Capture.Err))
		}
	}
	f.Evidence = truncateEvidence(strings.Join(lines, "\n"))
	f.Recommendation = "investigate the probe outcome and either remove the entry from commands.safe or fix the target's behavior"

	r.Findings[idx] = f
}

// AppendUnfinishedAsSkipped fills r.Findings with synthetic skip findings
// for any principle that did not produce a finding before cancellation.
// Preserves the manifest's declaration order — sorting happens at render
// time (MEM007). Idempotent: principles already present in r.Findings are
// skipped. After this call the report carries exactly len(manifest
// principles) findings, satisfying the "16 findings, always" invariant
// even when SIGINT hit mid-loop.
func AppendUnfinishedAsSkipped(r *report.Report) {
	seen := make(map[string]bool, len(r.Findings))
	for _, f := range r.Findings {
		seen[f.PrincipleID] = true
	}
	for _, p := range manifest.Embedded.Principles {
		id := p.PrincipleID()
		if seen[id] {
			continue
		}
		r.Findings = append(r.Findings, report.Finding{
			PrincipleID:    id,
			Title:          p.Title,
			Category:       p.Category,
			Status:         report.StatusSkip,
			Kind:           report.KindRequiresReview,
			Severity:       severityFor(id),
			Evidence:       "audit interrupted before this principle ran",
			Recommendation: "re-run audit",
			Hint:           p.URL,
		})
	}
}

// safeRun runs a single check inside a defer/recover and converts panics
// into requires-review findings. R008 isolation contract: a panic in any
// one check must NOT abort the audit or mask the other 15 findings.
// No stack trace is captured so evidence stays byte-stable for determinism.
func (e *Engine) safeRun(ctx context.Context, env *CheckEnv, p manifest.Principle, check Check) (f report.Finding) {
	defer func() {
		if rec := recover(); rec != nil {
			f = report.Finding{
				PrincipleID:    p.PrincipleID(),
				Title:          p.Title,
				Category:       p.Category,
				Status:         report.StatusReview,
				Kind:           report.KindRequiresReview,
				Severity:       severityFor(p.PrincipleID()),
				Evidence:       fmt.Sprintf("check panicked: %v", rec),
				Recommendation: "this is a bug in afcli — please report",
				Hint:           p.URL,
			}
		}
	}()
	return check(ctx, env)
}
