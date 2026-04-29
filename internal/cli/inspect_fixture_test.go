package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Each fixture has its own sync.Once + cached binary path + cached
// build error so a failing build for one fixture does not poison the
// other two for the rest of the test process. Mirrors the
// recorderBinPath/recorderBinBuildErr/recorderBinBuildOne pattern from
// probe_fixture_test.go.
var (
	cobraCliBinPath     string
	cobraCliBinBuildErr error
	cobraCliBinBuildOne sync.Once

	urfaveCliBinPath     string
	urfaveCliBinBuildErr error
	urfaveCliBinBuildOne sync.Once

	flagParseBinPath     string
	flagParseBinBuildErr error
	flagParseBinBuildOne sync.Once
)

// buildFixture compiles a testdata/fixtures/<name> package once per
// test process into a per-fixture temp dir, returning the cached binary
// path. Mirrors buildArgvRecorder's structure: runtime.Caller for repo
// root + os.MkdirTemp + go build, with a separate temp dir per fixture
// so concurrent builds don't collide.
func buildFixture(t *testing.T, name string, once *sync.Once, binPath *string, buildErr *error) string {
	t.Helper()
	once.Do(func() {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			*buildErr = errAssertion("could not locate test file via runtime.Caller")
			return
		}
		// inspect_fixture_test.go lives at <root>/internal/cli/inspect_fixture_test.go
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
		dir, err := os.MkdirTemp("", "afcli-fixture-"+name+"-")
		if err != nil {
			*buildErr = err
			return
		}
		bin := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", bin, "./testdata/fixtures/"+name)
		cmd.Dir = repoRoot
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			*buildErr = errAssertion("go build " + name + " failed: " + err.Error() + "\n" + string(out))
			return
		}
		*binPath = bin
	})
	if *buildErr != nil {
		t.Fatalf("build %s: %v", name, *buildErr)
	}
	return *binPath
}

// buildCobraCli compiles testdata/fixtures/cobra-cli once per test
// process. Used by T02's parser tests and the smoke test below.
func buildCobraCli(t *testing.T) string {
	return buildFixture(t, "cobra-cli", &cobraCliBinBuildOne, &cobraCliBinPath, &cobraCliBinBuildErr)
}

// buildUrfaveCli compiles testdata/fixtures/urfave-cli once per test
// process. Used by T02's parser tests and the smoke test below.
func buildUrfaveCli(t *testing.T) string {
	return buildFixture(t, "urfave-cli", &urfaveCliBinBuildOne, &urfaveCliBinPath, &urfaveCliBinBuildErr)
}

// buildFlagParse compiles testdata/fixtures/flag-parse once per test
// process. Used by T02's parser tests and the smoke test below.
func buildFlagParse(t *testing.T) string {
	return buildFixture(t, "flag-parse", &flagParseBinBuildOne, &flagParseBinPath, &flagParseBinBuildErr)
}

// TestInspectFixturesBuildAndEmitHelp builds all three inspect-parser
// fixtures and runs `<fixture> --help`, asserting each captured stdout
// contains the expected safe verbs (list/get/status/version) and the
// expected destructive verbs (delete/apply for cobra, delete for
// urfave). This guards against silent regressions in the fixture
// sources that would feed bad input to T02's classifier — and confirms
// each fixture exits 0 with --help output on stdout (the inspect probe
// reads target stdout in production, so stderr-only --help would be
// invisible to the classifier).
func TestInspectFixturesBuildAndEmitHelp(t *testing.T) {
	cases := []struct {
		name           string
		build          func(*testing.T) string
		wantSafe       []string
		wantDestructive []string
	}{
		{
			name:            "cobra-cli",
			build:           buildCobraCli,
			wantSafe:        []string{"list", "get", "status", "version"},
			wantDestructive: []string{"delete", "apply"},
		},
		{
			name:            "urfave-cli",
			build:           buildUrfaveCli,
			wantSafe:        []string{"list", "get", "status", "version"},
			wantDestructive: []string{"delete"},
		},
		{
			name:            "flag-parse",
			build:           buildFlagParse,
			wantSafe:        []string{"list", "status", "version"},
			wantDestructive: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := tc.build(t)
			cmd := exec.Command(bin, "--help")
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s --help: %v\nstderr:\n%s", tc.name, err, stderr.String())
			}
			got := stdout.String()
			if got == "" {
				t.Fatalf("%s --help produced empty stdout (stderr was %q)", tc.name, stderr.String())
			}
			for _, want := range tc.wantSafe {
				if !strings.Contains(got, want) {
					t.Errorf("%s --help stdout missing safe verb %q\nfull stdout:\n%s", tc.name, want, got)
				}
			}
			for _, want := range tc.wantDestructive {
				if !strings.Contains(got, want) {
					t.Errorf("%s --help stdout missing destructive verb %q\nfull stdout:\n%s", tc.name, want, got)
				}
			}
		})
	}
}
