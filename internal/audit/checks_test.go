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
// for every registered check when given an empty Help/Bogus capture. Kind
// is allowed to be either KindAutomated (S03 heuristic checks) or
// KindRequiresReview (S06 review-only checks like P2/P5/P9/P11/P12 where
// no automated heuristic is feasible).
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
			if f.Kind != report.KindAutomated && f.Kind != report.KindRequiresReview {
				t.Errorf("Kind = %q, want automated or requires-review", f.Kind)
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

func TestDefaultEngineRegistersAllRealChecks(t *testing.T) {
	eng := DefaultEngine()
	want := []string{
		"P1", "P2", "P3", "P4", "P5", "P6", "P7", "P8",
		"P9", "P10", "P11", "P12", "P13", "P14", "P15", "P16",
	}
	if len(eng.Registry) != len(want) {
		t.Errorf("len(Registry) = %d, want %d", len(eng.Registry), len(want))
	}
	for _, id := range want {
		if _, ok := eng.Registry[id]; !ok {
			t.Errorf("Registry missing %q", id)
		}
	}
}

// TestDefaultEngineProducesAllSixteenRealChecks exercises the full
// DefaultEngine wiring (real registry + injected fake probe) and asserts
// (a) 16 entries in the registry, (b) 16 findings produced by Run, and
// (c) zero findings carry the stub blurb. After T03 every principle has a
// real check; stubCheck is unreachable in production.
func TestDefaultEngineProducesAllSixteenRealChecks(t *testing.T) {
	def := DefaultEngine()
	if len(def.Registry) != 16 {
		t.Fatalf("len(DefaultEngine().Registry) = %d, want 16", len(def.Registry))
	}
	eng := &Engine{
		Registry:     def.Registry,
		ProbeTimeout: def.ProbeTimeout,
		Probe:        fakeOKProbe,
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng.Run(context.Background(), "/fake", r, nil)

	if len(r.Findings) != 16 {
		t.Fatalf("len(findings) = %d, want 16", len(r.Findings))
	}
	for _, f := range r.Findings {
		if strings.Contains(f.Evidence, "no automated check yet") {
			t.Errorf("principle %s: finding still carries stub blurb evidence: %q", f.PrincipleID, f.Evidence)
		}
	}
}

// TestP2 / TestP5 / TestP9 / TestP11 / TestP12 — each unconditionally emits
// status:review, kind:requires-review with principle-specific (non-stub)
// evidence. These checks read no probe data, so they are robust to nil
// captures. Severity is sourced from severityFor.
func TestP2(t *testing.T)  { assertReviewOnlyCheck(t, "P2", checkP2) }
func TestP5(t *testing.T)  { assertReviewOnlyCheck(t, "P5", checkP5) }
func TestP9(t *testing.T)  { assertReviewOnlyCheck(t, "P9", checkP9) }
func TestP11(t *testing.T) { assertReviewOnlyCheck(t, "P11", checkP11) }
func TestP12(t *testing.T) { assertReviewOnlyCheck(t, "P12", checkP12) }

// TestP1 — token-based pass/review. Pass on any --output json /
// --json / --format json variant; review when --help has no structured
// output declaration. The token table is intentionally distinct from
// P15's introspection-affordance scan so the two checks can disagree.
func TestP1(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-output-json", &Capture{Stdout: "  --output json   structured output\n"}, report.StatusPass, "--output json"},
		{"pass-output-ndjson", &Capture{Stdout: "  --output ndjson\n"}, report.StatusPass, "--output ndjson"},
		{"pass-output-yaml", &Capture{Stdout: "  --output yaml\n"}, report.StatusPass, "--output yaml"},
		{"pass-output-csv", &Capture{Stdout: "  --output csv\n"}, report.StatusPass, "--output csv"},
		{"pass-json-flag", &Capture{Stdout: "  --json   machine output\n"}, report.StatusPass, "--json"},
		{"pass-ndjson-flag", &Capture{Stdout: "  --ndjson stream\n"}, report.StatusPass, "--ndjson"},
		{"pass-format-json", &Capture{Stdout: "  --format json\n"}, report.StatusPass, "--format json"},
		{"review-no-format", &Capture{Stdout: "Usage: foo [--verbose]"}, report.StatusReview, "no structured output format"},
		{"review-probe-failed", &Capture{Err: context.DeadlineExceeded}, report.StatusReview, "probe failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P1", tc.help, &Capture{})
			f := checkP1(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
		})
	}
}

// TestP3 — review-only with evidence varying on env.Behavioral length.
// Empty captures yield the "out of scope for v1" rationale; non-empty
// captures yield the "ran but did not compare" rationale. Always
// kind:requires-review.
func TestP3(t *testing.T) {
	cases := []struct {
		name       string
		behavioral []BehavioralCapture
		wantInEv   string
	}{
		{"empty-behavioral", nil, "requires multiple invocations"},
		{"populated-behavioral", []BehavioralCapture{{Cmd: "--version", Capture: &Capture{}}}, "behavioral probes ran"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P3", &Capture{}, &Capture{})
			env.Behavioral = tc.behavioral
			f := checkP3(context.Background(), env)
			if f.Status != report.StatusReview {
				t.Errorf("Status = %q, want %q", f.Status, report.StatusReview)
			}
			if f.Kind != report.KindRequiresReview {
				t.Errorf("Kind = %q, want %q", f.Kind, report.KindRequiresReview)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
			if !strings.Contains(f.Recommendation, "ordering guarantees") {
				t.Errorf("Recommendation = %q, want substring %q", f.Recommendation, "ordering guarantees")
			}
		})
	}
}

func TestP4(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-progress", &Capture{Stdout: "  --progress  show a progress bar\n"}, report.StatusPass, "--progress"},
		{"pass-quiet", &Capture{Stdout: "  --quiet  suppress output\n"}, report.StatusPass, "--quiet"},
		{"pass-silent", &Capture{Stdout: "  --silent\n"}, report.StatusPass, "--silent"},
		{"pass-no-progress", &Capture{Stdout: "  --no-progress\n"}, report.StatusPass, "--no-progress"},
		{"review-no-progress-flags", &Capture{Stdout: "Usage: foo"}, report.StatusReview, "no progress or silence"},
		{"review-probe-failed", &Capture{Err: context.DeadlineExceeded}, report.StatusReview, "probe failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P4", tc.help, &Capture{})
			f := checkP4(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
		})
	}
}

// TestP8 — review-only for v1 (the canonical signal needs a </dev/null
// stdin probe we do not yet run). No probe input is consulted.
func TestP8(t *testing.T) { assertReviewOnlyCheck(t, "P8", checkP8) }

func TestP10(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-dry-run", &Capture{Stdout: "  --dry-run  simulate only\n"}, report.StatusPass, "--dry-run"},
		{"pass-simulate", &Capture{Stdout: "  --simulate\n"}, report.StatusPass, "--simulate"},
		{"pass-check", &Capture{Stdout: "  --check\n"}, report.StatusPass, "--check"},
		{"pass-what-if", &Capture{Stdout: "  --what-if\n"}, report.StatusPass, "--what-if"},
		{"review-no-dry-run", &Capture{Stdout: "Usage: foo [--verbose]"}, report.StatusReview, "no dry-run"},
		{"review-probe-failed", &Capture{Err: context.DeadlineExceeded}, report.StatusReview, "probe failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P10", tc.help, &Capture{})
			f := checkP10(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
		})
	}
}

func TestP13(t *testing.T) {
	cases := []struct {
		name       string
		help       *Capture
		wantStatus report.Status
		wantInEv   string
	}{
		{"pass-version", &Capture{Stdout: "  --version  print version and exit\n"}, report.StatusPass, "--version"},
		{"review-no-version", &Capture{Stdout: "Usage: foo [--help]"}, report.StatusReview, "no --version"},
		{"review-probe-failed", &Capture{Err: context.DeadlineExceeded}, report.StatusReview, "probe failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := envFor(t, "P13", tc.help, &Capture{})
			f := checkP13(context.Background(), env)
			if f.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", f.Status, tc.wantStatus)
			}
			if !strings.Contains(f.Evidence, tc.wantInEv) {
				t.Errorf("Evidence = %q, want substring %q", f.Evidence, tc.wantInEv)
			}
		})
	}
}

func assertReviewOnlyCheck(t *testing.T, id string, check Check) {
	t.Helper()
	env := envFor(t, id, &Capture{}, &Capture{})
	f := check(context.Background(), env)
	if f.PrincipleID != id {
		t.Errorf("PrincipleID = %q, want %q", f.PrincipleID, id)
	}
	if f.Status != report.StatusReview {
		t.Errorf("Status = %q, want review", f.Status)
	}
	if f.Kind != report.KindRequiresReview {
		t.Errorf("Kind = %q, want requires-review", f.Kind)
	}
	if f.Evidence == "" {
		t.Error("Evidence empty")
	}
	if strings.Contains(f.Evidence, "no automated check yet") {
		t.Errorf("Evidence still carries stub blurb: %q", f.Evidence)
	}
	if f.Recommendation == "" {
		t.Error("Recommendation empty")
	}
	if f.Severity != severityFor(id) {
		t.Errorf("Severity = %q, want %q", f.Severity, severityFor(id))
	}
}
