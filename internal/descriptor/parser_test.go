package descriptor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentfirstcli/afcli/internal/report"
)

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "afcli.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func loadTyped(t *testing.T, body string) (*Descriptor, *Error) {
	t.Helper()
	d, err := Load(writeFixture(t, body))
	if err == nil {
		return d, nil
	}
	var typed *Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected *descriptor.Error, got %T: %v", err, err)
	}
	return d, typed
}

func TestLoadValidatesGoldenPath(t *testing.T) {
	body := `format_version: "1"
target: "./bin/foo"
commands:
  safe:
    - "--version"
    - "--help"
  destructive: []
env:
  FOO: bar
skip_principles:
  - P12
relax_principles:
  P7: medium
`
	d, typed := loadTyped(t, body)
	if typed != nil {
		t.Fatalf("expected success, got %v", typed)
	}
	if d.FormatVersion != "1" {
		t.Errorf("FormatVersion = %q, want \"1\"", d.FormatVersion)
	}
	if got := d.Commands.Safe; len(got) != 2 || got[0] != "--version" {
		t.Errorf("Commands.Safe = %v", got)
	}
	if !ShouldSkip(d, "P12") {
		t.Errorf("ShouldSkip(P12) = false, want true")
	}
	cap, ok := RelaxCap(d, "P7")
	if !ok || cap != report.SeverityMedium {
		t.Errorf("RelaxCap(P7) = (%v, %v), want (medium, true)", cap, ok)
	}
}

func TestLoadRejectsUnknownTopLevelKey(t *testing.T) {
	body := `format_version: "1"
foo: bar
`
	_, typed := loadTyped(t, body)
	if typed == nil {
		t.Fatal("expected error for unknown top-level key")
	}
	if typed.Code != CodeDescriptorInvalid {
		t.Errorf("Code = %q, want DESCRIPTOR_INVALID", typed.Code)
	}
	if line, ok := typed.Details["line"].(int); !ok || line < 1 {
		t.Errorf("Details.line = %v (%T), want positive int", typed.Details["line"], typed.Details["line"])
	}
}

func TestLoadRejectsTypeMismatchOnCommandsSafe(t *testing.T) {
	body := `format_version: "1"
commands:
  safe: "git status"
`
	_, typed := loadTyped(t, body)
	if typed == nil {
		t.Fatal("expected type-mismatch error")
	}
	if typed.Code != CodeDescriptorInvalid {
		t.Errorf("Code = %q, want DESCRIPTOR_INVALID", typed.Code)
	}
	if line, ok := typed.Details["line"].(int); !ok || line < 1 {
		t.Errorf("Details.line = %v, want positive int", typed.Details["line"])
	}
}

func TestLoadRejectsUnknownPrincipleInSkip(t *testing.T) {
	body := `format_version: "1"
skip_principles:
  - P99
`
	_, typed := loadTyped(t, body)
	if typed == nil {
		t.Fatal("expected error for unknown principle")
	}
	if typed.Code != CodeDescriptorInvalid {
		t.Errorf("Code = %q, want DESCRIPTOR_INVALID", typed.Code)
	}
	if got := typed.Details["key"]; got != "skip_principles[0]" {
		t.Errorf("Details.key = %v, want skip_principles[0]", got)
	}
}

func TestLoadRejectsBadSeverityValue(t *testing.T) {
	body := `format_version: "1"
relax_principles:
  P7: nuclear
`
	_, typed := loadTyped(t, body)
	if typed == nil {
		t.Fatal("expected error for bad severity")
	}
	if typed.Code != CodeDescriptorInvalid {
		t.Errorf("Code = %q, want DESCRIPTOR_INVALID", typed.Code)
	}
	allowed, ok := typed.Details["allowed"].([]string)
	if !ok {
		t.Fatalf("Details.allowed = %v (%T), want []string", typed.Details["allowed"], typed.Details["allowed"])
	}
	want := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	for _, a := range allowed {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("allowed missing entries: %v (got %v)", want, allowed)
	}
}

func TestLoadRejectsUnsupportedFormatVersion(t *testing.T) {
	body := `format_version: "9"
`
	_, typed := loadTyped(t, body)
	if typed == nil {
		t.Fatal("expected error for bad format_version")
	}
	if typed.Code != CodeDescriptorInvalid {
		t.Errorf("Code = %q, want DESCRIPTOR_INVALID", typed.Code)
	}
	if got := typed.Details["key"]; got != "format_version" {
		t.Errorf("Details.key = %v, want format_version", got)
	}
}

func TestLoadMissingPathReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(filepath.Join(dir, "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	var typed *Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected *descriptor.Error, got %T: %v", err, err)
	}
	if typed.Code != CodeDescriptorNotFound {
		t.Errorf("Code = %q, want DESCRIPTOR_NOT_FOUND", typed.Code)
	}
}

func TestShouldSkipAndRelaxCapAreNilSafe(t *testing.T) {
	if ShouldSkip(nil, "P1") {
		t.Error("ShouldSkip(nil, P1) = true, want false")
	}
	if cap, ok := RelaxCap(nil, "P1"); ok || cap != "" {
		t.Errorf("RelaxCap(nil, P1) = (%v, %v), want (\"\", false)", cap, ok)
	}
	d := &Descriptor{}
	if ShouldSkip(d, "P1") {
		t.Error("ShouldSkip(empty, P1) = true, want false")
	}
	if _, ok := RelaxCap(d, "P1"); ok {
		t.Error("RelaxCap(empty, P1) returned ok=true")
	}
}

func TestLoadParsesCommandsNondeterministic(t *testing.T) {
	body := `format_version: "1"
commands:
  safe:
    - "--top"
    - "--version"
  nondeterministic:
    - "--top"
`
	d, typed := loadTyped(t, body)
	if typed != nil {
		t.Fatalf("expected success, got %v", typed)
	}
	got := d.Commands.Nondeterministic
	if len(got) != 1 || got[0] != "--top" {
		t.Errorf("Commands.Nondeterministic = %v, want [--top]", got)
	}
}

func TestLoadRejectsNondeterministicNotInSafe(t *testing.T) {
	body := `format_version: "1"
commands:
  safe:
    - "--version"
  nondeterministic:
    - "--top"
`
	_, typed := loadTyped(t, body)
	if typed == nil {
		t.Fatal("expected error for nondeterministic entry not in safe")
	}
	if typed.Code != CodeDescriptorInvalid {
		t.Errorf("Code = %q, want DESCRIPTOR_INVALID", typed.Code)
	}
	if got := typed.Details["key"]; got != "commands.nondeterministic[0]" {
		t.Errorf("Details.key = %v, want commands.nondeterministic[0]", got)
	}
	if got := typed.Details["value"]; got != "--top" {
		t.Errorf("Details.value = %v, want --top", got)
	}
}

func TestLoadRejectsNondeterministicTypeMismatch(t *testing.T) {
	body := `format_version: "1"
commands:
  safe:
    - "--top"
  nondeterministic: "--top"
`
	_, typed := loadTyped(t, body)
	if typed == nil {
		t.Fatal("expected type-mismatch error")
	}
	if typed.Code != CodeDescriptorInvalid {
		t.Errorf("Code = %q, want DESCRIPTOR_INVALID", typed.Code)
	}
	if line, ok := typed.Details["line"].(int); !ok || line < 1 {
		t.Errorf("Details.line = %v, want positive int", typed.Details["line"])
	}
}

func TestApplyCapsAndPreserves(t *testing.T) {
	d := &Descriptor{
		FormatVersion:   "1",
		RelaxPrinciples: map[string]string{"P3": "medium"},
	}

	high := report.Finding{PrincipleID: "P3", Severity: report.SeverityHigh}
	Apply(d, &high)
	if high.Severity != report.SeverityMedium {
		t.Errorf("high cap medium = %q, want medium", high.Severity)
	}

	low := report.Finding{PrincipleID: "P3", Severity: report.SeverityLow}
	Apply(d, &low)
	if low.Severity != report.SeverityLow {
		t.Errorf("low cap medium = %q, want low (cap is a ceiling)", low.Severity)
	}

	other := report.Finding{PrincipleID: "P5", Severity: report.SeverityCritical}
	Apply(d, &other)
	if other.Severity != report.SeverityCritical {
		t.Errorf("uncapped principle changed: got %q, want critical", other.Severity)
	}

	// nil-safe.
	Apply(nil, &high)
	Apply(d, nil)
}
