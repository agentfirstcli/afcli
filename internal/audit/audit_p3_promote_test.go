package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/report"
)

// scriptedProbe returns a Capture per (argv-key, call-index) pair so a
// single probe can answer differently on the first vs the second
// invocation for the same argv (the rerun path). The scripted entries
// are consumed in order; once the script for an argv is exhausted, the
// last entry is replayed.
type scriptedProbe struct {
	help    *Capture
	bogus   *Capture
	scripts map[string][]*Capture
	calls   map[string]int
}

func newScriptedProbe() *scriptedProbe {
	return &scriptedProbe{
		help:    &Capture{Stdout: "fake stdout", ExitCode: 0},
		bogus:   &Capture{Stdout: "fake stdout", ExitCode: 0},
		scripts: map[string][]*Capture{},
		calls:   map[string]int{},
	}
}

func (sp *scriptedProbe) on(argv string, results ...*Capture) {
	sp.scripts[argv] = results
}

func (sp *scriptedProbe) probe(_ context.Context, _ string, args []string, _ time.Duration, _ map[string]string) *Capture {
	if len(args) == 1 && args[0] == "--help" {
		c := *sp.help
		c.Args = args
		return &c
	}
	if len(args) == 1 && args[0] == bogusFlagArg {
		c := *sp.bogus
		c.Args = args
		return &c
	}
	key := strings.Join(args, " ")
	idx := sp.calls[key]
	sp.calls[key] = idx + 1
	results := sp.scripts[key]
	if len(results) == 0 {
		return &Capture{Args: args, Stdout: "", ExitCode: 0}
	}
	if idx >= len(results) {
		idx = len(results) - 1
	}
	c := *results[idx]
	c.Args = args
	return &c
}

// TestP3PromoteEqualStdoutEmitsAutomatedPass — fake probe returns
// identical stdout twice → P3 promotes to kind=automated, status=pass.
// Evidence carries the rerun count and the command list so an operator
// can correlate the verdict to the safe entry that produced it.
func TestP3PromoteEqualStdoutEmitsAutomatedPass(t *testing.T) {
	sp := newScriptedProbe()
	sp.on("--version",
		&Capture{Stdout: "afcli 1.2.3\n", ExitCode: 0},
		&Capture{Stdout: "afcli 1.2.3\n", ExitCode: 0},
	)
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        sp.probe,
	}
	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if p3.Status != report.StatusPass {
		t.Errorf("P3.Status = %q, want %q", p3.Status, report.StatusPass)
	}
	if p3.Kind != report.KindAutomated {
		t.Errorf("P3.Kind = %q, want %q", p3.Kind, report.KindAutomated)
	}
	if !strings.Contains(p3.Evidence, "deterministic: 1/1") {
		t.Errorf("P3.Evidence = %q must mention rerun count 1/1", p3.Evidence)
	}
	if !strings.Contains(p3.Evidence, "--version") {
		t.Errorf("P3.Evidence = %q must mention the command --version", p3.Evidence)
	}
	if p3.Severity != severityFor("P3") {
		t.Errorf("P3.Severity = %q, want %q (preserved via baseFinding)", p3.Severity, severityFor("P3"))
	}
	if p3.Title == "" || p3.Category == "" {
		t.Errorf("P3 must inherit Title/Category from manifest; got Title=%q Category=%q", p3.Title, p3.Category)
	}
}

// TestP3PromoteStructuralDiffEmitsAutomatedFail — runs differ on a
// non-allowlisted line → kind=automated, status=fail. Evidence carries
// the diff line and stays under the 200-char evidenceLimit.
func TestP3PromoteStructuralDiffEmitsAutomatedFail(t *testing.T) {
	sp := newScriptedProbe()
	sp.on("--version",
		&Capture{Stdout: "a\nb\nc", ExitCode: 0},
		&Capture{Stdout: "a\nx\nc", ExitCode: 0},
	)
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        sp.probe,
	}
	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if p3.Status != report.StatusFail {
		t.Errorf("P3.Status = %q, want %q", p3.Status, report.StatusFail)
	}
	if p3.Kind != report.KindAutomated {
		t.Errorf("P3.Kind = %q, want %q", p3.Kind, report.KindAutomated)
	}
	if !strings.Contains(p3.Evidence, "diff at line 2:") {
		t.Errorf("P3.Evidence = %q must contain 'diff at line 2:'", p3.Evidence)
	}
	if len(p3.Evidence) > 200 {
		t.Errorf("P3.Evidence length = %d, want <= 200 (evidenceLimit)", len(p3.Evidence))
	}
	if !strings.Contains(p3.Recommendation, "commands.nondeterministic") {
		t.Errorf("P3.Recommendation = %q must point at commands.nondeterministic escape hatch", p3.Recommendation)
	}
}

// TestP3PromoteAllowlistOnlyEmitsReview — runs differ only on an
// allowlisted timestamp. The mask collapses both into the same
// canonical form, so the verdict is requires-review (we did NOT prove
// determinism) and the evidence carries the masked sentinel so an
// operator sees what varied without seeing the raw timestamp.
func TestP3PromoteAllowlistOnlyEmitsReview(t *testing.T) {
	sp := newScriptedProbe()
	sp.on("--status",
		&Capture{Stdout: "started 2026-04-29T10:00:00Z", ExitCode: 0},
		&Capture{Stdout: "started 2026-04-29T10:00:01Z", ExitCode: 0},
	)
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--status"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        sp.probe,
	}
	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if p3.Status != report.StatusReview {
		t.Errorf("P3.Status = %q, want %q", p3.Status, report.StatusReview)
	}
	if p3.Kind != report.KindRequiresReview {
		t.Errorf("P3.Kind = %q, want %q", p3.Kind, report.KindRequiresReview)
	}
	if !strings.Contains(p3.Evidence, p3MaskSentinel) {
		t.Errorf("P3.Evidence = %q must contain mask sentinel %q", p3.Evidence, p3MaskSentinel)
	}
	if !strings.Contains(p3.Evidence, "masked diff (allowlisted variation)") {
		t.Errorf("P3.Evidence = %q must label the variation as allowlisted", p3.Evidence)
	}
	if !strings.Contains(p3.Recommendation, "commands.nondeterministic") {
		t.Errorf("P3.Recommendation = %q must point at the escape hatch", p3.Recommendation)
	}
}

// TestP3PromoteSkipPolicyWins — descriptor skips P3 + Safe authorizes
// a deterministic argv. The skip-by-policy verdict must survive: this
// aggregator MUST NOT replace a StatusSkip finding under any
// circumstance. Mirrors TestProbeEvidenceSkipPreservedOverAggregator.
func TestP3PromoteSkipPolicyWins(t *testing.T) {
	sp := newScriptedProbe()
	sp.on("--version",
		&Capture{Stdout: "afcli 1.2.3\n", ExitCode: 0},
		&Capture{Stdout: "afcli 1.2.3\n", ExitCode: 0},
	)
	d := &descriptor.Descriptor{
		SkipPrinciples: []string{"P3"},
		Commands:       descriptor.Commands{Safe: []string{"--version"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        sp.probe,
	}
	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if p3.Status != report.StatusSkip {
		t.Errorf("P3.Status = %q, want %q (skip-by-policy must win over promotion)", p3.Status, report.StatusSkip)
	}
	if p3.Evidence != "skipped per descriptor" {
		t.Errorf("P3.Evidence = %q, want %q", p3.Evidence, "skipped per descriptor")
	}
}

// TestP3PromoteFailureAggregatorWins — one safe entry hangs (timeout),
// one is deterministic. The failure aggregator runs first and stamps
// "probe timeout:" evidence; the promotion aggregator MUST detect the
// prefix and no-op so the failure verdict survives.
func TestP3PromoteFailureAggregatorWins(t *testing.T) {
	probe := func(_ context.Context, _ string, args []string, _ time.Duration, _ map[string]string) *Capture {
		if len(args) == 1 && (args[0] == "--help" || args[0] == bogusFlagArg) {
			return &Capture{Args: args, Stdout: "ok", ExitCode: 0}
		}
		if len(args) == 1 && args[0] == "--hang" {
			return &Capture{Args: args, Err: errProbeTimeout, Duration: 250 * time.Millisecond, ExitCode: -1}
		}
		return &Capture{Args: args, Stdout: "deterministic", ExitCode: 0}
	}
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--hang", "--version"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        probe,
	}
	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if !strings.HasPrefix(p3.Evidence, "probe timeout:") {
		t.Errorf("P3.Evidence = %q must start with 'probe timeout:' (failure aggregator wins)", p3.Evidence)
	}
	if p3.Status != report.StatusReview {
		t.Errorf("P3.Status = %q, want %q (failure aggregator stamps review)", p3.Status, report.StatusReview)
	}
}

// TestP3PromoteNondeterministicOptOutEmitsReview — descriptor lists the
// argv in BOTH safe and nondeterministic. The first probe runs (so the
// aggregator can see the captured stdout) but the rerun is suppressed.
// Without a second observation we cannot prove determinism, so the
// verdict is requires-review with explicit "opt-out:" evidence.
func TestP3PromoteNondeterministicOptOutEmitsReview(t *testing.T) {
	sp := newScriptedProbe()
	sp.on("--top", &Capture{Stdout: "process list\n", ExitCode: 0})
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{
			Safe:             []string{"--top"},
			Nondeterministic: []string{"--top"},
		},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        sp.probe,
	}
	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if p3.Status != report.StatusReview {
		t.Errorf("P3.Status = %q, want %q", p3.Status, report.StatusReview)
	}
	if p3.Kind != report.KindRequiresReview {
		t.Errorf("P3.Kind = %q, want %q", p3.Kind, report.KindRequiresReview)
	}
	if !strings.Contains(p3.Evidence, "opt-out: --top") {
		t.Errorf("P3.Evidence = %q must contain 'opt-out: --top'", p3.Evidence)
	}
}

// TestP3PromoteRerunOnlyFailureLeavesStub — first probe succeeds, the
// rerun returns errProbeTimeout. The failure aggregator only walks
// Capture.Err and so leaves P3 alone; the promotion aggregator must
// detect Rerun.Err != nil and decline so the checkP3 stub review
// survives. This is the documented gap (see T04 plan §Failure Modes).
func TestP3PromoteRerunOnlyFailureLeavesStub(t *testing.T) {
	sp := newScriptedProbe()
	sp.on("--version",
		&Capture{Stdout: "afcli 1.2.3\n", ExitCode: 0},
		&Capture{Args: []string{"--version"}, Err: errProbeTimeout, Duration: 250 * time.Millisecond, ExitCode: -1},
	)
	d := &descriptor.Descriptor{
		Commands: descriptor.Commands{Safe: []string{"--version"}},
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     DefaultEngine().Registry,
		ProbeEnabled: true,
		ProbeTimeout: time.Second,
		Probe:        sp.probe,
	}
	eng.Run(context.Background(), "/fake", r, d)

	p3 := findP3(t, r)
	if p3.Status != report.StatusReview {
		t.Errorf("P3.Status = %q, want %q (stub survives)", p3.Status, report.StatusReview)
	}
	if p3.Kind != report.KindRequiresReview {
		t.Errorf("P3.Kind = %q, want %q", p3.Kind, report.KindRequiresReview)
	}
	// Stub evidence ends with "out of scope for v1" — distinguishes it
	// from automated/pass evidence ("deterministic: N/N reruns ...")
	// and from any aggregator-stamped evidence.
	if !strings.Contains(p3.Evidence, "out of scope for v1") {
		t.Errorf("P3.Evidence = %q must be the checkP3 stub text (rerun-only failure leaves stub)", p3.Evidence)
	}
}
