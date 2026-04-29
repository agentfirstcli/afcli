package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/descriptor"
)

// TestInspectQuotesTrickyTokens — every spliced token in the YAML
// template is run through strconv.Quote, so a target name containing
// the YAML string-context breakers (`"` and `\`) must still round-trip
// through descriptor.Load. The fixture is a no-op shell script whose
// FILENAME contains the tricky bytes; Linux filenames allow both, so
// this is a real-world-shaped test rather than a mocked input.
func TestInspectQuotesTrickyTokens(t *testing.T) {
	dir := t.TempDir()
	tricky := `evil"name\with\backslash`
	scriptPath := filepath.Join(dir, tricky)
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write tricky-name script: %v", err)
	}

	stdout, stderr, code := runAfcli(t, "inspect", scriptPath)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}

	// Round-trip the emitted YAML through descriptor.Load.
	out := filepath.Join(dir, "out.yaml")
	if err := os.WriteFile(out, stdout, 0o644); err != nil {
		t.Fatalf("write captured stdout: %v", err)
	}
	d, err := descriptor.Load(out)
	if err != nil {
		t.Fatalf("descriptor.Load on inspect output: %v\nbody=%s", err, stdout)
	}
	if d.Target != scriptPath {
		t.Errorf("target round-trip: want %q, got %q", scriptPath, d.Target)
	}
	if d.FormatVersion != "1" {
		t.Errorf("format_version: want \"1\", got %q", d.FormatVersion)
	}
}

// TestInspectRecursionDepthCap — the recursive walker MUST terminate on
// realistic Cobra fixtures and produce de-duplicated output. The plan's
// Acceptable simplification is checked here: emitted YAML has no
// duplicate safe verbs and the call completes well under 30s.
func TestInspectRecursionDepthCap(t *testing.T) {
	bin := buildCobraCli(t)

	start := time.Now()
	stdout, stderr, code := runAfcli(t, "inspect", bin)
	dur := time.Since(start)

	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if dur > 30*time.Second {
		t.Fatalf("inspect took %v; expected <30s under recursion caps", dur)
	}

	// Decode the YAML and assert no duplicate safe verbs (the visited
	// set in walkHelp dedupes prefixes; the safe map dedupes verbs).
	out := filepath.Join(t.TempDir(), "out.yaml")
	if err := os.WriteFile(out, stdout, 0o644); err != nil {
		t.Fatalf("write captured stdout: %v", err)
	}
	d, err := descriptor.Load(out)
	if err != nil {
		t.Fatalf("descriptor.Load on inspect output: %v\nbody=%s", err, stdout)
	}
	seen := map[string]bool{}
	for _, v := range d.Commands.Safe {
		if seen[v] {
			t.Fatalf("duplicate safe verb %q in inspect output:\n%s", v, stdout)
		}
		seen[v] = true
	}
	if len(d.Commands.Safe) == 0 {
		t.Fatalf("expected at least one safe verb from cobra-cli fixture\n%s", stdout)
	}
}

// TestInspectTargetNotFound — passing a path that does not exist must
// produce the documented TARGET_NOT_FOUND envelope on stderr and exit 3.
// classifyResolveError is reused from audit.go (same code path).
func TestInspectTargetNotFound(t *testing.T) {
	stdout, stderr, code := runAfcli(t, "inspect", "/no/such/path-afcli-inspect-test")
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &env); err != nil {
		t.Fatalf("decode stderr envelope: %v\nstderr=%s", err, stderr)
	}
	if env.Error.Code != "TARGET_NOT_FOUND" {
		t.Errorf("error.code: want TARGET_NOT_FOUND, got %q", env.Error.Code)
	}
}

// TestInspectTargetNotExecutable — a non-executable file (mode 0644)
// triggers TARGET_NOT_EXECUTABLE / exit 3 via classifyResolveError's
// re-stat path.
func TestInspectTargetNotExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-exec")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatalf("write non-exec file: %v", err)
	}

	stdout, stderr, code := runAfcli(t, "inspect", path)
	if code != 3 {
		t.Fatalf("exit code: want 3, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &env); err != nil {
		t.Fatalf("decode stderr envelope: %v\nstderr=%s", err, stderr)
	}
	if env.Error.Code != "TARGET_NOT_EXECUTABLE" {
		t.Errorf("error.code: want TARGET_NOT_EXECUTABLE, got %q", env.Error.Code)
	}
}

// TestHelpSchemaListsInspectSubcommand — sibling to the init test; the
// --help-schema JSON document must list inspect under
// command.subcommands so future tooling that reflects on the public CLI
// surface picks it up automatically.
func TestHelpSchemaListsInspectSubcommand(t *testing.T) {
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
	}
	if err := json.Unmarshal(stdout, &doc); err != nil {
		t.Fatalf("decode help schema: %v\nstdout=%s", err, stdout)
	}

	got := false
	for _, s := range doc.Command.Subcommands {
		if s.Name == "inspect" {
			got = true
			break
		}
	}
	if !got {
		names := make([]string, 0, len(doc.Command.Subcommands))
		for _, s := range doc.Command.Subcommands {
			names = append(names, s.Name)
		}
		t.Errorf("subcommands missing inspect: got %s", strings.Join(names, ", "))
	}
}

// TestInspectEmitsCommentBlockHeader — the cobra-cli fixture has the
// destructive verbs delete + apply, so the emitted YAML must contain
// a header `# REVIEW:` block, a populated `commands:\n  safe:` block,
// and a `destructive:` block with `# REVIEW:` lines (commented
// candidates, never active list entries).
func TestInspectEmitsCommentBlockHeader(t *testing.T) {
	bin := buildCobraCli(t)
	stdout, stderr, code := runAfcli(t, "inspect", bin)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d\nstderr=%s", code, stderr)
	}
	body := string(stdout)

	if !strings.Contains(body, "# REVIEW:") {
		t.Errorf("output missing `# REVIEW:` marker:\n%s", body)
	}
	if !strings.Contains(body, "commands:\n  safe:\n    - ") {
		t.Errorf("output missing populated `commands.safe[]` block:\n%s", body)
	}
	// destructive: [] must remain empty (active list); the candidates
	// only appear as `# REVIEW:` comments.
	if !strings.Contains(body, "destructive: []") {
		t.Errorf("output must keep destructive: [] empty:\n%s", body)
	}
	// At least two # REVIEW: lines below `destructive: []` (one for
	// "delete", one for "apply") — they live in the commands subtree
	// as comments.
	commentCount := strings.Count(body, "  # REVIEW: ")
	if commentCount < 2 {
		t.Errorf("expected ≥2 indented `  # REVIEW: ` lines, got %d:\n%s", commentCount, body)
	}
}

// TestInspectRoundTripsThroughDescriptorLoad — the contract that locks
// the slice goal: every emitted YAML body, against every supported
// fixture style (cobra, urfave, hand-rolled flag.Parse), parses back
// through descriptor.Load with no error and FormatVersion == "1".
func TestInspectRoundTripsThroughDescriptorLoad(t *testing.T) {
	cases := []struct {
		name  string
		build func(*testing.T) string
	}{
		{name: "cobra-cli", build: buildCobraCli},
		{name: "urfave-cli", build: buildUrfaveCli},
		{name: "flag-parse", build: buildFlagParse},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := tc.build(t)
			stdout, stderr, code := runAfcli(t, "inspect", bin)
			if code != 0 {
				t.Fatalf("exit code: want 0, got %d\nstderr=%s", code, stderr)
			}
			out := filepath.Join(t.TempDir(), "out.yaml")
			if err := os.WriteFile(out, stdout, 0o644); err != nil {
				t.Fatalf("write captured stdout: %v", err)
			}
			d, err := descriptor.Load(out)
			if err != nil {
				t.Fatalf("descriptor.Load: %v\nbody=%s", err, stdout)
			}
			if d.FormatVersion != "1" {
				t.Errorf("format_version: want \"1\", got %q", d.FormatVersion)
			}
			if !bytes.Contains(stdout, []byte("format_version: \"1\"")) {
				t.Errorf("output missing literal `format_version: \"1\"` line:\n%s", stdout)
			}
		})
	}
}
