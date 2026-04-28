// Package version exposes the binary's release identity. The variables
// are populated at build time via -ldflags '-X' from goreleaser; an
// unbuilt `go build ./cmd/afcli` falls back to the dev sentinels so an
// unreleased binary still reports an honest, parseable identity.
package version

import "fmt"

// Version is the semantic version of the build (e.g. "0.1.0"). The
// "0.0.0-dev" sentinel marks an unreleased local build.
var Version = "0.0.0-dev"

// Commit is the short git SHA the binary was built from. "unknown"
// when no -ldflags injection happened.
var Commit = "unknown"

// Date is the RFC3339 build timestamp. "unknown" when no -ldflags
// injection happened.
var Date = "unknown"

// String returns a single human-readable identity line suitable for
// `afcli --version` output.
func String() string {
	return fmt.Sprintf("afcli %s (%s, built %s)", Version, Commit, Date)
}
