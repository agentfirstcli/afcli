package report

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// schemaPath is the project-relative path to the canonical report schema.
// The test package lives at internal/report, so the schema sits two levels up.
const schemaPath = "../../testdata/report.schema.json"

var (
	compiledSchema     *jsonschema.Schema
	compiledSchemaErr  error
	compiledSchemaOnce sync.Once
)

func loadSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiledSchemaOnce.Do(func() {
		abs, err := filepath.Abs(schemaPath)
		if err != nil {
			compiledSchemaErr = err
			return
		}
		compiledSchema, compiledSchemaErr = jsonschema.Compile(abs)
	})
	if compiledSchemaErr != nil {
		t.Fatalf("compile schema: %v", compiledSchemaErr)
	}
	return compiledSchema
}

// validateJSON renders r through RenderJSON, decodes the bytes into the
// generic value jsonschema expects, and validates against the canonical
// schema. Returns nil on schema-valid output, the schema error otherwise.
func validateJSON(t *testing.T, r *Report, opts RenderOptions) error {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderJSON(&buf, r, opts); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var v any
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v\n%s", err, buf.String())
	}
	return loadSchema(t).Validate(v)
}

// TestSchemaEmptyReport — zero-finding report renders schema-valid output.
// This is the demo path for slice S01 (afcli audit /bin/echo --output json).
func TestSchemaEmptyReport(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      0,
	}
	if err := validateJSON(t, r, RenderOptions{}); err != nil {
		t.Errorf("empty report failed schema: %v", err)
	}
}

// TestSchemaMultiFindingReport — every Status, Kind, and Severity enum
// value appears in at least one finding so a tightening of any enum gets
// caught here.
func TestSchemaMultiFindingReport(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      123,
		Findings: []Finding{
			{
				PrincipleID: "P6", Title: "Help is exhaustive", Category: "discoverability",
				Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow,
				Evidence: "found --help", Recommendation: "",
			},
			{
				PrincipleID: "P7", Title: "Errors are actionable", Category: "errors",
				Status: StatusFail, Kind: KindAutomated, Severity: SeverityMedium,
				Evidence: "exit 1 with no message", Recommendation: "print remediation",
				Hint: "see manifest",
			},
			{
				PrincipleID: "P14", Title: "Output is structured", Category: "interop",
				Status: StatusReview, Kind: KindRequiresReview, Severity: SeverityHigh,
				Evidence: "output is freeform text", Recommendation: "consider --output json",
			},
			{
				PrincipleID: "P15", Title: "Side effects are reversible", Category: "safety",
				Status: StatusSkip, Kind: KindAutomated, Severity: SeverityCritical,
				Evidence: "skipped: probe disabled", Recommendation: "",
			},
		},
	}
	if err := validateJSON(t, r, RenderOptions{}); err != nil {
		t.Errorf("multi-finding report failed schema: %v", err)
	}
}

// TestSchemaErrorOnlyReport — an error envelope with `details` populated
// is the documented TARGET_NOT_FOUND path; verify it validates.
func TestSchemaErrorOnlyReport(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/nonexistent",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      0,
		Error: &ErrorEnvelope{
			Code:    CodeTargetNotFound,
			Message: "target not found",
			Hint:    "check $PATH",
			Details: map[string]any{"resolved": "/nonexistent", "errno": 2},
		},
	}
	if err := validateJSON(t, r, RenderOptions{}); err != nil {
		t.Errorf("error-only report failed schema: %v", err)
	}
}

// TestSchemaPartialInterruptedReport — SIGINT path: interrupted=true plus
// any partial findings collected before the signal arrived.
func TestSchemaPartialInterruptedReport(t *testing.T) {
	r := &Report{
		ManifestVersion: "v1",
		AfcliVersion:    "0.0.0",
		Target:          "/bin/echo",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      999,
		Interrupted:     true,
		Findings: []Finding{
			{
				PrincipleID: "P6", Title: "Help is exhaustive", Category: "discoverability",
				Status: StatusPass, Kind: KindAutomated, Severity: SeverityLow,
				Evidence: "found --help", Recommendation: "",
			},
		},
	}
	if err := validateJSON(t, r, RenderOptions{}); err != nil {
		t.Errorf("partial/interrupted report failed schema: %v", err)
	}
}

// TestSchemaRejectsInvalidStatus — negative test. A bogus status string
// must be rejected by the schema; otherwise the enum constraint is dead.
func TestSchemaRejectsInvalidStatus(t *testing.T) {
	// Build the JSON object directly so we can inject an invalid enum
	// value that the typed Report struct would otherwise allow.
	bad := map[string]any{
		"manifest_version": "v1",
		"afcli_version":    "0.0.0",
		"target":           "/bin/echo",
		"started_at":       "2026-01-01T00:00:00Z",
		"duration_ms":      0,
		"findings": []map[string]any{
			{
				"principle_id":   "P6",
				"title":          "x",
				"category":       "c",
				"status":         "bogus",
				"kind":           "automated",
				"severity":       "low",
				"evidence":       "e",
				"recommendation": "r",
			},
		},
	}
	if err := loadSchema(t).Validate(bad); err == nil {
		t.Errorf("schema should have rejected status=bogus")
	}
}

// TestSchemaRejectsInvalidPrincipleID — principle_id must match ^P\d+$.
// Catches a class of typos (lowercase, wrong prefix, missing digits) that
// would silently break correlation with manifest entries.
func TestSchemaRejectsInvalidPrincipleID(t *testing.T) {
	cases := []string{"p6", "PX", "P", "6", "P6a"}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			bad := map[string]any{
				"manifest_version": "v1",
				"afcli_version":    "0.0.0",
				"target":           "/bin/echo",
				"started_at":       "0",
				"duration_ms":      0,
				"findings": []map[string]any{
					{
						"principle_id":   id,
						"title":          "x",
						"category":       "c",
						"status":         "pass",
						"kind":           "automated",
						"severity":       "low",
						"evidence":       "e",
						"recommendation": "r",
					},
				},
			}
			if err := loadSchema(t).Validate(bad); err == nil {
				t.Errorf("schema should have rejected principle_id=%q", id)
			}
		})
	}
}

// TestSchemaRejectsMissingRequired — drop a required header field and
// confirm the schema flags it. Locks the required-field set.
func TestSchemaRejectsMissingRequired(t *testing.T) {
	bad := map[string]any{
		// missing manifest_version
		"afcli_version": "0.0.0",
		"target":        "/bin/echo",
		"started_at":    "0",
		"duration_ms":   0,
		"findings":      []any{},
	}
	if err := loadSchema(t).Validate(bad); err == nil {
		t.Errorf("schema should have rejected report missing manifest_version")
	}
}

// TestSchemaRejectsUnknownErrorCode — error envelope codes are pinned by
// enum, so a typo in CodeXXX constants would surface here.
func TestSchemaRejectsUnknownErrorCode(t *testing.T) {
	bad := map[string]any{
		"manifest_version": "v1",
		"afcli_version":    "0.0.0",
		"target":           "x",
		"started_at":       "0",
		"duration_ms":      0,
		"findings":         []any{},
		"error": map[string]any{
			"code":    "MADE_UP_CODE",
			"message": "nope",
		},
	}
	if err := loadSchema(t).Validate(bad); err == nil {
		t.Errorf("schema should have rejected unknown error code")
	}
}
