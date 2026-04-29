package cli_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/agentfirstcli/afcli/internal/cli"
)

// TestInspectVerbDictionaryLocked has exactly one assertion per token
// in safeVerbs and one per token in destructiveVerbs. Adding a verb to
// either list MUST be paired with a new assertion here — that is the
// review surface the slice plan calls out as "additions show up as
// additional assertions in code review".
func TestInspectVerbDictionaryLocked(t *testing.T) {
	// safeVerbs — one assertion per token.
	if !slices.Contains(cli.SafeVerbs, "list") {
		t.Fatal("safeVerbs missing 'list'")
	}
	if !slices.Contains(cli.SafeVerbs, "get") {
		t.Fatal("safeVerbs missing 'get'")
	}
	if !slices.Contains(cli.SafeVerbs, "show") {
		t.Fatal("safeVerbs missing 'show'")
	}
	if !slices.Contains(cli.SafeVerbs, "status") {
		t.Fatal("safeVerbs missing 'status'")
	}
	if !slices.Contains(cli.SafeVerbs, "describe") {
		t.Fatal("safeVerbs missing 'describe'")
	}
	if !slices.Contains(cli.SafeVerbs, "version") {
		t.Fatal("safeVerbs missing 'version'")
	}
	if !slices.Contains(cli.SafeVerbs, "help") {
		t.Fatal("safeVerbs missing 'help'")
	}
	if !slices.Contains(cli.SafeVerbs, "ls") {
		t.Fatal("safeVerbs missing 'ls'")
	}
	if got, want := len(cli.SafeVerbs), 8; got != want {
		t.Fatalf("safeVerbs len = %d, want %d (assertions above must enumerate every entry)", got, want)
	}

	// destructiveVerbs — one assertion per token.
	if !slices.Contains(cli.DestructiveVerbs, "delete") {
		t.Fatal("destructiveVerbs missing 'delete'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "rm") {
		t.Fatal("destructiveVerbs missing 'rm'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "destroy") {
		t.Fatal("destructiveVerbs missing 'destroy'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "drop") {
		t.Fatal("destructiveVerbs missing 'drop'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "purge") {
		t.Fatal("destructiveVerbs missing 'purge'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "reset") {
		t.Fatal("destructiveVerbs missing 'reset'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "wipe") {
		t.Fatal("destructiveVerbs missing 'wipe'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "force") {
		t.Fatal("destructiveVerbs missing 'force'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "kill") {
		t.Fatal("destructiveVerbs missing 'kill'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "apply") {
		t.Fatal("destructiveVerbs missing 'apply'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "create") {
		t.Fatal("destructiveVerbs missing 'create'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "update") {
		t.Fatal("destructiveVerbs missing 'update'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "set") {
		t.Fatal("destructiveVerbs missing 'set'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "push") {
		t.Fatal("destructiveVerbs missing 'push'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "publish") {
		t.Fatal("destructiveVerbs missing 'publish'")
	}
	if !slices.Contains(cli.DestructiveVerbs, "deploy") {
		t.Fatal("destructiveVerbs missing 'deploy'")
	}
	if got, want := len(cli.DestructiveVerbs), 16; got != want {
		t.Fatalf("destructiveVerbs len = %d, want %d (assertions above must enumerate every entry)", got, want)
	}
}

// TestInspectClassifierBothListsCanonical guards against the same
// token appearing in both safeVerbs and destructiveVerbs. Even though
// classify() resolves precedence in destructive's favor, having a
// duplicate would silently mean the safe-list assertion above is
// claiming coverage the classifier intentionally rejects.
func TestInspectClassifierBothListsCanonical(t *testing.T) {
	for _, v := range cli.SafeVerbs {
		if slices.Contains(cli.DestructiveVerbs, v) {
			t.Errorf("token %q appears in BOTH safeVerbs and destructiveVerbs", v)
		}
	}
}

// TestInspectClassifierNeverPutsDestructiveInSafe is the explicit
// adversarial guard called out in the slice plan's must-haves: feed
// synthetic input that pairs a safe verb with a destructive one and
// assert the classifier never lets the destructive token cross into
// the active safe list. False-positive destructive-in-safe is the
// worst failure mode in this slice (CONTEXT line 239) because audit
// --probe will execute everything in commands.safe[].
func TestInspectClassifierNeverPutsDestructiveInSafe(t *testing.T) {
	text := "Available Commands:\n  list   List things\n  delete   Delete things\n"
	safe, _, dest := cli.ParseHelp(text)
	if !slices.Contains(safe, "list") {
		t.Fatalf("expected 'list' in safe, got %v", safe)
	}
	if slices.Contains(safe, "delete") {
		t.Fatalf("'delete' must NEVER appear in safe; got safe=%v", safe)
	}
	if !slices.Contains(dest, "delete") {
		t.Fatalf("expected 'delete' in destructiveCandidates, got %v", dest)
	}
}

// TestInspectParseCobraHelp builds the cobra fixture from T01 and
// feeds its captured --help output through the parser. Asserts the
// classifier picks safe verbs (list/get/status/version) out of the
// "Available Commands:" block and routes destructive verbs
// (delete/apply) to destructiveCandidates.
func TestInspectParseCobraHelp(t *testing.T) {
	bin := buildCobraCli(t)
	text := captureHelp(t, bin, "cobra-cli")
	safe, _, dest := cli.ParseHelp(text)
	for _, v := range []string{"list", "get", "status", "version"} {
		if !slices.Contains(safe, v) {
			t.Errorf("cobra: safe missing %q (got %v)\nhelp:\n%s", v, safe, text)
		}
	}
	for _, v := range []string{"delete", "apply"} {
		if !slices.Contains(dest, v) {
			t.Errorf("cobra: destructiveCandidates missing %q (got %v)\nhelp:\n%s", v, dest, text)
		}
		if slices.Contains(safe, v) {
			t.Errorf("cobra: destructive verb %q leaked into safe (got %v)", v, safe)
		}
	}
}

// TestInspectParseUrfaveHelp does the same against the urfave/cli/v2
// fixture, which prints "COMMANDS:" instead of "Available Commands:".
// Note urfave renders aliases as "help, h" — that row's first
// whitespace token is "help," (with trailing comma), which verbRe
// rejects, so it never reaches classify(). That is intentional and
// covered by TestInspectParseFlagPackageHelp's "no panic on weird
// shapes" guarantee.
func TestInspectParseUrfaveHelp(t *testing.T) {
	bin := buildUrfaveCli(t)
	text := captureHelp(t, bin, "urfave-cli")
	safe, _, dest := cli.ParseHelp(text)
	for _, v := range []string{"list", "get", "status", "version"} {
		if !slices.Contains(safe, v) {
			t.Errorf("urfave: safe missing %q (got %v)\nhelp:\n%s", v, safe, text)
		}
	}
	if !slices.Contains(dest, "delete") {
		t.Errorf("urfave: destructiveCandidates missing 'delete' (got %v)\nhelp:\n%s", dest, text)
	}
	if slices.Contains(safe, "delete") {
		t.Errorf("urfave: destructive verb 'delete' leaked into safe (got %v)", safe)
	}
}

// TestInspectParseFlagPackageHelp feeds the captured flag.Parse-style
// fixture --help (no "Available Commands:" / "COMMANDS:" block) and
// asserts the parser degrades gracefully: empty subcommands, empty
// safe, empty destructiveCandidates, no panic.
func TestInspectParseFlagPackageHelp(t *testing.T) {
	bin := buildFlagParse(t)
	text := captureHelp(t, bin, "flag-parse")
	safe, subs, dest := cli.ParseHelp(text)
	if len(safe) != 0 {
		t.Errorf("flag-parse: expected empty safe, got %v\nhelp:\n%s", safe, text)
	}
	if len(subs) != 0 {
		t.Errorf("flag-parse: expected empty subcommands, got %v\nhelp:\n%s", subs, text)
	}
	if len(dest) != 0 {
		t.Errorf("flag-parse: expected empty destructiveCandidates, got %v\nhelp:\n%s", dest, text)
	}
}

// captureHelp runs `<bin> --help` and returns stdout. Mirrors the
// shape of TestInspectFixturesBuildAndEmitHelp (T01) — the inspect
// probe in production reads target stdout, so all three fixtures emit
// --help text on stdout.
func captureHelp(t *testing.T, bin, label string) string {
	t.Helper()
	cmd := exec.Command(bin, "--help")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s --help: %v\nstderr:\n%s", label, err, stderr.String())
	}
	return stdout.String()
}
