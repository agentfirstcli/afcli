package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestRenderBadgeJSONShape decodes the output into a generic map and
// asserts exactly the documented keys are present — no extras, no
// missing keys. Guards against accidental field additions that could
// change the wire shape consumed by shields.io.
func TestRenderBadgeJSONShape(t *testing.T) {
	r := &Report{Findings: mixedFindings(8, 2)}
	var buf bytes.Buffer
	if err := RenderBadgeJSON(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderBadgeJSON: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v\nraw: %s", err, buf.String())
	}

	want := map[string]bool{
		"schemaVersion": true,
		"label":         true,
		"message":       true,
		"color":         true,
		"score":         true,
		"total":         true,
	}
	for k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("missing required key %q in sidecar:\n%s", k, buf.String())
		}
	}
	for k := range m {
		if !want[k] {
			t.Errorf("unexpected extra key %q in sidecar:\n%s", k, buf.String())
		}
	}

	// Forbidden field — couples badge to fork-specific path (RESEARCH §Open Questions item 4).
	if _, present := m["target"]; present {
		t.Errorf("sidecar must not include %q field — breaks fork portability", "target")
	}
}

// TestRenderBadgeJSONCamelCase guards the deliberate casing exception:
// shields.io's endpoint protocol uses camelCase, and the rest of afcli
// uses snake_case, so we assert the literal "schemaVersion" appears
// in the raw output. If a future change converts to snake_case this
// test fires immediately.
func TestRenderBadgeJSONCamelCase(t *testing.T) {
	r := &Report{Findings: passFindings(3)}
	var buf bytes.Buffer
	if err := RenderBadgeJSON(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderBadgeJSON: %v", err)
	}
	raw := buf.String()
	if !strings.Contains(raw, `"schemaVersion"`) {
		t.Errorf("expected camelCase key %q in raw output:\n%s", `"schemaVersion"`, raw)
	}
	if strings.Contains(raw, `"schema_version"`) {
		t.Errorf("snake_case key %q must not appear — shields.io demands camelCase:\n%s", `"schema_version"`, raw)
	}
}

// TestRenderBadgeJSONDeterministic asserts byte-equality across two
// renders of the same input. encoding/json with stable struct field
// order is deterministic, but the test guards against accidental map
// usage or time-sensitive fields slipping in.
func TestRenderBadgeJSONDeterministic(t *testing.T) {
	r := &Report{
		ManifestVersion: "1",
		AfcliVersion:    "test",
		Target:          "/tmp/contrived",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      42,
		Findings:        mixedFindings(8, 2),
	}
	opts := RenderOptions{Deterministic: true}

	var a, b bytes.Buffer
	if err := RenderBadgeJSON(&a, r, opts); err != nil {
		t.Fatalf("first render: %v", err)
	}
	if err := RenderBadgeJSON(&b, r, opts); err != nil {
		t.Fatalf("second render: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Errorf("renders diverged\n--- first ---\n%s\n--- second ---\n%s", a.String(), b.String())
	}
}

// TestRenderBadgeJSONColorMatchesScore is table-driven across the three
// color bands defined in score.go. Confirms the sidecar color is the
// same hex string the SVG renderer uses, so a consumer reading either
// artefact gets the same color signal.
func TestRenderBadgeJSONColorMatchesScore(t *testing.T) {
	cases := []struct {
		name      string
		pass      int
		fail      int
		wantScore int
		wantTotal int
		wantColor string
		wantMsg   string
	}{
		{"green-100pct", 10, 0, 10, 10, "#4c1", "10/10"},
		{"green-90pct", 9, 1, 9, 10, "#4c1", "9/10"},
		{"yellow-89pct", 89, 11, 89, 100, "#dfb317", "89/100"},
		{"yellow-70pct", 7, 3, 7, 10, "#dfb317", "7/10"},
		{"red-69pct", 69, 31, 69, 100, "#e05d44", "69/100"},
		{"red-0pct", 0, 5, 0, 5, "#e05d44", "0/5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Report{Findings: mixedFindings(tc.pass, tc.fail)}
			var buf bytes.Buffer
			if err := RenderBadgeJSON(&buf, r, RenderOptions{}); err != nil {
				t.Fatalf("RenderBadgeJSON: %v", err)
			}
			var s BadgeSidecar
			if err := json.Unmarshal(buf.Bytes(), &s); err != nil {
				t.Fatalf("decode: %v\nraw: %s", err, buf.String())
			}
			if s.SchemaVersion != 1 {
				t.Errorf("SchemaVersion: got %d, want 1", s.SchemaVersion)
			}
			if s.Label != "agent-first" {
				t.Errorf("Label: got %q, want %q", s.Label, "agent-first")
			}
			if s.Color != tc.wantColor {
				t.Errorf("Color: got %q, want %q", s.Color, tc.wantColor)
			}
			if s.Message != tc.wantMsg {
				t.Errorf("Message: got %q, want %q", s.Message, tc.wantMsg)
			}
			if s.Score != tc.wantScore {
				t.Errorf("Score: got %d, want %d", s.Score, tc.wantScore)
			}
			if s.Total != tc.wantTotal {
				t.Errorf("Total: got %d, want %d", s.Total, tc.wantTotal)
			}
		})
	}
}
