package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// RenderText writes r to w as a human-readable plain-text summary.
//
// The shape is: a single header line, an optional error block, and one
// line per finding with evidence/recommendation indented underneath.
// Findings are stable-sorted by PrincipleID; deterministic mode zeroes
// timestamps and rewrites Target to a path relative to the cwd — the
// normalization is shared with RenderJSON and RenderMarkdown.
func RenderText(w io.Writer, r *Report, opts RenderOptions) error {
	out := normalizeReport(r, opts)

	header := fmt.Sprintf("afcli %s | manifest %s | target=%s | duration=%dms",
		out.AfcliVersion, out.ManifestVersion, out.Target, out.DurationMs)
	if out.Interrupted {
		header += " | interrupted"
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}

	if out.Error != nil {
		if err := writeTextError(w, out.Error); err != nil {
			return err
		}
	}

	if len(out.Findings) == 0 {
		if out.Error == nil {
			_, err := fmt.Fprintln(w, "no findings")
			return err
		}
		return nil
	}

	for _, f := range out.Findings {
		if _, err := fmt.Fprintf(w, "[%s] %s %s — %s (%s)\n",
			f.Status, f.PrincipleID, f.Title, f.Category, f.Severity); err != nil {
			return err
		}
		if f.Evidence != "" {
			if err := writeIndented(w, "  evidence: ", f.Evidence); err != nil {
				return err
			}
		}
		if f.Recommendation != "" {
			if err := writeIndented(w, "  recommendation: ", f.Recommendation); err != nil {
				return err
			}
		}
		if f.Hint != "" {
			if err := writeIndented(w, "  hint: ", f.Hint); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeTextError(w io.Writer, e *ErrorEnvelope) error {
	if _, err := fmt.Fprintln(w, "error:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  code: %s\n", e.Code); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  message: %s\n", e.Message); err != nil {
		return err
	}
	if e.Hint != "" {
		if _, err := fmt.Fprintf(w, "  hint: %s\n", e.Hint); err != nil {
			return err
		}
	}
	if len(e.Details) > 0 {
		if _, err := fmt.Fprintln(w, "  details:"); err != nil {
			return err
		}
		for _, k := range sortedKeys(e.Details) {
			if _, err := fmt.Fprintf(w, "    %s: %v\n", k, e.Details[k]); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeIndented(w io.Writer, prefix, body string) error {
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if i == 0 {
			if _, err := fmt.Fprintln(w, prefix+ln); err != nil {
				return err
			}
			continue
		}
		// Continuation lines align under the prefix's whitespace.
		pad := strings.Repeat(" ", len(prefix))
		if _, err := fmt.Fprintln(w, pad+ln); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
