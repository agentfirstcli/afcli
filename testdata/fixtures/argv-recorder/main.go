// argv-recorder is a standalone test fixture used by S05's probe tests
// to prove ground-truth that commands.destructive[] is never invoked.
// It records every argv it sees (and a probe-env marker) to the file
// named in ARGV_RECORD_FILE, then dispatches on the first argument:
//
//	--help              prints usage, exits 0
//	--afcli-bogus-flag  writes an unknown-option message to stderr, exits 1
//	--hang              blocks forever (so per-probe timeout has work to kill)
//	(default)           exits 0
//
// Lives under testdata/ so `go build ./...` does not pick it up; tests
// build it explicitly via the buildArgvRecorder helper.
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	recordFile := os.Getenv("ARGV_RECORD_FILE")
	if recordFile != "" {
		writeRecord(recordFile)
	}

	if len(os.Args) < 2 {
		os.Exit(0)
	}

	switch os.Args[1] {
	case "--help":
		fmt.Fprint(os.Stdout, "Usage: argv-recorder [flags]\n  --hang  block forever\n")
		os.Exit(0)
	case "--afcli-bogus-flag":
		fmt.Fprint(os.Stderr, "argv-recorder: unknown option: --afcli-bogus-flag\n")
		os.Exit(1)
	case "--hang":
		// Block until killed (probe timeout / parent cancellation).
		// `select {}` would trigger Go's all-goroutines-asleep deadlock
		// detector and abort with exit 2 in <1ms — too short for the
		// per-probe timeout to fire. Sleeping in a loop keeps the timer
		// goroutine alive so the runtime considers main() "busy".
		for {
			time.Sleep(time.Hour)
		}
	default:
		os.Exit(0)
	}
}

// writeRecord appends a per-invocation block to the record file.
// Format (in order): an optional "ENV:AFCLI_TEST_PROBE=<value>" line if
// the env var is set, then one line per argv (args[1:]), then a single
// blank-line separator so multi-arg invocations are distinguishable.
// Best-effort: any I/O error is silently ignored — the recorder must
// not crash the probe under test.
func writeRecord(path string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	if v := os.Getenv("AFCLI_TEST_PROBE"); v != "" {
		fmt.Fprintf(f, "ENV:AFCLI_TEST_PROBE=%s\n", v)
	}
	for _, a := range os.Args[1:] {
		fmt.Fprintln(f, a)
	}
	fmt.Fprintln(f)
}
