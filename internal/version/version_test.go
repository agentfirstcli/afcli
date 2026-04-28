package version

import (
	"strings"
	"testing"
)

// TestDefaultsAreDevSentinels guards that a vanilla `go build` reports
// the dev sentinels — the goreleaser pipeline relies on these defaults
// being detectable so a missing -ldflags injection is loud.
func TestDefaultsAreDevSentinels(t *testing.T) {
	if Version != "0.0.0-dev" {
		t.Errorf("Version default = %q, want %q", Version, "0.0.0-dev")
	}
	if Commit != "unknown" {
		t.Errorf("Commit default = %q, want %q", Commit, "unknown")
	}
	if Date != "unknown" {
		t.Errorf("Date default = %q, want %q", Date, "unknown")
	}
}

func TestStringFormat(t *testing.T) {
	got := String()
	if !strings.HasPrefix(got, "afcli ") {
		t.Errorf("String() = %q, want prefix %q", got, "afcli ")
	}
	for _, want := range []string{Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing field %q", got, want)
		}
	}
}
