// Badge JSON sidecar — shields.io endpoint shape consumed by the
// `afcli audit --badge` surface. This is the SOLE place in afcli that
// emits camelCase JSON keys (schemaVersion, label, message, color):
// shields.io's endpoint protocol is byte-defined upstream, so matching
// it here keeps a future shields.io endpoint integration drop-in clean.
// All other afcli JSON surfaces are snake_case.
package report

import (
	"encoding/json"
	"io"
)

// BadgeSidecar is the shields.io endpoint-shaped payload. SchemaVersion,
// Label, Message, and Color are the four fields shields.io requires;
// Score and Total are afcli extensions for programmatic consumers that
// need the raw numbers instead of parsing Message.
type BadgeSidecar struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
	Score         int    `json:"score"`
	Total         int    `json:"total"`
}

// RenderBadgeJSON writes a shields.io endpoint-shaped JSON sidecar for
// r to w. Output is byte-stable for any given (r, opts) pair: the same
// normalizeReport pipeline used by RenderJSON runs first, the score is
// computed via the locked ScoreReport formula, and encoding/json with
// SetIndent("", "  ") is deterministic given fixed struct field order.
//
// The sidecar deliberately omits the audit Target — coupling the badge
// to a path breaks fork portability (S04-RESEARCH §Open Questions item 4).
func RenderBadgeJSON(w io.Writer, r *Report, opts RenderOptions) error {
	out := normalizeReport(r, opts)
	sum := ScoreReport(out)
	sidecar := BadgeSidecar{
		SchemaVersion: 1,
		Label:         scoreLabel,
		Message:       sum.Message,
		Color:         sum.Color,
		Score:         sum.Score,
		Total:         sum.Total,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sidecar)
}
