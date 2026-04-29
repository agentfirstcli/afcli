package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runAuditNoFatal mirrors runAudit but tolerates non-zero exit codes
// without t.Fatal — the envelope-error tests need to inspect the
// post-failure filesystem, which means the run must return an exit
// code without aborting the test.
func runAuditNoFatal(t *testing.T, args ...string) (stdout, stderr []byte, exitCode int) {
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

// TestAuditBadgeWritesBothArtefacts — running audit with --badge against a
// known-good target lands both badge.svg and badge.json in --badge-out and
// they are non-empty (locked-formula scoring + literal SVG template).
func TestAuditBadgeWritesBothArtefacts(t *testing.T) {
	skipIfNoGit(t)

	tmp := t.TempDir()
	stdout, stderr, code := runAuditNoFatal(t, "audit", "/usr/bin/git",
		"--output", "json", "--badge", "--badge-out="+tmp)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	svg, err := os.ReadFile(filepath.Join(tmp, "badge.svg"))
	if err != nil {
		t.Fatalf("read badge.svg: %v", err)
	}
	if len(svg) == 0 {
		t.Errorf("badge.svg is empty")
	}
	if !bytes.Contains(svg, []byte("agent-first")) {
		t.Errorf("badge.svg missing 'agent-first' label:\n%s", svg)
	}

	js, err := os.ReadFile(filepath.Join(tmp, "badge.json"))
	if err != nil {
		t.Fatalf("read badge.json: %v", err)
	}
	if len(js) == 0 {
		t.Errorf("badge.json is empty")
	}
	if !bytes.Contains(js, []byte(`"schemaVersion": 1`)) {
		t.Errorf("badge.json missing schemaVersion key:\n%s", js)
	}
	if !bytes.Contains(js, []byte(`"label": "agent-first"`)) {
		t.Errorf("badge.json missing label:\n%s", js)
	}
}

// TestAuditBadgeOmittedWhenFlagOff — default audit path writes neither
// badge artefact. Guards default byte-identity: any future regression that
// invokes writeBadgeArtefacts unconditionally fails here.
func TestAuditBadgeOmittedWhenFlagOff(t *testing.T) {
	skipIfNoGit(t)

	tmp := t.TempDir()
	// Pass --badge-out so a regression that ignores --badge but honors
	// --badge-out still produces files inside this temp dir; the test
	// cleans up via t.TempDir regardless.
	stdout, stderr, code := runAuditNoFatal(t, "audit", "/usr/bin/git",
		"--output", "json", "--badge-out="+tmp)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	if _, err := os.Stat(filepath.Join(tmp, "badge.svg")); !os.IsNotExist(err) {
		t.Errorf("badge.svg should not exist when --badge is off, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "badge.json")); !os.IsNotExist(err) {
		t.Errorf("badge.json should not exist when --badge is off, got err=%v", err)
	}
}

// TestAuditBadgeNotWrittenOnEnvelopeError — TARGET_NOT_FOUND short-circuits
// before the success path, so --badge MUST NOT mint stale artefacts. The
// badge surface is a strict superset of a clean stdout report (S04 plan
// "Failure visibility").
func TestAuditBadgeNotWrittenOnEnvelopeError(t *testing.T) {
	tmp := t.TempDir()
	_, _, code := runAuditNoFatal(t, "audit", "/no/such/binary-afcli-badge-test",
		"--output", "json", "--badge", "--badge-out="+tmp)
	// Expect exit 3 (CouldNotAudit) — but the test's contract is the
	// filesystem state, not the exact code, so we only require non-zero
	// to confirm we hit an envelope error.
	if code == 0 {
		t.Fatalf("expected non-zero exit on missing target, got 0")
	}

	if _, err := os.Stat(filepath.Join(tmp, "badge.svg")); !os.IsNotExist(err) {
		t.Errorf("badge.svg should not exist on envelope error, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "badge.json")); !os.IsNotExist(err) {
		t.Errorf("badge.json should not exist on envelope error, got err=%v", err)
	}
}

// TestAuditBadgeDeterministic — two consecutive runs with
// AFCLI_DETERMINISTIC=1 produce byte-identical badge artefacts. Locks the
// determinism contract for the badge surface (mirrors
// TestAuditDeterministicByteIdentical for the JSON report). runAuditNoFatal
// already sets AFCLI_DETERMINISTIC=1.
func TestAuditBadgeDeterministic(t *testing.T) {
	skipIfNoGit(t)

	tmp1 := t.TempDir()
	_, _, code1 := runAuditNoFatal(t, "audit", "/usr/bin/git",
		"--output", "json", "--badge", "--badge-out="+tmp1)
	if code1 != 0 {
		t.Fatalf("first run exit code: want 0, got %d", code1)
	}

	tmp2 := t.TempDir()
	_, _, code2 := runAuditNoFatal(t, "audit", "/usr/bin/git",
		"--output", "json", "--badge", "--badge-out="+tmp2)
	if code2 != 0 {
		t.Fatalf("second run exit code: want 0, got %d", code2)
	}

	svg1, err := os.ReadFile(filepath.Join(tmp1, "badge.svg"))
	if err != nil {
		t.Fatalf("read run1 badge.svg: %v", err)
	}
	svg2, err := os.ReadFile(filepath.Join(tmp2, "badge.svg"))
	if err != nil {
		t.Fatalf("read run2 badge.svg: %v", err)
	}
	if !bytes.Equal(svg1, svg2) {
		t.Errorf("badge.svg byte-identity violated\nrun1=%s\nrun2=%s", svg1, svg2)
	}

	js1, err := os.ReadFile(filepath.Join(tmp1, "badge.json"))
	if err != nil {
		t.Fatalf("read run1 badge.json: %v", err)
	}
	js2, err := os.ReadFile(filepath.Join(tmp2, "badge.json"))
	if err != nil {
		t.Fatalf("read run2 badge.json: %v", err)
	}
	if !bytes.Equal(js1, js2) {
		t.Errorf("badge.json byte-identity violated\nrun1=%s\nrun2=%s", js1, js2)
	}
}
