// Score model — single source of truth for the badge surface (S04).
//
// Both badge renderers (SVG and shields.io JSON sidecar) consume the
// ScoreSummary returned by ScoreReport. The formula is locked:
// score counts findings whose status is pass OR skip; review and fail
// do not count. Skip is operator-authorized via descriptor and is
// treated as 'satisfied'; review means 'needs human' and explicitly
// does not contribute to the badge number.
//
// Color thresholds form three bands on pct = score/total:
// green (#4c1) for pct >= 0.90, yellow (#dfb317) for pct >= 0.70,
// red (#e05d44) below that. total == 0 collapses to red with a "0/0"
// message — there is nothing to certify.
package report

import "fmt"

const (
	scoreColorGreen  = "#4c1"
	scoreColorYellow = "#dfb317"
	scoreColorRed    = "#e05d44"

	scoreLabel = "agent-first"

	scoreThresholdGreen  = 0.90
	scoreThresholdYellow = 0.70
)

// ScoreSummary is the locked badge payload. Score is satisfied
// findings (pass + skip), Total is len(findings), Color is the
// hex string consumed by the SVG renderer and shields.io JSON,
// Message is the "<score>/<total>" string rendered as the badge value.
type ScoreSummary struct {
	Score   int
	Total   int
	Color   string
	Message string
}

// ScoreReport computes the locked ScoreSummary for r. Pure: no I/O,
// no descriptor lookup, no capture access. r must be non-nil.
func ScoreReport(r *Report) ScoreSummary {
	total := len(r.Findings)
	score := 0
	for _, f := range r.Findings {
		if f.Status == StatusPass || f.Status == StatusSkip {
			score++
		}
	}

	if total == 0 {
		return ScoreSummary{
			Score:   0,
			Total:   0,
			Color:   scoreColorRed,
			Message: "0/0",
		}
	}

	pct := float64(score) / float64(total)
	color := scoreColorRed
	switch {
	case pct >= scoreThresholdGreen:
		color = scoreColorGreen
	case pct >= scoreThresholdYellow:
		color = scoreColorYellow
	}

	return ScoreSummary{
		Score:   score,
		Total:   total,
		Color:   color,
		Message: fmt.Sprintf("%d/%d", score, total),
	}
}
