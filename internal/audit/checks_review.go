// Package audit checks: P2 (Token Efficiency), P5 (Partial Failure Output),
// P9 (Idempotent Operations), P11 (Graceful Cancellation), and P12 (Stable
// Schema). These five principles unconditionally emit kind:requires-review
// findings — automated detection is either out of scope for v1 (P2/P5/P11)
// or fundamentally not single-audit decidable (P9/P12). They replace the
// generic stub blurb with principle-specific evidence + recommendation so
// an agent reading the JSON report sees actionable guidance per finding.
package audit

import (
	"context"

	"github.com/agentfirstcli/afcli/internal/report"
)

func checkP2(_ context.Context, env *CheckEnv) report.Finding {
	f := baseFinding(env)
	f.Status = report.StatusReview
	f.Kind = report.KindRequiresReview
	f.Evidence = "automated token-efficiency analysis is out of scope for v1 — manual review against the principle's manifest entry required"
	f.Recommendation = "audit --help and command output for verbosity, banners, and decoration; trim to signal-only by default"
	return f
}

func checkP5(_ context.Context, env *CheckEnv) report.Finding {
	f := baseFinding(env)
	f.Status = report.StatusReview
	f.Kind = report.KindRequiresReview
	f.Evidence = "partial-failure semantics require fixture-driven probing not in scope for v1"
	f.Recommendation = "document partial-failure behavior and ensure successful items are reported even when the run fails overall"
	return f
}

func checkP9(_ context.Context, env *CheckEnv) report.Finding {
	f := baseFinding(env)
	f.Status = report.StatusReview
	f.Kind = report.KindRequiresReview
	f.Evidence = "idempotency requires a descriptor-declared idempotent command and repeated invocation analysis — out of scope for v1"
	f.Recommendation = "declare idempotent commands in afcli.yaml under commands.safe[] and verify repeated invocations converge"
	return f
}

func checkP11(_ context.Context, env *CheckEnv) report.Finding {
	f := baseFinding(env)
	f.Status = report.StatusReview
	f.Kind = report.KindRequiresReview
	f.Evidence = "behavioral SIGINT analysis requires sending signals mid-run — out of scope for v1"
	f.Recommendation = "document signal handling in --help and ensure SIGINT triggers a clean partial-result emission, not a hard abort"
	return f
}

func checkP12(_ context.Context, env *CheckEnv) report.Finding {
	f := baseFinding(env)
	f.Status = report.StatusReview
	f.Kind = report.KindRequiresReview
	f.Evidence = "schema stability is a meta-property over time — single-audit detection not possible"
	f.Recommendation = "version your output schema, document additions in changelog, and never silently change a field's type or semantics"
	return f
}
