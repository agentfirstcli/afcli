package audit

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestP3MaskNonCanonical exercises every pattern in p3MaskPatterns
// with at least one positive and one negative case, plus the
// idempotence and sentinel-safety properties that make the mask layer
// safe to apply twice.
func TestP3MaskNonCanonical(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// timestamps
		{"timestamp-rfc3339-utc", "ran at 2024-05-01T12:00:00Z", "ran at <MASKED>"},
		{"timestamp-rfc3339-offset", "started 2024-05-01T12:00:00+02:00", "started <MASKED>"},
		{"timestamp-rfc3339-fractional", "ts=2024-05-01T12:00:00.123456Z", "ts=<MASKED>"},
		{"timestamp-negative-bare-date", "expires 2024-05-01", "expires 2024-05-01"},

		// durations
		{"duration-ms", "took 250ms", "took <MASKED>"},
		{"duration-fractional-seconds", "elapsed 1.5s", "elapsed <MASKED>"},
		{"duration-microseconds-µs", "wait 100µs", "wait <MASKED>"},
		{"duration-hours", "ran for 3h", "ran for <MASKED>"},
		{"duration-negative-bare-number", "count 1234", "count 1234"},
		{"duration-negative-no-unit", "took 250 ms", "took 250 ms"},

		// pids — the regex permits exactly one optional separator
		// (=, :, or single space) between "pid" and the digits.
		{"pid-equals", "child pid=42 spawned", "child <MASKED> spawned"},
		{"pid-colon", "child pid:42 ok", "child <MASKED> ok"},
		{"pid-bare", "child pid 42 ok", "child <MASKED> ok"},
		{"pid-no-separator", "child pid42 ok", "child <MASKED> ok"},
		{"pid-negative-no-digit", "see also pidfile", "see also pidfile"},

		// ports — the ":" arm requires a word-boundary transition
		// before the colon, so the bracketed form only fires when
		// preceded by a word char (e.g. "host:8080").
		{"port-bracketed", "bound localhost:8080 ok", "bound localhost<MASKED> ok"},
		{"port-keyword", "listening on port 49152 now", "listening on <MASKED> now"},
		{"port-negative-three-digits", "bind localhost:80 ok", "bind localhost:80 ok"},
		{"port-negative-zero-pad", "see localhost:01234", "see localhost:01234"},

		// git SHAs
		{"sha-7-chars", "rev abc1234 done", "rev <MASKED> done"},
		{"sha-40-chars", "head 0123456789abcdef0123456789abcdef01234567 cut", "head <MASKED> cut"},
		{"sha-negative-too-short", "tag abc123 v1", "tag abc123 v1"},
		{"sha-negative-uppercase", "tag CAFEBABE v1", "tag CAFEBABE v1"},

		// combinations — fixed order means each token is masked exactly
		// once even though several patterns cooccur.
		{
			"combination-timestamp-duration-pid",
			"2024-05-01T12:00:00Z spawn pid=42 took 1.5s",
			"<MASKED> spawn <MASKED> took <MASKED>",
		},

		// sentinel safety: input already containing the sentinel must
		// pass through unchanged (key for idempotence).
		{"sentinel-passthrough", "raw <MASKED> stays", "raw <MASKED> stays"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskNonCanonical(tc.in)
			if got != tc.want {
				t.Errorf("maskNonCanonical(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Idempotence: a second pass must be a no-op.
			if got2 := maskNonCanonical(got); got2 != got {
				t.Errorf("maskNonCanonical not idempotent: first=%q second=%q", got, got2)
			}
		})
	}
}

// TestP3FirstStructuralDiff covers the equal / structural / borderline
// branches plus the malformed and length-mismatch boundary cases
// called out in the task plan's Negative Tests.
func TestP3FirstStructuralDiff(t *testing.T) {
	cases := []struct {
		name      string
		a, b      string
		wantOK    bool
		wantLine  int
		wantSubEv string // substring expected in evidence (when ok)
	}{
		{"equal-identical", "alpha\nbeta", "alpha\nbeta", false, 0, ""},
		{"equal-empty-both", "", "", false, 0, ""},
		{"empty-vs-nonempty", "", "x", true, 1, "diff at line 1: -\n+x"},
		{"single-line-no-newline", "abc", "xyz", true, 1, "diff at line 1: -abc\n+xyz"},

		// allowlist-only diffs collapse to equal after masking.
		{"allowlist-pid-only", "pid=123", "pid=456", false, 0, ""},
		{
			"allowlist-timestamp-only",
			"line1\nstamp 2024-05-01T12:00:00Z\nline3",
			"line1\nstamp 2024-05-01T13:30:00Z\nline3",
			false, 0, "",
		},

		// structural diffs at the first and a later line.
		{
			"structural-first-line",
			"alpha\nbeta\ngamma",
			"alphX\nbeta\ngamma",
			true, 1, "diff at line 1: -alpha\n+alphX",
		},
		{
			"structural-line-3",
			"alpha\nbeta\ngamma",
			"alpha\nbeta\nDELTA",
			true, 3, "diff at line 3: -gamma\n+DELTA",
		},

		// length mismatch — extra line on either side.
		{
			"length-mismatch-b-longer",
			"a\nb\nc",
			"a\nb\nc\nd",
			true, 4, "diff at line 4: -\n+d",
		},
		{
			"length-mismatch-a-longer",
			"a\nb\nc\nd",
			"a\nb\nc",
			true, 4, "diff at line 4: -d\n+",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line, ev, ok := firstStructuralDiff(tc.a, tc.b)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (line=%d ev=%q)", ok, tc.wantOK, line, ev)
			}
			if !ok {
				return
			}
			if line != tc.wantLine {
				t.Errorf("line = %d, want %d", line, tc.wantLine)
			}
			if !strings.Contains(ev, tc.wantSubEv) {
				t.Errorf("evidence = %q, want substring %q", ev, tc.wantSubEv)
			}
		})
	}
}

// TestP3FormatDiffEvidence locks in the truncation contract: under
// the limit returns the full string; over the limit truncates at
// evidenceLimit bytes; UTF-8 boundary requires backing up so the
// result stays valid UTF-8.
func TestP3FormatDiffEvidence(t *testing.T) {
	t.Run("under-limit", func(t *testing.T) {
		got := formatDiffEvidence(7, "abc", "xyz")
		want := "diff at line 7: -abc\n+xyz"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if len(got) > evidenceLimit {
			t.Errorf("len = %d, must be <= %d", len(got), evidenceLimit)
		}
	})

	t.Run("over-limit-ascii-truncates-cleanly", func(t *testing.T) {
		// Build aLine long enough to push the formatted string past
		// the 200-byte limit using only ASCII so no rune boundary
		// concerns apply.
		aLine := strings.Repeat("a", 500)
		got := formatDiffEvidence(1, aLine, "")
		if len(got) != evidenceLimit {
			t.Errorf("len = %d, want exact %d (ascii cut)", len(got), evidenceLimit)
		}
		if !utf8.ValidString(got) {
			t.Errorf("result is not valid UTF-8: %q", got)
		}
	})

	t.Run("exactly-at-limit", func(t *testing.T) {
		// prefix "diff at line 1: -" = 17 bytes; suffix "\n+" = 2
		// bytes. aLine of 181 ASCII chars makes the total exactly 200.
		aLine := strings.Repeat("a", 181)
		got := formatDiffEvidence(1, aLine, "")
		if len(got) != evidenceLimit {
			t.Errorf("len = %d, want exact %d", len(got), evidenceLimit)
		}
		if got[len(got)-2:] != "\n+" {
			t.Errorf("expected suffix to be preserved at exact limit; got tail %q", got[len(got)-5:])
		}
	})

	t.Run("utf8-boundary-backs-up", func(t *testing.T) {
		// Place a 4-byte rune (𝄞 = U+1D11E) so its bytes occupy
		// positions 198-201 in the formatted string. The naive cut at
		// byte 200 would split the rune; truncateEvidenceUTF8 must
		// back up to byte 198 and produce valid UTF-8.
		// prefix 17 + ASCII filler 181 = 198, then 4-byte rune begins
		// at byte index 198.
		aLine := strings.Repeat("a", 181) + "𝄞" + "tail"
		got := formatDiffEvidence(1, aLine, "")
		if !utf8.ValidString(got) {
			t.Errorf("result is not valid UTF-8: bytes=%v", []byte(got))
		}
		if len(got) != 198 {
			t.Errorf("len = %d, want 198 (backed up before multi-byte rune)", len(got))
		}
		if strings.ContainsRune(got, '𝄞') {
			t.Errorf("result must not include the partially-cut rune: %q", got)
		}
	})

	t.Run("utf8-multibyte-inside-evidence-preserved", func(t *testing.T) {
		// When the multi-byte rune is well within the limit it must
		// survive intact and the result must remain valid UTF-8.
		got := formatDiffEvidence(1, "took 100µs", "took 250µs")
		if !utf8.ValidString(got) {
			t.Errorf("result is not valid UTF-8: %q", got)
		}
		if !strings.Contains(got, "µs") {
			t.Errorf("expected µs to survive: %q", got)
		}
	})
}
