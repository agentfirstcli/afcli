package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/report"
)

// recordingProbe captures every Probe invocation so tests can assert on
// argv ordering, extraEnv propagation, and per-call uniqueness.
type recordingProbe struct {
	mu    sync.Mutex
	calls []recordedProbeCall
}

type recordedProbeCall struct {
	args     []string
	extraEnv map[string]string
}

func (rp *recordingProbe) probe(_ context.Context, _ string, args []string, _ time.Duration, extraEnv map[string]string) *Capture {
	rp.mu.Lock()
	rp.calls = append(rp.calls, recordedProbeCall{args: args, extraEnv: extraEnv})
	rp.mu.Unlock()
	return &Capture{Args: args, Stdout: "rec", ExitCode: 0}
}

// TestBehavioralCapturePopulatedInDeclarationOrder — Engine.Run iterates
// descriptor.Commands.Safe[] in declaration order; the resulting
// CheckEnv.Behavioral must match that order exactly (no sorting).
func TestBehavioralCapturePopulatedInDeclarationOrder(t *testing.T) {
	rp := &recordingProbe{}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        rp.probe,
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{
			Safe: []string{"--version", "--help-schema", "status --short"},
		},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	if len(r.Findings) != 16 {
		t.Fatalf("expected 16 findings, got %d", len(r.Findings))
	}
	// Every CheckEnv constructed by Engine.Run must carry the same
	// Behavioral slice; sample the first finding's check input by
	// re-reading from one of the per-principle captures via the
	// recording probe's call log.
	// Instead, assert by inspecting recordingProbe — the static probes
	// (--help, --afcli-bogus-flag) come first, then the three behavioral
	// captures in Safe[] declaration order.
	want := [][]string{
		{"--help"},
		{"--afcli-bogus-flag"},
		{"--version"},
		{"--help-schema"},
		{"status", "--short"},
	}
	if len(rp.calls) != len(want) {
		t.Fatalf("Probe call count = %d, want %d (%v)", len(rp.calls), len(want), rp.calls)
	}
	for i, w := range want {
		if !equalStrings(rp.calls[i].args, w) {
			t.Errorf("call %d args = %v, want %v", i, rp.calls[i].args, w)
		}
	}
}

// TestBehavioralCaptureExposedToCheckEnv — a stub Check captures its
// CheckEnv; assert Behavioral has the right entries and order.
func TestBehavioralCaptureExposedToCheckEnv(t *testing.T) {
	rp := &recordingProbe{}
	var captured *CheckEnv
	probe := func(ctx context.Context, _ string, args []string, _ time.Duration, env map[string]string) *Capture {
		return rp.probe(ctx, "", args, 0, env)
	}
	registry := map[string]Check{
		"P3": func(_ context.Context, env *CheckEnv) report.Finding {
			captured = env
			return baseFinding(env)
		},
	}
	eng := &Engine{
		Registry:     registry,
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        probe,
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version", "--help-schema"}},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	if captured == nil {
		t.Fatal("P3 check was not invoked")
	}
	if len(captured.Behavioral) != 2 {
		t.Fatalf("Behavioral len = %d, want 2", len(captured.Behavioral))
	}
	if captured.Behavioral[0].Cmd != "--version" {
		t.Errorf("Behavioral[0].Cmd = %q, want %q", captured.Behavioral[0].Cmd, "--version")
	}
	if captured.Behavioral[1].Cmd != "--help-schema" {
		t.Errorf("Behavioral[1].Cmd = %q, want %q", captured.Behavioral[1].Cmd, "--help-schema")
	}
	if captured.Behavioral[0].Capture == nil || captured.Behavioral[0].Capture.Err != nil {
		t.Errorf("Behavioral[0].Capture should be a clean capture; got %+v", captured.Behavioral[0].Capture)
	}
}

// TestBehavioralCaptureNilDescriptorEmpty — ProbeEnabled=true but
// descriptor=nil → no behavioral pass; CheckEnv.Behavioral is nil.
func TestBehavioralCaptureNilDescriptorEmpty(t *testing.T) {
	rp := &recordingProbe{}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        rp.probe,
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, nil)

	// Only the two static probes should have been invoked.
	if len(rp.calls) != 2 {
		t.Fatalf("Probe calls = %d, want 2 (only --help and --afcli-bogus-flag)", len(rp.calls))
	}
}

// TestBehavioralCaptureProbeDisabledEmpty — ProbeEnabled=false with a
// non-nil descriptor → no behavioral pass.
func TestBehavioralCaptureProbeDisabledEmpty(t *testing.T) {
	rp := &recordingProbe{}
	eng := &Engine{
		ProbeEnabled: false,
		ProbeTimeout: time.Second,
		Probe:        rp.probe,
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	if len(rp.calls) != 2 {
		t.Fatalf("Probe calls = %d, want 2 (only static probes)", len(rp.calls))
	}
}

// TestBehavioralCaptureDescriptorEnvPropagated — descriptor.Env keys
// reach the probe's extraEnv argument verbatim. Static probes
// (--help / --afcli-bogus-flag) still receive nil.
func TestBehavioralCaptureDescriptorEnvPropagated(t *testing.T) {
	rp := &recordingProbe{}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        rp.probe,
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
		Env:      map[string]string{"AFCLI_TEST_PROBE": "1"},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	if len(rp.calls) != 3 {
		t.Fatalf("Probe calls = %d, want 3", len(rp.calls))
	}
	if rp.calls[0].extraEnv != nil {
		t.Errorf("static --help probe got extraEnv %v; want nil", rp.calls[0].extraEnv)
	}
	if rp.calls[1].extraEnv != nil {
		t.Errorf("static --afcli-bogus-flag probe got extraEnv %v; want nil", rp.calls[1].extraEnv)
	}
	got := rp.calls[2].extraEnv
	if got == nil || got["AFCLI_TEST_PROBE"] != "1" {
		t.Errorf("behavioral probe extraEnv = %v, want AFCLI_TEST_PROBE=1", got)
	}
}

// TestBehavioralCaptureEmptyAndWhitespaceEntriesSkipped — strings.Fields
// of "" or "   " yields an empty argv; engine skips entirely (no probe
// call, no Behavioral entry).
func TestBehavioralCaptureEmptyAndWhitespaceEntriesSkipped(t *testing.T) {
	rp := &recordingProbe{}
	var captured *CheckEnv
	registry := map[string]Check{
		"P3": func(_ context.Context, env *CheckEnv) report.Finding {
			if captured == nil {
				captured = env
			}
			return baseFinding(env)
		},
	}
	eng := &Engine{
		Registry:     registry,
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        rp.probe,
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"", "   ", "--version"}},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	// Two static + one real behavioral = 3.
	if len(rp.calls) != 3 {
		t.Fatalf("Probe calls = %d, want 3 (whitespace-only entries skipped)", len(rp.calls))
	}
	if captured == nil {
		t.Fatal("captured CheckEnv missing")
	}
	if len(captured.Behavioral) != 1 {
		t.Fatalf("Behavioral len = %d, want 1", len(captured.Behavioral))
	}
	if captured.Behavioral[0].Cmd != "--version" {
		t.Errorf("Behavioral[0].Cmd = %q, want %q", captured.Behavioral[0].Cmd, "--version")
	}
}

// TestBehavioralCaptureDestructiveOverlapDeniedAtRuntime — when an entry
// appears in BOTH Commands.Safe and Commands.Destructive, the engine's
// authorizeProbe call rejects it; the Behavioral slice records a
// synthesized *Capture carrying an *AuthError, and the probe is never
// invoked for that entry.
func TestBehavioralCaptureDestructiveOverlapDeniedAtRuntime(t *testing.T) {
	rp := &recordingProbe{}
	var captured *CheckEnv
	registry := map[string]Check{
		"P3": func(_ context.Context, env *CheckEnv) report.Finding {
			if captured == nil {
				captured = env
			}
			return baseFinding(env)
		},
	}
	eng := &Engine{
		Registry:     registry,
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        rp.probe,
	}
	// "--burn" is the second Safe entry AND in Destructive. The first
	// occurrence ALSO denies (Destructive overlap is checked before Safe
	// match), so both behavioral entries land as denied.
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{
			Safe:        []string{"--burn", "--burn"},
			Destructive: []string{"--burn"},
		},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	// Only the two static probes hit recordingProbe; both --burn entries
	// short-circuited via authorizeProbe.
	if len(rp.calls) != 2 {
		t.Fatalf("Probe calls = %d, want 2 (--burn never invoked)", len(rp.calls))
	}
	if captured == nil {
		t.Fatal("captured CheckEnv missing")
	}
	if len(captured.Behavioral) != 2 {
		t.Fatalf("Behavioral len = %d, want 2", len(captured.Behavioral))
	}
	for i, bc := range captured.Behavioral {
		if bc.Capture == nil {
			t.Fatalf("Behavioral[%d].Capture nil", i)
		}
		var ae *AuthError
		if !errors.As(bc.Capture.Err, &ae) {
			t.Errorf("Behavioral[%d].Capture.Err = %v, want *AuthError", i, bc.Capture.Err)
		}
	}
}

// TestBehavioralCaptureMultipleSafeEntriesAllRunWhenAuthorized — when no
// destructive overlap, every Safe entry produces one probe invocation.
func TestBehavioralCaptureMultipleSafeEntriesAllRunWhenAuthorized(t *testing.T) {
	rp := &recordingProbe{}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        rp.probe,
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--a", "--b", "--c"}},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	if len(rp.calls) != 5 {
		t.Fatalf("Probe calls = %d, want 5 (2 static + 3 behavioral)", len(rp.calls))
	}
	for i, want := range []string{"--a", "--b", "--c"} {
		if got := rp.calls[2+i].args; len(got) != 1 || got[0] != want {
			t.Errorf("behavioral call %d args = %v, want [%q]", i, got, want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
