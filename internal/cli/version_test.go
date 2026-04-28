package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agentfirstcli/afcli/internal/version"
)

// TestVersionPaths verifies the three documented invocations all emit
// the same identity line on stdout, exit 0, and never trigger arg
// validation on subcommands that normally require an argument.
func TestVersionPaths(t *testing.T) {
	want := version.String()

	cases := []struct {
		name string
		args []string
	}{
		{"root flag", []string{"--version"}},
		{"version subcommand", []string{"version"}},
		{"audit flag bypass", []string{"audit", "--version"}},
		{"init flag bypass", []string{"init", "--version"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runAfcli(t, tc.args...)
			if code != 0 {
				t.Fatalf("exit code: want 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			line := strings.TrimSpace(string(stdout))
			if line != want {
				t.Fatalf("stdout: want %q, got %q", want, line)
			}
			if !bytes.HasPrefix(stdout, []byte("afcli ")) {
				t.Fatalf("stdout must start with 'afcli ': %q", stdout)
			}
			if len(stderr) != 0 {
				t.Fatalf("stderr must be empty, got %q", stderr)
			}
		})
	}
}
