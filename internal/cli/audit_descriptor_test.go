package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// fixtureDescriptorPath returns the absolute path to a checked-in
// descriptor fixture under testdata/descriptors/. Locates the repo root
// via runtime.Caller — same trick buildAfcli uses — so the tests run from
// any working directory.
func fixtureDescriptorPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	p := filepath.Join(repoRoot, "testdata", "descriptors", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture missing: %s (%v)", p, err)
	}
	return p
}

// reportSchemaPath returns the absolute path to testdata/report.schema.json.
func reportSchemaPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "report.schema.json"))
}

// validateAgainstReportSchema asserts raw is a Report-shape JSON document
// that satisfies testdata/report.schema.json. Used on every envelope-on-
// stderr emission to re-prove the envelope-as-Report contract from S01.
func validateAgainstReportSchema(t *testing.T, raw []byte) {
	t.Helper()
	schema, err := jsonschema.Compile(reportSchemaPath(t))
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("decode for schema validation: %v\nraw=%s", err, raw)
	}
	if err := schema.Validate(generic); err != nil {
		t.Fatalf("schema validation failed: %v\nraw=%s", err, raw)
	}
}

// errorEnvelope is the on-the-wire shape of report.error after the
// envelope is rendered through renderEnvelope. Mirrors the schema's
// errorEnvelope $def — keep in sync if the schema grows fields.
type errorEnvelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Hint    string         `json:"hint,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// decodeStderrEnvelope parses an envelope-on-stderr Report and returns
// the inner errorEnvelope. Fails the test if the JSON is unparseable or
// the .error field is missing.
func decodeStderrEnvelope(t *testing.T, stderr []byte) errorEnvelope {
	t.Helper()
	if len(stderr) == 0 {
		t.Fatalf("stderr empty — expected envelope-bearing report")
	}
	var rep auditReport
	if err := json.Unmarshal(stderr, &rep); err != nil {
		t.Fatalf("stderr is not parseable JSON: %v\nstderr=%s", err, stderr)
	}
	if len(rep.Error) == 0 {
		t.Fatalf("report.error missing from stderr\nstderr=%s", stderr)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rep.Error, &env); err != nil {
		t.Fatalf("error envelope unparseable: %v\nraw=%s", err, rep.Error)
	}
	return env
}

// pickAuditTarget returns a target binary that exists on the test host.
// /usr/bin/git is preferred for stability with the S03 check registry; if
// absent (CI macOS, minimal containers), /bin/echo is used as a fallback
// that still exercises every check path.
func pickAuditTarget(t *testing.T) string {
	t.Helper()
	for _, cand := range []string{"/usr/bin/git", "/bin/echo"} {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	t.Skip("neither /usr/bin/git nor /bin/echo available on this host")
	return ""
}

// TestAuditDescriptorAppliesSkip — the synthetic skip finding contract.
// With skip_principles: [P12] in the descriptor, P12 must surface as
// status:skip / kind:requires-review / evidence:"skipped per descriptor"
// without invoking checkP12 (which is a stub today, but the contract
// still applies once a real check lands).
func TestAuditDescriptorAppliesSkip(t *testing.T) {
	target := pickAuditTarget(t)
	desc := fixtureDescriptorPath(t, "valid-skip-relax.yaml")

	stdout, stderr, code := runAudit(t, "audit", target, "--descriptor", desc, "--output", "json")
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	r := decodeReport(t, stdout)
	p12, ok := findingByID(r, "P12")
	if !ok {
		t.Fatalf("P12 finding missing\nstdout=%s", stdout)
	}
	if p12.Status != "skip" {
		t.Fatalf("P12.status: want skip, got %q", p12.Status)
	}
	if p12.Kind != "requires-review" {
		t.Fatalf("P12.kind: want requires-review, got %q", p12.Kind)
	}
	if p12.Evidence != "skipped per descriptor" {
		t.Fatalf("P12.evidence: want %q, got %q", "skipped per descriptor", p12.Evidence)
	}
}

// TestAuditDescriptorAppliesRelax — relax_principles caps severity. P7's
// manifest-default is high; with P7: medium the rendered finding must be
// medium or low, never high or critical. The cap is a ceiling, not a
// rewrite — we accept either of the two below-cap values so the test
// stays green regardless of the underlying check verdict.
func TestAuditDescriptorAppliesRelax(t *testing.T) {
	target := pickAuditTarget(t)
	desc := fixtureDescriptorPath(t, "valid-skip-relax.yaml")

	stdout, stderr, code := runAudit(t, "audit", target, "--descriptor", desc, "--output", "json")
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	r := decodeReport(t, stdout)
	p7, ok := findingByID(r, "P7")
	if !ok {
		t.Fatalf("P7 finding missing\nstdout=%s", stdout)
	}
	switch p7.Severity {
	case "medium", "low":
		// ok — relaxed at or below the cap.
	default:
		t.Fatalf("P7.severity: want medium or low (capped from high), got %q", p7.Severity)
	}
}

// TestAuditDescriptorNotFound — a missing descriptor short-circuits to
// DESCRIPTOR_NOT_FOUND with details.path echoing the user-supplied path
// (T03's classifyDescriptorError contract; reasserted at the binary
// boundary on the full envelope-on-stderr path).
func TestAuditDescriptorNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "afcli-desc-missing-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	missing := filepath.Join(dir, "definitely-missing.yaml")

	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--descriptor", missing, "--output", "json")
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	env := decodeStderrEnvelope(t, stderr)
	if env.Code != "DESCRIPTOR_NOT_FOUND" {
		t.Fatalf("error.code: want DESCRIPTOR_NOT_FOUND, got %q", env.Code)
	}
	if got, _ := env.Details["path"].(string); got != missing {
		t.Fatalf("error.details.path: want %q, got %q", missing, got)
	}
	validateAgainstReportSchema(t, stderr)
}

// TestAuditDescriptorInvalidUnknownKey — KnownFields(true) rejects the
// top-level `foo` key. The envelope must carry a numeric details.line so
// an agent can localize the offending line without grepping the file.
func TestAuditDescriptorInvalidUnknownKey(t *testing.T) {
	desc := fixtureDescriptorPath(t, "unknown-key.yaml")
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--descriptor", desc, "--output", "json")
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	env := decodeStderrEnvelope(t, stderr)
	if env.Code != "DESCRIPTOR_INVALID" {
		t.Fatalf("error.code: want DESCRIPTOR_INVALID, got %q", env.Code)
	}
	if _, ok := env.Details["line"].(float64); !ok {
		t.Fatalf("error.details.line: want number, got %v (type %T)", env.Details["line"], env.Details["line"])
	}
	validateAgainstReportSchema(t, stderr)
}

// TestAuditDescriptorTypeMismatch — `commands.safe: "git status"` is a
// scalar where a sequence is expected; yaml.v3 fires TypeError. We assert
// envelope code, line presence, and that one of message/hint mentions
// the type-mismatch nature so an agent gets actionable diagnostic text.
func TestAuditDescriptorTypeMismatch(t *testing.T) {
	desc := fixtureDescriptorPath(t, "type-mismatch.yaml")
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--descriptor", desc, "--output", "json")
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	env := decodeStderrEnvelope(t, stderr)
	if env.Code != "DESCRIPTOR_INVALID" {
		t.Fatalf("error.code: want DESCRIPTOR_INVALID, got %q", env.Code)
	}
	if _, ok := env.Details["line"].(float64); !ok {
		t.Fatalf("error.details.line: want number, got %v (type %T)", env.Details["line"], env.Details["line"])
	}
	combined := env.Message + " " + env.Hint
	mentionsTypeIssue := false
	for _, needle := range []string{"sequence", "scalar", "expected"} {
		if strings.Contains(combined, needle) {
			mentionsTypeIssue = true
			break
		}
	}
	if !mentionsTypeIssue {
		t.Fatalf("expected message or hint to mention sequence/scalar/expected\nmessage=%q\nhint=%q", env.Message, env.Hint)
	}
	validateAgainstReportSchema(t, stderr)
}

// TestAuditDescriptorBadPrinciple — skip_principles=[P99] passes YAML
// parsing but fails Validate's manifest cross-check. The envelope's
// details.key must localize the offending entry as "skip_principles[N]".
func TestAuditDescriptorBadPrinciple(t *testing.T) {
	desc := fixtureDescriptorPath(t, "bad-principle.yaml")
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--descriptor", desc, "--output", "json")
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	env := decodeStderrEnvelope(t, stderr)
	if env.Code != "DESCRIPTOR_INVALID" {
		t.Fatalf("error.code: want DESCRIPTOR_INVALID, got %q", env.Code)
	}
	key, _ := env.Details["key"].(string)
	if !strings.HasPrefix(key, "skip_principles") {
		t.Fatalf("error.details.key: want prefix skip_principles, got %q", key)
	}
	validateAgainstReportSchema(t, stderr)
}

// TestAuditDescriptorBadSeverity — relax_principles.P7=nuclear fails the
// closed-set severity check. The envelope must carry details.allowed as
// a list including "medium" so an agent knows the valid alternatives.
func TestAuditDescriptorBadSeverity(t *testing.T) {
	desc := fixtureDescriptorPath(t, "bad-severity.yaml")
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--descriptor", desc, "--output", "json")
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	env := decodeStderrEnvelope(t, stderr)
	if env.Code != "DESCRIPTOR_INVALID" {
		t.Fatalf("error.code: want DESCRIPTOR_INVALID, got %q", env.Code)
	}
	allowedRaw, ok := env.Details["allowed"].([]any)
	if !ok {
		t.Fatalf("error.details.allowed: want []any, got %v (type %T)", env.Details["allowed"], env.Details["allowed"])
	}
	hasMedium := false
	for _, v := range allowedRaw {
		if s, _ := v.(string); s == "medium" {
			hasMedium = true
			break
		}
	}
	if !hasMedium {
		t.Fatalf("error.details.allowed: want list including \"medium\", got %v", allowedRaw)
	}
	validateAgainstReportSchema(t, stderr)
}

// TestAuditDescriptorFormatMismatch — format_version "9" is not the
// supported "1". The envelope's details.key must equal "format_version"
// so an agent's structured handler can branch on it directly.
func TestAuditDescriptorFormatMismatch(t *testing.T) {
	desc := fixtureDescriptorPath(t, "format-mismatch.yaml")
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--descriptor", desc, "--output", "json")
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	env := decodeStderrEnvelope(t, stderr)
	if env.Code != "DESCRIPTOR_INVALID" {
		t.Fatalf("error.code: want DESCRIPTOR_INVALID, got %q", env.Code)
	}
	if got, _ := env.Details["key"].(string); got != "format_version" {
		t.Fatalf("error.details.key: want %q, got %q", "format_version", got)
	}
	validateAgainstReportSchema(t, stderr)
}
