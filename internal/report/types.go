// Package report defines the canonical afcli audit report types.
//
// Every renderer (json, text, markdown), check, descriptor handler, probe,
// and signal path reads or writes these types. JSON tags are the documented
// snake_case wire contract — do not rename them without bumping
// ManifestVersion / the schema fixture.
package report

// Status is the per-finding evaluation outcome.
type Status string

const (
	StatusPass   Status = "pass"
	StatusFail   Status = "fail"
	StatusSkip   Status = "skip"
	StatusReview Status = "review"
)

// Kind distinguishes mechanically-checked findings from those that require
// human review (fuzzy principles, probe timeouts, internal check errors).
type Kind string

const (
	KindAutomated       Kind = "automated"
	KindRequiresReview  Kind = "requires-review"
)

// Severity is the manifest-declared severity of a principle.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Report is the top-level afcli audit document. The shape is identical
// across all output formats — renderers consume this struct.
type Report struct {
	ManifestVersion string         `json:"manifest_version"`
	AfcliVersion    string         `json:"afcli_version"`
	Target          string         `json:"target"`
	StartedAt       string         `json:"started_at"`
	DurationMs      int64          `json:"duration_ms"`
	Interrupted     bool           `json:"interrupted,omitempty"`
	Findings        []Finding      `json:"findings"`
	Error           *ErrorEnvelope `json:"error,omitempty"`
}

// Finding is a single principle evaluation result.
type Finding struct {
	PrincipleID    string   `json:"principle_id"`
	Title          string   `json:"title"`
	Category       string   `json:"category"`
	Status         Status   `json:"status"`
	Kind           Kind     `json:"kind"`
	Severity       Severity `json:"severity"`
	Evidence       string   `json:"evidence"`
	Recommendation string   `json:"recommendation"`
	Hint           string   `json:"hint,omitempty"`
}
