package report

// ErrorEnvelope is the documented error contract surfaced when afcli
// cannot complete (or refuses to attempt) an audit. The shape is stable
// across all output formats; new codes may be added but existing codes
// never change semantics.
type ErrorEnvelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Hint    string         `json:"hint,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// Stable error codes. Add new codes here; never repurpose existing ones.
const (
	CodeTargetNotFound      = "TARGET_NOT_FOUND"
	CodeTargetNotExecutable = "TARGET_NOT_EXECUTABLE"
	CodeDescriptorInvalid   = "DESCRIPTOR_INVALID"
	CodeDescriptorNotFound  = "DESCRIPTOR_NOT_FOUND"
	CodeProbeTimeout        = "PROBE_TIMEOUT"
	CodeProbeDenied         = "PROBE_DENIED"
	CodeInternal            = "INTERNAL"
	CodeUsage               = "USAGE"
	CodeInitFileExists      = "INIT_FILE_EXISTS"
)
