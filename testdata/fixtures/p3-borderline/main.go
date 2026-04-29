// p3-borderline is a fixture whose `--version` output differs across
// runs only in the leading RFC3339Nano timestamp — exactly the kind of
// allowlisted non-canonical variation the P3 mask must collapse to
// requires-review (not automated:fail). Nanosecond resolution
// guarantees two consecutive probes never collide on the same instant.
package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(0)
	}
	switch os.Args[1] {
	case "--help":
		fmt.Fprint(os.Stdout, "Usage: p3-borderline [--version|--help]\n")
		os.Exit(0)
	case "--afcli-bogus-flag":
		fmt.Fprint(os.Stderr, "p3-borderline: unknown option: --afcli-bogus-flag\n")
		os.Exit(1)
	case "--version":
		fmt.Println(time.Now().UTC().Format(time.RFC3339Nano))
		fmt.Println("ok")
		os.Exit(0)
	default:
		os.Exit(0)
	}
}
