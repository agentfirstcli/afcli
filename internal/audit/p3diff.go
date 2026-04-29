// P3 Deterministic Ordering — diff/mask layer.
//
// This file is the regex-locked false-positive surface for the
// probe-and-rerun mechanic that promotes P3 from requires-review to
// automated. The module is intentionally pure: no *Capture, no
// *descriptor.Descriptor, no I/O. Inputs are two strings (stdout of run
// A and stdout of run B); outputs are (kind, evidence) discriminating
// equal / allowlist-only-diff / structural-diff.
//
// Determinism is structural: regex order is fixed (timestamp →
// duration → pid → port → sha), the sentinel is a constant, and the
// replacement is byte-stable so AFCLI_DETERMINISTIC=1 callers can rely
// on identical evidence across reruns.
package audit

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// p3MaskSentinel replaces every allowlisted non-canonical token. It is
// deliberately uppercase and bracket-wrapped so none of the mask
// patterns can match it again — this is what makes maskNonCanonical
// idempotent.
const p3MaskSentinel = "<MASKED>"

// p3MaskPatterns are applied in fixed order on every call. Order is
// part of the contract: the first pattern that consumes a span wins,
// and the locked order ensures the same input always yields the same
// masked output. Each pattern targets one canonical noise source the
// borderline path must tolerate.
var p3MaskPatterns = []*regexp.Regexp{
	// RFC3339 timestamps: 2006-01-02T15:04:05, optional fractional
	// seconds, optional Z or ±hh:mm offset.
	regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`),
	// Go-style duration literals: 1.5s, 250ms, 100µs, 3h, ...
	regexp.MustCompile(`\b\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h)\b`),
	// PID markers: pid=42, pid: 42, pid 42, pid42.
	regexp.MustCompile(`\bpid[=: ]?\d+\b`),
	// Ephemeral ports: "port 8080", "port: 49152", ":54321" (4-5 digit
	// ports starting with 1-9).
	regexp.MustCompile(`\b(?:port|:)\s*[1-9]\d{3,4}\b`),
	// Git SHAs: 7-40 lowercase hex chars on word boundaries.
	regexp.MustCompile(`\b[0-9a-f]{7,40}\b`),
}

// maskNonCanonical applies p3MaskPatterns in fixed order, replacing
// each match with p3MaskSentinel. Idempotent: maskNonCanonical applied
// twice yields the same result as once because the sentinel matches
// none of the patterns.
func maskNonCanonical(s string) string {
	for _, re := range p3MaskPatterns {
		s = re.ReplaceAllString(s, p3MaskSentinel)
	}
	return s
}

// firstStructuralDiff masks both inputs, splits each on '\n', and
// walks pairwise. Returns the 1-based line number of the first
// differing pair, the formatted evidence string for that pair, and
// ok=true. When the masked outputs are byte-identical (no structural
// diff — either truly equal or differing only on allowlisted noise),
// returns (0, "", false). Length-mismatch lines count as diffs: the
// shorter side contributes "" for the missing line.
func firstStructuralDiff(a, b string) (line int, evidence string, ok bool) {
	ma := maskNonCanonical(a)
	mb := maskNonCanonical(b)
	if ma == mb {
		return 0, "", false
	}
	la := strings.Split(ma, "\n")
	lb := strings.Split(mb, "\n")
	n := len(la)
	if len(lb) > n {
		n = len(lb)
	}
	for i := 0; i < n; i++ {
		var aLine, bLine string
		if i < len(la) {
			aLine = la[i]
		}
		if i < len(lb) {
			bLine = lb[i]
		}
		if aLine != bLine {
			return i + 1, formatDiffEvidence(i+1, aLine, bLine), true
		}
	}
	// Unreachable: ma != mb implies at least one line differs.
	return 0, "", false
}

// formatDiffEvidence builds the canonical "diff at line N: -<a>\n+<b>"
// evidence string and truncates to evidenceLimit at a UTF-8 rune
// boundary so a multi-byte rune straddling the cutoff never produces
// invalid UTF-8 in the report.
func formatDiffEvidence(line int, aLine, bLine string) string {
	return truncateEvidenceUTF8(fmt.Sprintf("diff at line %d: -%s\n+%s", line, aLine, bLine))
}

// truncateEvidenceUTF8 caps s at evidenceLimit bytes, backing up to
// the previous rune-start byte if the cut would land in the middle of
// a multi-byte rune. Mirrors truncateEvidence's contract for ASCII
// input but is rune-safe for the diff path, which can carry user
// stdout containing arbitrary UTF-8.
func truncateEvidenceUTF8(s string) string {
	if len(s) <= evidenceLimit {
		return s
	}
	end := evidenceLimit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}
