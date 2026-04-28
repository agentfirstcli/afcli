package audit

import "github.com/agentfirstcli/afcli/internal/report"

// severityFor returns the manifest-aligned severity for a principle ID.
// S03 hard-codes a small table; S06 may refine. Default is medium so a
// missing entry never accidentally trips the high-severity threshold.
func severityFor(id string) report.Severity {
	switch id {
	case "P6", "P7", "P15", "P16":
		return report.SeverityHigh
	case "P14":
		return report.SeverityMedium
	default:
		return report.SeverityMedium
	}
}
