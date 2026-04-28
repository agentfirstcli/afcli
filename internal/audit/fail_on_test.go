package audit

import (
	"context"
	"testing"
	"time"

	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/report"
)

// TestFailOnEngineProducesFailFindingMapsToOne wires a check that returns
// status=fail/severity=critical and confirms exit.MapFromReport returns 1
// at every threshold low|medium|high|critical. The Execute() never-bool
// short-circuit lives in internal/cli; here we just prove the engine's
// fail finding survives Run() with the right shape so the threshold gate
// can act on it. Subprocess coverage of --fail-on=never lives in
// internal/cli/fail_on_test.go.
func TestFailOnEngineProducesFailFindingMapsToOne(t *testing.T) {
	failingCheck := func(_ context.Context, env *CheckEnv) report.Finding {
		f := baseFinding(env)
		f.Status = report.StatusFail
		f.Kind = report.KindAutomated
		f.Severity = report.SeverityCritical
		f.Evidence = "synthetic fail for fail-on test"
		return f
	}
	r := &report.Report{Findings: []report.Finding{}}
	eng := &Engine{
		Registry:     map[string]Check{"P6": failingCheck},
		ProbeTimeout: 5 * time.Second,
		Probe:        fakeOKProbe,
	}
	eng.Run(context.Background(), "/fake", r, nil)

	for _, threshold := range []report.Severity{report.SeverityLow, report.SeverityMedium, report.SeverityHigh, report.SeverityCritical} {
		if got := exit.MapFromReport(r, threshold); got != exit.FindingsAtThreshold {
			t.Errorf("threshold=%s: want exit %d, got %d", threshold, exit.FindingsAtThreshold, got)
		}
	}
}
