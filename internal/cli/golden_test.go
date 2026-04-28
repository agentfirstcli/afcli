package cli_test

// Golden file refresh procedure:
//
//  1. Verify the diff is intentional (e.g. a deliberate bump of the embedded
//     manifest, AfcliVersion, or a check's evidence template). Drift from any
//     other cause is a bug — root-cause it before refreshing.
//  2. Run: go test ./internal/cli/... -run TestGoldenSelfAudit -update
//  3. Include the regenerated testdata/golden-self-audit.json in the same PR
//     as the change that caused it.
//
// The test gates -update on len(findings) == 16 and zero fail findings, so it
// cannot silently bake a regression — only refresh the byte form of an
// already-clean self-audit.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestGoldenSelfAudit pins the deterministic JSON output of
// `afcli audit ./afcli --deterministic --output json` against
// testdata/golden-self-audit.json. This is the wire-level canary for
// unintended drift in any of the contracts S01–S06 froze (sort order,
// timestamp/duration zeroing, relativized paths, sorted Details map keys,
// panic evidence shape).
func TestGoldenSelfAudit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("golden file is anchored to Unix paths; matches the GitHub Actions matrix (Linux + macOS)")
	}

	bin := buildAfcli(t)
	root := repoRoot(t)

	// Build the production binary into <repoRoot>/afcli so the audit target
	// `./afcli` resolves there and --deterministic relativizes consistently.
	prodBin := filepath.Join(root, "afcli")
	build := exec.Command("go", "build", "-o", prodBin, "./cmd/afcli")
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build production binary: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = os.Remove(prodBin) })

	cmd := exec.Command(bin, "audit", "./afcli", "--deterministic", "--output", "json")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "AFCLI_DETERMINISTIC=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Exit code 1 is allowed (findings at threshold); any other error is a real failure.
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("self-audit failed: %v\nstderr=%s", err, stderr.String())
		}
	}

	produced := stdout.Bytes()

	// Sanity gates — must hold for every clean self-audit. These guards
	// prevent -update from silently baking a regression.
	var report struct {
		Findings []struct {
			Status string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(produced, &report); err != nil {
		t.Fatalf("decode produced JSON: %v\nstdout=%s", err, stdout.String())
	}
	if got := len(report.Findings); got != 16 {
		t.Fatalf("self-audit produced %d findings, want 16 — refusing to write golden", got)
	}
	for i, f := range report.Findings {
		if f.Status == "fail" {
			t.Fatalf("self-audit findings[%d].status == \"fail\" — refusing to write golden (R013 demands zero fail at default threshold)", i)
		}
	}

	goldenPath := filepath.Join(root, "testdata", "golden-self-audit.json")

	if *update {
		if err := os.WriteFile(goldenPath, produced, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create)", err)
	}

	if bytes.Equal(produced, want) {
		return
	}

	// Find the first diverging line and report it cleanly.
	prodLines := bytes.Split(produced, []byte("\n"))
	wantLines := bytes.Split(want, []byte("\n"))
	max := len(prodLines)
	if len(wantLines) > max {
		max = len(wantLines)
	}
	for i := 0; i < max; i++ {
		var p, w []byte
		if i < len(prodLines) {
			p = prodLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if !bytes.Equal(p, w) {
			t.Fatalf("golden mismatch at line %d:\n  golden: %s\n  actual: %s\n\nIf this drift is intentional, refresh with:\n  go test ./internal/cli/... -run TestGoldenSelfAudit -update",
				i+1, w, p)
		}
	}
	t.Fatalf("golden mismatch (different length: produced=%d want=%d bytes). Refresh with: go test ./internal/cli/... -run TestGoldenSelfAudit -update",
		len(produced), len(want))
}
