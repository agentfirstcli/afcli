package audit

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
)

func fakeOKProbe(_ context.Context, _ string, args []string, _ time.Duration) *Capture {
	return &Capture{Args: args, Stdout: "fake stdout", Stderr: "", ExitCode: 0}
}

func TestEngineProducesAllSixteenStubFindings(t *testing.T) {
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
		if f.PrincipleID == "" {
			t.Errorf("finding has empty PrincipleID")
		}
	}
}

func TestEnginePanicIsolation(t *testing.T) {
	r := &report.Report{Findings: []report.Finding{}}
	panicker := func(_ context.Context, _ *CheckEnv) report.Finding {
		panic("kaboom")
	}
	eng := &Engine{
		Registry:     map[string]Check{"P3": panicker},
		ProbeTimeout: 5 * time.Second,
		Probe:        fakeOKProbe,
	}

	eng.Run(context.Background(), "/fake", r, nil)

	if len(r.Findings) != 16 {
		t.Fatalf("panic must not abort audit: expected 16 findings, got %d", len(r.Findings))
	}

	var p3 *report.Finding
	for i := range r.Findings {
		if r.Findings[i].PrincipleID == "P3" {
			p3 = &r.Findings[i]
			break
		}
	}
	if p3 == nil {
		t.Fatal("P3 finding missing after panic")
	}
	if p3.Status != report.StatusReview {
		t.Errorf("P3: expected status=review, got %q", p3.Status)
	}
	if p3.Kind != report.KindRequiresReview {
		t.Errorf("P3: expected kind=requires-review, got %q", p3.Kind)
	}
	if !strings.Contains(p3.Evidence, "check panicked") || !strings.Contains(p3.Evidence, "kaboom") {
		t.Errorf("P3: expected evidence to contain 'check panicked' and 'kaboom', got %q", p3.Evidence)
	}
	if p3.Severity != severityFor("P3") {
		t.Errorf("P3: expected severity=%q, got %q", severityFor("P3"), p3.Severity)
	}

	for _, f := range r.Findings {
		if f.PrincipleID == "P3" {
			continue
		}
		if strings.Contains(f.Evidence, "check panicked") {
			t.Errorf("principle %s: panic evidence leaked across boundary: %q", f.PrincipleID, f.Evidence)
		}
	}
}

func TestEnginePanicDoesNotChangeExitMapping(t *testing.T) {
	r := &report.Report{Findings: []report.Finding{}}
	panicker := func(_ context.Context, _ *CheckEnv) report.Finding {
		panic("kaboom")
	}
	eng := &Engine{
		Registry:     map[string]Check{"P3": panicker},
		ProbeTimeout: 5 * time.Second,
		Probe:        fakeOKProbe,
	}

	eng.Run(context.Background(), "/fake", r, nil)

	code := exit.MapFromReport(r, report.SeverityHigh)
	if code != exit.OK {
		t.Errorf("panic must not promote exit code beyond OK; got %d", code)
	}
}

func TestSeverityForKnownIDs(t *testing.T) {
	cases := []struct {
		id   string
		want report.Severity
	}{
		{"P1", report.SeverityMedium},
		{"P2", report.SeverityMedium},
		{"P3", report.SeverityMedium},
		{"P4", report.SeverityMedium},
		{"P5", report.SeverityMedium},
		{"P6", report.SeverityHigh},
		{"P7", report.SeverityHigh},
		{"P8", report.SeverityMedium},
		{"P9", report.SeverityMedium},
		{"P10", report.SeverityMedium},
		{"P11", report.SeverityMedium},
		{"P12", report.SeverityMedium},
		{"P13", report.SeverityMedium},
		{"P14", report.SeverityMedium},
		{"P15", report.SeverityHigh},
		{"P16", report.SeverityHigh},
	}
	for _, c := range cases {
		if got := severityFor(c.id); got != c.want {
			t.Errorf("severityFor(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestStubCheckShape(t *testing.T) {
	p := manifest.Embedded.Principles[0]
	env := &CheckEnv{Target: "/fake", Principle: p}
	f := stubCheck(context.Background(), env)

	if f.PrincipleID != p.PrincipleID() {
		t.Errorf("PrincipleID = %q, want %q", f.PrincipleID, p.PrincipleID())
	}
	if f.Title != p.Title {
		t.Errorf("Title = %q, want %q", f.Title, p.Title)
	}
	if f.Category != p.Category {
		t.Errorf("Category = %q, want %q", f.Category, p.Category)
	}
	if f.Status != report.StatusReview {
		t.Errorf("Status = %q, want review", f.Status)
	}
	if f.Kind != report.KindRequiresReview {
		t.Errorf("Kind = %q, want requires-review", f.Kind)
	}
	if f.Severity == "" {
		t.Errorf("Severity must be set")
	}
	if f.Evidence == "" {
		t.Errorf("Evidence must be set")
	}
	if f.Recommendation == "" {
		t.Errorf("Recommendation must be set")
	}
	if f.Hint != p.URL {
		t.Errorf("Hint = %q, want %q", f.Hint, p.URL)
	}
}

func TestRunProbeRespectsContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in -short mode")
	}
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skipf("/bin/sleep unavailable: %v", err)
	}

	start := time.Now()
	c := runProbe(context.Background(), "/bin/sleep", []string{"5"}, 100*time.Millisecond)
	dur := time.Since(start)

	if c.Err == nil && c.ExitCode == 0 {
		t.Errorf("expected probe to be cancelled by context deadline, got clean exit")
	}
	if dur > time.Second {
		t.Errorf("probe took %v; expected <1s due to 100ms timeout", dur)
	}
}
