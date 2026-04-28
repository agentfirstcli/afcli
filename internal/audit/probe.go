package audit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Capture holds the result of a single probe invocation. Callers receive
// a non-nil *Capture even when the subprocess fails to start or the
// context deadline expires — Err and ExitCode discriminate the cases.
type Capture struct {
	Args     []string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
	Duration time.Duration
}

const captureLimit = 64 * 1024

// runProbe invokes target with args under a per-probe timeout. Env is
// intentionally minimal and locale-stable so evidence strings are
// byte-stable across machines and CI runs. ctx threading preserves the
// SIGINT cancellation contract from S01.
func runProbe(ctx context.Context, target string, args []string, timeout time.Duration) *Capture {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, target, args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"LC_ALL=C",
		"LANG=C",
		"GIT_PAGER=cat",
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	c := &Capture{
		Args:     args,
		Stdout:   truncate(stdout.String()),
		Stderr:   truncate(stderr.String()),
		Duration: dur,
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			c.ExitCode = exitErr.ExitCode()
		} else {
			c.Err = err
			c.ExitCode = -1
		}
	}
	return c
}

func truncate(s string) string {
	if len(s) > captureLimit {
		return s[:captureLimit]
	}
	return s
}
