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

var (
	recorderBinPath     string
	recorderBinBuildErr error
	recorderBinBuildOne sync.Once
)

// buildArgvRecorder compiles testdata/fixtures/argv-recorder once per
// test process. Mirrors buildAfcli's pattern (sync.Once + runtime.Caller
// for repo-root + os.MkdirTemp + go build) but uses a separate temp dir
// so two fixture builds don't collide. The fixture lives under
// testdata/, so `go build ./...` does not pick it up; tests must build
// it explicitly.
func buildArgvRecorder(t *testing.T) string {
	t.Helper()
	recorderBinBuildOne.Do(func() {
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			recorderBinBuildErr = errAssertion("could not locate test file via runtime.Caller")
			return
		}
		// probe_fixture_test.go lives at <root>/internal/cli/probe_fixture_test.go
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
		dir, err := os.MkdirTemp("", "afcli-argv-recorder-")
		if err != nil {
			recorderBinBuildErr = err
			return
		}
		bin := filepath.Join(dir, "argv-recorder")
		cmd := exec.Command("go", "build", "-o", bin, "./testdata/fixtures/argv-recorder")
		cmd.Dir = repoRoot
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			recorderBinBuildErr = errAssertion("go build argv-recorder failed: " + err.Error() + "\n" + string(out))
			return
		}
		recorderBinPath = bin
	})
	if recorderBinBuildErr != nil {
		t.Fatalf("build argv-recorder: %v", recorderBinBuildErr)
	}
	return recorderBinPath
}

// TestArgvRecorderBuildsAndRecords is a smoke test for the fixture.
// It builds the recorder, runs it twice with different argv against a
// single record file, and asserts each argv appears as its own line.
// This guards against regressions in the fixture's record format that
// would silently break T05's destructive-argv-not-invoked assertions.
func TestArgvRecorderBuildsAndRecords(t *testing.T) {
	bin := buildArgvRecorder(t)

	logPath := filepath.Join(t.TempDir(), "argv.log")

	for _, argv := range [][]string{
		{"--version", "--foo"},
		{"--help"},
	} {
		cmd := exec.Command(bin, argv...)
		cmd.Env = append(os.Environ(), "ARGV_RECORD_FILE="+logPath)
		// --help exits 0; --version --foo falls through to default exit 0.
		// Capture output to avoid noisy test logs but otherwise ignore it.
		_ = cmd.Run()
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read record file: %v", err)
	}
	got := string(data)
	for _, want := range []string{"--version", "--foo", "--help"} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("record file missing line %q\nfull contents:\n%s", want, got)
		}
	}
}
