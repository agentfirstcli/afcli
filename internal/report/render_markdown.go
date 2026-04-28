package report

import (
	"fmt"
	"io"
	"sort"
)

// RenderMarkdown writes r to w as a CommonMark document.
//
// Layout: an `# afcli audit report` heading, a metadata table, an optional
// error block, then per-finding `### ` sections grouped by category.
// Findings are stable-sorted by PrincipleID; categories iterate in
// alphabetical order so the same input always yields the same output.
// Deterministic mode shares normalization with RenderJSON / RenderText.
func RenderMarkdown(w io.Writer, r *Report, opts RenderOptions) error {
	out := normalizeReport(r, opts)

	if _, err := fmt.Fprintln(w, "# afcli audit report"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "| field | value |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | --- |"); err != nil {
		return err
	}
	rows := [][2]string{
		{"manifest_version", out.ManifestVersion},
		{"afcli_version", out.AfcliVersion},
		{"target", out.Target},
		{"duration_ms", fmt.Sprintf("%d", out.DurationMs)},
		{"interrupted", fmt.Sprintf("%t", out.Interrupted)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "| %s | %s |\n", row[0], row[1]); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	if out.Error != nil {
		if err := writeMarkdownError(w, out.Error); err != nil {
			return err
		}
	}

	if len(out.Findings) == 0 {
		if out.Error == nil {
			if _, err := fmt.Fprintln(w, "_no findings_"); err != nil {
				return err
			}
		}
		return nil
	}

	groups := groupByCategory(out.Findings)
	cats := make([]string, 0, len(groups))
	for c := range groups {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	for _, cat := range cats {
		if _, err := fmt.Fprintf(w, "## %s\n\n", cat); err != nil {
			return err
		}
		for _, f := range groups[cat] {
			if _, err := fmt.Fprintf(w, "### %s — %s\n\n", f.PrincipleID, f.Title); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "- status: `%s`\n", f.Status); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "- kind: `%s`\n", f.Kind); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "- severity: `%s`\n", f.Severity); err != nil {
				return err
			}
			if f.Evidence != "" {
				if _, err := fmt.Fprintf(w, "- evidence: %s\n", f.Evidence); err != nil {
					return err
				}
			}
			if f.Recommendation != "" {
				if _, err := fmt.Fprintf(w, "- recommendation: %s\n", f.Recommendation); err != nil {
					return err
				}
			}
			if f.Hint != "" {
				if _, err := fmt.Fprintf(w, "- hint: %s\n", f.Hint); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeMarkdownError(w io.Writer, e *ErrorEnvelope) error {
	if _, err := fmt.Fprintln(w, "## error"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- code: `%s`\n", e.Code); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- message: %s\n", e.Message); err != nil {
		return err
	}
	if e.Hint != "" {
		if _, err := fmt.Fprintf(w, "- hint: %s\n", e.Hint); err != nil {
			return err
		}
	}
	if len(e.Details) > 0 {
		if _, err := fmt.Fprintln(w, "- details:"); err != nil {
			return err
		}
		for _, k := range sortedKeys(e.Details) {
			if _, err := fmt.Fprintf(w, "  - `%s`: %v\n", k, e.Details[k]); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return nil
}

func groupByCategory(findings []Finding) map[string][]Finding {
	groups := make(map[string][]Finding)
	for _, f := range findings {
		groups[f.Category] = append(groups[f.Category], f)
	}
	return groups
}
