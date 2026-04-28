package cli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAuditDescriptorMissingFile is the lightweight T03-level smoke
// test for the new --descriptor flag wiring. Heavier integration cases
// (skip/relax application, malformed yaml, unknown keys) land in T04.
//
// Contract verified here:
//   - exit code is 3 (CouldNotAudit)
//   - stderr is a single ErrorEnvelope-bearing report
//   - error.code == DESCRIPTOR_NOT_FOUND
//   - error.details.path echoes the user-supplied path
func TestAuditDescriptorMissingFile(t *testing.T) {
	t.Parallel()

	const missing = "/definitely/missing.yaml"
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--descriptor", missing, "--output", "json")

	if code != 3 {
		t.Fatalf("expected exit 3 (CouldNotAudit), got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if len(stderr) == 0 {
		t.Fatalf("expected envelope on stderr, got empty\nstdout=%s", stdout)
	}

	var rep auditReport
	if err := json.Unmarshal(stderr, &rep); err != nil {
		t.Fatalf("stderr is not parseable JSON report: %v\nstderr=%s", err, stderr)
	}
	if len(rep.Error) == 0 {
		t.Fatalf("expected report.error to be populated\nstderr=%s", stderr)
	}

	var env struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Hint    string         `json:"hint,omitempty"`
		Details map[string]any `json:"details,omitempty"`
	}
	if err := json.Unmarshal(rep.Error, &env); err != nil {
		t.Fatalf("error envelope unparseable: %v\nraw=%s", err, rep.Error)
	}
	if env.Code != "DESCRIPTOR_NOT_FOUND" {
		t.Fatalf("expected error.code=DESCRIPTOR_NOT_FOUND, got %q\nstderr=%s", env.Code, stderr)
	}
	if got, _ := env.Details["path"].(string); got != missing {
		t.Fatalf("expected details.path=%q, got %q\ndetails=%v", missing, got, env.Details)
	}
	if env.Hint == "" {
		t.Fatalf("expected non-empty hint on DESCRIPTOR_NOT_FOUND envelope\nenvelope=%+v", env)
	}
	if !strings.Contains(env.Message, missing) {
		// not a hard requirement — message is allowed to elide the path
		// — but log it so a future change here is visible in test output
		t.Logf("note: error.message does not contain the missing path: %q", env.Message)
	}
}
