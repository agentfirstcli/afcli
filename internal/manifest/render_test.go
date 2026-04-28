package manifest

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJSONIncludes16Principles(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, Embedded, "json", false); err != nil {
		t.Fatalf("Render: %v", err)
	}
	var env struct {
		ManifestVersion string `json:"manifest_version"`
		Principles      []struct {
			PrincipleID string `json:"principle_id"`
			Number      int    `json:"number"`
			Title       string `json:"title"`
			Tagline     string `json:"tagline"`
			Category    string `json:"category"`
		} `json:"principles"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("Unmarshal: %v\noutput:\n%s", err, buf.String())
	}
	if env.ManifestVersion != Embedded.Version {
		t.Fatalf("manifest_version = %q, want %q", env.ManifestVersion, Embedded.Version)
	}
	if got := len(env.Principles); got != 16 {
		t.Fatalf("len(principles) = %d, want 16", got)
	}
	for i, p := range env.Principles {
		if p.PrincipleID == "" || p.Title == "" || p.Category == "" {
			t.Errorf("principle[%d] missing required field: %+v", i, p)
		}
	}
}

func TestRenderJSONUnknownFormatFallsBackToJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, Embedded, "xml", false); err != nil {
		t.Fatalf("Render: %v", err)
	}
	var env struct {
		Principles []any `json:"principles"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unknown format should fall back to JSON: %v\noutput:\n%s", err, buf.String())
	}
	if len(env.Principles) != 16 {
		t.Fatalf("fallback JSON had %d principles, want 16", len(env.Principles))
	}
}

func TestRenderTextHas16Lines(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, Embedded, "text", false); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := strings.TrimRight(buf.String(), "\n")
	lines := strings.Split(out, "\n")
	if len(lines) != 16 {
		t.Fatalf("text output had %d lines, want 16:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "P1") {
		t.Errorf("first line should reference P1, got %q", lines[0])
	}
}

func TestRenderMarkdownGroupsByCategory(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, Embedded, "markdown", false); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "# afcli manifest (v"+Embedded.Version+")") {
		t.Errorf("markdown should start with versioned H1, got first line: %q", strings.SplitN(out, "\n", 2)[0])
	}
	categories := make(map[string]bool)
	for _, p := range Embedded.Principles {
		categories[p.Category] = true
	}
	for cat := range categories {
		if !strings.Contains(out, "## "+cat+"\n") {
			t.Errorf("missing H2 header for category %q", cat)
		}
	}
}

func TestRenderDeterministicAcrossInvocations(t *testing.T) {
	for _, format := range []string{"json", "text", "markdown"} {
		var a, b bytes.Buffer
		if err := Render(&a, Embedded, format, true); err != nil {
			t.Fatalf("Render(%s) #1: %v", format, err)
		}
		if err := Render(&b, Embedded, format, true); err != nil {
			t.Fatalf("Render(%s) #2: %v", format, err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Fatalf("Render(%s) not byte-identical across calls\nfirst:\n%s\nsecond:\n%s", format, a.String(), b.String())
		}
	}
}

func TestRenderMarkdownCategoryOrderStable(t *testing.T) {
	shuffled := *Embedded
	shuffled.Principles = append([]Principle(nil), Embedded.Principles...)
	// Reverse to disturb input order; markdown grouping must still emit
	// alphabetical category headers.
	for i, j := 0, len(shuffled.Principles)-1; i < j; i, j = i+1, j-1 {
		shuffled.Principles[i], shuffled.Principles[j] = shuffled.Principles[j], shuffled.Principles[i]
	}

	var ref, shuf bytes.Buffer
	if err := Render(&ref, Embedded, "markdown", false); err != nil {
		t.Fatalf("Render reference: %v", err)
	}
	if err := Render(&shuf, &shuffled, "markdown", false); err != nil {
		t.Fatalf("Render shuffled: %v", err)
	}

	headersOf := func(s string) []string {
		var out []string
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "## ") {
				out = append(out, line)
			}
		}
		return out
	}
	refHeaders := headersOf(ref.String())
	shufHeaders := headersOf(shuf.String())
	if len(refHeaders) == 0 {
		t.Fatalf("no H2 headers in reference output")
	}
	if strings.Join(refHeaders, "\n") != strings.Join(shufHeaders, "\n") {
		t.Fatalf("category header order changed under input shuffle\nref:  %v\nshuf: %v", refHeaders, shufHeaders)
	}
}
