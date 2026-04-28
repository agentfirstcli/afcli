// Package exit defines afcli's semantic exit codes and the mapper that
// turns a Report into a process exit code. Codes are part of the public
// CLI contract — agents and CI gates switch on them.
package exit

import "github.com/agentfirstcli/afcli/internal/report"

// Exit codes. See M001 CONTEXT for rationale.
const (
	OK                  = 0
	FindingsAtThreshold = 1
	Usage               = 2
	CouldNotAudit       = 3
	Internal            = 4
	Interrupted         = 130
)

// severityRank orders Severity values for threshold comparison. Unknown
// severities sort below "low" so a malformed manifest cannot accidentally
// trip the threshold gate.
func severityRank(s report.Severity) int {
	switch s {
	case report.SeverityLow:
		return 1
	case report.SeverityMedium:
		return 2
	case report.SeverityHigh:
		return 3
	case report.SeverityCritical:
		return 4
	default:
		return 0
	}
}

// MapFromReport returns the process exit code for a finished audit.
//
// Precedence (highest first):
//  1. Interrupted flag → Interrupted (130)
//  2. Error envelope present → mapped from envelope code
//     (USAGE→2, INTERNAL→4, anything else→3)
//  3. Any Finding with Status=="fail" and Severity ≥ threshold
//     → FindingsAtThreshold (1)
//  4. Otherwise → OK (0)
//
// "skip" and "review" findings never trigger a non-zero exit on their own —
// they are signals, not failures. Threshold of "" (empty) is treated as
// SeverityHigh, the documented default.
func MapFromReport(r *report.Report, threshold report.Severity) int {
	if r == nil {
		return Internal
	}
	if r.Interrupted {
		return Interrupted
	}
	if r.Error != nil {
		switch r.Error.Code {
		case report.CodeUsage:
			return Usage
		case report.CodeInternal:
			return Internal
		default:
			return CouldNotAudit
		}
	}
	if threshold == "" {
		threshold = report.SeverityHigh
	}
	floor := severityRank(threshold)
	for _, f := range r.Findings {
		if f.Status == report.StatusFail && severityRank(f.Severity) >= floor {
			return FindingsAtThreshold
		}
	}
	return OK
}
