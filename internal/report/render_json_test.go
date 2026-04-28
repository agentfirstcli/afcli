package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderJSONEmpty — zero-finding report renders the documented
// schema-valid shape: indented, snake_case keys, `findings: []` (not null).
func TestRenderJSONEmpty(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      42,
	}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, `"findings": []`) {
		t.Errorf("empty findings should render as []: %s", s)
	}
	if !strings.Contains(s, `"manifest_version": "v1"`) {
		t.Errorf("missing manifest_version: %s", s)
	}
	if !strings.Contains(s, "\n  \"") {
		t.Errorf("expected two-space indent, got: %s", s)
	}
	var got Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Target != "/bin/echo" {
		t.Errorf("non-deterministic should preserve absolute target: %q", got.Target)
	}
	if got.StartedAt != "2026-01-01T00:00:00Z" || got.DurationMs != 42 {
		t.Errorf("non-deterministic should preserve timestamps: %+v", got)
	}
}

// TestRenderJSONStableSort — findings are emitted in lexicographic
// PrincipleID order regardless of input order, and the source slice is
// not mutated. This is the slice contract, not a deterministic-only mode.
func TestRenderJSONStableSort(t *testing.T) {
	in := []Finding{
		{PrincipleID: "P14", Title: "x", Category: "c", Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow},
		{PrincipleID: "P2", Title: "y", Category: "c", Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow},
		{PrincipleID: "P10", Title: "z", Category: "c", Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow},
		{PrincipleID: "P1", Title: "a", Category: "c", Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow},
	}
	original := make([]Finding, len(in))
	copy(original, in)

	r := &Report{ManifestVersion: "v1", AfcliVersion: "0.0.0", Target: "t", StartedAt: "0", Findings: in}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var got Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"P1", "P10", "P14", "P2"} // lexicographic, not numeric
	if len(got.Findings) != len(want) {
		t.Fatalf("len mismatch: got %d want %d", len(got.Findings), len(want))
	}
	for i, w := range want {
		if got.Findings[i].PrincipleID != w {
			t.Errorf("order[%d]: got %q want %q", i, got.Findings[i].PrincipleID, w)
		}
	}
	for i := range in {
		if in[i] != original[i] {
			t.Errorf("input mutated at [%d]: got %+v want %+v", i, in[i], original[i])
		}
	}
}

// TestRenderJSONErrorOnly — error envelope renders, findings collapse to
// `[]`, and Details survives the round trip.
func TestRenderJSONErrorOnly(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/nonexistent",
		StartedAt:       "0",
		Error: &ErrorEnvelope{
			Code:    CodeTargetNotFound,
			Message: "target not found",
			Hint:    "check $PATH",
			Details: map[string]any{"resolved": "/nonexistent"},
		},
	}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, `"findings": []`) {
		t.Errorf("findings should be empty array even with error: %s", s)
	}
	var got Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Error == nil || got.Error.Code != CodeTargetNotFound {
		t.Fatalf("error envelope lost: %+v", got.Error)
	}
	if v, ok := got.Error.Details["resolved"].(string); !ok || v != "/nonexistent" {
		t.Errorf("error details lost: %+v", got.Error.Details)
	}
}

// TestRenderJSONDeterministicDivergence — the same input renders
// differently in deterministic vs non-deterministic mode (timestamps
// zeroed) and identically across runs in deterministic mode.
func TestRenderJSONDeterministicDivergence(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "2026-04-28T10:00:00Z",
		DurationMs:      1234,
		Findings: []Finding{
			{PrincipleID: "P6", Title: "t", Category: "c",
				Status: StatusFail, Kind: KindAutomated, Severity: SeverityHigh,
				Evidence: "e", Recommendation: "r"},
		},
	}

	var nondet, det bytes.Buffer
	if err := RenderJSON(&nondet, r, RenderOptions{Deterministic: false}); err != nil {
		t.Fatal(err)
	}
	if err := RenderJSON(&det, r, RenderOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if nondet.String() == det.String() {
		t.Errorf("deterministic and non-deterministic outputs should differ on the same input")
	}
	if !strings.Contains(det.String(), `"started_at": ""`) {
		t.Errorf("deterministic should zero started_at: %s", det.String())
	}
	if !strings.Contains(det.String(), `"duration_ms": 0`) {
		t.Errorf("deterministic should zero duration_ms: %s", det.String())
	}
	if !strings.Contains(nondet.String(), `"started_at": "2026-04-28T10:00:00Z"`) {
		t.Errorf("non-deterministic should preserve started_at: %s", nondet.String())
	}
	if !strings.Contains(nondet.String(), `"duration_ms": 1234`) {
		t.Errorf("non-deterministic should preserve duration_ms: %s", nondet.String())
	}

	var det2 bytes.Buffer
	if err := RenderJSON(&det2, r, RenderOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if det.String() != det2.String() {
		t.Errorf("deterministic mode produced divergent output across runs:\nA=%s\nB=%s", det.String(), det2.String())
	}
}

// TestRenderJSONDeterministicRelativePath — deterministic mode rewrites
// an absolute Target under cwd to a relative path so golden files don't
// embed a developer's home directory.
func TestRenderJSONDeterministicRelativePath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(cwd, "fixtures", "echo")
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          abs,
		StartedAt:       "0",
	}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, r, RenderOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"target": "fixtures/echo"`) {
		t.Errorf("expected relative target 'fixtures/echo', got: %s", buf.String())
	}
	if r.Target != abs {
		t.Errorf("input report mutated: target=%q want=%q", r.Target, abs)
	}
}
