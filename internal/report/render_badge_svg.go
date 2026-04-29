// Badge SVG renderer — shields.io-flat-style badge consumed by the
// `afcli audit --badge` surface. Geometry is fixed (label 80px, message
// 50px, total 130px) so we never measure text and never depend on
// font-rendering nondeterminism. All dynamic strings flow through
// html.EscapeString defensively, even though ScoreSummary.Message is
// always numeric ("N/N") — the locked formula in score.go forbids
// non-digit characters, but escaping protects against future drift.
//
// Determinism contract: same Report + same RenderOptions ⇒ byte-identical
// output. No time, no map iteration, no float formatting, no locale.
package report

import (
	"html"
	"io"
	"strings"
)

// badgeSVGTemplate is a literal shields.io flat-style template. Width is
// fixed at 130px (label rect 80, message rect 50). Placeholders are
// double-underscored sentinels replaced by html-escaped values.
const badgeSVGTemplate = `<svg xmlns="http://www.w3.org/2000/svg" width="130" height="20" role="img" aria-label="__ARIA__">
  <title>__ARIA__</title>
  <linearGradient id="s" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r">
    <rect width="130" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#r)">
    <rect width="80" height="20" fill="#555"/>
    <rect x="80" width="50" height="20" fill="__COLOR__"/>
    <rect width="130" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="11">
    <text x="40" y="14">__LABEL__</text>
    <text x="105" y="14">__MESSAGE__</text>
  </g>
</svg>
`

// RenderBadgeSVG writes a shields.io-flat-style SVG badge for r to w.
// Output is byte-stable for any given (r, opts) pair: normalizeReport
// applies identical transformations to those used by RenderJSON, the
// score is computed via the locked ScoreReport formula, and all dynamic
// strings are spliced into a literal template via strings.ReplaceAll
// after html.EscapeString — no time, map iteration, or locale-sensitive
// formatting touches the output.
func RenderBadgeSVG(w io.Writer, r *Report, opts RenderOptions) error {
	out := normalizeReport(r, opts)
	sum := ScoreReport(out)
	body := buildBadgeSVG(scoreLabel, sum.Message, sum.Color)
	_, err := io.WriteString(w, body)
	return err
}

// buildBadgeSVG splices label / message / color into the literal SVG
// template. Defensively html-escapes every substitution so a future
// caller passing user-controlled text cannot break out of an attribute
// value or inject markup. Exposed package-private for tests that need
// to feed contrived inputs without going through ScoreReport.
func buildBadgeSVG(label, message, color string) string {
	s := badgeSVGTemplate
	s = strings.ReplaceAll(s, "__ARIA__", html.EscapeString(label+": "+message))
	s = strings.ReplaceAll(s, "__LABEL__", html.EscapeString(label))
	s = strings.ReplaceAll(s, "__MESSAGE__", html.EscapeString(message))
	s = strings.ReplaceAll(s, "__COLOR__", html.EscapeString(color))
	return s
}
