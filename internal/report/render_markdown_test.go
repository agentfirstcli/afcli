package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderMarkdownEmpty — zero-finding report renders the H1, the
// metadata table, and a `_no findings_` placeholder.
func TestRenderMarkdownEmpty(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      42,
	}
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	got := buf.String()
	for _, want := range []string{
		"# afcli audit report",
		"| field | value |",
		"| --- | --- |",
		"| manifest_version | v1 |",
		"| afcli_version | 0.0.0 |",
		"| target | /bin/echo |",
		"| duration_ms | 42 |",
		"| interrupted | false |",
		"_no findings_",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestRenderMarkdownFindingsGroupedByCategory — findings group under H2
// category headings (alphabetical) and each finding is an H3 section.
func TestRenderMarkdownFindingsGroupedByCategory(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "t",
		StartedAt:       "0",
		Findings: []Finding{
			{PrincipleID: "P7", Title: "no-color-isatty", Category: "ergonomics",
				Status: StatusPass, Kind: KindAutomated, Severity: SeverityMedium,
				Evidence: "respects NO_COLOR"},
			{PrincipleID: "P14", Title: "json-on-stdout", Category: "machine-readable",
				Status: StatusFail, Kind: KindAutomated, Severity: SeverityHigh,
				Evidence: "log noise on stdout", Recommendation: "route logs to stderr"},
			{PrincipleID: "P6", Title: "fail-fast", Category: "ergonomics",
				Status: StatusFail, Kind: KindAutomated, Severity: SeverityHigh},
		},
	}
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	iErg := strings.Index(got, "## ergonomics")
	iMach := strings.Index(got, "## machine-readable")
	if iErg < 0 || iMach < 0 || iErg >= iMach {
		t.Errorf("expected `## ergonomics` to precede `## machine-readable`, got:\n%s", got)
	}

	// Inside ergonomics, P6 must precede P7 (lexicographic stable sort).
	iP6 := strings.Index(got, "### P6")
	iP7 := strings.Index(got, "### P7")
	if iP6 < 0 || iP7 < 0 || iP6 >= iP7 {
		t.Errorf("expected P6 before P7 inside ergonomics, got:\n%s", got)
	}
	// P7 must still come before the next category.
	if iP7 >= iMach {
		t.Errorf("P7 should appear before machine-readable category, got:\n%s", got)
	}

	for _, want := range []string{
		"### P14 — json-on-stdout",
		"- status: `fail`",
		"- severity: `high`",
		"- evidence: log noise on stdout",
		"- recommendation: route logs to stderr",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestRenderMarkdownErrorEnvelope — error envelope surfaces all four
// fields in a labeled `## error` block with deterministic detail order.
func TestRenderMarkdownErrorEnvelope(t *testing.T) {
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
	if err := RenderMarkdown(&buf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"## error",
		"- code: `TARGET_NOT_FOUND`",
		"- message: target not found",
		"- hint: check $PATH",
		"- details:",
		"  - `errno`: 2",
		"  - `resolved`: /nonexistent",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "errno") >= strings.Index(got, "resolved") {
		t.Errorf("error details should be alphabetically ordered: %s", got)
	}
}

// TestRenderMarkdownDeterministic — same input → byte-identical output
// across runs, with absolute target rewritten to relative and duration
// zeroed.
func TestRenderMarkdownDeterministic(t *testing.T) {
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
		Findings: []Finding{
			{PrincipleID: "P6", Title: "t", Category: "c",
				Status: StatusFail, Kind: KindAutomated, Severity: SeverityHigh},
		},
	}
	var a, b bytes.Buffer
	if err := RenderMarkdown(&a, r, RenderOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if err := RenderMarkdown(&b, r, RenderOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Errorf("deterministic mode diverged:\nA=%s\nB=%s", a.String(), b.String())
	}
	if !strings.Contains(a.String(), "| duration_ms | 0 |") {
		t.Errorf("deterministic should zero duration: %s", a.String())
	}
	if !strings.Contains(a.String(), "| target | fixtures/echo |") {
		t.Errorf("deterministic should rewrite target relative to cwd: %s", a.String())
	}
}

// TestErrorEnvelopeCrossFormatParity — the slice contract requires that
// every renderer surfaces code/message/hint and detail keys for the same
// envelope. This test guards the cross-format invariant by checking each
// output contains those four fields.
func TestErrorEnvelopeCrossFormatParity(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "t",
		StartedAt:       "0",
		Error: &ErrorEnvelope{
			Code:    CodeUsage,
			Message: "unknown flag --bogus",
			Hint:    "see --help",
			Details: map[string]any{"flag": "--bogus"},
		},
	}
	var jsonBuf, txtBuf, mdBuf bytes.Buffer
	if err := RenderJSON(&jsonBuf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := RenderText(&txtBuf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := RenderMarkdown(&mdBuf, r, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{
		"json":     jsonBuf.String(),
		"text":     txtBuf.String(),
		"markdown": mdBuf.String(),
	} {
		for _, must := range []string{"USAGE", "unknown flag --bogus", "see --help", "--bogus"} {
			if !strings.Contains(out, must) {
				t.Errorf("%s renderer missing %q in:\n%s", name, must, out)
			}
		}
	}
}
