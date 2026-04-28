package descriptor

import "github.com/agentfirstcli/afcli/internal/report"

// Error is the typed failure surface returned by Load and Validate.
// internal/cli wraps these into the user-visible *auditError +
// ErrorEnvelope shape; everyone else just inspects Code.
//
// Code is always one of report.CodeDescriptorInvalid or
// report.CodeDescriptorNotFound — defined here as constants for
// callers that only depend on internal/descriptor.
type Error struct {
	Code    string
	Message string
	Hint    string
	Details map[string]any
}

// Error returns "CODE: message" so wrapping it with errors.Is/As is
// straightforward without depending on cli formatting.
func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

// Re-exported for convenience so callers can type-switch on Code
// without pulling in internal/report. Values match exactly.
const (
	CodeDescriptorInvalid  = report.CodeDescriptorInvalid
	CodeDescriptorNotFound = report.CodeDescriptorNotFound
)
