// cobra-cli is a standalone test fixture for S02's inspect parser tests.
// It is a small Cobra-rooted CLI with both safe verbs (list/get/status/
// version) and destructive verbs (delete/apply) so the inspect classifier
// has real `Available Commands:` output to chew on. Lives under testdata/
// so `go build ./...` does not pick it up; tests build it explicitly via
// the buildCobraCli helper.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "cobra-cli",
		Short: "cobra-cli is a fixture for inspect parser tests",
	}

	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "list available items",
		Run:   func(cmd *cobra.Command, args []string) {},
	})
	root.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "get a single item",
		Run:   func(cmd *cobra.Command, args []string) {},
	})
	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "status of the system",
		Run:   func(cmd *cobra.Command, args []string) {},
	})
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "version of cobra-cli",
		Run:   func(cmd *cobra.Command, args []string) {},
	})
	root.AddCommand(&cobra.Command{
		Use:   "delete",
		Short: "delete an item permanently",
		Run:   func(cmd *cobra.Command, args []string) {},
	})
	root.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "apply a change to the cluster",
		Run:   func(cmd *cobra.Command, args []string) {},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
