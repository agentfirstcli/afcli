package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// TestBuildHelpSchemaRoot verifies the root-anchored walk surfaces the
// public subcommand set in deterministic alphabetical order.
func TestBuildHelpSchemaRoot(t *testing.T) {
	hs := BuildHelpSchema(rootCmd)
	if hs.SchemaVersion != "1" {
		t.Errorf("schema_version: want 1, got %q", hs.SchemaVersion)
	}
	if hs.AfcliVersion == "" {
		t.Errorf("afcli_version is empty")
	}
	if hs.ManifestVersion == "" {
		t.Errorf("manifest_version is empty")
	}

	names := make([]string, 0, len(hs.Command.Subcommands))
	for _, c := range hs.Command.Subcommands {
		names = append(names, c.Name)
	}
	want := []string{"audit", "init", "inspect", "manifest", "version"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("root subcommands: want %v, got %v", want, names)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("root subcommands not alphabetically sorted: %v", names)
	}
}

// TestBuildHelpSchemaPerSubcommand verifies that anchoring the walk at a
// subcommand (e.g. audit) produces a schema rooted there, not at the world.
// Also pins audit's local flags in alphabetical order — collectFlags sorts
// by name so this list is the wire-visible surface that --help-schema
// consumers (and the website-generator) lock onto.
func TestBuildHelpSchemaPerSubcommand(t *testing.T) {
	hs := BuildHelpSchema(auditCmd)
	if hs.Command.Name != "audit" {
		t.Errorf("command.name: want audit, got %q", hs.Command.Name)
	}
	if len(hs.Command.Subcommands) != 0 {
		t.Errorf("audit should have no subcommands, got %d", len(hs.Command.Subcommands))
	}

	localNames := []string{}
	for _, f := range hs.Command.Flags {
		if !f.Persistent {
			localNames = append(localNames, f.Name)
		}
	}
	wantLocal := []string{
		"badge",
		"badge-out",
		"descriptor",
		"fail-on",
		"probe",
		"probe-timeout",
	}
	if !reflect.DeepEqual(localNames, wantLocal) {
		t.Errorf("audit local flags:\n want %v\n got  %v", wantLocal, localNames)
	}
	if !sort.StringsAreSorted(localNames) {
		t.Errorf("audit local flags not sorted: %v", localNames)
	}
}

// TestHelpSchemaSkipsHiddenFlags verifies that pflag.Hidden flags
// (e.g. --debug-sleep) are filtered out, matching --help's behaviour.
func TestHelpSchemaSkipsHiddenFlags(t *testing.T) {
	hs := BuildHelpSchema(auditCmd)
	for _, f := range hs.Command.Flags {
		if f.Name == "debug-sleep" {
			t.Fatalf("hidden flag --debug-sleep leaked into help schema")
		}
	}
}

// TestRenderHelpSchemaValidatesAgainstFixture renders the schema and
// validates it against the on-disk JSON Schema fixture so any drift in
// the document shape fails CI.
func TestRenderHelpSchemaValidatesAgainstFixture(t *testing.T) {
	hs := BuildHelpSchema(rootCmd)

	var buf bytes.Buffer
	if err := RenderHelpSchema(&buf, hs); err != nil {
		t.Fatalf("RenderHelpSchema: %v", err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "help-schema.schema.json"))
	schema, err := jsonschema.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	var generic any
	if err := json.Unmarshal(buf.Bytes(), &generic); err != nil {
		t.Fatalf("decode rendered JSON: %v\n%s", err, buf.String())
	}
	if err := schema.Validate(generic); err != nil {
		t.Fatalf("rendered help schema failed validation: %v\n%s", err, buf.String())
	}
}

// TestHelpSchemaErrorCodesExactSet pins the sorted set of documented
// error codes. New codes are an additive change and must update both
// internal/report constants and this test in the same commit.
func TestHelpSchemaErrorCodesExactSet(t *testing.T) {
	hs := BuildHelpSchema(rootCmd)
	want := []string{
		"DESCRIPTOR_INVALID",
		"DESCRIPTOR_NOT_FOUND",
		"INIT_FILE_EXISTS",
		"INTERNAL",
		"PROBE_DENIED",
		"PROBE_TIMEOUT",
		"TARGET_NOT_EXECUTABLE",
		"TARGET_NOT_FOUND",
		"USAGE",
	}
	if !reflect.DeepEqual(hs.ErrorCodes, want) {
		t.Errorf("error_codes:\n want %v\n got  %v", want, hs.ErrorCodes)
	}
}

// TestHelpSchemaExitCodesCoverContract verifies every documented exit
// code from M001-CONTEXT is present with a stable name.
func TestHelpSchemaExitCodesCoverContract(t *testing.T) {
	hs := BuildHelpSchema(rootCmd)
	wantNames := map[int]string{
		0:   "OK",
		1:   "FindingsAtThreshold",
		2:   "Usage",
		3:   "CouldNotAudit",
		4:   "Internal",
		130: "Interrupted",
	}
	got := map[int]string{}
	for _, ec := range hs.ExitCodes {
		got[ec.Code] = ec.Name
		if ec.Meaning == "" {
			t.Errorf("exit code %d has empty meaning", ec.Code)
		}
	}
	if !reflect.DeepEqual(got, wantNames) {
		t.Errorf("exit_codes:\n want %v\n got  %v", wantNames, got)
	}
}

var (
	helpSchemaBin     string
	helpSchemaBinErr  error
	helpSchemaBinOnce sync.Once
)

func buildHelpSchemaBin(t *testing.T) string {
	t.Helper()
	helpSchemaBinOnce.Do(func() {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			helpSchemaBinErr = errAssertionHS("could not locate test file via runtime.Caller")
			return
		}
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
		dir, err := os.MkdirTemp("", "afcli-helpschema-test-")
		if err != nil {
			helpSchemaBinErr = err
			return
		}
		bin := filepath.Join(dir, "afcli")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/afcli")
		cmd.Dir = repoRoot
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			helpSchemaBinErr = errAssertionHS("go build failed: " + err.Error() + "\n" + string(out))
			return
		}
		helpSchemaBin = bin
	})
	if helpSchemaBinErr != nil {
		t.Fatalf("build afcli: %v", helpSchemaBinErr)
	}
	return helpSchemaBin
}

type errAssertionHS string

func (e errAssertionHS) Error() string { return string(e) }

// TestHelpSchemaSubprocessDeterministic builds a real binary, invokes
// --help-schema twice, and asserts the output is byte-identical. Mirrors
// the subprocess pattern from signals_test.go and locks the determinism
// contract: any future change that sneaks in map-iteration or
// registration-order dependence breaks here.
func TestHelpSchemaSubprocessDeterministic(t *testing.T) {
	bin := buildHelpSchemaBin(t)

	first, err := exec.Command(bin, "--help-schema").Output()
	if err != nil {
		t.Fatalf("first invocation: %v", err)
	}
	second, err := exec.Command(bin, "--help-schema").Output()
	if err != nil {
		t.Fatalf("second invocation: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("help-schema output differs between invocations:\nfirst=%s\nsecond=%s", first, second)
	}

	// The bytes must also parse and validate, so determinism does not
	// silently mask a corrupt document.
	_, thisFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "help-schema.schema.json"))
	schema, err := jsonschema.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	var generic any
	if err := json.Unmarshal(first, &generic); err != nil {
		t.Fatalf("decode subprocess JSON: %v\n%s", err, first)
	}
	if err := schema.Validate(generic); err != nil {
		t.Fatalf("subprocess help schema failed validation: %v\n%s", err, first)
	}
}
