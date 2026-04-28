package audit

import (
	"context"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/report"
)

// TestEngineDescriptorSkipShortCircuitsCheck proves that ShouldSkip is
// consulted BEFORE the registry lookup: a principle named in
// skip_principles produces a synthetic skip/requires-review finding and
// the registered check is never invoked.
func TestEngineDescriptorSkipShortCircuitsCheck(t *testing.T) {
	var p12Calls int
	sentinel := func(_ context.Context, _ *CheckEnv) report.Finding {
		p12Calls++
		return report.Finding{
			PrincipleID: "P12",
			Status:      report.StatusFail,
			Kind:        report.KindAutomated,
			Severity:    report.SeverityHigh,
			Evidence:    "sentinel ran",
		}
	}
	eng := &Engine{
		Registry:     map[string]Check{"P12": sentinel},
		ProbeTimeout: 5 * time.Second,
		Probe:        fakeOKProbe,
	}
	d := &descriptor.Descriptor{SkipPrinciples: []string{"P12"}}
	r := &report.Report{Findings: []report.Finding{}}

	eng.Run(context.Background(), "/fake", r, d)

	if len(r.Findings) != 16 {
		t.Fatalf("expected 16 findings, got %d", len(r.Findings))
	}
	if p12Calls != 0 {
		t.Errorf("registered P12 check must not be invoked when skipped; ran %d times", p12Calls)
	}

	var p12 *report.Finding
	for i := range r.Findings {
		if r.Findings[i].PrincipleID == "P12" {
			p12 = &r.Findings[i]
			break
		}
	}
	if p12 == nil {
		t.Fatal("P12 finding missing after skip")
	}
	if p12.Status != report.StatusSkip {
		t.Errorf("P12.Status = %q, want %q", p12.Status, report.StatusSkip)
	}
	if p12.Kind != report.KindRequiresReview {
		t.Errorf("P12.Kind = %q, want %q", p12.Kind, report.KindRequiresReview)
	}
	if p12.Evidence != "skipped per descriptor" {
		t.Errorf("P12.Evidence = %q, want %q", p12.Evidence, "skipped per descriptor")
	}
	if p12.Severity != severityFor("P12") {
		t.Errorf("P12.Severity = %q, want %q", p12.Severity, severityFor("P12"))
	}
	if p12.Title == "" || p12.Category == "" {
		t.Errorf("P12 skip finding must inherit Title/Category from manifest; got Title=%q Category=%q", p12.Title, p12.Category)
	}
}

// TestEngineDescriptorRelaxCapsSeverity proves that Apply runs AFTER
// safeRun and caps a high-severity finding to the descriptor ceiling
// without touching Status, Kind, Evidence, or Recommendation.
func TestEngineDescriptorRelaxCapsSeverity(t *testing.T) {
	highFail := func(_ context.Context, env *CheckEnv) report.Finding {
		f := baseFinding(env)
		f.Status = report.StatusFail
		f.Kind = report.KindAutomated
		f.Severity = report.SeverityHigh
		f.Evidence = "fake high failure"
		f.Recommendation = "do not touch"
		return f
	}
	eng := &Engine{
		Registry:     map[string]Check{"P3": highFail},
		ProbeTimeout: 5 * time.Second,
		Probe:        fakeOKProbe,
	}
	d := &descriptor.Descriptor{
		RelaxPrinciples: map[string]string{"P3": "medium"},
	}
	r := &report.Report{Findings: []report.Finding{}}

	eng.Run(context.Background(), "/fake", r, d)

	var p3 *report.Finding
	for i := range r.Findings {
		if r.Findings[i].PrincipleID == "P3" {
			p3 = &r.Findings[i]
			break
		}
	}
	if p3 == nil {
		t.Fatal("P3 finding missing")
	}
	if p3.Severity != report.SeverityMedium {
		t.Errorf("P3.Severity = %q, want %q (capped from high)", p3.Severity, report.SeverityMedium)
	}
	if p3.Status != report.StatusFail {
		t.Errorf("P3.Status = %q, want %q (relax must not touch status)", p3.Status, report.StatusFail)
	}
	if p3.Kind != report.KindAutomated {
		t.Errorf("P3.Kind = %q, want %q (relax must not touch kind)", p3.Kind, report.KindAutomated)
	}
	if p3.Evidence != "fake high failure" {
		t.Errorf("P3.Evidence = %q, want unchanged", p3.Evidence)
	}
	if p3.Recommendation != "do not touch" {
		t.Errorf("P3.Recommendation = %q, want unchanged", p3.Recommendation)
	}
}

// TestEngineDescriptorRelaxIsCeilingNotFloor proves Apply never
// promotes a low-severity finding up to the descriptor's ceiling — the
// ceiling clamps maxima, it does not set the value.
func TestEngineDescriptorRelaxIsCeilingNotFloor(t *testing.T) {
	lowReview := func(_ context.Context, env *CheckEnv) report.Finding {
		f := baseFinding(env)
		f.Status = report.StatusReview
		f.Kind = report.KindAutomated
		f.Severity = report.SeverityLow
		f.Evidence = "fake low review"
		f.Recommendation = "preserve me"
		return f
	}
	eng := &Engine{
		Registry:     map[string]Check{"P3": lowReview},
		ProbeTimeout: 5 * time.Second,
		Probe:        fakeOKProbe,
	}
	d := &descriptor.Descriptor{
		RelaxPrinciples: map[string]string{"P3": "medium"},
	}
	r := &report.Report{Findings: []report.Finding{}}

	eng.Run(context.Background(), "/fake", r, d)

	var p3 *report.Finding
	for i := range r.Findings {
		if r.Findings[i].PrincipleID == "P3" {
			p3 = &r.Findings[i]
			break
		}
	}
	if p3 == nil {
		t.Fatal("P3 finding missing")
	}
	if p3.Severity != report.SeverityLow {
		t.Errorf("P3.Severity = %q, want %q (relax is a ceiling, must not raise)", p3.Severity, report.SeverityLow)
	}
}

// TestEngineNilDescriptorPassthrough proves that passing d=nil is
// behaviourally identical to the pre-S04 engine: 16 stub findings, all
// review/requires-review when the registry is empty.
func TestEngineNilDescriptorPassthrough(t *testing.T) {
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{Registry: nil, ProbeTimeout: 5 * time.Second, Probe: fakeOKProbe}

	eng.Run(context.Background(), "/fake", r, nil)

	if len(r.Findings) != 16 {
		t.Fatalf("expected 16 findings, got %d", len(r.Findings))
	}
	for _, f := range r.Findings {
		if f.Status != report.StatusReview {
			t.Errorf("principle %s: expected status=review, got %q", f.PrincipleID, f.Status)
		}
		if f.Kind != report.KindRequiresReview {
			t.Errorf("principle %s: expected kind=requires-review, got %q", f.PrincipleID, f.Kind)
		}
	}
}
