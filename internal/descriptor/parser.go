package descriptor

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
	"gopkg.in/yaml.v3"
)

// supportedFormatVersion is the only format_version this build accepts.
// Bumping it is a wire-contract change and must coincide with a parser
// migration path.
const supportedFormatVersion = "1"

// principleIDPattern matches "P" followed by one or more digits.
// Used by Validate before cross-checking against the manifest.
var principleIDPattern = regexp.MustCompile(`^P\d+$`)

// allowedSeverities is the closed set of severity strings accepted in
// relax_principles. Order matches the ordinal table in apply.go and is
// the order surfaced in details.allowed for human-readable errors.
var allowedSeverities = []string{
	string(report.SeverityLow),
	string(report.SeverityMedium),
	string(report.SeverityHigh),
	string(report.SeverityCritical),
}

// yamlTypeErrorLine extracts the leading "line N:" prefix that
// yaml.v3's TypeError messages embed. Best-effort; if the message
// shape ever changes we still return the typed error without a line.
var yamlTypeErrorLine = regexp.MustCompile(`line (\d+):`)

// Load reads and validates the descriptor at path. Returns a typed
// *Error on every failure path so the cli layer can map cleanly to
// DESCRIPTOR_INVALID / DESCRIPTOR_NOT_FOUND envelopes.
func Load(path string) (*Descriptor, error) {
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, &Error{
				Code:    CodeDescriptorNotFound,
				Message: fmt.Sprintf("descriptor not found: %s", path),
				Hint:    "check the path and that the file exists",
				Details: map[string]any{"path": path},
			}
		}
		return nil, &Error{
			Code:    CodeDescriptorInvalid,
			Message: fmt.Sprintf("cannot stat descriptor: %v", statErr),
			Details: map[string]any{"path": path, "os": statErr.Error()},
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, &Error{
			Code:    CodeDescriptorInvalid,
			Message: fmt.Sprintf("cannot open descriptor: %v", err),
			Details: map[string]any{"path": path, "os": err.Error()},
		}
	}
	defer f.Close()

	var d Descriptor
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return nil, decodeErrorToTyped(err, path)
	}

	if vErr := Validate(&d); vErr != nil {
		// Validate returns *Error already; just stamp path in details.
		var typed *Error
		if errors.As(vErr, &typed) {
			if typed.Details == nil {
				typed.Details = map[string]any{}
			}
			if _, ok := typed.Details["path"]; !ok {
				typed.Details["path"] = path
			}
			return nil, typed
		}
		return nil, vErr
	}
	return &d, nil
}

// decodeErrorToTyped converts a yaml decode error into a *Error.
// yaml.TypeError carries one or more "line N: ..." messages — we
// surface the first line number in details.line plus the original
// message so a malformed type/unknown-field error remains diagnosable.
func decodeErrorToTyped(err error, path string) *Error {
	details := map[string]any{"path": path}
	msg := err.Error()
	hint := ""

	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) && len(typeErr.Errors) > 0 {
		msg = strings.Join(typeErr.Errors, "; ")
		if line, ok := firstYAMLLine(typeErr.Errors); ok {
			details["line"] = line
		}
		hint = "check field types — see afcli.yaml schema"
	} else if line, ok := firstYAMLLine([]string{msg}); ok {
		details["line"] = line
	}

	return &Error{
		Code:    CodeDescriptorInvalid,
		Message: msg,
		Hint:    hint,
		Details: details,
	}
}

func firstYAMLLine(msgs []string) (int, bool) {
	for _, m := range msgs {
		if match := yamlTypeErrorLine.FindStringSubmatch(m); match != nil {
			if n, err := strconv.Atoi(match[1]); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// Validate enforces the documented descriptor invariants: format_version
// equals the supported version, every skip_principles entry is a known
// principle id, every relax_principles key is a known principle id and
// every value is one of the four documented severities.
//
// Returns *Error so callers can populate ErrorEnvelope details directly.
func Validate(d *Descriptor) error {
	if d == nil {
		return &Error{
			Code:    CodeDescriptorInvalid,
			Message: "descriptor is empty",
		}
	}

	if d.FormatVersion == "" {
		return &Error{
			Code:    CodeDescriptorInvalid,
			Message: "format_version is required",
			Hint:    "set format_version: \"1\"",
			Details: map[string]any{
				"key":       "format_version",
				"supported": []string{supportedFormatVersion},
			},
		}
	}
	if d.FormatVersion != supportedFormatVersion {
		return &Error{
			Code:    CodeDescriptorInvalid,
			Message: fmt.Sprintf("unsupported format_version %q", d.FormatVersion),
			Hint:    "this build supports format_version \"1\"",
			Details: map[string]any{
				"key":       "format_version",
				"got":       d.FormatVersion,
				"supported": []string{supportedFormatVersion},
			},
		}
	}

	known := knownPrincipleIDs()
	for i, id := range d.SkipPrinciples {
		if !principleIDPattern.MatchString(id) {
			return &Error{
				Code:    CodeDescriptorInvalid,
				Message: fmt.Sprintf("skip_principles[%d] %q is not a principle id", i, id),
				Hint:    "use the form P<number>, e.g. P12",
				Details: map[string]any{
					"key":      fmt.Sprintf("skip_principles[%d]", i),
					"value":    id,
					"expected": "P<number>",
				},
			}
		}
		if !known[id] {
			return &Error{
				Code:    CodeDescriptorInvalid,
				Message: fmt.Sprintf("skip_principles[%d] %q is not in the embedded manifest", i, id),
				Hint:    "see https://agentfirstcli.com/principles for the canonical list",
				Details: map[string]any{
					"key":   fmt.Sprintf("skip_principles[%d]", i),
					"value": id,
				},
			}
		}
	}

	for id, sev := range d.RelaxPrinciples {
		if !principleIDPattern.MatchString(id) {
			return &Error{
				Code:    CodeDescriptorInvalid,
				Message: fmt.Sprintf("relax_principles key %q is not a principle id", id),
				Hint:    "use the form P<number>, e.g. P7",
				Details: map[string]any{
					"key":      fmt.Sprintf("relax_principles.%s", id),
					"value":    id,
					"expected": "P<number>",
				},
			}
		}
		if !known[id] {
			return &Error{
				Code:    CodeDescriptorInvalid,
				Message: fmt.Sprintf("relax_principles key %q is not in the embedded manifest", id),
				Hint:    "see https://agentfirstcli.com/principles for the canonical list",
				Details: map[string]any{
					"key":   fmt.Sprintf("relax_principles.%s", id),
					"value": id,
				},
			}
		}
		if !severityAllowed(sev) {
			return &Error{
				Code:    CodeDescriptorInvalid,
				Message: fmt.Sprintf("relax_principles.%s value %q is not a valid severity", id, sev),
				Hint:    "use one of low, medium, high, critical",
				Details: map[string]any{
					"key":     fmt.Sprintf("relax_principles.%s", id),
					"got":     sev,
					"allowed": append([]string(nil), allowedSeverities...),
				},
			}
		}
	}

	return nil
}

func knownPrincipleIDs() map[string]bool {
	out := make(map[string]bool, len(manifest.Embedded.Principles))
	for _, p := range manifest.Embedded.Principles {
		out[p.PrincipleID()] = true
	}
	return out
}

func severityAllowed(s string) bool {
	for _, allowed := range allowedSeverities {
		if s == allowed {
			return true
		}
	}
	return false
}
