package report

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// RenderJSON writes r to w as indented JSON matching the documented
// snake_case wire contract. The input report is not mutated — a defensive
// copy is taken before normalization. Findings are always stable-sorted
// by PrincipleID; deterministic mode additionally zeroes timestamps and
// rewrites Target to a path relative to the current working directory.
func RenderJSON(w io.Writer, r *Report, opts RenderOptions) error {
	out := normalizeReport(r, opts)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// normalizeReport returns a copy of r with renderer-invariant
// transformations applied. Exported renderers (text, markdown — T04)
// share this so the same input always yields the same logical report
// across formats.
func normalizeReport(r *Report, opts RenderOptions) *Report {
	cp := *r
	if len(r.Findings) == 0 {
		// Preserve `[]` over `null` in JSON — schema and S07 golden
		// file expect an empty array on zero-check reports.
		cp.Findings = []Finding{}
	} else {
		cp.Findings = make([]Finding, len(r.Findings))
		copy(cp.Findings, r.Findings)
		sort.SliceStable(cp.Findings, func(i, j int) bool {
			return cp.Findings[i].PrincipleID < cp.Findings[j].PrincipleID
		})
	}
	if opts.Deterministic {
		cp.StartedAt = ""
		cp.DurationMs = 0
		if cp.Target != "" {
			if cwd, err := os.Getwd(); err == nil {
				if rel, err := filepath.Rel(cwd, cp.Target); err == nil {
					cp.Target = rel
				}
			}
		}
	}
	return &cp
}
