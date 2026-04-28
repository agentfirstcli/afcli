package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderTextEmpty — zero-finding report renders the documented
// header line and a `no findings` body.
func TestRenderTextEmpty(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      42,
	}
	var buf bytes.Buffer
	if err := RenderText(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	got := buf.String()
	wantHeader := "afcli 0.0.0 | manifest v1 | target=/bin/echo | duration=42ms"
	if !strings.HasPrefix(got, wantHeader+"\n") {
		t.Errorf("expected header %q, got: %s", wantHeader, got)
	}
	if !strings.Contains(got, "no findings") {
		t.Errorf("expected 'no findings' marker, got: %s", got)
	}
}

// TestRenderTextFindings — finding lines follow the documented
// `[<status>] <pid> <title> — <category> (<severity>)` shape, with
// indented evidence and recommendation.
func TestRenderTextFindings(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "t",
		StartedAt:       "0",
		Findings: []Finding{
			{PrincipleID: "P6", Title: "fail-fast", Category: "ergonomics",
				Status: StatusFail, Kind: KindAutomated, Severity: SeverityHigh,
				Evidence: "exited 0 on bad flag", Recommendation: "exit 2 on usage error"},
		},
	}
	var buf bytes.Buffer
	if err := RenderText(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "[fail] P6 fail-fast — ergonomics (high)") {
		t.Errorf("missing finding line: %s", got)
	}
	if !strings.Contains(got, "  evidence: exited 0 on bad flag") {
		t.Errorf("missing indented evidence: %s", got)
	}
	if !strings.Contains(got, "  recommendation: exit 2 on usage error") {
		t.Errorf("missing indented recommendation: %s", got)
	}
}

// TestRenderTextStableSort — findings emit in lexicographic PrincipleID
// order regardless of input order.
func TestRenderTextStableSort(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "t",
		StartedAt:       "0",
		Findings: []Finding{
			{PrincipleID: "P14", Title: "x", Category: "c", Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow},
			{PrincipleID: "P2", Title: "y", Category: "c", Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow},
			{PrincipleID: "P1", Title: "a", Category: "c", Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow},
		},
	}
	var buf bytes.Buffer
	if err := RenderText(&buf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	i1 := strings.Index(got, "P1 ")
	i14 := strings.Index(got, "P14 ")
	i2 := strings.Index(got, "P2 ")
	if i1 < 0 || i14 < 0 || i2 < 0 || !(i1 < i14 && i14 < i2) {
		t.Errorf("expected P1 < P14 < P2 in output, got: %s", got)
	}
}

// TestRenderTextErrorEnvelope — error block surfaces code/message/hint
// and details (with deterministic key order).
func TestRenderTextErrorEnvelope(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/nonexistent",
		StartedAt:       "0",
		Error: &ErrorEnvelope{
			Code:    CodeTargetNotFound,
			Message: "target not found",
			Hint:    "check $PATH",
			Details: map[string]any{"resolved": "/nonexistent", "errno": 2},
		},
	}
	var buf bytes.Buffer
	if err := RenderText(&buf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"error:",
		"  code: TARGET_NOT_FOUND",
		"  message: target not found",
		"  hint: check $PATH",
		"  details:",
		"    errno: 2",
		"    resolved: /nonexistent",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// errno sorts before resolved alphabetically.
	if strings.Index(got, "errno") >= strings.Index(got, "resolved") {
		t.Errorf("error details should be alphabetically ordered: %s", got)
	}
}

// TestRenderTextDeterministic — the same input renders identically
// across two runs in deterministic mode, with timestamps zeroed and
// absolute target rewritten relative to cwd.
func TestRenderTextDeterministic(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(cwd, "fixtures", "echo")
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          abs,
		StartedAt:       "2026-04-28T10:00:00Z",
		DurationMs:      1234,
	}
	var a, b bytes.Buffer
	if err := RenderText(&a, r, RenderOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if err := RenderText(&b, r, RenderOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Errorf("deterministic mode diverged:\nA=%s\nB=%s", a.String(), b.String())
	}
	if !strings.Contains(a.String(), "duration=0ms") {
		t.Errorf("deterministic should zero duration: %s", a.String())
	}
	if !strings.Contains(a.String(), "target=fixtures/echo") {
		t.Errorf("deterministic should rewrite target relative to cwd: %s", a.String())
	}
	if r.Target != abs {
		t.Errorf("input report mutated: target=%q want=%q", r.Target, abs)
	}
}

// TestRenderTextInterruptedHeader — the header surfaces an interrupted
// flag so a partial report is distinguishable from a clean zero-check run.
func TestRenderTextInterruptedHeader(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1", AfcliVersion: "0.0.0", Target: "t", StartedAt: "0",
		Interrupted: true,
	}
	var buf bytes.Buffer
	if err := RenderText(&buf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "interrupted") {
		t.Errorf("header should surface interrupted flag: %s", buf.String())
	}
}
