package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentfirstcli/afcli/internal/descriptor"
)

// TestInspectIntegration drives the slice S02 success criterion end-to-end:
// for each fixture style (cobra, urfave, hand-rolled flag.Parse) the
// pipeline `afcli inspect <fixture> > tmp.yaml` followed by
// `afcli audit <fixture> --descriptor tmp.yaml --output json` must
// produce a non-trivial verdict — no DESCRIPTOR_NOT_FOUND, no
// DESCRIPTOR_INVALID, no envelope-level error, exactly the canonical
// 16-finding manifest, and an exit code in {0, 1} (NOT 3 — code 3
// indicates a descriptor or target resolution failure that would
// invalidate the inspect→audit round-trip).
func TestInspectIntegration(t *testing.T) {
	cases := []struct {
		name  string
		build func(*testing.T) string
	}{
		{name: "cobra-cli", build: buildCobraCli},
		{name: "urfave-cli", build: buildUrfaveCli},
		{name: "flag-parse", build: buildFlagParse},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixtureBin := tc.build(t)
			afcliBin := buildAfcli(t)

			tmpDescriptor := filepath.Join(t.TempDir(), "afcli.yaml")

			// Step 1: afcli inspect <fixture> > tmpDescriptor
			inspectCmd := exec.Command(afcliBin, "inspect", fixtureBin)
			var inspectOut, inspectErr bytes.Buffer
			inspectCmd.Stdout = &inspectOut
			inspectCmd.Stderr = &inspectErr
			inspectCmd.Env = os.Environ()
			if err := inspectCmd.Run(); err != nil {
				t.Fatalf("afcli inspect %s: %v\nstderr=%s", fixtureBin, err, inspectErr.String())
			}
			if code := inspectCmd.ProcessState.ExitCode(); code != 0 {
				t.Fatalf("afcli inspect exit code: want 0, got %d\nstderr=%s", code, inspectErr.String())
			}
			if err := os.WriteFile(tmpDescriptor, inspectOut.Bytes(), 0o644); err != nil {
				t.Fatalf("write captured descriptor: %v", err)
			}

			// Step 2: descriptor.Load round-trips in-process (asserts
			// the emitted YAML is well-formed and Targets the fixture).
			d, err := descriptor.Load(tmpDescriptor)
			if err != nil {
				t.Fatalf("descriptor.Load on inspect output: %v\nbody=%s", err, inspectOut.String())
			}
			if d.FormatVersion != "1" {
				t.Errorf("format_version: want \"1\", got %q", d.FormatVersion)
			}
			if d.Target != fixtureBin {
				t.Errorf("descriptor target round-trip: want %q, got %q", fixtureBin, d.Target)
			}

			// Step 3: afcli audit <fixture> --descriptor tmp.yaml --output json
			auditCmd := exec.Command(afcliBin, "audit", fixtureBin,
				"--descriptor", tmpDescriptor,
				"--output", "json",
				"--deterministic",
			)
			var auditOut, auditErrBuf bytes.Buffer
			auditCmd.Stdout = &auditOut
			auditCmd.Stderr = &auditErrBuf
			auditCmd.Env = append(os.Environ(), "AFCLI_DETERMINISTIC=1")
			waitErr := auditCmd.Run()
			auditCode := 0
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				auditCode = exitErr.ExitCode()
			} else if waitErr != nil {
				t.Fatalf("afcli audit %s: %v\nstderr=%s", fixtureBin, waitErr, auditErrBuf.String())
			}
			// Exit 3 indicates DESCRIPTOR_NOT_FOUND/INVALID or TARGET_*
			// — that would mean the inspect→audit round-trip failed,
			// which is exactly the regression this test guards against.
			if auditCode != 0 && auditCode != 1 {
				t.Fatalf("afcli audit exit code: want 0 or 1, got %d\nstdout=%s\nstderr=%s",
					auditCode, auditOut.String(), auditErrBuf.String())
			}

			// Step 4: report shape — no envelope error, exactly 16 findings.
			var rep auditReport
			if err := json.Unmarshal(auditOut.Bytes(), &rep); err != nil {
				t.Fatalf("decode audit report: %v\nstdout=%s", err, auditOut.String())
			}
			if len(rep.Error) != 0 {
				t.Errorf("audit report carries envelope error: %s", string(rep.Error))
			}
			if len(rep.Findings) != 16 {
				t.Errorf("findings count: want 16, got %d\nstdout=%s",
					len(rep.Findings), auditOut.String())
			}
		})
	}
}
