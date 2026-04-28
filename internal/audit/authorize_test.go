package audit

import (
	"errors"
	"strings"
	"testing"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/report"
)

func TestAuthorizeProbeExactMatchAccepts(t *testing.T) {
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	if err := authorizeProbe(d, []string{"--version"}); err != nil {
		t.Fatalf("expected nil for exact match, got %v", err)
	}
}

func TestAuthorizeProbeMultiArgExactMatchAccepts(t *testing.T) {
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"status --short"}},
	}
	if err := authorizeProbe(d, []string{"status", "--short"}); err != nil {
		t.Fatalf("expected nil for joined exact match, got %v", err)
	}
}

func TestAuthorizeProbePrefixSupersetIsRejected(t *testing.T) {
	// Safe contains only ["--version"] but candidate is ["--version", "--quiet"].
	// Joined-string equality fails, so PROBE_DENIED.
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	err := authorizeProbe(d, []string{"--version", "--quiet"})
	var ae *AuthError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AuthError, got %T %v", err, err)
	}
	if ae.Code != report.CodeProbeDenied {
		t.Errorf("Code = %q, want %q", ae.Code, report.CodeProbeDenied)
	}
	if !strings.Contains(ae.Reason, "not in commands.safe") {
		t.Errorf("Reason = %q, want substring %q", ae.Reason, "not in commands.safe")
	}
	if ae.Cmd != "--version --quiet" {
		t.Errorf("Cmd = %q, want %q", ae.Cmd, "--version --quiet")
	}
}

func TestAuthorizeProbeDestructiveOverlapRejected(t *testing.T) {
	// --burn appears in BOTH Safe and Destructive — defense in depth: any
	// match against Destructive denies even when also listed in Safe.
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{
			Safe:        []string{"--burn"},
			Destructive: []string{"--burn"},
		},
	}
	err := authorizeProbe(d, []string{"--burn"})
	var ae *AuthError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AuthError, got %T %v", err, err)
	}
	if !strings.Contains(ae.Reason, "matches commands.destructive") {
		t.Errorf("Reason = %q, want substring %q", ae.Reason, "matches commands.destructive")
	}
	if ae.Code != report.CodeProbeDenied {
		t.Errorf("Code = %q, want %q", ae.Code, report.CodeProbeDenied)
	}
}

func TestAuthorizeProbeNilDescriptorRejected(t *testing.T) {
	err := authorizeProbe(nil, []string{"--version"})
	var ae *AuthError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AuthError, got %T %v", err, err)
	}
	if ae.Code != report.CodeProbeDenied {
		t.Errorf("Code = %q, want %q", ae.Code, report.CodeProbeDenied)
	}
}

func TestAuthorizeProbeEmptySafeRejected(t *testing.T) {
	d := &descriptor.Descriptor{}
	err := authorizeProbe(d, []string{"--version"})
	var ae *AuthError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AuthError, got %T %v", err, err)
	}
	if !strings.Contains(ae.Reason, "not in commands.safe") {
		t.Errorf("Reason = %q, want substring %q", ae.Reason, "not in commands.safe")
	}
}

func TestAuthorizeProbeEmptyCandidateRejected(t *testing.T) {
	// strings.Fields(" ") yields []string{}; the engine skips empty argv
	// before calling authorize, but authorize itself must still reject.
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	err := authorizeProbe(d, nil)
	var ae *AuthError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AuthError, got %T %v", err, err)
	}
	if ae.Cmd != "" {
		t.Errorf("Cmd = %q, want empty", ae.Cmd)
	}
}

func TestAuthorizeProbeWhitespaceOnlySafeEntryNeverMatches(t *testing.T) {
	// A whitespace-only Safe entry post-joining is "   "; no real
	// candidate's joined form will ever equal that, so the candidate is
	// rejected. (The engine separately skips whitespace-only entries via
	// strings.Fields, but authorize must not mis-fire if they slip in.)
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"   "}},
	}
	err := authorizeProbe(d, []string{"--version"})
	if err == nil {
		t.Fatal("expected rejection for non-matching candidate")
	}
}

func TestAuthErrorMessageShape(t *testing.T) {
	ae := &AuthError{Code: report.CodeProbeDenied, Cmd: "--burn", Reason: "matches commands.destructive"}
	want := "PROBE_DENIED: matches commands.destructive: --burn"
	if got := ae.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
