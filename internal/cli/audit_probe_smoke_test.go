package cli_test

import (
	"strings"
	"testing"
)

// TestProbeFlagsAcceptedWithoutDescriptor — the new --probe and
// --probe-timeout flags must parse cleanly even when no descriptor is
// supplied. T01 only plants the flags and the Engine.ProbeEnabled knob;
// behavioral probing arrives in T03. So the contract here is narrow:
// the binary accepts the flags, runs the same 16-finding S04 surface,
// and exits without panicking.
func TestProbeFlagsAcceptedWithoutDescriptor(t *testing.T) {
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--probe", "--probe-timeout=2s", "--output", "json")
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if strings.Contains(string(stderr), "panic") {
		t.Fatalf("stderr contains panic:\n%s", stderr)
	}
	if strings.Contains(string(stderr), "unknown flag") {
		t.Fatalf("flags rejected:\n%s", stderr)
	}

	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}
}

// TestProbeFlagsAdvertisedInHelp — afcli audit --help must list both
// --probe and --probe-timeout so an agent inspecting the surface can
// discover them without reading source. The slice verification checks
// the same via grep; this in-process variant catches regressions during
// `go test` even when the integration script is not run.
func TestProbeFlagsAdvertisedInHelp(t *testing.T) {
	stdout, _, code := runAudit(t, "audit", "--help")
	if code != 0 {
		t.Fatalf("audit --help exit code: want 0, got %d", code)
	}
	out := string(stdout)
	if !strings.Contains(out, "--probe ") && !strings.Contains(out, "--probe\n") {
		t.Errorf("audit --help missing --probe flag:\n%s", out)
	}
	if !strings.Contains(out, "--probe-timeout") {
		t.Errorf("audit --help missing --probe-timeout flag:\n%s", out)
	}
}
