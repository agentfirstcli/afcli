package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// InstallSignalHandler returns a child context that is cancelled when the
// process receives SIGINT or SIGTERM, plus a cleanup function the caller
// must invoke (typically via defer) to detach the signal channel and
// release the cancel resources.
//
// Without this handler Go's default SIGINT behavior would terminate the
// process with exit code 2, colliding with afcli's documented USAGE exit.
// Cancelling the context lets the audit pipeline finalize a partial
// report — interrupted: true plus skip statuses for unfinished principles —
// and exit with the dedicated 130 code (R012).
func InstallSignalHandler(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}
