// Package descriptor parses and validates afcli.yaml descriptors and
// applies their skip / relax policies to per-principle findings.
//
// The package is intentionally independent of internal/cli and
// internal/audit so it can be imported from either side without a
// circular dependency.
package descriptor

// Descriptor is the in-memory shape of an afcli.yaml descriptor.
// Field tags are the documented wire contract; never rename them
// without bumping format_version.
type Descriptor struct {
	FormatVersion   string            `yaml:"format_version"`
	Target          string            `yaml:"target,omitempty"`
	Commands        Commands          `yaml:"commands,omitempty"`
	Env             map[string]string `yaml:"env,omitempty"`
	SkipPrinciples  []string          `yaml:"skip_principles,omitempty"`
	RelaxPrinciples map[string]string `yaml:"relax_principles,omitempty"`
}

// Commands carries the descriptor's allowlist for probe execution.
// S05 will read Safe; Destructive is reserved for future explicit-opt-in
// probes and is parsed today only so unknown-key strict-mode does not
// reject descriptors that declare it. Nondeterministic is the structural
// opt-out from P3 probe-rerun: every entry MUST also appear in Safe
// (Validate enforces this) — you cannot opt out of a probe you have not
// authorized.
type Commands struct {
	Safe             []string `yaml:"safe,omitempty"`
	Destructive      []string `yaml:"destructive,omitempty"`
	Nondeterministic []string `yaml:"nondeterministic,omitempty"`
}
