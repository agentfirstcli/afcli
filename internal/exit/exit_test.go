package exit

import (
	"testing"

	"github.com/agentfirstcli/afcli/internal/report"
)

func TestMapFromReportNil(t *testing.T) {
	if got := MapFromReport(nil, report.SeverityHigh); got != Internal {
		t.Errorf("nil report: got %d want %d", got, Internal)
	}
}

func TestMapFromReportInterruptedDominates(t *testing.T) {
	// Interrupted overrides finding-failure exit, error envelope, everything.
	r := &report.Report{
		Interrupted: true,
		Error:       &report.ErrorEnvelope{Code: report.CodeInternal},
		Findings: []report.Finding{
			{PrincipleID: "P6", Status: report.StatusFail, Severity: report.SeverityCritical},
		},
	}
	if got := MapFromReport(r, report.SeverityHigh); got != Interrupted {
		t.Errorf("interrupted: got %d want %d", got, Interrupted)
	}
}

func TestMapFromReportErrorEnvelope(t *testing.T) {
	cases := []struct {
		name string
		code string
		want int
	}{
		{"usage", report.CodeUsage, Usage},
		{"internal", report.CodeInternal, Internal},
		{"target-not-found", report.CodeTargetNotFound, CouldNotAudit},
		{"target-not-executable", report.CodeTargetNotExecutable, CouldNotAudit},
		{"descriptor-invalid", report.CodeDescriptorInvalid, CouldNotAudit},
		{"descriptor-not-found", report.CodeDescriptorNotFound, CouldNotAudit},
		{"probe-timeout", report.CodeProbeTimeout, CouldNotAudit},
		{"probe-denied", report.CodeProbeDenied, CouldNotAudit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &report.Report{Error: &report.ErrorEnvelope{Code: tc.code}}
			if got := MapFromReport(r, report.SeverityHigh); got != tc.want {
				t.Errorf("%s: got %d want %d", tc.code, got, tc.want)
			}
		})
	}
}

func TestMapFromReportEmptyFindings(t *testing.T) {
	r := &report.Report{}
	if got := MapFromReport(r, report.SeverityHigh); got != OK {
		t.Errorf("empty: got %d want %d", got, OK)
	}
}

// TestMapFromReportThresholdMatrix covers every Status × Severity × threshold
// combination. Only Status=="fail" with Severity >= threshold trips exit 1.
func TestMapFromReportThresholdMatrix(t *testing.T) {
	statuses := []report.Status{report.StatusPass, report.StatusFail, report.StatusSkip, report.StatusReview}
	severities := []report.Severity{report.SeverityLow, report.SeverityMedium, report.SeverityHigh, report.SeverityCritical}
	thresholds := []report.Severity{report.SeverityLow, report.SeverityMedium, report.SeverityHigh, report.SeverityCritical}

	rank := map[report.Severity]int{
		report.SeverityLow: 1, report.SeverityMedium: 2,
		report.SeverityHigh: 3, report.SeverityCritical: 4,
	}

	for _, st := range statuses {
		for _, sev := range severities {
			for _, th := range thresholds {
				r := &report.Report{
					Findings: []report.Finding{{
						PrincipleID: "P_test", Status: st, Severity: sev,
					}},
				}
				want := OK
				if st == report.StatusFail && rank[sev] >= rank[th] {
					want = FindingsAtThreshold
				}
				got := MapFromReport(r, th)
				if got != want {
					t.Errorf("status=%s severity=%s threshold=%s: got %d want %d",
						st, sev, th, got, want)
				}
			}
		}
	}
}

// TestMapFromReportDefaultThreshold — empty threshold means "high".
func TestMapFromReportDefaultThreshold(t *testing.T) {
	r := &report.Report{
		Findings: []report.Finding{
			{Status: report.StatusFail, Severity: report.SeverityMedium},
		},
	}
	if got := MapFromReport(r, ""); got != OK {
		t.Errorf("medium-fail under default-high threshold: got %d want %d", got, OK)
	}
	r.Findings = []report.Finding{
		{Status: report.StatusFail, Severity: report.SeverityHigh},
	}
	if got := MapFromReport(r, ""); got != FindingsAtThreshold {
		t.Errorf("high-fail under default-high threshold: got %d want %d", got, FindingsAtThreshold)
	}
}

// TestMapFromReportFirstFailWins — even if first finding is pass, a later
// fail at threshold still trips exit 1.
func TestMapFromReportAnyFailTrips(t *testing.T) {
	r := &report.Report{
		Findings: []report.Finding{
			{Status: report.StatusPass, Severity: report.SeverityCritical},
			{Status: report.StatusReview, Severity: report.SeverityCritical},
			{Status: report.StatusSkip, Severity: report.SeverityCritical},
			{Status: report.StatusFail, Severity: report.SeverityHigh},
		},
	}
	if got := MapFromReport(r, report.SeverityHigh); got != FindingsAtThreshold {
		t.Errorf("got %d want %d", got, FindingsAtThreshold)
	}
}

// TestExitConstantValues pins the documented numeric exit codes.
func TestExitConstantValues(t *testing.T) {
	cases := map[string]struct {
		got, want int
	}{
		"OK":                  {OK, 0},
		"FindingsAtThreshold": {FindingsAtThreshold, 1},
		"Usage":               {Usage, 2},
		"CouldNotAudit":       {CouldNotAudit, 3},
		"Internal":            {Internal, 4},
		"Interrupted":         {Interrupted, 130},
	}
	for name, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %d want %d", name, tc.got, tc.want)
		}
	}
}
