package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readRecord reads the argv-recorder log file. Returns "" if the file
// does not exist (no probe ever ran with ARGV_RECORD_FILE set).
func readRecord(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read record file: %v", err)
	}
	return string(b)
}

// runAuditWithEnv mirrors runAudit but threads extraEnv into the
// subprocess. Required because the S05 probe tests need ARGV_RECORD_FILE
// in the parent env so the probe allowlist forwards it into the
// fixture's environment.
func runAuditWithEnv(t *testing.T, extraEnv map[string]string, args ...string) (stdout, stderr []byte, exitCode int) {
	t.Helper()
	bin := buildAfcli(t)
	cmd := exec.Command(bin, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	cmd.Env = append(os.Environ(), "AFCLI_DETERMINISTIC=1")
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	waitErr := cmd.Run()
	code := 0
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if waitErr != nil {
		t.Fatalf("run %v: %v\nstderr=%s", args, waitErr, errBuf.String())
	}
	return out.Bytes(), errBuf.Bytes(), code
}

// TestProbeOnlyInvokesSafeArgv — with --probe and probe-authorizing.yaml
// in play, the recorder fixture must observe --help, --afcli-bogus-flag,
// and --version (the descriptor's safe[] entry); it must NOT observe
// --burn-the-disk (the destructive entry); descriptor.Env must reach the
// behavioral probe (ENV:AFCLI_TEST_PROBE=1 marker).
func TestProbeOnlyInvokesSafeArgv(t *testing.T) {
	bin := buildArgvRecorder(t)
	desc := fixtureDescriptorPath(t, "probe-authorizing.yaml")
	logPath := filepath.Join(t.TempDir(), "argv.log")

	stdout, stderr, code := runAuditWithEnv(t,
		map[string]string{"ARGV_RECORD_FILE": logPath},
		"audit", bin,
		"--probe",
		"--descriptor", desc,
		"--output", "json",
	)
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstderr=%s", code, stderr)
	}
	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}

	got := readRecord(t, logPath)
	for _, want := range []string{"--help\n", "--afcli-bogus-flag\n", "--version\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("record file missing %q\ncontents:\n%s", want, got)
		}
	}
	if strings.Contains(got, "--burn-the-disk") {
		t.Fatalf("record file MUST NOT contain --burn-the-disk\ncontents:\n%s", got)
	}
	if !strings.Contains(got, "ENV:AFCLI_TEST_PROBE=1") {
		t.Errorf("record file missing descriptor env marker ENV:AFCLI_TEST_PROBE=1\ncontents:\n%s", got)
	}
}

// TestProbeOffKeepsDefaultBehavior — without --probe, the same descriptor
// is loaded but no behavioral probe runs. Recorder sees only --help and
// --afcli-bogus-flag; --version (the safe[] entry) is never invoked.
func TestProbeOffKeepsDefaultBehavior(t *testing.T) {
	bin := buildArgvRecorder(t)
	desc := fixtureDescriptorPath(t, "probe-authorizing.yaml")
	logPath := filepath.Join(t.TempDir(), "argv.log")

	stdout, stderr, code := runAuditWithEnv(t,
		map[string]string{"ARGV_RECORD_FILE": logPath},
		"audit", bin,
		"--descriptor", desc,
		"--output", "json",
	)
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstderr=%s", code, stderr)
	}
	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}

	got := readRecord(t, logPath)
	if !strings.Contains(got, "--help\n") {
		t.Errorf("record file missing --help line\ncontents:\n%s", got)
	}
	if !strings.Contains(got, "--afcli-bogus-flag\n") {
		t.Errorf("record file missing --afcli-bogus-flag line\ncontents:\n%s", got)
	}
	if strings.Contains(got, "--version") {
		t.Fatalf("record file MUST NOT contain --version when --probe is off\ncontents:\n%s", got)
	}
}

// TestProbeTimeoutFiresPerProbe — probe-hanging.yaml's commands.safe is
// [--hang], which blocks the recorder forever. With --probe-timeout=200ms
// the per-probe deadline fires; the audit completes (exit 0 or 1, never
// 3 or 4), wall time stays well under 2s, len(findings)==16, and P3 is
// decorated with status=review + "timeout" in evidence.
func TestProbeTimeoutFiresPerProbe(t *testing.T) {
	bin := buildArgvRecorder(t)
	desc := fixtureDescriptorPath(t, "probe-hanging.yaml")

	start := time.Now()
	stdout, stderr, code := runAudit(t, "audit", bin,
		"--probe",
		"--probe-timeout=200ms",
		"--descriptor", desc,
		"--output", "json",
	)
	elapsed := time.Since(start)

	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1 (NOT 3 or 4), got %d\nstderr=%s", code, stderr)
	}
	if elapsed > 2*time.Second {
		t.Errorf("wall time too long: %v (want <2s)", elapsed)
	}

	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}
	p3, ok := findingByID(r, "P3")
	if !ok {
		t.Fatalf("P3 finding missing\nstdout=%s", stdout)
	}
	if p3.Status != "review" {
		t.Fatalf("P3.status: want review, got %q", p3.Status)
	}
	if !strings.Contains(p3.Evidence, "timeout") {
		t.Fatalf("P3.evidence must mention 'timeout', got %q", p3.Evidence)
	}
}

// TestProbeSIGINTFinalizesPartialReport — SIGINT delivered while a
// behavioral probe is hanging must produce a schema-valid partial report
// on stdout (interrupted=true, exactly 16 findings) and exit 130.
// Mirrors TestSignalInterruptsAuditAndExits130 but routes through the
// behavioral probe pass instead of debug-sleep.
func TestProbeSIGINTFinalizesPartialReport(t *testing.T) {
	bin := buildAfcli(t)
	recorder := buildArgvRecorder(t)
	desc := fixtureDescriptorPath(t, "probe-hanging.yaml")

	cmd := exec.Command(bin, "audit", recorder,
		"--probe",
		"--probe-timeout=30s",
		"--descriptor", desc,
		"--output", "json",
	)
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
		t.Fatalf("wait: unexpected error %v\nstderr=%s", waitErr, stderr.String())
	}
	if exitCode != 130 {
		t.Fatalf("exit code: want 130, got %d\nstdout=%s\nstderr=%s",
			exitCode, stdout.String(), stderr.String())
	}

	var report struct {
		Interrupted bool `json:"interrupted"`
		Findings    []struct {
			PrincipleID string `json:"principle_id"`
			Status      string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode partial report: %v\nstdout=%s", err, stdout.String())
	}
	if !report.Interrupted {
		t.Fatalf("interrupted: want true\nstdout=%s", stdout.String())
	}
	if len(report.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(report.Findings))
	}
	validateAgainstReportSchema(t, stdout.Bytes())
}

// TestProbeHelpSchemaShowsFlags — `audit --help-schema --output json`
// emits a schema document whose flag list includes --probe and
// --probe-timeout, so an agent inspecting the surface can discover them
// without running the binary in audit mode.
func TestProbeHelpSchemaShowsFlags(t *testing.T) {
	stdout, stderr, code := runAudit(t, "audit", "--help-schema", "--output", "json")
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstderr=%s", code, stderr)
	}
	var doc struct {
		Command struct {
			Flags []struct {
				Name string `json:"name"`
			} `json:"flags"`
		} `json:"command"`
	}
	if err := json.Unmarshal(stdout, &doc); err != nil {
		t.Fatalf("decode help-schema: %v\nstdout=%s", err, stdout)
	}
	have := map[string]bool{}
	for _, f := range doc.Command.Flags {
		have[f.Name] = true
	}
	for _, want := range []string{"probe", "probe-timeout"} {
		if !have[want] {
			t.Errorf("help-schema missing flag %q (have: %v)", want, have)
		}
	}
}

// TestProbeOffByteIdenticalToS04 — preserves R004's default-off byte
// identity. Two `--deterministic` runs without --probe against the
// recorder produce byte-identical stdout, regardless of the descriptor
// machinery being wired in this slice.
func TestProbeOffByteIdenticalToS04(t *testing.T) {
	recorder := buildArgvRecorder(t)
	out1, _, code1 := runAudit(t, "audit", recorder, "--output", "json", "--deterministic")
	if code1 != 0 && code1 != 1 {
		t.Fatalf("first run exit code: want 0 or 1, got %d", code1)
	}
	out2, _, code2 := runAudit(t, "audit", recorder, "--output", "json", "--deterministic")
	if code2 != 0 && code2 != 1 {
		t.Fatalf("second run exit code: want 0 or 1, got %d", code2)
	}
	if !bytes.Equal(out1, out2) {
		t.Fatalf("byte-identical determinism violated\nrun1=%s\nrun2=%s", out1, out2)
	}
}
