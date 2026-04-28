// Package audit implements the static check engine that populates Report.Findings.
package audit

import (
	"context"
	"fmt"
	"time"

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
// captured once per audit and shared across all 16 checks.
type CheckEnv struct {
	Target    string
	Principle manifest.Principle
	Help      *Capture
	Bogus     *Capture
}

// Engine is the static check engine. Construct via DefaultEngine for
// production use; tests build instances directly to inject a fake Probe
// or override the Registry.
type Engine struct {
	Registry     map[string]Check
	ProbeTimeout time.Duration
	Probe        func(ctx context.Context, target string, args []string, timeout time.Duration) *Capture
}

// DefaultEngine returns an Engine wired with the S03 check registry.
// T02 populates Registry with the five real checks (P6/P7/P14/P15/P16).
func DefaultEngine() *Engine {
	return &Engine{
		Registry:     map[string]Check{},
		ProbeTimeout: defaultProbeTimeout,
		Probe:        runProbe,
	}
}

// Run iterates every principle in manifest.Embedded.Principles, executes
// each registered Check (or stubCheck) inside safeRun, and appends each
// finding to r.Findings. Probes are captured ONCE before any check runs.
// Engine never sorts — normalizeReport in the renderer is the single sort
// site (MEM007).
func (e *Engine) Run(ctx context.Context, target string, r *report.Report) {
	help := e.Probe(ctx, target, []string{"--help"}, e.ProbeTimeout)
	bogus := e.Probe(ctx, target, []string{bogusFlagArg}, e.ProbeTimeout)

	for _, p := range manifest.Embedded.Principles {
		env := &CheckEnv{Target: target, Principle: p, Help: help, Bogus: bogus}
		check, ok := e.Registry[p.PrincipleID()]
		if !ok {
			check = stubCheck
		}
		r.Findings = append(r.Findings, e.safeRun(ctx, env, p, check))
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
