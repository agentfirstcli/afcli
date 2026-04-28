package audit

import (
	"context"
	"strings"
	"testing"

	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
)

// envFor constructs a CheckEnv anchored at the real principle entry from
// manifest.Embedded so the manifest-derived fields (Title/Category/URL)
// in the resulting Finding are populated as they would be in production.
func envFor(t *testing.T, id string, help, bogus *Capture) *CheckEnv {
	t.Helper()
	for _, p := range manifest.Embedded.Principles {
		if p.PrincipleID() == id {
			return &CheckEnv{Target: "/usr/bin/git", Principle: p, Help: help, Bogus: bogus}
		}
	}
	t.Fatalf("envFor: unknown principle %q", id)
	return nil
}

// TestP6 — see S03-RESEARCH.md §"Reality check on git": git's top-level
// --help does NOT mention exit codes, so against /usr/bin/git this check
// lands review (still a real, deterministic finding).
func TestP6(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-exit-code-mention", &Capture{Stdout: "exit code 0 means success\n"}, report.StatusPass, "exit code"},
		{"pass-EXIT-STATUS-header", &Capture{Stdout: "blah\nEXIT STATUS\nfoo\n"}, report.StatusPass, "EXIT STATUS"},
		{"review-empty-help", &Capture{}, report.StatusReview, "does not document"},
		{"review-no-mention", &Capture{Stdout: "git is a content tracker"}, report.StatusReview, "does not document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P6", tc.help, &Capture{})
			f := checkP6(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
			if f.Severity != report.SeverityHigh {
				t.Errorf("Severity = %q, want high", f.Severity)
			}
		})
	}
}

// TestP7 — against /usr/bin/git, T03's integration test will see pass
// (git rejects unknown flags with `unknown option:` on stderr).
func TestP7(t *testing.T) {
	cases := []struct {
		name       string
		bogus      *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{
			"pass-structured-stderr",
			&Capture{ExitCode: 129, Stderr: "unknown option: --bogus\nusage: git ..."},
			report.StatusPass, "unknown option",
		},
		{
			"pass-binary-prefix",
			&Capture{ExitCode: 1, Stderr: "git: 'foo' is not a git command"},
			report.StatusPass, "git:",
		},
		{
			"pass-json-stderr",
			&Capture{ExitCode: 1, Stderr: `{"error":"bad flag"}`},
			report.StatusPass, `{"error"`,
		},
		{
			"fail-zero-exit",
			&Capture{ExitCode: 0, Stderr: ""},
			report.StatusFail, "exit=0",
		},
		{
			"review-empty-stderr",
			&Capture{ExitCode: 1, Stderr: ""},
			report.StatusReview, "exit=1",
		},
		{
			"review-unstructured-prose",
			&Capture{ExitCode: 1, Stderr: "oops something happened"},
			report.StatusReview, "oops something",
		},
		{
			"review-probe-failed",
			&Capture{Err: context.DeadlineExceeded},
			report.StatusReview, "probe failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P7", &Capture{}, tc.bogus)
			f := checkP7(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
			if f.Severity != report.SeverityHigh {
				t.Errorf("Severity = %q, want high", f.Severity)
			}
		})
	}
}

// TestP14 — against /usr/bin/git, expect pass via `[-v | --version]`.
func TestP14(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-version-flag", &Capture{Stdout: "Usage: foo [--version]"}, report.StatusPass, "--version"},
		{"pass-capabilities-flag", &Capture{Stdout: "  --capabilities  list features\n"}, report.StatusPass, "--capabilities"},
		{"pass-help-schema-flag", &Capture{Stdout: "  --help-schema   emit metadata\n"}, report.StatusPass, "--help-schema"},
		{"review-no-version", &Capture{Stdout: "just a tool that does things"}, report.StatusReview, "no capability"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P14", tc.help, &Capture{})
			f := checkP14(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
			if f.Severity != report.SeverityMedium {
				t.Errorf("Severity = %q, want medium", f.Severity)
			}
		})
	}
}

// TestP15 — against git's top-level help expect review (no machine-readable
// affordance); this is still a real finding kind=automated.
func TestP15(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-help-schema", &Capture{Stdout: "  --help-schema  emit JSON\n"}, report.StatusPass, "--help-schema"},
		{"pass-output-json", &Capture{Stdout: "  --output json\n"}, report.StatusPass, "--output json"},
		{"pass-json-flag", &Capture{Stdout: "  --json   machine-readable output\n"}, report.StatusPass, "--json"},
		{"review-only-text-help", &Capture{Stdout: "Usage: foo [--verbose] [--help]"}, report.StatusReview, "no machine-readable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P15", tc.help, &Capture{})
			f := checkP15(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
		})
	}
}

// TestP16 — never fails in S03 (research §P16 — uncertainty is review,
// S05 refines via probes). Against git's top-level help expect review.
func TestP16(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-force-flag", &Capture{Stdout: "  -f, --force   skip prompts\n"}, report.StatusPass, "--force"},
		{"pass-destructive-warning", &Capture{Stdout: "WARNING: this is destructive."}, report.StatusPass, "destructive"},
		{"pass-irreversible", &Capture{Stdout: "Operation is IRREVERSIBLE."}, report.StatusPass, "irreversible"},
		{"review-no-warnings", &Capture{Stdout: "List items in a directory"}, report.StatusReview, "no danger"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P16", tc.help, &Capture{})
			f := checkP16(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
		})
	}
}

// TestP6BinaryGarbageDoesNotPanic guards against malformed-input regressions:
// a Capture stuffed with binary bytes must traverse firstNonEmptyLine /
// p6MatchedLine without crashing the engine.
func TestP6BinaryGarbageDoesNotPanic(t *testing.T) {
	garbage := string([]byte{0x00, 0x01, 0xFF, 0xFE, '\n', 0xC3, 0x28, '\n'})
	env := envFor(t, "P6", &Capture{Stdout: garbage}, &Capture{})
	f := checkP6(context.Background(), env)
	if f.PrincipleID != "P6" {
		t.Errorf("PrincipleID = %q, want P6", f.PrincipleID)
	}
}

// TestAllChecksProduceWellFormedFindings asserts the Finding shape contract
// for every registered check when given an empty Help/Bogus capture.
func TestAllChecksProduceWellFormedFindings(t *testing.T) {
	eng := DefaultEngine()
	for id, check := range eng.Registry {
		t.Run(id, func(t *testing.T) {
			env := envFor(t, id, &Capture{}, &Capture{})
			f := check(context.Background(), env)
			if f.PrincipleID != id {
				t.Errorf("PrincipleID = %q, want %q", f.PrincipleID, id)
			}
			if f.Title == "" {
				t.Error("Title empty")
			}
			if f.Category == "" {
				t.Error("Category empty")
			}
			if f.Status == "" {
				t.Error("Status empty")
			}
			if f.Kind != report.KindAutomated {
				t.Errorf("Kind = %q, want automated", f.Kind)
			}
			if f.Severity != severityFor(id) {
				t.Errorf("Severity = %q, want %q", f.Severity, severityFor(id))
			}
			if f.Evidence == "" {
				t.Error("Evidence empty")
			}
			if f.Recommendation == "" {
				t.Error("Recommendation empty")
			}
			if f.Hint == "" {
				t.Error("Hint empty (Principle.URL should populate it)")
			}
		})
	}
}

func TestDefaultEngineRegistersFiveChecks(t *testing.T) {
	eng := DefaultEngine()
	want := []string{"P6", "P7", "P14", "P15", "P16"}
	if len(eng.Registry) != len(want) {
		t.Errorf("len(Registry) = %d, want %d", len(eng.Registry), len(want))
	}
	for _, id := range want {
		if _, ok := eng.Registry[id]; !ok {
			t.Errorf("Registry missing %q", id)
		}
	}
}

// TestDefaultEngineProducesFiveAutomatedAndElevenStubFindings exercises
// the full DefaultEngine wiring (real registry + injected fake probe) and
// asserts the kind split: 5 automated (P6/P7/P14/P15/P16) + 11 stub
// (KindRequiresReview) findings, totalling 16.
func TestDefaultEngineProducesFiveAutomatedAndElevenStubFindings(t *testing.T) {
	def := DefaultEngine()
	eng := &Engine{
		Registry:     def.Registry,
		ProbeTimeout: def.ProbeTimeout,
		Probe:        fakeOKProbe,
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng.Run(context.Background(), "/fake", r)

	if len(r.Findings) != 16 {
		t.Fatalf("len(findings) = %d, want 16", len(r.Findings))
	}
	var automated, review int
	for _, f := range r.Findings {
		switch f.Kind {
		case report.KindAutomated:
			automated++
		case report.KindRequiresReview:
			review++
		}
	}
	if automated != 5 {
		t.Errorf("automated = %d, want 5", automated)
	}
	if review != 11 {
		t.Errorf("review = %d, want 11", review)
	}
}
