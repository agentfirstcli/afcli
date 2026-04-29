package report

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// passFindings produces n synthetic pass findings — sufficient to drive
// ScoreReport into the green band when n >= 1 and total = n.
func passFindings(n int) []Finding {
	out := make([]Finding, n)
	for i := range out {
		out[i] = Finding{
			PrincipleID: "P-test",
			Title:       "test",
			Category:    "test",
			Status:      StatusPass,
			Kind:        KindAutomated,
			Severity:    SeverityLow,
		}
	}
	return out
}

func mixedFindings(pass, fail int) []Finding {
	out := make([]Finding, 0, pass+fail)
	out = append(out, passFindings(pass)...)
	for i := 0; i < fail; i++ {
		out = append(out, Finding{
			PrincipleID: "P-fail",
			Title:       "fail",
			Category:    "test",
			Status:      StatusFail,
			Kind:        KindAutomated,
			Severity:    SeverityHigh,
		})
	}
	return out
}

func TestRenderBadgeSVGContainsLabel(t *testing.T) {
	r := &Report{Findings: passFindings(5)}
	var buf bytes.Buffer
	if err := RenderBadgeSVG(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderBadgeSVG: %v", err)
	}
	if !strings.Contains(buf.String(), "agent-first") {
		t.Errorf("expected label %q in SVG output, got:\n%s", "agent-first", buf.String())
	}
}

func TestRenderBadgeSVGScoreText(t *testing.T) {
	r := &Report{Findings: mixedFindings(7, 3)}
	var buf bytes.Buffer
	if err := RenderBadgeSVG(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderBadgeSVG: %v", err)
	}
	if !strings.Contains(buf.String(), "7/10") {
		t.Errorf("expected score text %q in SVG output, got:\n%s", "7/10", buf.String())
	}
}

func TestRenderBadgeSVGColorBands(t *testing.T) {
	cases := []struct {
		name      string
		pass      int
		fail      int
		wantColor string
	}{
		{"green-100pct", 10, 0, "#4c1"},
		{"green-90pct", 9, 1, "#4c1"},
		{"yellow-89pct", 89, 11, "#dfb317"},
		{"yellow-70pct", 7, 3, "#dfb317"},
		{"red-69pct", 69, 31, "#e05d44"},
		{"red-0pct", 0, 5, "#e05d44"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Report{Findings: mixedFindings(tc.pass, tc.fail)}
			var buf bytes.Buffer
			if err := RenderBadgeSVG(&buf, r, RenderOptions{}); err != nil {
				t.Fatalf("RenderBadgeSVG: %v", err)
			}
			if !strings.Contains(buf.String(), tc.wantColor) {
				t.Errorf("expected color %q in SVG output for %s, got:\n%s",
					tc.wantColor, tc.name, buf.String())
			}
		})
	}
}

func TestRenderBadgeSVGDeterministic(t *testing.T) {
	r := &Report{
		ManifestVersion: "1",
		AfcliVersion:    "test",
		Target:          "/tmp/contrived",
		StartedAt:       "2026-01-01T00:00:00Z",
		DurationMs:      42,
		Findings:        mixedFindings(8, 2),
	}
	opts := RenderOptions{Deterministic: true}

	var a, b bytes.Buffer
	if err := RenderBadgeSVG(&a, r, opts); err != nil {
		t.Fatalf("first render: %v", err)
	}
	if err := RenderBadgeSVG(&b, r, opts); err != nil {
		t.Fatalf("second render: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Errorf("renders diverged\n--- first ---\n%s\n--- second ---\n%s", a.String(), b.String())
	}
}

// TestRenderBadgeSVGWellFormedXML parses the rendered SVG via the
// stdlib XML decoder. A valid SVG body is well-formed XML; this guards
// against accidental tag drift, unclosed elements, and unescaped sentinels.
func TestRenderBadgeSVGWellFormedXML(t *testing.T) {
	r := &Report{Findings: mixedFindings(8, 2)}
	var buf bytes.Buffer
	if err := RenderBadgeSVG(&buf, r, RenderOptions{}); err != nil {
		t.Fatalf("RenderBadgeSVG: %v", err)
	}
	dec := xml.NewDecoder(&buf)
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("SVG is not well-formed XML: %v", err)
		}
	}
}

// TestRenderBadgeSVGXMLEscapes is the defensive escaping guard. The
// production score formula always produces purely numeric "N/N" messages,
// but the renderer must still escape every dynamic string so a future
// caller cannot break out of an attribute value. We exercise the package-
// private buildBadgeSVG helper with a contrived "<bad>" message and assert
// the raw "<bad>" substring does not survive into the output.
func TestRenderBadgeSVGXMLEscapes(t *testing.T) {
	out := buildBadgeSVG("agent-first", "<bad>", "#4c1")

	if strings.Contains(out, "<bad>") {
		t.Errorf("raw %q leaked into SVG output:\n%s", "<bad>", out)
	}
	if !strings.Contains(out, "&lt;bad&gt;") {
		t.Errorf("expected escaped %q in SVG output, got:\n%s", "&lt;bad&gt;", out)
	}

	dec := xml.NewDecoder(strings.NewReader(out))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("escaped SVG is not well-formed XML: %v", err)
		}
	}
}
