package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentfirstcli/afcli/internal/descriptor"
)

// runAfcli is a thin wrapper over the prebuilt binary for the init tests.
// Mirrors runAudit in audit_test.go but doesn't force the audit subcommand
// or AFCLI_DETERMINISTIC=1 — init is deterministic by construction and
// some tests need to inspect ordinary stderr.
func runAfcli(t *testing.T, args ...string) (stdout, stderr []byte, exitCode int) {
	t.Helper()
	bin := buildAfcli(t)

	cmd := exec.Command(bin, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	cmd.Env = os.Environ()

	waitErr := cmd.Run()
	code := 0
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if waitErr != nil {
		t.Fatalf("run %v: %v\nstderr=%s", args, waitErr, errBuf.String())
	}
	return out.Bytes(), errBuf.Bytes(), code
}

// TestInitWritesRoundTripSafeYAML — `afcli init mytool` produces a file
// that descriptor.Load parses without error and that round-trips the
// expected scaffold contract: format_version "1", Target "mytool", and
// every list/map section present but empty.
func TestInitWritesRoundTripSafeYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "afcli.yaml")

	stdout, stderr, code := runAfcli(t, "init", "mytool", "--out", path)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	d, err := descriptor.Load(path)
	if err != nil {
		body, _ := os.ReadFile(path)
		t.Fatalf("descriptor.Load: %v\nbody=%s", err, body)
	}
	if d.FormatVersion != "1" {
		t.Errorf("format_version: want \"1\", got %q", d.FormatVersion)
	}
	if d.Target != "mytool" {
		t.Errorf("target: want \"mytool\", got %q", d.Target)
	}
	if len(d.Commands.Safe) != 0 {
		t.Errorf("commands.safe: want empty, got %v", d.Commands.Safe)
	}
	if len(d.Commands.Destructive) != 0 {
		t.Errorf("commands.destructive: want empty, got %v", d.Commands.Destructive)
	}
	if len(d.Env) != 0 {
		t.Errorf("env: want empty, got %v", d.Env)
	}
	if len(d.SkipPrinciples) != 0 {
		t.Errorf("skip_principles: want empty, got %v", d.SkipPrinciples)
	}
	if len(d.RelaxPrinciples) != 0 {
		t.Errorf("relax_principles: want empty, got %v", d.RelaxPrinciples)
	}
}

// TestInitRefusesOverwriteWithoutForce — second invocation with the same
// out path must exit 3 with INIT_FILE_EXISTS on stderr and details.path
// echoing the protected file (the observability promise in the task plan).
func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "afcli.yaml")

	if _, _, code := runAfcli(t, "init", "mytool", "--out", path); code != 0 {
		t.Fatalf("first init exit code: want 0, got %d", code)
	}

	stdout, stderr, code := runAfcli(t, "init", "mytool", "--out", path)
	if code != 3 {
		t.Fatalf("second init exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if !bytes.Contains(stderr, []byte("INIT_FILE_EXISTS")) {
		t.Fatalf("stderr missing INIT_FILE_EXISTS\nstderr=%s", stderr)
	}
	if !bytes.Contains(stderr, []byte(path)) {
		t.Fatalf("stderr missing protected path %q\nstderr=%s", path, stderr)
	}
}

// TestInitForceOverwrites — --force lets a second invocation succeed and
// rewrite the file contents to the canonical template body.
func TestInitForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "afcli.yaml")

	if _, _, code := runAfcli(t, "init", "mytool", "--out", path); code != 0 {
		t.Fatalf("first init exit code: want 0, got %d", code)
	}
	if err := os.WriteFile(path, []byte("# poisoned\n"), 0o644); err != nil {
		t.Fatalf("poison file: %v", err)
	}

	stdout, stderr, code := runAfcli(t, "init", "mytool", "--out", path, "--force")
	if code != 0 {
		t.Fatalf("force init exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if bytes.Contains(body, []byte("# poisoned")) {
		t.Fatalf("overwrite did not replace poisoned content\nbody=%s", body)
	}
	if !bytes.Contains(body, []byte("format_version: \"1\"")) {
		t.Fatalf("overwrite produced no format_version line\nbody=%s", body)
	}
}

// TestInitEscapesQuoteInTargetName — the YAML body must remain parseable
// when the target arg contains characters that would otherwise break out
// of the YAML string (`"` and `\`). strconv.Quote in init.go is what
// guards this; the test pins the contract.
func TestInitEscapesQuoteInTargetName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "afcli.yaml")
	tricky := `evil"name\with\backslash`

	stdout, stderr, code := runAfcli(t, "init", tricky, "--out", path)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	d, err := descriptor.Load(path)
	if err != nil {
		body, _ := os.ReadFile(path)
		t.Fatalf("descriptor.Load: %v\nbody=%s", err, body)
	}
	if d.Target != tricky {
		t.Errorf("target round-trip: want %q, got %q", tricky, d.Target)
	}
}

// TestInitOutCustomPath — --out routes the write to an arbitrary path
// rather than ./afcli.yaml in the cwd. Uses a t.TempDir so the test
// cannot leak files into the working directory.
func TestInitOutCustomPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "custom.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	stdout, stderr, code := runAfcli(t, "init", "mytool", "--out", path)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("custom out path not written: %v", err)
	}
}

// TestHelpSchemaListsInitSubcommandAndErrorCode — the documented help
// schema surface must list `init` as a subcommand and INIT_FILE_EXISTS in
// the error_codes array. R014 hygiene check: agents discover the new
// failure mode via --help-schema without invoking the subcommand.
func TestHelpSchemaListsInitSubcommandAndErrorCode(t *testing.T) {
	stdout, stderr, code := runAfcli(t, "--help-schema", "--output", "json")
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstderr=%s", code, stderr)
	}

	var doc struct {
		Command struct {
			Subcommands []struct {
				Name string `json:"name"`
			} `json:"subcommands"`
		} `json:"command"`
		ErrorCodes []string `json:"error_codes"`
	}
	if err := json.Unmarshal(stdout, &doc); err != nil {
		t.Fatalf("decode help schema: %v\nstdout=%s", err, stdout)
	}

	gotSub := false
	for _, s := range doc.Command.Subcommands {
		if s.Name == "init" {
			gotSub = true
			break
		}
	}
	if !gotSub {
		names := make([]string, 0, len(doc.Command.Subcommands))
		for _, s := range doc.Command.Subcommands {
			names = append(names, s.Name)
		}
		t.Errorf("subcommands missing init: got %v", names)
	}

	gotCode := false
	for _, c := range doc.ErrorCodes {
		if c == "INIT_FILE_EXISTS" {
			gotCode = true
			break
		}
	}
	if !gotCode {
		t.Errorf("error_codes missing INIT_FILE_EXISTS: got %s", strings.Join(doc.ErrorCodes, ", "))
	}
}
