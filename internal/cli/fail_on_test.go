package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestFailOnDefaultIsHigh — without --fail-on the threshold defaults to
// "high"; /bin/echo's P7 finding lands as fail at high severity, so the
// process must exit 1 once Execute() routes through MapFromReport.
func TestFailOnDefaultIsHigh(t *testing.T) {
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--output", "json")
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}
	hasHighFail := false
	for _, f := range r.Findings {
		if f.Status == "fail" && f.Severity == "high" {
			hasHighFail = true
			break
		}
	}
	if hasHighFail && code != 1 {
		t.Fatalf("default --fail-on=high with a fail/high finding must exit 1; got %d", code)
	}
	if !hasHighFail && code != 0 {
		t.Fatalf("no fail/high finding present, exit must be 0; got %d", code)
	}
}

// TestFailOnAcceptsAllFiveValues — every documented --fail-on value
// parses cleanly and produces a deterministic exit code.
func TestFailOnAcceptsAllFiveValues(t *testing.T) {
	cases := []struct {
		value string
	}{
		{"low"}, {"medium"}, {"high"}, {"critical"}, {"never"},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--output", "json", "--fail-on", tc.value)
			// Bogus parsing would surface USAGE on stderr at exit 2 — so
			// any of {0,1} is valid; what matters is no parse rejection.
			if code != 0 && code != 1 {
				t.Fatalf("--fail-on=%s exit code: want 0 or 1, got %d\nstderr=%s", tc.value, code, stderr)
			}
			if len(stdout) == 0 {
				t.Fatalf("--fail-on=%s produced empty stdout (parser rejected silently?)", tc.value)
			}
		})
	}
}

// TestFailOnRejectsBogusValue — unknown --fail-on values short-circuit to
// a USAGE envelope on stderr at exit 2; stdout stays empty.
func TestFailOnRejectsBogusValue(t *testing.T) {
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--output", "json", "--fail-on", "nuclear")
	if code != 2 {
		t.Fatalf("exit code: want 2, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if len(stdout) != 0 {
		t.Fatalf("stdout must be empty on USAGE rejection; got %s", stdout)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Hint    string `json:"hint"`
			Message string `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &env); err != nil {
		t.Fatalf("decode envelope: %v\nstderr=%s", err, stderr)
	}
	if env.Error.Code != "USAGE" {
		t.Fatalf("error.code: want USAGE, got %q", env.Error.Code)
	}
	if !strings.Contains(env.Error.Hint, "low") || !strings.Contains(env.Error.Hint, "never") {
		t.Fatalf("hint missing allowed-set listing: %q", env.Error.Hint)
	}
}

// TestFailOnNeverSkipsThresholdGate — `--fail-on never` must return 0
// even when the engine produces fail findings at any severity. /bin/echo
// reliably trips P7=fail/high, which under default --fail-on=high would
// exit 1; under --fail-on=never the never-bool short-circuit in Execute()
// wins and the process exits 0.
func TestFailOnNeverSkipsThresholdGate(t *testing.T) {
	stdout, stderr, code := runAudit(t, "audit", "/bin/echo", "--output", "json", "--fail-on", "never")
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

// TestFailOnInterruptedStillExits130 — the Interrupted short-circuit in
// MapFromReport (and the errInterrupted sentinel in Execute()) must win
// over any --fail-on threshold. SIGINT mid-audit with --fail-on=critical
// still exits 130, not 1 or 0.
func TestFailOnInterruptedStillExits130(t *testing.T) {
	bin := buildAfcli(t)

	cmd := exec.Command(bin, "audit", "/bin/echo", "--output", "json", "--debug-sleep=2s", "--fail-on", "critical")
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
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if waitErr != nil {
		t.Fatalf("wait: %v", waitErr)
	}
	if exitCode != 130 {
		t.Fatalf("exit code: want 130, got %d\nstderr=%s", exitCode, stderr.String())
	}
}

// TestHelpSchemaListsFailOnFlag — the reflection-driven help schema must
// surface the new --fail-on flag automatically; agents discover it
// without parsing human help.
func TestHelpSchemaListsFailOnFlag(t *testing.T) {
	stdout, stderr, code := runAudit(t, "audit", "--help-schema", "--output", "json")
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstderr=%s", code, stderr)
	}
	var schema struct {
		Command struct {
			Flags []struct {
				Name string `json:"name"`
			} `json:"flags"`
		} `json:"command"`
	}
	if err := json.Unmarshal(stdout, &schema); err != nil {
		t.Fatalf("decode help-schema: %v\nstdout=%s", err, stdout)
	}
	for _, f := range schema.Command.Flags {
		if f.Name == "fail-on" {
			return
		}
	}
	t.Fatalf("--fail-on flag missing from help-schema; flags=%+v", schema.Command.Flags)
}
