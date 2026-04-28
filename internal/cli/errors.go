package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/agentfirstcli/afcli/internal/descriptor"
	"github.com/agentfirstcli/afcli/internal/exit"
	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
	"github.com/agentfirstcli/afcli/internal/version"
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
		AfcliVersion:    version.Version,
		Target:          target,
		StartedAt:       startedAt(opts),
		DurationMs:      0,
		Findings:        []report.Finding{},
		Error:           env,
	}
	return renderReport(w, r, opts, format)
}

// classifyDescriptorError converts a descriptor.Load failure into an
// *auditError carrying the documented DESCRIPTOR_INVALID /
// DESCRIPTOR_NOT_FOUND envelope. The path is always echoed in
// details.path so an agent can see which file failed without parsing
// the message string. If err does not unwrap to *descriptor.Error, we
// still emit a generic DESCRIPTOR_INVALID envelope so the wire contract
// is preserved.
func classifyDescriptorError(path string, err error) *auditError {
	var dErr *descriptor.Error
	if errors.As(err, &dErr) {
		hint := dErr.Hint
		code := dErr.Code
		if code != report.CodeDescriptorNotFound && code != report.CodeDescriptorInvalid {
			code = report.CodeDescriptorInvalid
		}
		if hint == "" {
			if code == report.CodeDescriptorNotFound {
				hint = "check the path and that the file exists"
			} else {
				hint = "see https://agentfirstcli.com/descriptor for the descriptor schema"
			}
		}
		return newAuditError(
			code,
			dErr.Message,
			hint,
			path,
			mergeDescriptorDetails(path, dErr.Details),
			exit.CouldNotAudit,
		)
	}
	return newAuditError(
		report.CodeDescriptorInvalid,
		fmt.Sprintf("descriptor could not be loaded: %v", err),
		"see https://agentfirstcli.com/descriptor for the descriptor schema",
		path,
		map[string]any{"path": path},
		exit.CouldNotAudit,
	)
}

// mergeDescriptorDetails returns a fresh map seeded with path then
// overlaid with the descriptor.Error's Details so callers see the
// localizing fields (line, key, value, expected, got, allowed) when the
// parser populated them. The descriptor's own "path" wins over the
// CLI-supplied one only if it differs, preserving what the parser saw.
func mergeDescriptorDetails(path string, details map[string]any) map[string]any {
	out := map[string]any{"path": path}
	for k, v := range details {
		out[k] = v
	}
	return out
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
