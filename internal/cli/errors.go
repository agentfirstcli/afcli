package cli

import (
	"fmt"
	"io"

	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
)

// auditError is returned by command handlers when the CLI cannot complete
// the requested audit. It carries the wire envelope plus the semantic exit
// code so Execute() can render to stderr in the user-selected format and
// exit consistently across success, target-not-found, and usage paths.
type auditError struct {
	envelope *report.ErrorEnvelope
	target   string
	exitCode int
}

func (e *auditError) Error() string {
	if e.envelope == nil {
		return "audit error"
	}
	return e.envelope.Code + ": " + e.envelope.Message
}

func newAuditError(code, message, hint, target string, details map[string]any, exitCode int) *auditError {
	return &auditError{
		envelope: &report.ErrorEnvelope{
			Code:    code,
			Message: message,
			Hint:    hint,
			Details: details,
		},
		target:   target,
		exitCode: exitCode,
	}
}

// renderReport dispatches r to the renderer matching format, writing to w.
// An unknown format falls back to JSON so a misconfigured caller still
// produces a parseable artifact instead of nothing.
func renderReport(w io.Writer, r *report.Report, opts report.RenderOptions, format string) error {
	switch format {
	case "text":
		return report.RenderText(w, r, opts)
	case "markdown":
		return report.RenderMarkdown(w, r, opts)
	case "json", "":
		return report.RenderJSON(w, r, opts)
	default:
		return report.RenderJSON(w, r, opts)
	}
}

// renderEnvelope wraps env in a minimal Report so every output format
// emits the same field set. The schema requires manifest_version,
// afcli_version, target, started_at, duration_ms, and findings even on
// envelope-only reports — populating placeholders here keeps stderr
// envelopes schema-valid.
func renderEnvelope(w io.Writer, env *report.ErrorEnvelope, target string, opts report.RenderOptions, format string) error {
	r := &report.Report{
		ManifestVersion: manifest.Embedded.Version,
		AfcliVersion:    AfcliVersion,
		Target:          target,
		StartedAt:       startedAt(opts),
		DurationMs:      0,
		Findings:        []report.Finding{},
		Error:           env,
	}
	return renderReport(w, r, opts, format)
}

// usageEnvelope converts a Cobra parse-time error (unknown flag, missing
// arg, unknown subcommand) into the documented USAGE envelope. The hint
// directs the user to --help — Cobra's auto-usage is silenced so the
// envelope is the only thing the caller sees on stderr.
func usageEnvelope(err error) *report.ErrorEnvelope {
	return &report.ErrorEnvelope{
		Code:    report.CodeUsage,
		Message: fmt.Sprintf("invalid command-line invocation: %s", err.Error()),
		Hint:    "run with --help for usage",
	}
}
