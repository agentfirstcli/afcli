// Package audit checks: P6 (Semantic Exit Codes), P7 (Parseable Errors),
// P14 (Capability Negotiation), P15 (Machine-Readable Help), and P16
// (Signal Danger). See .gsd/milestones/M001/slices/S03/S03-RESEARCH.md
// §"Manifest findings" for the per-principle pass/fail/review heuristics.
//
// Every check reads only env.Help / env.Bogus — there is no probe
// re-invocation. Probe failures (Capture.Err != nil) short-circuit to a
// requires-review finding so a probe outage never masquerades as a
// substantive verdict.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agentfirstcli/afcli/internal/report"
)

const evidenceLimit = 200

var p6Re = regexp.MustCompile(`(?i)\b(exit code|exit status|returns 0|returns 1)\b|\bexit \d|\b(EXIT STATUS|EXIT CODES)\b`)

var p7StderrSignals = []string{
	"error:",
	"Error:",
	"unknown option",
	"invalid argument",
	"usage:",
}

var p14Tokens = []string{
	"--help-schema",
	"--capabilities",
	"--features",
	"--version",
	" version ",
}

var p15Tokens = []string{
	"--help-schema",
	"--output json",
	"--help --format json",
	"--json",
}

var p16Tokens = []string{
	"--force",
	"--no-confirm",
	"--no-prompt",
	"destructive",
	"permanent",
	"cannot be undone",
	"irreversible",
}

// baseFinding builds the manifest-derived finding skeleton used by every
// real check. Status / Kind / Evidence / Recommendation are filled by
// the caller so each branch is self-contained and never overwrites a
// default it does not own.
func baseFinding(env *CheckEnv) report.Finding {
	id := env.Principle.PrincipleID()
	return report.Finding{
		PrincipleID: id,
		Title:       env.Principle.Title,
		Category:    env.Principle.Category,
		Severity:    severityFor(id),
		Hint:        env.Principle.URL,
	}
}

// combined concatenates a probe's stdout and stderr, joined by a newline,
// for substring/regex matching. Both streams are scanned because some
// CLIs emit help to stderr.
func combined(c *Capture) string {
	if c == nil {
		return ""
	}
	return c.Stdout + "\n" + c.Stderr
}

// firstNonEmptyLine returns the first non-empty trimmed line in s,
// truncated to evidenceLimit characters.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		return truncateEvidence(t)
	}
	return ""
}

func truncateEvidence(s string) string {
	if len(s) > evidenceLimit {
		return s[:evidenceLimit]
	}
	return s
}

// p6MatchedLine returns the first line of s containing a P6 regex match,
// trimmed and truncated. Falls back to empty when no line matches.
func p6MatchedLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if p6Re.MatchString(line) {
			return truncateEvidence(strings.TrimSpace(line))
		}
	}
	return ""
}

// probeFailedFinding builds a requires-review finding for the case where
// the underlying Capture.Err is non-nil. Evidence carries the err string
// verbatim so a future agent can correlate timeouts with the probe layer.
func probeFailedFinding(env *CheckEnv, err error) report.Finding {
	f := baseFinding(env)
	f.Status = report.StatusReview
	f.Kind = report.KindRequiresReview
	f.Evidence = fmt.Sprintf("probe failed: %v", err)
	f.Recommendation = "re-run audit when the probe can be captured"
	return f
}

func checkP6(_ context.Context, env *CheckEnv) report.Finding {
	if env.Help != nil && env.Help.Err != nil {
		return probeFailedFinding(env, env.Help.Err)
	}

	f := baseFinding(env)
	f.Kind = report.KindAutomated

	text := combined(env.Help)
	if line := p6MatchedLine(text); line != "" {
		f.Status = report.StatusPass
		f.Evidence = line
		f.Recommendation = "keep documenting exit-code semantics in --help"
		return f
	}

	f.Status = report.StatusReview
	f.Evidence = "--help does not document exit-code semantics"
	f.Recommendation = "document exit codes in --help under an EXIT STATUS section"
	return f
}

func checkP7(_ context.Context, env *CheckEnv) report.Finding {
	if env.Bogus != nil && env.Bogus.Err != nil {
		return probeFailedFinding(env, env.Bogus.Err)
	}

	f := baseFinding(env)
	f.Kind = report.KindAutomated

	stderrLine := firstNonEmptyLine(env.Bogus.Stderr)
	evidence := fmt.Sprintf("exit=%d; stderr[0]=%s", env.Bogus.ExitCode, stderrLine)

	if env.Bogus.ExitCode == 0 {
		f.Status = report.StatusFail
		f.Evidence = evidence
		f.Recommendation = "exit non-zero on unknown flags so callers can detect failure"
		return f
	}

	if env.Bogus.Stderr == "" || stderrLine == "" {
		f.Status = report.StatusReview
		f.Evidence = evidence
		f.Recommendation = "emit a structured error to stderr on unknown-flag rejection"
		return f
	}

	if isStructuredErrorLine(stderrLine, env.Target) {
		f.Status = report.StatusPass
		f.Evidence = evidence
		f.Recommendation = "preserve the structured error shape so parsers can rely on it"
		return f
	}

	f.Status = report.StatusReview
	f.Evidence = evidence
	f.Recommendation = "prefix unknown-flag errors with the binary name or a recognizable label"
	return f
}

func isStructuredErrorLine(line, target string) bool {
	if base := filepath.Base(target); base != "" && base != "." && strings.HasPrefix(line, base+":") {
		return true
	}
	for _, sig := range p7StderrSignals {
		if strings.Contains(line, sig) {
			return true
		}
	}
	if strings.HasPrefix(line, "{") && json.Valid([]byte(line)) {
		return true
	}
	return false
}

func checkP14(_ context.Context, env *CheckEnv) report.Finding {
	if env.Help != nil && env.Help.Err != nil {
		return probeFailedFinding(env, env.Help.Err)
	}

	f := baseFinding(env)
	f.Kind = report.KindAutomated

	text := combined(env.Help)
	for _, tok := range p14Tokens {
		if strings.Contains(text, tok) {
			f.Status = report.StatusPass
			f.Evidence = truncateEvidence(strings.TrimSpace(tok))
			f.Recommendation = "keep advertising version/capability negotiation in --help"
			return f
		}
	}

	f.Status = report.StatusReview
	f.Evidence = "no capability-negotiation affordance found in --help"
	f.Recommendation = "expose --version, --capabilities, or --help-schema for callers to negotiate"
	return f
}

func checkP15(_ context.Context, env *CheckEnv) report.Finding {
	if env.Help != nil && env.Help.Err != nil {
		return probeFailedFinding(env, env.Help.Err)
	}

	f := baseFinding(env)
	f.Kind = report.KindAutomated

	text := combined(env.Help)
	for _, tok := range p15Tokens {
		if strings.Contains(text, tok) {
			f.Status = report.StatusPass
			f.Evidence = truncateEvidence(tok)
			f.Recommendation = "keep advertising machine-readable help in --help"
			return f
		}
	}

	f.Status = report.StatusReview
	f.Evidence = "no machine-readable help affordance found in --help"
	f.Recommendation = "expose --help-schema or --output json for machine consumption"
	return f
}

func checkP16(_ context.Context, env *CheckEnv) report.Finding {
	if env.Help != nil && env.Help.Err != nil {
		return probeFailedFinding(env, env.Help.Err)
	}

	f := baseFinding(env)
	f.Kind = report.KindAutomated

	textLower := strings.ToLower(combined(env.Help))
	for _, tok := range p16Tokens {
		if strings.Contains(textLower, strings.ToLower(tok)) {
			f.Status = report.StatusPass
			f.Evidence = truncateEvidence(tok)
			f.Recommendation = "continue surfacing destructive-operation warnings in --help"
			return f
		}
	}

	f.Status = report.StatusReview
	f.Evidence = "no danger or confirmation-bypass keywords found in --help"
	f.Recommendation = "document destructive operations and confirmation-bypass flags in --help"
	return f
}
