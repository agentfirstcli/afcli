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
	// With T03's rerun pass and an empty nondeterministic list, every
	// safe entry fires twice — once for Capture, once for Rerun.
	want := [][]string{
		{"--help"},
		{"--afcli-bogus-flag"},
		{"--version"},
		{"--version"},
		{"--help-schema"},
		{"--help-schema"},
		{"status", "--short"},
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

	// 2 static + 1 behavioral + 1 rerun (no nondeterministic opt-out).
	if len(rp.calls) != 4 {
		t.Fatalf("Probe calls = %d, want 4", len(rp.calls))
	}
	if rp.calls[0].extraEnv != nil {
		t.Errorf("static --help probe got extraEnv %v; want nil", rp.calls[0].extraEnv)
	}
	if rp.calls[1].extraEnv != nil {
		t.Errorf("static --afcli-bogus-flag probe got extraEnv %v; want nil", rp.calls[1].extraEnv)
	}
	if got := rp.calls[2].extraEnv; got == nil || got["AFCLI_TEST_PROBE"] != "1" {
		t.Errorf("behavioral probe extraEnv = %v, want AFCLI_TEST_PROBE=1", got)
	}
	if got := rp.calls[3].extraEnv; got == nil || got["AFCLI_TEST_PROBE"] != "1" {
		t.Errorf("rerun probe extraEnv = %v, want AFCLI_TEST_PROBE=1", got)
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

	// Two static + one real behavioral + one rerun = 4.
	if len(rp.calls) != 4 {
		t.Fatalf("Probe calls = %d, want 4 (whitespace-only entries skipped, rerun fires once)", len(rp.calls))
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

	// 2 static + 3 behavioral + 3 reruns (empty nondeterministic).
	if len(rp.calls) != 8 {
		t.Fatalf("Probe calls = %d, want 8 (2 static + 3 behavioral + 3 reruns)", len(rp.calls))
	}
	for i, want := range []string{"--a", "--a", "--b", "--b", "--c", "--c"} {
		if got := rp.calls[2+i].args; len(got) != 1 || got[0] != want {
			t.Errorf("behavioral call %d args = %v, want [%q]", i, got, want)
		}
	}
}

// TestBehavioralCaptureRerunPopulatedWhenAuthorized — empty
// Nondeterministic list and a passing first probe → Rerun is populated
// and probe is invoked twice with byte-equal argv.
func TestBehavioralCaptureRerunPopulatedWhenAuthorized(t *testing.T) {
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
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	// 4 calls: --help, --afcli-bogus-flag, --version, --version (rerun).
	wantArgs := [][]string{
		{"--help"},
		{bogusFlagArg},
		{"--version"},
		{"--version"},
	}
	if len(rp.calls) != len(wantArgs) {
		t.Fatalf("Probe calls = %d, want %d (%v)", len(rp.calls), len(wantArgs), rp.calls)
	}
	for i, w := range wantArgs {
		if !equalStrings(rp.calls[i].args, w) {
			t.Errorf("call %d args = %v, want %v", i, rp.calls[i].args, w)
		}
	}
	if captured == nil {
		t.Fatal("captured CheckEnv missing")
	}
	if len(captured.Behavioral) != 1 {
		t.Fatalf("Behavioral len = %d, want 1", len(captured.Behavioral))
	}
	if captured.Behavioral[0].Rerun == nil {
		t.Fatal("Behavioral[0].Rerun is nil; expected populated rerun capture")
	}
	if captured.Behavioral[0].Rerun.Err != nil {
		t.Errorf("Behavioral[0].Rerun.Err = %v, want nil", captured.Behavioral[0].Rerun.Err)
	}
}

// TestBehavioralCaptureRerunSuppressedByNondeterministic — descriptor
// opt-out via Commands.Nondeterministic suppresses the rerun. The first
// probe still fires; the rerun does not.
func TestBehavioralCaptureRerunSuppressedByNondeterministic(t *testing.T) {
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
		Commands: descriptor.Commands{
			Safe:             []string{"--top"},
			Nondeterministic: []string{"--top"},
		},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	// 3 calls: --help, --afcli-bogus-flag, --top. No rerun.
	if len(rp.calls) != 3 {
		t.Fatalf("Probe calls = %d, want 3 (rerun must be suppressed for nondet entry)", len(rp.calls))
	}
	if captured == nil {
		t.Fatal("captured CheckEnv missing")
	}
	if len(captured.Behavioral) != 1 {
		t.Fatalf("Behavioral len = %d, want 1", len(captured.Behavioral))
	}
	if captured.Behavioral[0].Rerun != nil {
		t.Errorf("Behavioral[0].Rerun = %+v, want nil (opted out via Nondeterministic)", captured.Behavioral[0].Rerun)
	}
	if captured.Behavioral[0].Capture == nil || captured.Behavioral[0].Capture.Err != nil {
		t.Errorf("Behavioral[0].Capture should be a clean capture; got %+v", captured.Behavioral[0].Capture)
	}
}

// TestBehavioralCaptureRerunSuppressedOnFirstProbeError — when the first
// probe errors (e.g. timeout), the rerun must not fire — the failure
// aggregator owns this case via bc.Capture.Err.
func TestBehavioralCaptureRerunSuppressedOnFirstProbeError(t *testing.T) {
	var probeCount int
	probe := func(_ context.Context, _ string, args []string, _ time.Duration, _ map[string]string) *Capture {
		if len(args) == 1 && (args[0] == "--help" || args[0] == bogusFlagArg) {
			return &Capture{Args: args, Stdout: "ok", ExitCode: 0}
		}
		probeCount++
		return &Capture{Args: args, Err: errProbeTimeout, Duration: 100 * time.Millisecond, ExitCode: -1}
	}
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
		Probe:        probe,
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--hang"}},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	if probeCount != 1 {
		t.Fatalf("behavioral probe call count = %d, want 1 (rerun must be suppressed when first probe errored)", probeCount)
	}
	if captured == nil {
		t.Fatal("captured CheckEnv missing")
	}
	if len(captured.Behavioral) != 1 {
		t.Fatalf("Behavioral len = %d, want 1", len(captured.Behavioral))
	}
	if captured.Behavioral[0].Rerun != nil {
		t.Errorf("Behavioral[0].Rerun = %+v, want nil (first probe errored)", captured.Behavioral[0].Rerun)
	}
	if !IsProbeTimeout(captured.Behavioral[0].Capture.Err) {
		t.Errorf("Behavioral[0].Capture.Err = %v, want errProbeTimeout", captured.Behavioral[0].Capture.Err)
	}
}

// TestBehavioralCaptureRerunReusesAuthorizedArgv — when the first probe
// is denied via authorizeProbe (destructive overlap), the rerun must not
// fire and no second authorization gate is invoked. Capture carries the
// *AuthError; Rerun is nil.
func TestBehavioralCaptureRerunReusesAuthorizedArgv(t *testing.T) {
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
		Commands: descriptor.Commands{
			Safe:        []string{"--burn"},
			Destructive: []string{"--burn"},
		},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	// Only static probes should reach recordingProbe.
	if len(rp.calls) != 2 {
		t.Fatalf("Probe calls = %d, want 2 (denied entry never invokes Probe, neither does its rerun)", len(rp.calls))
	}
	if captured == nil {
		t.Fatal("captured CheckEnv missing")
	}
	if len(captured.Behavioral) != 1 {
		t.Fatalf("Behavioral len = %d, want 1", len(captured.Behavioral))
	}
	bc := captured.Behavioral[0]
	if bc.Capture == nil {
		t.Fatal("Behavioral[0].Capture nil")
	}
	var ae *AuthError
	if !errors.As(bc.Capture.Err, &ae) {
		t.Errorf("Behavioral[0].Capture.Err = %v, want *AuthError", bc.Capture.Err)
	}
	if bc.Rerun != nil {
		t.Errorf("Behavioral[0].Rerun = %+v, want nil (authorization failed; rerun must not fire)", bc.Rerun)
	}
}

// TestBehavioralCaptureRerunSetMembershipNotFirstMatch — duplicate Safe
// entries that ALL appear in Nondeterministic must each suppress the
// rerun (set-membership semantics, not first-match).
func TestBehavioralCaptureRerunSetMembershipNotFirstMatch(t *testing.T) {
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
		Commands: descriptor.Commands{
			Safe:             []string{"--version", "--version"},
			Nondeterministic: []string{"--version"},
		},
	}
	r := &report.Report{}
	eng.Run(context.Background(), "/fake", r, d)

	// 2 static + 2 behavioral (one per Safe entry, no reruns) = 4.
	if len(rp.calls) != 4 {
		t.Fatalf("Probe calls = %d, want 4 (both --version entries opted out of rerun)", len(rp.calls))
	}
	if captured == nil {
		t.Fatal("captured CheckEnv missing")
	}
	if len(captured.Behavioral) != 2 {
		t.Fatalf("Behavioral len = %d, want 2", len(captured.Behavioral))
	}
	for i, bc := range captured.Behavioral {
		if bc.Rerun != nil {
			t.Errorf("Behavioral[%d].Rerun = %+v, want nil (both occurrences opted out)", i, bc.Rerun)
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
