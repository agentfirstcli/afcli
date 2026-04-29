// flag-parse is a standalone test fixture for S02's inspect parser
// tests. It is a hand-rolled `flag.Parse` CLI — no subcommands, just a
// flat `--help` flag dump — so the inspect classifier degrades
// gracefully on the simplest `--help` shape (no Cobra `Available
// Commands:` block, no urfave `COMMANDS:` block). Lives under testdata/
// so `go build ./...` does not pick it up; tests build it explicitly
// via the buildFlagParse helper.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// Route Usage output to stdout so `--help` exit-0 text is captured
	// from stdout (the inspect probe reads target stdout). The flag
	// package defaults Output() to stderr.
	flag.CommandLine.SetOutput(os.Stdout)

	flag.Bool("version", false, "print version and exit")
	flag.Bool("list", false, "list available items")
	flag.Bool("status", false, "show status of the system")

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "Usage: flag-parse [flags]")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Flags:")
		flag.PrintDefaults()
	}

	// flag.Parse() handles -h / -help / --help specially: it calls
	// flag.Usage and, with the default ExitOnError, terminates with
	// exit code 0. No explicit --help bool is needed.
	flag.Parse()
}
