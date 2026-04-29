package audit

import (
	"strings"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/report"
)

// BehavioralCapture is one entry in the descriptor-authorized probe pass.
// Cmd is the original Commands.Safe[] entry verbatim; Argv is the
// strings.Fields-split form passed to the subprocess; Capture carries
// the RunProbe result (or a synthesized *Capture with Err set to an
// *AuthError when authorizeProbe rejected the candidate).
// Rerun, when non-nil, is the paired second invocation used by the P3
// promotion aggregator. Nil when the descriptor opted out via
// Commands.Nondeterministic, when authorization failed, or when the
// first probe errored.
type BehavioralCapture struct {
	Cmd     string
	Argv    []string
	Capture *Capture
	Rerun   *Capture
}

// AuthError discriminates "the probe was structurally rejected before
// any subprocess started" from "the probe ran and failed". The Code is
// always report.CodeProbeDenied today; Reason is the human-facing tail
// the aggregator can splice into evidence verbatim.
type AuthError struct {
	Code   string
	Cmd    string
	Reason string
}

func (e *AuthError) Error() string {
	return e.Code + ": " + e.Reason + ": " + e.Cmd
}

// authorizeProbe enforces exact-argv allow-listing against
// descriptor.Commands.Safe with a paranoid Commands.Destructive overlap
// rejection. The comparison is purely string-equal on the
// space-joined candidate — no prefix match, no substring match — so a
// candidate like ["--version", "--quiet"] is denied when only
// ["--version"] is in Safe.
func authorizeProbe(d *descriptor.Descriptor, candidate []string) error {
	joined := strings.Join(candidate, " ")
	if d == nil || len(d.Commands.Safe) == 0 {
		return &AuthError{Code: report.CodeProbeDenied, Cmd: joined, Reason: "not in commands.safe"}
	}
	for _, dest := range d.Commands.Destructive {
		if dest == joined {
			return &AuthError{Code: report.CodeProbeDenied, Cmd: joined, Reason: "matches commands.destructive"}
		}
	}
	for _, safe := range d.Commands.Safe {
		if safe == joined {
			return nil
		}
	}
	return &AuthError{Code: report.CodeProbeDenied, Cmd: joined, Reason: "not in commands.safe"}
}
