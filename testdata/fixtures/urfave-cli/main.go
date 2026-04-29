// urfave-cli is a standalone test fixture for S02's inspect parser tests.
// It is a small urfave/cli/v2-rooted CLI with both safe verbs (list/get/
// status/version) and one destructive verb (delete) so the inspect
// classifier has real `COMMANDS:` output to chew on. Lives under
// testdata/ so `go build ./...` does not pick it up; tests build it
// explicitly via the buildUrfaveCli helper.
package main

import (
	"fmt"
	"os"

	cli "github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "urfave-cli",
		Usage: "fixture for inspect parser tests",
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "list available items",
				Action: func(c *cli.Context) error { return nil },
			},
			{
				Name:   "get",
				Usage:  "get a single item",
				Action: func(c *cli.Context) error { return nil },
			},
			{
				Name:   "status",
				Usage:  "status of the system",
				Action: func(c *cli.Context) error { return nil },
			},
			{
				Name:   "version",
				Usage:  "version of urfave-cli",
				Action: func(c *cli.Context) error { return nil },
			},
			{
				Name:   "delete",
				Usage:  "delete an item permanently",
				Action: func(c *cli.Context) error { return nil },
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
