//go:build tools
// +build tools

// Package tools tracks build-time-only dependencies that are not
// imported by production code but are required by `go build` of test
// fixtures under testdata/. Files in testdata/ are invisible to
// `go mod tidy`, so without this blank import the urfave/cli/v2
// require line would be silently removed from go.mod, breaking the
// build of testdata/fixtures/urfave-cli. The `tools` build tag keeps
// this file out of every regular build while remaining visible to
// `go mod tidy` (which considers files under all build tags).
package tools

import (
	_ "github.com/urfave/cli/v2"
)
