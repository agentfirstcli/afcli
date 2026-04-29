package cli

// Test-only exports for white-box assertions in inspect_parse_test.go
// (package cli_test). This file compiles only into the test binary —
// the unexported safeVerbs / destructiveVerbs slices remain unexported
// in production builds.
var (
	SafeVerbs        = safeVerbs
	DestructiveVerbs = destructiveVerbs
)
