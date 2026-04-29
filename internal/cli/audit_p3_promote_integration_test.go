package cli_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// Per-fixture sync.Once + cached path + cached error so a build failure
// for one of the three P3 fixtures does not poison the other two for the
// rest of the test process. Mirrors the pattern in inspect_fixture_test.go.
var (
	p3PassingBinPath     string
	p3PassingBinBuildErr error
	p3PassingBinBuildOne sync.Once

	p3FailingBinPath     string
	p3FailingBinBuildErr error
	p3FailingBinBuildOne sync.Once

	p3BorderlineBinPath     string
	p3BorderlineBinBuildErr error
	p3BorderlineBinBuildOne sync.Once
)

func buildP3Passing(t *testing.T) string {
	return buildFixture(t, "p3-passing", &p3PassingBinBuildOne, &p3PassingBinPath, &p3PassingBinBuildErr)
}

func buildP3Failing(t *testing.T) string {
	return buildFixture(t, "p3-failing", &p3FailingBinBuildOne, &p3FailingBinPath, &p3FailingBinBuildErr)
}

func buildP3Borderline(t *testing.T) string {
	return buildFixture(t, "p3-borderline", &p3BorderlineBinBuildOne, &p3BorderlineBinPath, &p3BorderlineBinBuildErr)
}

// TestAuditP3Promotion is the slice-S03 demo: probe-on audit against
// each of the three fixtures must emit the verdict the slice plan
// promised. The three sub-tests are the operator-runnable proof that
// P3 promotion works end-to-end through the CLI.
func TestAuditP3Promotion(t *testing.T) {
	t.Run("passing", func(t *testing.T) {
		bin := buildP3Passing(t)
		desc := fixtureDescriptorPath(t, "p3-passing.yaml")

		stdout, stderr, code := runAudit(t,
			"audit", bin,
			"--probe",
			"--descriptor", desc,
			"--output", "json",
		)
		if code != 0 && code != 1 {
			t.Fatalf("exit code: want 0 or 1, got %d\nstderr=%s", code, stderr)
		}
		r := decodeReport(t, stdout)
		p3, ok := findingByID(r, "P3")
		if !ok {
			t.Fatalf("P3 finding missing")
		}
		if p3.Kind != "automated" {
			t.Errorf("P3.Kind = %q, want %q", p3.Kind, "automated")
		}
		if p3.Status != "pass" {
			t.Errorf("P3.Status = %q, want %q", p3.Status, "pass")
		}
		if !strings.Contains(p3.Evidence, "deterministic") {
			t.Errorf("P3.Evidence = %q must contain 'deterministic'", p3.Evidence)
		}
		if !strings.Contains(p3.Evidence, "--version") {
			t.Errorf("P3.Evidence = %q must mention the safe argv --version", p3.Evidence)
		}
	})

	t.Run("failing", func(t *testing.T) {
		bin := buildP3Failing(t)
		desc := fixtureDescriptorPath(t, "p3-failing.yaml")

		stdout, stderr, code := runAudit(t,
			"audit", bin,
			"--probe",
			"--descriptor", desc,
			"--output", "json",
		)
		if code != 0 && code != 1 {
			t.Fatalf("exit code: want 0 or 1, got %d\nstderr=%s", code, stderr)
		}
		r := decodeReport(t, stdout)
		p3, ok := findingByID(r, "P3")
		if !ok {
			t.Fatalf("P3 finding missing")
		}
		if p3.Kind != "automated" {
			t.Errorf("P3.Kind = %q, want %q", p3.Kind, "automated")
		}
		if p3.Status != "fail" {
			t.Errorf("P3.Status = %q, want %q", p3.Status, "fail")
		}
		if !strings.Contains(p3.Evidence, "diff at line") {
			t.Errorf("P3.Evidence = %q must contain 'diff at line'", p3.Evidence)
		}
		if len(p3.Evidence) > 200 {
			t.Errorf("P3.Evidence length = %d, want <= 200 (evidenceLimit)", len(p3.Evidence))
		}
		if !strings.Contains(p3.Recommendation, "commands.nondeterministic") {
			t.Errorf("P3.Recommendation = %q must point at the descriptor escape hatch", p3.Recommendation)
		}
	})

	t.Run("borderline", func(t *testing.T) {
		bin := buildP3Borderline(t)
		desc := fixtureDescriptorPath(t, "p3-borderline.yaml")

		stdout, stderr, code := runAudit(t,
			"audit", bin,
			"--probe",
			"--descriptor", desc,
			"--output", "json",
		)
		if code != 0 && code != 1 {
			t.Fatalf("exit code: want 0 or 1, got %d\nstderr=%s", code, stderr)
		}
		r := decodeReport(t, stdout)
		p3, ok := findingByID(r, "P3")
		if !ok {
			t.Fatalf("P3 finding missing")
		}
		if p3.Kind != "requires-review" {
			t.Errorf("P3.Kind = %q, want %q", p3.Kind, "requires-review")
		}
		if !strings.Contains(p3.Evidence, "<MASKED>") {
			t.Errorf("P3.Evidence = %q must contain mask sentinel <MASKED>", p3.Evidence)
		}
		if !strings.Contains(p3.Evidence, "masked diff") {
			t.Errorf("P3.Evidence = %q must label the variation as 'masked diff'", p3.Evidence)
		}
	})
}

// TestAuditProbeOnPromotesP3ToAutomated is the manifest-count contract
// for probe-on. The no-probe baseline (TestAuditGitProducesSixteenFindings
// in audit_test.go) asserts 9 automated + 7 review; with --probe and a
// deterministic target P3 lifts from review to automated, yielding
// 10 automated + 6 review. The 16-total invariant (R008) is preserved.
func TestAuditProbeOnPromotesP3ToAutomated(t *testing.T) {
	bin := buildP3Passing(t)
	desc := fixtureDescriptorPath(t, "p3-passing.yaml")

	stdout, stderr, code := runAudit(t,
		"audit", bin,
		"--probe",
		"--descriptor", desc,
		"--output", "json",
	)
	if code != 0 && code != 1 {
		t.Fatalf("exit code: want 0 or 1, got %d\nstderr=%s", code, stderr)
	}
	r := decodeReport(t, stdout)
	if len(r.Findings) != 16 {
		t.Fatalf("findings count: want 16, got %d", len(r.Findings))
	}

	autoCount, reviewCount := 0, 0
	var p3 auditFinding
	var p3Found bool
	for _, f := range r.Findings {
		switch f.Kind {
		case "automated":
			autoCount++
		case "requires-review":
			reviewCount++
		}
		if f.PrincipleID == "P3" {
			p3 = f
			p3Found = true
		}
	}
	if autoCount != 10 {
		t.Errorf("automated findings: want 10 (probe-on promotes P3), got %d", autoCount)
	}
	if reviewCount != 6 {
		t.Errorf("requires-review findings: want 6 (probe-on promotes P3), got %d", reviewCount)
	}
	if !p3Found {
		t.Fatalf("P3 finding missing")
	}
	if p3.Kind != "automated" {
		t.Errorf("P3.Kind = %q, want %q (probe-on must promote P3)", p3.Kind, "automated")
	}
}

// TestAuditP3PromotionDeterministic extends TestAuditDeterministicByteIdentical
// to cover the probe-on surface. Two `audit --probe --deterministic` runs
// against the deterministic fixture must produce byte-identical stdout —
// the rerun-derived evidence string is keyed only on the rerun count and
// command list, never on a timestamp or PID, so it stays stable.
func TestAuditP3PromotionDeterministic(t *testing.T) {
	bin := buildP3Passing(t)
	desc := fixtureDescriptorPath(t, "p3-passing.yaml")

	out1, _, code1 := runAudit(t,
		"audit", bin,
		"--probe",
		"--descriptor", desc,
		"--output", "json",
		"--deterministic",
	)
	if code1 != 0 && code1 != 1 {
		t.Fatalf("first run exit code: want 0 or 1, got %d", code1)
	}
	out2, _, code2 := runAudit(t,
		"audit", bin,
		"--probe",
		"--descriptor", desc,
		"--output", "json",
		"--deterministic",
	)
	if code2 != 0 && code2 != 1 {
		t.Fatalf("second run exit code: want 0 or 1, got %d", code2)
	}
	if !bytes.Equal(out1, out2) {
		t.Fatalf("byte-identical determinism violated under --probe\nrun1=%s\nrun2=%s", out1, out2)
	}
}
