// Package cli — inspect_parse.go
//
// ParseHelp classifies tokens discovered in a target binary's --help
// output into three disjoint slices: safe verbs (read-only —
// list/get/show/status/version/...), destructive candidates
// (delete/apply/push/...), and a flat list of candidate subcommands
// used by T03's recursive walker. The destructive-verb dictionary is
// locked by TestInspectVerbDictionaryLocked so accidental additions
// are visible in code review.
//
// The parser has zero I/O and zero cobra/audit dependencies. The hard
// rule, locked by test: a token only lands in `safe` when it appears
// in safeVerbs AND not in destructiveVerbs — destructive precedence
// wins.

package cli

import (
	"regexp"
	"slices"
	"strings"
)

// safeVerbs are canonical read-only verbs that may be surfaced in the
// emitted descriptor's commands.safe[] block. A token must appear in
// this list AND NOT in destructiveVerbs to be classified as safe.
var safeVerbs = []string{
	"list",
	"get",
	"show",
	"status",
	"describe",
	"version",
	"help",
	"ls",
}

// destructiveVerbs mutate state. A token in this list is ALWAYS routed
// to destructiveCandidates (surfaced in the descriptor as a `# REVIEW:`
// marker, never in the active commands.safe[] block). Additions land
// in code review via the locked-dictionary unit test.
var destructiveVerbs = []string{
	"delete",
	"rm",
	"destroy",
	"drop",
	"purge",
	"reset",
	"wipe",
	"force",
	"kill",
	"apply",
	"create",
	"update",
	"set",
	"push",
	"publish",
	"deploy",
}

// verbRe matches plausible CLI subcommand verbs: lowercase letters,
// digits, and hyphens. Tokens like "help," (urfave's "help, h" alias
// row) are intentionally rejected — the parser errs toward dropping
// uncertain candidates rather than misclassifying them.
var verbRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// classify routes a candidate token. destructiveVerbs wins over
// safeVerbs by explicit precedence so that even a token mistakenly
// listed in both never lands in the active safe list.
func classify(token string) (isSafe, isDestructive bool) {
	if slices.Contains(destructiveVerbs, token) {
		return false, true
	}
	if slices.Contains(safeVerbs, token) {
		return true, false
	}
	return false, false
}

// ParseHelp scans the --help output of a target binary and returns
// three slices: safe verbs (classified into safeVerbs, with destructive
// precedence applied), candidate subcommands (every plausible verb,
// regardless of classification — used by the recursive walker), and
// destructive candidates.
//
// Block detection is case-sensitive against real Cobra/urfave output:
// Cobra prints "Available Commands:", urfave prints "COMMANDS:". The
// block ends at the first blank line or the first non-indented line.
// Inputs without a recognized block (e.g. flag.Parse-style help) yield
// empty outputs — the classifier degrades gracefully.
func ParseHelp(text string) (safe []string, subcommands []string, destructiveCandidates []string) {
	inBlock := false
	for _, line := range strings.Split(text, "\n") {
		trimmedRight := strings.TrimRight(line, " \t\r")
		if !inBlock {
			if trimmedRight == "Available Commands:" || trimmedRight == "COMMANDS:" {
				inBlock = true
			}
			continue
		}
		if trimmedRight == "" {
			inBlock = false
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inBlock = false
			continue
		}
		fields := strings.Fields(trimmedRight)
		if len(fields) == 0 {
			continue
		}
		token := strings.ToLower(fields[0])
		if !verbRe.MatchString(token) {
			continue
		}
		subcommands = append(subcommands, token)
		isSafe, isDestructive := classify(token)
		switch {
		case isDestructive:
			destructiveCandidates = append(destructiveCandidates, token)
		case isSafe:
			safe = append(safe, token)
		}
	}
	return safe, subcommands, destructiveCandidates
}
