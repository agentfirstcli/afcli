package manifest

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
)

// Render writes m to w in the requested format. Supported formats:
//
//   - "json" (default; also picked for "" and unknown values): pretty-printed
//     {"manifest_version": ..., "principles": [...]} with stable field order.
//   - "text": one tab-separated line per principle, in P-number order.
//   - "markdown": header listing the manifest version, then per-category H2
//     sections (categories sorted alphabetically, principles by number).
//
// The deterministic flag is plumbed through for forward-compatibility — the
// embedded manifest carries no timestamps/durations today, so the parameter
// changes nothing right now, but downstream consumers (S07 golden file) rely
// on the same threading the report renderers already use.
func Render(out io.Writer, m *Manifest, format string, deterministic bool) error {
	switch format {
	case "text":
		return renderText(out, m)
	case "markdown":
		return renderMarkdown(out, m)
	case "json", "":
		return renderJSON(out, m)
	default:
		return renderJSON(out, m)
	}
}

type renderEntry struct {
	PrincipleID string `json:"principle_id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Tagline     string `json:"tagline"`
	Category    string `json:"category"`
}

type renderEnvelope struct {
	ManifestVersion string        `json:"manifest_version"`
	Principles      []renderEntry `json:"principles"`
}

func toEntries(m *Manifest) []renderEntry {
	out := make([]renderEntry, 0, len(m.Principles))
	ps := append([]Principle(nil), m.Principles...)
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Number < ps[j].Number })
	for _, p := range ps {
		out = append(out, renderEntry{
			PrincipleID: p.PrincipleID(),
			Number:      p.Number,
			Title:       p.Title,
			Tagline:     p.Tagline,
			Category:    p.Category,
		})
	}
	return out
}

func renderJSON(w io.Writer, m *Manifest) error {
	env := renderEnvelope{
		ManifestVersion: m.Version,
		Principles:      toEntries(m),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

func renderText(w io.Writer, m *Manifest) error {
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
	for _, e := range toEntries(m) {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.PrincipleID, e.Title, e.Category, e.Tagline); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func renderMarkdown(w io.Writer, m *Manifest) error {
	if _, err := fmt.Fprintf(w, "# afcli manifest (v%s)\n\n", m.Version); err != nil {
		return err
	}

	byCategory := make(map[string][]Principle)
	for _, p := range m.Principles {
		byCategory[p.Category] = append(byCategory[p.Category], p)
	}

	cats := make([]string, 0, len(byCategory))
	for c := range byCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	for _, cat := range cats {
		ps := append([]Principle(nil), byCategory[cat]...)
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Number < ps[j].Number })
		if _, err := fmt.Fprintf(w, "## %s\n\n", cat); err != nil {
			return err
		}
		for _, p := range ps {
			if _, err := fmt.Fprintf(w, "**%s — %s**: %s\n", p.PrincipleID(), p.Title, p.Tagline); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s\n\n", p.URL); err != nil {
				return err
			}
		}
	}
	return nil
}
