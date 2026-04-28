package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/report"
)

// timeoutProbe returns a Capture whose Err is errProbeTimeout for any
// args other than --help / --afcli-bogus-flag (which use the OK shape so
// the static checks behave normally). Duration is fixed for evidence
// determinism.
func timeoutProbe(_ context.Context, _ string, args []string, _ time.Duration, _ map[string]string) *Capture {
	if len(args) == 1 && (args[0] == "--help" || args[0] == bogusFlagArg) {
		return &Capture{Args: args, Stdout: "fake stdout", ExitCode: 0}
	}
	return &Capture{Args: args, Err: errProbeTimeout, Duration: 250 * time.Millisecond, ExitCode: -1}
}

func findP3(t *testing.T, r *report.Report) *report.Finding {
	t.Helper()
	for i := range r.Findings {
		if r.Findings[i].PrincipleID == "P3" {
			return &r.Findings[i]
		}
	}
	t.Fatal("P3 finding missing from report")
	return nil
}

func TestProbeEvidenceTimeoutDecoratesP3(t *testing.T) {
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--hang"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     nil,
		ProbeTimeout: 200 * time.Millisecond,
		ProbeEnabled: true,
		Probe:        timeoutProbe,
	}

	eng.Run(context.Background(), "/fake", r, d)

	if len(r.Findings) != 16 {
		t.Fatalf("expected 16 findings, got %d", len(r.Findings))
	}
	p3 := findP3(t, r)
	if p3.Status != report.StatusReview {
		t.Errorf("P3.Status = %q, want %q", p3.Status, report.StatusReview)
	}
	if p3.Kind != report.KindRequiresReview {
		t.Errorf("P3.Kind = %q, want %q", p3.Kind, report.KindRequiresReview)
	}
	if !strings.Contains(p3.Evidence, "probe timeout") {
		t.Errorf("P3.Evidence = %q, want substring %q", p3.Evidence, "probe timeout")
	}
	if !strings.Contains(p3.Evidence, "--hang") {
		t.Errorf("P3.Evidence = %q must mention failing cmd %q", p3.Evidence, "--hang")
	}
	if !strings.Contains(p3.Evidence, "exceeded 250ms") {
		t.Errorf("P3.Evidence = %q must mention duration %q", p3.Evidence, "exceeded 250ms")
	}
	if p3.Severity != severityFor("P3") {
		t.Errorf("P3.Severity = %q, want %q (preserved via baseFinding)", p3.Severity, severityFor("P3"))
	}
	if p3.Title == "" || p3.Category == "" {
		t.Errorf("P3 must inherit Title/Category from manifest; got Title=%q Category=%q", p3.Title, p3.Category)
	}
	if !strings.Contains(p3.Recommendation, "commands.safe") {
		t.Errorf("P3.Recommendation = %q, want substring %q", p3.Recommendation, "commands.safe")
	}
}

func TestProbeEvidenceDeniedDecoratesP3(t *testing.T) {
	// --burn appears in BOTH Safe and Destructive — authorizeProbe will
	// return *AuthError before the Probe is called, so the BehavioralCapture
	// carries an AuthError on Capture.Err.
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{
			Safe:        []string{"--burn"},
			Destructive: []string{"--burn"},
		},
	}
	var probeCalls int
	wrapped := func(ctx context.Context, target string, args []string, d time.Duration, env map[string]string) *Capture {
		if len(args) == 1 && (args[0] == "--help" || args[0] == bogusFlagArg) {
			return fakeOKProbe(ctx, target, args, d, env)
		}
		probeCalls++
		return fakeOKProbe(ctx, target, args, d, env)
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     nil,
		ProbeTimeout: 5 * time.Second,
		ProbeEnabled: true,
		Probe:        wrapped,
	}

	eng.Run(context.Background(), "/fake", r, d)

	if probeCalls != 0 {
		t.Errorf("destructive-overlap candidate must be denied before Probe runs; saw %d invocations", probeCalls)
	}
	if len(r.Findings) != 16 {
		t.Fatalf("expected 16 findings, got %d", len(r.Findings))
	}
	p3 := findP3(t, r)
	if p3.Status != report.StatusReview {
		t.Errorf("P3.Status = %q, want %q", p3.Status, report.StatusReview)
	}
	if !strings.Contains(p3.Evidence, "probe denied") {
		t.Errorf("P3.Evidence = %q, want substring %q", p3.Evidence, "probe denied")
	}
	if !strings.Contains(p3.Evidence, "--burn") {
		t.Errorf("P3.Evidence = %q must mention denied cmd %q", p3.Evidence, "--burn")
	}
	if !strings.Contains(p3.Evidence, "matches commands.destructive") {
		t.Errorf("P3.Evidence = %q must carry the AuthError reason", p3.Evidence)
	}
}

func TestProbeEvidenceSkipPreservedOverAggregator(t *testing.T) {
	// Descriptor skips P3 AND has a probe entry that times out. Skip wins.
	d := &descriptor.Descriptor{
		SkipPrinciples: []string{"P3"},
		Commands:       descriptor.Commands{Safe: []string{"--hang"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     nil,
		ProbeTimeout: 200 * time.Millisecond,
		ProbeEnabled: true,
		Probe:        timeoutProbe,
	}

	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if p3.Status != report.StatusSkip {
		t.Errorf("P3.Status = %q, want %q (skip-by-policy must win over aggregator)", p3.Status, report.StatusSkip)
	}
	if p3.Evidence != "skipped per descriptor" {
		t.Errorf("P3.Evidence = %q, want %q", p3.Evidence, "skipped per descriptor")
	}
}

func TestProbeEvidenceMultipleTimeoutsConcatenated(t *testing.T) {
	// Two safe entries, both time out — evidence must enumerate both
	// invocations so an agent can correlate each failure.
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--hang", "--also-hang"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     nil,
		ProbeTimeout: 200 * time.Millisecond,
		ProbeEnabled: true,
		Probe:        timeoutProbe,
	}

	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if !strings.Contains(p3.Evidence, "--hang") {
		t.Errorf("P3.Evidence = %q must mention --hang", p3.Evidence)
	}
	if !strings.Contains(p3.Evidence, "--also-hang") {
		t.Errorf("P3.Evidence = %q must mention --also-hang", p3.Evidence)
	}
	// Two separate "probe timeout:" prefixes — one per invocation.
	if got := strings.Count(p3.Evidence, "probe timeout:"); got != 2 {
		t.Errorf("expected 2 'probe timeout:' lines, got %d in evidence %q", got, p3.Evidence)
	}
}

func TestProbeEvidenceNoOpWhenNoFailures(t *testing.T) {
	// All probes succeed (errors == nil). The aggregator must NOT
	// replace P3 — the stub finding (or registered check) survives.
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     nil,
		ProbeTimeout: 5 * time.Second,
		ProbeEnabled: true,
		Probe:        fakeOKProbe,
	}

	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if strings.Contains(p3.Evidence, "probe timeout") || strings.Contains(p3.Evidence, "probe denied") {
		t.Errorf("P3.Evidence = %q must not carry probe-failure decoration when all probes succeeded", p3.Evidence)
	}
}
