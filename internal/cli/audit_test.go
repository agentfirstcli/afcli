package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// auditReport mirrors the renderer's wire shape just enough for the
// integration tests below — full schema validation lives in
// TestAuditGitJSONValidatesAgainstSchema.
type auditReport struct {
	ManifestVersion string          `json:"manifest_version"`
	AfcliVersion    string          `json:"afcli_version"`
	Target          string          `json:"target"`
	StartedAt       string          `json:"started_at"`
	DurationMs      int64           `json:"duration_ms"`
	Interrupted     bool            `json:"interrupted,omitempty"`
	Findings        []auditFinding  `json:"findings"`
	Error           json.RawMessage `json:"error,omitempty"`
}

type auditFinding struct {
	PrincipleID    string `json:"principle_id"`
	Title          string `json:"title"`
	Category       string `json:"category"`
	Status         string `json:"status"`
	Kind           string `json:"kind"`
	Severity       string `json:"severity"`
	Evidence       string `json:"evidence"`
	Recommendation string `json:"recommendation"`
	Hint           string `json:"hint,omitempty"`
}

func runAudit(t *testing.T, args ...string) (stdout, stderr []byte, exitCode int) {
	t.Helper()
	bin := buildAfcli(t)

	cmd := exec.Command(bin, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	cmd.Env = append(os.Environ(), "AFCLI_DETERMINISTIC=1")

	waitErr := cmd.Run()
	code := 0
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if waitErr != nil {
		t.Fatalf("run %v: %v\nstderr=%s", args, waitErr, errBuf.String())
	}
	return out.Bytes(), errBuf.Bytes(), code
}

func decodeReport(t *testing.T, raw []byte) auditReport {
	t.Helper()
	var r auditReport
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("decode report JSON: %v\nstdout=%s", err, string(raw))
	}
	return r
}

func findingByID(r auditReport, id string) (auditFinding, bool) {
	for _, f := range r.Findings {
		if f.PrincipleID == id {
			return f, true
		}
	}
	return auditFinding{}, false
}

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		t.Skipf("/usr/bin/git not present (%v); skipping git-dependent integration test", err)
	}
}

// TestAuditGitProducesSixteenFindings — DefaultEngine must always emit one
// finding per manifest principle (R008 every-principle-signal). After
// S06, all 16 principles produce real findings: nine carry kind:automated
// (P1/P4/P6/P7/P10/P13/P14/P15/P16 — heuristic checks that read --help
// or --bogus output); seven carry kind:requires-review (P2/P3/P5/P8/P9/
// P11/P12 — review-only checks whose verdict is probe-independent or
// out-of-scope for v1). No finding may carry the stub blurb.
func TestAuditGitProducesSixteenFindings(t *testing.T) {
	skipIfNoGit(t)

	stdout, stderr, code := runAudit(t, "audit", "/usr/bin/git", "--output", "json")
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}
	if r.ManifestVersion != "0.0.1" {
		t.Fatalf("manifest_version: want 0.0.1, got %q", r.ManifestVersion)
	}

	automatedIDs := map[string]bool{
		"P1": true, "P4": true, "P6": true, "P7": true, "P10": true,
		"P13": true, "P14": true, "P15": true, "P16": true,
	}
	reviewOnlyIDs := map[string]bool{
		"P2": true, "P3": true, "P5": true, "P8": true, "P9": true,
		"P11": true, "P12": true,
	}
	autoCount := 0
	reviewCount := 0
	for _, f := range r.Findings {
		if f.PrincipleID == "" || f.Title == "" || f.Category == "" ||
			f.Status == "" || f.Kind == "" || f.Severity == "" {
			t.Errorf("finding %s missing required field: %+v", f.PrincipleID, f)
		}
		if strings.Contains(f.Evidence, "no automated check yet") {
			t.Errorf("finding %s still carries stub blurb: %q", f.PrincipleID, f.Evidence)
		}
		switch f.Kind {
		case "automated":
			autoCount++
			if !automatedIDs[f.PrincipleID] {
				t.Errorf("unexpected automated kind on principle %s", f.PrincipleID)
			}
		case "requires-review":
			reviewCount++
			if !reviewOnlyIDs[f.PrincipleID] {
				t.Errorf("unexpected requires-review kind on principle %s", f.PrincipleID)
			}
		default:
			t.Errorf("unknown kind %q on %s", f.Kind, f.PrincipleID)
		}
	}
	if autoCount != 9 {
		t.Errorf("automated findings: want 9, got %d", autoCount)
	}
	if reviewCount != 7 {
		t.Errorf("requires-review findings: want 7, got %d", reviewCount)
	}
}

// TestAuditGitP7IsPass — git --afcli-bogus-flag exits 129 with
// "unknown option: --afcli-bogus-flag"; the structured-error heuristic in
// checkP7 must classify that as pass.
func TestAuditGitP7IsPass(t *testing.T) {
	skipIfNoGit(t)

	stdout, stderr, code := runAudit(t, "audit", "/usr/bin/git", "--output", "json")
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstderr=%s", code, stderr)
	}

	r := decodeReport(t, stdout)
	p7, ok := findingByID(r, "P7")
	if !ok {
		t.Fatalf("P7 finding missing")
	}
	if p7.Status != "pass" {
		t.Fatalf("P7 status: want pass, got %q (evidence=%q)", p7.Status, p7.Evidence)
	}
}

// TestAuditGitJSONValidatesAgainstSchema — the engine's first real-finding
// payload must validate against testdata/report.schema.json. S01 only
// validated empty-finding reports; this is the wire-format-drift guard for
// the populated case.
func TestAuditGitJSONValidatesAgainstSchema(t *testing.T) {
	skipIfNoGit(t)

	stdout, stderr, code := runAudit(t, "audit", "/usr/bin/git", "--output", "json")
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstderr=%s", code, stderr)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	schemaPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "report.schema.json"))
	schema, err := jsonschema.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	var generic any
	if err := json.Unmarshal(stdout, &generic); err != nil {
		t.Fatalf("decode rendered JSON: %v\nstdout=%s", err, stdout)
	}
	if err := schema.Validate(generic); err != nil {
		t.Fatalf("schema validation failed: %v\nstdout=%s", err, stdout)
	}
}

// TestAuditEchoExitsCleanly — /bin/echo prints --afcli-bogus-flag and
// exits 0 (no flag rejection), which lands P7 as fail. The exit-code-from-
// findings mapping is not yet wired into Execute(), so the process exits 0
// today; the test stays permissive (0 OR 1) so it remains green when S04+
// wires the threshold gate.
func TestAuditEchoExitsCleanly(t *testing.T) {
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--output", "json")
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}
	for _, f := range r.Findings {
		if strings.Contains(f.Evidence, "panicked") {
			t.Fatalf("unexpected panic evidence on %s: %q", f.PrincipleID, f.Evidence)
		}
	}
}

// TestAuditDeterministicByteIdentical — deterministic mode + real findings
// must produce byte-identical stdout across two subprocess invocations.
// New combination first exercised here: S01's deterministic test ran on
// empty findings; S02 covered the manifest surface; S03's evidence strings
// (probe captures, regex matches, stderr lines) widen the determinism
// surface area considerably.
func TestAuditDeterministicByteIdentical(t *testing.T) {
	skipIfNoGit(t)

	out1, _, code1 := runAudit(t, "audit", "/usr/bin/git", "--output", "json", "--deterministic")
	if code1 != 0 {
		t.Fatalf("first run exit code: want 0, got %d", code1)
	}
	out2, _, code2 := runAudit(t, "audit", "/usr/bin/git", "--output", "json", "--deterministic")
	if code2 != 0 {
		t.Fatalf("second run exit code: want 0, got %d", code2)
	}

	if !bytes.Equal(out1, out2) {
		t.Fatalf("byte-identical determinism violated\nrun1=%s\nrun2=%s", out1, out2)
	}
}
