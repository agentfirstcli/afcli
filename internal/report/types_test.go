package report

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReportJSONRoundTrip locks the snake_case wire contract for Report
// and Finding. If a tag changes the documented JSON keys disappear and
// every downstream renderer/golden-file breaks.
func TestReportJSONRoundTrip(t *testing.T) {
	in := Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "1970-01-01T00:00:00Z",
		DurationMs:      0,
		Interrupted:     true,
		Findings: []Finding{
			{
				PrincipleID:    "P6",
				Title:          "Help is exhaustive",
				Category:       "discoverability",
				Status:         StatusFail,
				Kind:           KindAutomated,
				Severity:       SeverityHigh,
				Evidence:       "no --help output",
				Recommendation: "implement --help",
				Hint:           "see manifest",
			},
		},
		Error: &ErrorEnvelope{
			Code:    CodeTargetNotFound,
			Message: "target not found",
			Hint:    "check $PATH",
			Details: map[string]any{"resolved": "/bin/echo"},
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)

	wantKeys := []string{
		`"manifest_version"`, `"afcli_version"`, `"target"`,
		`"started_at"`, `"duration_ms"`, `"interrupted"`,
		`"findings"`, `"error"`,
		`"principle_id"`, `"title"`, `"category"`, `"status"`,
		`"kind"`, `"severity"`, `"evidence"`, `"recommendation"`, `"hint"`,
		`"code"`, `"message"`, `"details"`,
	}
	for _, k := range wantKeys {
		if !strings.Contains(s, k) {
			t.Errorf("missing JSON key %s in: %s", k, s)
		}
	}

	var out Report
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ManifestVersion != in.ManifestVersion ||
		out.AfcliVersion != in.AfcliVersion ||
		out.Target != in.Target ||
		out.StartedAt != in.StartedAt ||
		out.DurationMs != in.DurationMs ||
		out.Interrupted != in.Interrupted {
		t.Errorf("report header round-trip mismatch: in=%+v out=%+v", in, out)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(out.Findings))
	}
	if out.Findings[0] != in.Findings[0] {
		t.Errorf("finding round-trip mismatch: in=%+v out=%+v", in.Findings[0], out.Findings[0])
	}
	if out.Error == nil || out.Error.Code != CodeTargetNotFound ||
		out.Error.Message != in.Error.Message || out.Error.Hint != in.Error.Hint {
		t.Errorf("error envelope round-trip mismatch: %+v", out.Error)
	}
	if v, ok := out.Error.Details["resolved"].(string); !ok || v != "/bin/echo" {
		t.Errorf("error envelope details lost: %+v", out.Error.Details)
	}
}

// TestReportOmitEmpty proves optional fields drop out of the JSON when
// unset — keeps empty-findings reports lean and renderers honest.
func TestReportOmitEmpty(t *testing.T) {
	r := Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "x",
		StartedAt:       "0",
		Findings:        []Finding{},
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, `"interrupted"`) {
		t.Errorf("interrupted should be omitempty when false: %s", s)
	}
	if strings.Contains(s, `"error"`) {
		t.Errorf("error should be omitempty when nil: %s", s)
	}
}

// TestFindingHintOmitEmpty — Hint is optional per schema.
func TestFindingHintOmitEmpty(t *testing.T) {
	f := Finding{
		PrincipleID: "P6", Title: "t", Category: "c",
		Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow,
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"hint"`) {
		t.Errorf("hint should be omitempty when empty: %s", raw)
	}
}

// TestErrorCodeConstants pins the wire-contract code strings. Renaming any
// of these silently breaks every consumer switching on error.code.
func TestErrorCodeConstants(t *testing.T) {
	cases := map[string]string{
		CodeTargetNotFound:      "TARGET_NOT_FOUND",
		CodeTargetNotExecutable: "TARGET_NOT_EXECUTABLE",
		CodeDescriptorInvalid:   "DESCRIPTOR_INVALID",
		CodeDescriptorNotFound:  "DESCRIPTOR_NOT_FOUND",
		CodeProbeTimeout:        "PROBE_TIMEOUT",
		CodeProbeDenied:         "PROBE_DENIED",
		CodeInternal:            "INTERNAL",
		CodeUsage:               "USAGE",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("error code drift: got %q want %q", got, want)
		}
	}
}

// TestEnumValues pins the documented enum strings — these become exit-code
// inputs and are part of the wire contract.
func TestEnumValues(t *testing.T) {
	if string(StatusPass) != "pass" || string(StatusFail) != "fail" ||
		string(StatusSkip) != "skip" || string(StatusReview) != "review" {
		t.Errorf("Status enum drift")
	}
	if string(KindAutomated) != "automated" || string(KindRequiresReview) != "requires-review" {
		t.Errorf("Kind enum drift")
	}
	if string(SeverityLow) != "low" || string(SeverityMedium) != "medium" ||
		string(SeverityHigh) != "high" || string(SeverityCritical) != "critical" {
		t.Errorf("Severity enum drift")
	}
}
