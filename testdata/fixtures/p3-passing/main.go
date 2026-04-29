// p3-passing is a deterministic fixture for the P3 probe-and-rerun
// promotion path: identical stdout twice on `--version`. Lives under
// testdata/ so `go build ./...` does not pick it up; the cli integration
// tests build it explicitly.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(0)
	}
	switch os.Args[1] {
	case "--help":
		fmt.Fprint(os.Stdout, "Usage: p3-passing [--version|--help]\n")
		os.Exit(0)
	case "--afcli-bogus-flag":
		fmt.Fprint(os.Stderr, "p3-passing: unknown option: --afcli-bogus-flag\n")
		os.Exit(1)
	case "--version":
		fmt.Println("afcli-p3-passing v1.0.0")
		os.Exit(0)
	default:
		os.Exit(0)
	}
}
