// p3-failing is a fixture that emits structurally-different stdout
// across runs. Empirically, Go's map-iteration randomization is not
// reliable enough on small maps under fast subprocess fork (consecutive
// runs frequently pick the same starting offset), so this fixture uses
// math/rand explicitly — the global source is auto-seeded in Go 1.20+,
// so two consecutive `--version` invocations diverge with probability
// 1 - 1/100^2 = 99.99%. The printed values are short two-digit integers
// inside a `token-N-M` shape, deliberately chosen so the diff line does
// NOT match any of the P3 mask patterns (no SHA-length hex, no port
// shape, no timestamp, no PID prefix, no duration suffix).
package main

import (
	"fmt"
	"math/rand"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(0)
	}
	switch os.Args[1] {
	case "--help":
		fmt.Fprint(os.Stdout, "Usage: p3-failing [--version|--help]\n")
		os.Exit(0)
	case "--afcli-bogus-flag":
		fmt.Fprint(os.Stderr, "p3-failing: unknown option: --afcli-bogus-flag\n")
		os.Exit(1)
	case "--version":
		fmt.Printf("token-%d-%d\n", rand.Intn(100), rand.Intn(100))
		os.Exit(0)
	default:
		os.Exit(0)
	}
}
