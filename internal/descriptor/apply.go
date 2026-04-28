package descriptor

import "github.com/agentfirstcli/afcli/internal/report"

// severityOrdinal maps the four documented severities onto an ordered
// integer scale so RelaxCap can express "no higher than X" via min().
// Unknown severities collapse to 0 — they sort below low and therefore
// never trigger a cap, which keeps Apply a no-op on garbage input.
var severityOrdinal = map[report.Severity]int{
	report.SeverityLow:      1,
	report.SeverityMedium:   2,
	report.SeverityHigh:     3,
	report.SeverityCritical: 4,
}

// ordinalSeverity is the inverse of severityOrdinal, used to turn the
// post-cap ordinal back into a Severity. Order matches severityOrdinal.
var ordinalSeverity = []report.Severity{
	"",                       // 0 — unknown, never written
	report.SeverityLow,       // 1
	report.SeverityMedium,    // 2
	report.SeverityHigh,      // 3
	report.SeverityCritical,  // 4
}

// ShouldSkip reports whether descriptor d marks principle id as
// skip-by-policy. Nil-safe: a nil descriptor never skips anything.
func ShouldSkip(d *Descriptor, id string) bool {
	if d == nil {
		return false
	}
	for _, s := range d.SkipPrinciples {
		if s == id {
			return true
		}
	}
	return false
}

// RelaxCap returns the per-principle severity ceiling configured in
// the descriptor. The bool reports whether a cap was found — a missing
// entry returns "", false and Apply leaves the finding alone.
func RelaxCap(d *Descriptor, id string) (report.Severity, bool) {
	if d == nil || len(d.RelaxPrinciples) == 0 {
		return "", false
	}
	raw, ok := d.RelaxPrinciples[id]
	if !ok {
		return "", false
	}
	return report.Severity(raw), true
}

// Apply caps f.Severity at any descriptor-configured ceiling for its
// principle. Status, Kind, Evidence and Recommendation are left
// untouched — relax_principles is documented as a severity-only knob.
//
// Apply is a no-op when d is nil, when f is nil, or when no cap exists
// for f.PrincipleID. It is idempotent: applying twice with the same
// descriptor produces the same result as applying once.
func Apply(d *Descriptor, f *report.Finding) {
	if d == nil || f == nil {
		return
	}
	cap, ok := RelaxCap(d, f.PrincipleID)
	if !ok {
		return
	}
	capOrd, capKnown := severityOrdinal[cap]
	if !capKnown {
		return
	}
	curOrd, curKnown := severityOrdinal[f.Severity]
	if !curKnown {
		return
	}
	if curOrd <= capOrd {
		return
	}
	f.Severity = ordinalSeverity[capOrd]
}
