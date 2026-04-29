package report

import (
	"testing"
)

func makeFindings(t *testing.T, counts map[Status]int) []Finding {
	t.Helper()
	var out []Finding
	for status, n := range counts {
		for i := 0; i < n; i++ {
			out = append(out, Finding{
				PrincipleID: "P-test",
				Title:       "test",
				Category:    "test",
				Status:      status,
				Kind:        KindAutomated,
				Severity:    SeverityLow,
			})
		}
	}
	return out
}

func TestScoreReport(t *testing.T) {
	cases := []struct {
		name   string
		counts map[Status]int
		want   ScoreSummary
	}{
		{
			name:   "all-pass",
			counts: map[Status]int{StatusPass: 5},
			want:   ScoreSummary{Score: 5, Total: 5, Color: "#4c1", Message: "5/5"},
		},
		{
			name:   "all-fail",
			counts: map[Status]int{StatusFail: 5},
			want:   ScoreSummary{Score: 0, Total: 5, Color: "#e05d44", Message: "0/5"},
		},
		{
			name:   "all-review",
			counts: map[Status]int{StatusReview: 5},
			want:   ScoreSummary{Score: 0, Total: 5, Color: "#e05d44", Message: "0/5"},
		},
		{
			name:   "all-skip",
			counts: map[Status]int{StatusSkip: 5},
			want:   ScoreSummary{Score: 5, Total: 5, Color: "#4c1", Message: "5/5"},
		},
		{
			name: "mixed",
			counts: map[Status]int{
				StatusPass:   8,
				StatusSkip:   2,
				StatusReview: 3,
				StatusFail:   3,
			},
			want: ScoreSummary{Score: 10, Total: 16, Color: "#e05d44", Message: "10/16"},
		},
		{
			name:   "empty-findings-zero-by-zero-red",
			counts: nil,
			want:   ScoreSummary{Score: 0, Total: 0, Color: "#e05d44", Message: "0/0"},
		},
		{
			name:   "boundary-90-percent-green",
			counts: map[Status]int{StatusPass: 9, StatusFail: 1},
			want:   ScoreSummary{Score: 9, Total: 10, Color: "#4c1", Message: "9/10"},
		},
		{
			name:   "boundary-89-percent-yellow",
			counts: map[Status]int{StatusPass: 89, StatusFail: 11},
			want:   ScoreSummary{Score: 89, Total: 100, Color: "#dfb317", Message: "89/100"},
		},
		{
			name:   "boundary-70-percent-yellow",
			counts: map[Status]int{StatusPass: 7, StatusFail: 3},
			want:   ScoreSummary{Score: 7, Total: 10, Color: "#dfb317", Message: "7/10"},
		},
		{
			name:   "boundary-69-percent-red",
			counts: map[Status]int{StatusPass: 69, StatusFail: 31},
			want:   ScoreSummary{Score: 69, Total: 100, Color: "#e05d44", Message: "69/100"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Report{Findings: makeFindings(t, tc.counts)}
			got := ScoreReport(r)
			if got != tc.want {
				t.Errorf("ScoreReport()\n got=  %+v\n want= %+v", got, tc.want)
			}
		})
	}
}
