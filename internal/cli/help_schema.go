package cli

import (
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/agentfirstcli/afcli/internal/manifest"
	"github.com/agentfirstcli/afcli/internal/report"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// errHelpSchema is returned by PersistentPreRunE after rendering the
// help-schema JSON to stdout. Execute() recognises this sentinel and
// exits 0 without re-rendering or running any subcommand. Mirrors the
// errInterrupted sentinel pattern from S01.
var errHelpSchema = errors.New("help schema emitted")

// HelpSchema is the top-level document emitted by --help-schema.
type HelpSchema struct {
	AfcliVersion    string     `json:"afcli_version"`
	ManifestVersion string     `json:"manifest_version"`
	SchemaVersion   string     `json:"schema_version"`
	Command         Command    `json:"command"`
	ExitCodes       []ExitCode `json:"exit_codes"`
	ErrorCodes      []string   `json:"error_codes"`
}

// Command is a node in the recursive command tree.
type Command struct {
	Name        string    `json:"name"`
	Use         string    `json:"use"`
	Short       string    `json:"short"`
	Long        string    `json:"long,omitempty"`
	Flags       []Flag    `json:"flags"`
	Subcommands []Command `json:"subcommands"`
}

// Flag describes a single CLI flag on a Command.
type Flag struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand,omitempty"`
	Type       string `json:"type"`
	Usage      string `json:"usage"`
	Default    string `json:"default,omitempty"`
	Persistent bool   `json:"persistent"`
}

// ExitCode describes one entry in the documented process exit-code surface.
type ExitCode struct {
	Code    int    `json:"code"`
	Name    string `json:"name"`
	Meaning string `json:"meaning"`
}

// BuildHelpSchema reflects cmd's command tree into a deterministic
// HelpSchema document. Hidden flags and the auto-injected `help`
// subcommand are excluded so the schema mirrors --help's user-visible
// surface.
func BuildHelpSchema(cmd *cobra.Command) *HelpSchema {
	return &HelpSchema{
		AfcliVersion:    AfcliVersion,
		ManifestVersion: manifest.Embedded.Version,
		SchemaVersion:   "1",
		Command:         walkCommand(cmd),
		ExitCodes:       buildExitCodes(),
		ErrorCodes:      buildErrorCodes(),
	}
}

func walkCommand(cmd *cobra.Command) Command {
	flags := collectFlags(cmd)

	subs := []Command{}
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		subs = append(subs, walkCommand(c))
	}
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].Name < subs[j].Name
	})

	return Command{
		Name:        cmd.Name(),
		Use:         cmd.Use,
		Short:       cmd.Short,
		Long:        cmd.Long,
		Flags:       flags,
		Subcommands: subs,
	}
}

// collectFlags merges persistent and local flags, deduping by name with
// persistent winning. pflag's VisitAll iterates in name-sorted order, so
// the resulting slice is deterministic without an extra sort.
func collectFlags(cmd *cobra.Command) []Flag {
	type entry struct {
		flag       *pflag.Flag
		persistent bool
	}
	byName := map[string]entry{}

	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		byName[f.Name] = entry{flag: f, persistent: true}
	})
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if _, exists := byName[f.Name]; exists {
			return
		}
		byName[f.Name] = entry{flag: f, persistent: false}
	})

	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]Flag, 0, len(names))
	for _, n := range names {
		e := byName[n]
		out = append(out, Flag{
			Name:       e.flag.Name,
			Shorthand:  e.flag.Shorthand,
			Type:       e.flag.Value.Type(),
			Usage:      e.flag.Usage,
			Default:    e.flag.DefValue,
			Persistent: e.persistent,
		})
	}
	return out
}

func buildExitCodes() []ExitCode {
	return []ExitCode{
		{Code: 0, Name: "OK", Meaning: "audit completed and no findings met or exceeded the threshold"},
		{Code: 1, Name: "FindingsAtThreshold", Meaning: "audit completed but at least one finding met or exceeded the configured severity threshold"},
		{Code: 2, Name: "Usage", Meaning: "invalid command-line invocation (unknown flag, missing arg, unknown subcommand)"},
		{Code: 3, Name: "CouldNotAudit", Meaning: "audit could not run to completion (target missing, not executable, descriptor invalid, probe denied/timed out)"},
		{Code: 4, Name: "Internal", Meaning: "afcli encountered an internal error and could not produce a report"},
		{Code: 130, Name: "Interrupted", Meaning: "audit was interrupted by SIGINT or SIGTERM and emitted a partial report"},
	}
}

// buildErrorCodes returns the canonical error codes from
// internal/report.Code* sorted alphabetically for byte-deterministic output.
func buildErrorCodes() []string {
	codes := []string{
		report.CodeDescriptorInvalid,
		report.CodeDescriptorNotFound,
		report.CodeInternal,
		report.CodeProbeDenied,
		report.CodeProbeTimeout,
		report.CodeTargetNotExecutable,
		report.CodeTargetNotFound,
		report.CodeUsage,
	}
	sort.Strings(codes)
	return codes
}

// RenderHelpSchema writes hs to w as indented JSON. SetEscapeHTML(false)
// keeps angle brackets and ampersands unescaped so the output is readable
// when piped through jq.
func RenderHelpSchema(w io.Writer, hs *HelpSchema) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(hs)
}
