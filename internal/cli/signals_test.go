package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var (
	binPath     string
	binBuildErr error
	binBuildOne sync.Once
)

// buildAfcli compiles the afcli binary once per test process into a
// shared tmp dir. The signal tests need a real subprocess so SIGINT
// delivery is realistic — running cobra in-process would not exercise
// the os/signal handler we are validating.
func buildAfcli(t *testing.T) string {
	t.Helper()
	binBuildOne.Do(func() {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			binBuildErr = errAssertion("could not locate test file via runtime.Caller")
			return
		}
		// signals_test.go lives at <root>/internal/cli/signals_test.go
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
		dir, err := os.MkdirTemp("", "afcli-signal-test-")
		if err != nil {
			binBuildErr = err
			return
		}
		bin := filepath.Join(dir, "afcli")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/afcli")
		cmd.Dir = repoRoot
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			binBuildErr = errAssertion("go build failed: " + err.Error() + "\n" + string(out))
			return
		}
		binPath = bin
	})
	if binBuildErr != nil {
		t.Fatalf("build afcli: %v", binBuildErr)
	}
	return binPath
}

type errAssertion string

func (e errAssertion) Error() string { return string(e) }

// TestSignalInterruptsAuditAndExits130 verifies the SIGINT/SIGTERM
// handler installed in InstallSignalHandler. A subprocess is launched
// with --debug-sleep=2s so the audit pipeline is mid-flight when SIGINT
// arrives 200ms later. The contract checked here is the slice's R012
// commitment: a partial JSON report on stdout with interrupted: true,
// exit code 130, no Go runtime panic on stderr.
func TestSignalInterruptsAuditAndExits130(t *testing.T) {
	bin := buildAfcli(t)

	cmd := exec.Command(bin, "audit", "/bin/echo", "--output", "json", "--debug-sleep=2s")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "AFCLI_DETERMINISTIC=1")

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal: %v", err)
	}

	waitErr := cmd.Wait()

	exitCode := -1
	if waitErr == nil {
		exitCode = 0
	} else if exitErr, ok := waitErr.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else {
		t.Fatalf("wait: unexpected error type %T: %v", waitErr, waitErr)
	}

	if exitCode != 130 {
		t.Fatalf("exit code: want 130, got %d\nstdout=%s\nstderr=%s", exitCode, stdout.String(), stderr.String())
	}

	if strings.Contains(stderr.String(), "panic") {
		t.Fatalf("stderr contains panic:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "goroutine ") {
		t.Fatalf("stderr contains goroutine dump:\n%s", stderr.String())
	}

	var report struct {
		ManifestVersion string        `json:"manifest_version"`
		AfcliVersion    string        `json:"afcli_version"`
		Target          string        `json:"target"`
		StartedAt       string        `json:"started_at"`
		DurationMs      int64         `json:"duration_ms"`
		Interrupted     bool          `json:"interrupted"`
		Findings        []interface{} `json:"findings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode stdout JSON: %v\nstdout=%s", err, stdout.String())
	}
	if !report.Interrupted {
		t.Fatalf("interrupted flag: want true, got false\nstdout=%s", stdout.String())
	}
	if report.AfcliVersion == "" {
		t.Fatalf("afcli_version missing\nstdout=%s", stdout.String())
	}
	if report.ManifestVersion == "" {
		t.Fatalf("manifest_version missing\nstdout=%s", stdout.String())
	}
	if report.Findings == nil {
		t.Fatalf("findings missing (must be [], not null)\nstdout=%s", stdout.String())
	}

	// Schema validation — the partial report must be a fully valid
	// document, not a half-formed shape that downstream consumers can't
	// parse. This is the slice's "schema-valid partial report" contract.
	_, thisFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "report.schema.json"))
	schema, err := jsonschema.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	var generic any
	if err := json.Unmarshal(stdout.Bytes(), &generic); err != nil {
		t.Fatalf("schema decode: %v", err)
	}
	if err := schema.Validate(generic); err != nil {
		t.Fatalf("schema validation failed: %v\nstdout=%s", err, stdout.String())
	}
}

// TestSignalSIGTERMAlsoExits130 verifies SIGTERM is wired to the same
// finalization path as SIGINT. The handler's signal.Notify list includes
// both — this guards against a regression where one is dropped.
func TestSignalSIGTERMAlsoExits130(t *testing.T) {
	bin := buildAfcli(t)

	cmd := exec.Command(bin, "audit", "/bin/echo", "--output", "json", "--debug-sleep=2s")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "AFCLI_DETERMINISTIC=1")

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	waitErr := cmd.Wait()
	exitCode := -1
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if waitErr != nil {
		t.Fatalf("wait: %v", waitErr)
	}
	if exitCode != 130 {
		t.Fatalf("exit code: want 130, got %d\nstderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"interrupted\": true") {
		t.Fatalf("stdout missing interrupted:true\n%s", stdout.String())
	}
}

// TestSignalCleanRunStillExitsZero guards against the signal handler
// inadvertently cancelling fast clean runs. With --debug-sleep unset
// (default 0), the audit completes immediately and must exit 0.
// --fail-on=never neutralises the threshold gate so the test isolates
// the signal-path contract from any fail-finding the engine emits.
func TestSignalCleanRunStillExitsZero(t *testing.T) {
	bin := buildAfcli(t)

	cmd := exec.Command(bin, "audit", "/bin/echo", "--output", "json", "--fail-on", "never")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "AFCLI_DETERMINISTIC=1")

	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"manifest_version\"") {
		t.Fatalf("stdout missing manifest_version: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "\"interrupted\":") {
		t.Fatalf("clean run should not emit interrupted flag: %s", stdout.String())
	}
}
