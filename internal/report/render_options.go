package report

// RenderOptions controls renderer behavior shared across all output formats.
//
// Deterministic, when true, produces byte-stable output across runs:
// StartedAt and DurationMs are zeroed and Target is normalized to a path
// relative to the current working directory. The S07 self-audit golden
// file depends on this mode being correct from S01 (see MEM003 / D003).
//
// Quiet, when true, suppresses pass and skip findings (and the header
// line) in the text and markdown renderers — leaving only the items that
// need attention (fail / review). JSON output is unaffected: the wire
// contract for machine consumers must always carry the full 16-finding
// envelope. Quiet is the silence affordance the P4 audit principle asks
// agents to expose.
//
// Findings are always stable-sorted by PrincipleID regardless of these
// flags — that ordering is part of the slice-level report contract, not a
// deterministic-only concession.
type RenderOptions struct {
	Deterministic bool
	Quiet         bool
}
