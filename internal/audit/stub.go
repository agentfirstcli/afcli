package audit

import (
	"context"

	"github.com/agentfirstcli/afcli/internal/report"
)

// stubCheck is the fallback used for any principle without a registered
// Check. Plumbs R008 (every-principle-signal) early; S06 upgrades each
// stub to a real check.
func stubCheck(_ context.Context, env *CheckEnv) report.Finding {
	return report.Finding{
		PrincipleID:    env.Principle.PrincipleID(),
		Title:          env.Principle.Title,
		Category:       env.Principle.Category,
		Status:         report.StatusReview,
		Kind:           report.KindRequiresReview,
		Severity:       severityFor(env.Principle.PrincipleID()),
		Evidence:       "no automated check yet — manual review required",
		Recommendation: "review the principle's markdown and assess manually",
		Hint:           env.Principle.URL,
	}
}
