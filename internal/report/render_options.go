package report

// RenderOptions controls renderer behavior shared across all output formats.
//
// Deterministic, when true, produces byte-stable output across runs:
// StartedAt and DurationMs are zeroed and Target is normalized to a path
// relative to the current working directory. The S07 self-audit golden
// file depends on this mode being correct from S01 (see MEM003 / D003).
//
// Findings are always stable-sorted by PrincipleID regardless of this
// flag — that ordering is part of the slice-level report contract, not a
// deterministic-only concession.
type RenderOptions struct {
	Deterministic bool
}
