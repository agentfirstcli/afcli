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

// errProbeTimeout is set on Capture.Err when the per-probe context
// deadline fired. errProbeCancelled is set when the parent context
// (SIGINT/SIGTERM) cancelled the probe before its own deadline. They are
// kept unexported so callers reach for IsProbeTimeout / IsProbeCancelled
// — the helpers carry the comparison intent and prevent accidental
// equality checks against a different error value.
var (
	errProbeTimeout   = errors.New("probe deadline exceeded")
	errProbeCancelled = errors.New("probe cancelled")
)

// IsProbeTimeout reports whether err originated from a per-probe deadline
// expiring (context.DeadlineExceeded on the probe context). T03/T04 use
// this to decorate P3 evidence with PROBE_TIMEOUT and to keep the audit
// running through a single bad probe (R008).
func IsProbeTimeout(err error) bool {
	return errors.Is(err, errProbeTimeout)
}

// IsProbeCancelled reports whether err originated from the parent
// context being cancelled (typically SIGINT mid-probe). The SIGINT
// finalizer uses this to mark unfinished principles as skip without
// confusing them with timed-out probes.
func IsProbeCancelled(err error) bool {
	return errors.Is(err, errProbeCancelled)
}

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
		// Discriminate context-driven failures BEFORE the generic
		// exec.ExitError branch so the SIGKILL that os/exec sends on a
		// context expiry is reported as a timeout/cancellation rather
		// than a "-1" mystery exit. probeCtx inherits parent
		// cancellation, so a SIGINT on the parent surfaces here as
		// context.Canceled too.
		switch {
		case errors.Is(probeCtx.Err(), context.DeadlineExceeded):
			c.Err = errProbeTimeout
			c.ExitCode = -1
			return c
		case errors.Is(probeCtx.Err(), context.Canceled):
			c.Err = errProbeCancelled
			c.ExitCode = -1
			return c
		}
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
