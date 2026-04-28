package audit

import (
	"context"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
)

// TestEngineRunBreaksLoopOnCancel proves that a context cancelled before
// Engine.Run is invoked causes the principle loop to never start — the
// engine writes zero findings, leaving the CLI's finalizer responsible
// for synthesising the unfinished tail via AppendUnfinishedAsSkipped.
func TestEngineRunBreaksLoopOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{Registry: nil, ProbeTimeout: 5 * time.Second, Probe: fakeOKProbe}

	eng.Run(ctx, "/fake", r, nil)

	if len(r.Findings) != 0 {
		t.Fatalf("expected 0 findings (loop broken on pre-cancelled ctx), got %d", len(r.Findings))
	}
}

// TestEngineRunBreaksLoopMidIteration cancels the context inside a check
// callback, confirming the loop exits at the next iteration boundary
// rather than running all 16 principles.
func TestEngineRunBreaksLoopMidIteration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cancellingCheck := func(_ context.Context, env *CheckEnv) report.Finding {
		cancel()
		f := baseFinding(env)
		f.Status = report.StatusReview
		f.Kind = report.KindRequiresReview
		f.Evidence = "ran before cancel"
		f.Recommendation = "n/a"
		return f
	}

	// Register only on P1 (first principle) so exactly one finding lands
	// before the loop breaks.
	first := manifest.Embedded.Principles[0].PrincipleID()
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     map[string]Check{first: cancellingCheck},
		ProbeTimeout: 5 * time.Second,
		Probe:        fakeOKProbe,
	}

	eng.Run(ctx, "/fake", r, nil)

	if got := len(r.Findings); got != 1 {
		t.Fatalf("expected exactly 1 finding (loop should break after cancel), got %d", got)
	}
	if r.Findings[0].PrincipleID != first {
		t.Errorf("first finding = %q, want %q", r.Findings[0].PrincipleID, first)
	}
}

func TestAppendUnfinishedAsSkippedEmptyFindings(t *testing.T) {
	r := &report.Report{Findings: []report.Finding{}}

	AppendUnfinishedAsSkipped(r)

	if len(r.Findings) != len(manifest.Embedded.Principles) {
		t.Fatalf("expected %d synthetic findings, got %d", len(manifest.Embedded.Principles), len(r.Findings))
	}
	for i, p := range manifest.Embedded.Principles {
		f := r.Findings[i]
		if f.PrincipleID != p.PrincipleID() {
			t.Errorf("findings[%d].PrincipleID = %q, want %q (manifest order)", i, f.PrincipleID, p.PrincipleID())
		}
		if f.Title != p.Title {
			t.Errorf("findings[%d].Title = %q, want %q", i, f.Title, p.Title)
		}
		if f.Category != p.Category {
			t.Errorf("findings[%d].Category = %q, want %q", i, f.Category, p.Category)
		}
		if f.Status != report.StatusSkip {
			t.Errorf("findings[%d].Status = %q, want %q", i, f.Status, report.StatusSkip)
		}
		if f.Kind != report.KindRequiresReview {
			t.Errorf("findings[%d].Kind = %q, want %q", i, f.Kind, report.KindRequiresReview)
		}
		if f.Severity != severityFor(p.PrincipleID()) {
			t.Errorf("findings[%d].Severity = %q, want %q", i, f.Severity, severityFor(p.PrincipleID()))
		}
		if f.Evidence != "audit interrupted before this principle ran" {
			t.Errorf("findings[%d].Evidence = %q, want interrupted-message", i, f.Evidence)
		}
		if f.Recommendation != "re-run audit" {
			t.Errorf("findings[%d].Recommendation = %q, want re-run audit", i, f.Recommendation)
		}
		if f.Hint != p.URL {
			t.Errorf("findings[%d].Hint = %q, want %q", i, f.Hint, p.URL)
		}
	}
}

func TestAppendUnfinishedAsSkippedPartialFindings(t *testing.T) {
	// Pre-populate findings for P1 and P3 (existing terminal entries).
	// AppendUnfinishedAsSkipped must leave them untouched and only fill
	// the missing ids.
	preserved := report.Finding{
		PrincipleID:    "P1",
		Title:          "preserved-title-P1",
		Category:       "preserved-cat",
		Status:         report.StatusPass,
		Kind:           report.KindAutomated,
		Severity:       report.SeverityHigh,
		Evidence:       "do not overwrite",
		Recommendation: "preserve me",
	}
	preservedP3 := preserved
	preservedP3.PrincipleID = "P3"
	preservedP3.Title = "preserved-title-P3"
	r := &report.Report{Findings: []report.Finding{preserved, preservedP3}}

	AppendUnfinishedAsSkipped(r)

	if len(r.Findings) != len(manifest.Embedded.Principles) {
		t.Fatalf("expected %d findings, got %d", len(manifest.Embedded.Principles), len(r.Findings))
	}
	// Pre-existing entries must remain unchanged at their original
	// positions.
	if r.Findings[0].Evidence != "do not overwrite" || r.Findings[0].PrincipleID != "P1" {
		t.Errorf("P1 finding mutated: %+v", r.Findings[0])
	}
	if r.Findings[1].Evidence != "do not overwrite" || r.Findings[1].PrincipleID != "P3" {
		t.Errorf("P3 finding mutated: %+v", r.Findings[1])
	}
	// All other principle ids must now be present as skip findings.
	have := make(map[string]report.Finding, len(r.Findings))
	for _, f := range r.Findings {
		have[f.PrincipleID] = f
	}
	for _, p := range manifest.Embedded.Principles {
		f, ok := have[p.PrincipleID()]
		if !ok {
			t.Errorf("principle %s missing after append", p.PrincipleID())
			continue
		}
		if p.PrincipleID() == "P1" || p.PrincipleID() == "P3" {
			continue
		}
		if f.Status != report.StatusSkip {
			t.Errorf("synthesized %s.Status = %q, want %q", p.PrincipleID(), f.Status, report.StatusSkip)
		}
	}
}

func TestAppendUnfinishedAsSkippedFullFindings(t *testing.T) {
	// All 16 principles already present — no-op.
	r := &report.Report{Findings: []report.Finding{}}
	for _, p := range manifest.Embedded.Principles {
		r.Findings = append(r.Findings, report.Finding{
			PrincipleID: p.PrincipleID(),
			Title:       "untouched-" + p.PrincipleID(),
			Status:      report.StatusPass,
			Kind:        report.KindAutomated,
			Severity:    report.SeverityHigh,
			Evidence:    "untouched",
		})
	}

	before := len(r.Findings)
	snapshot := append([]report.Finding(nil), r.Findings...)

	AppendUnfinishedAsSkipped(r)

	if len(r.Findings) != before {
		t.Fatalf("full-findings call must be a no-op; len changed from %d to %d", before, len(r.Findings))
	}
	for i := range snapshot {
		if r.Findings[i] != snapshot[i] {
			t.Errorf("findings[%d] mutated: before=%+v after=%+v", i, snapshot[i], r.Findings[i])
		}
	}
}
