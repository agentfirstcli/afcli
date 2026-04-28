package manifest

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
)

func TestEmbeddedHas16Principles(t *testing.T) {
	if got := len(Embedded.Principles); got != 16 {
		t.Fatalf("len(Embedded.Principles) = %d, want 16", got)
	}
}

func TestEmbeddedVersion(t *testing.T) {
	if Embedded.Version != "0.0.1" {
		t.Fatalf("Embedded.Version = %q, want %q", Embedded.Version, "0.0.1")
	}
}

func TestPrincipleIDFormat(t *testing.T) {
	re := regexp.MustCompile(`^P\d+$`)
	seen := make(map[string]bool, 16)
	for _, p := range Embedded.Principles {
		id := p.PrincipleID()
		if !re.MatchString(id) {
			t.Errorf("principle %d: PrincipleID() = %q, does not match ^P\\d+$", p.Number, id)
		}
		if seen[id] {
			t.Errorf("duplicate principle id: %q", id)
		}
		seen[id] = true
	}
	for i := 1; i <= 16; i++ {
		id := "P" + itoa(i)
		if !seen[id] {
			t.Errorf("missing principle id %q (set must be dense P1..P16)", id)
		}
	}
}

func TestCategoriesNonEmpty(t *testing.T) {
	for _, p := range Embedded.Principles {
		if p.Category == "" {
			t.Errorf("principle %s (%q) has empty Category", p.PrincipleID(), p.Title)
		}
	}
}

func TestRoundTripStable(t *testing.T) {
	b, err := json.Marshal(Embedded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundTrip Manifest
	if err := json.Unmarshal(b, &roundTrip); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*Embedded, roundTrip) {
		t.Fatalf("round-trip mismatch:\n got:  %+v\n want: %+v", roundTrip, *Embedded)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
