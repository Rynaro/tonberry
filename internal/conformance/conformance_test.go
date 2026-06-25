package conformance

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "fixtures"))
}

// findResult returns the first result with the given id (or empty).
func findResult(rep Report, id string) (Result, bool) {
	for _, r := range rep.Results {
		if r.ID == id {
			return r, true
		}
	}
	return Result{}, false
}

func countID(rep Report, id string) int {
	n := 0
	for _, r := range rep.Results {
		if r.ID == id {
			n++
		}
	}
	return n
}

func checkFixture(t *testing.T, group, name string, mode Mode) Report {
	t.Helper()
	dir := filepath.Join(fixtureRoot(t), group, name)
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs %s: %v", dir, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fixture %s missing: %v", abs, err)
	}
	return Check(abs, mode)
}

func TestConformantExitZeroInBlock(t *testing.T) {
	for _, name := range []string{"trivial-typo-fix", "lite-add-flag", "full-new-subsystem", "trivial-no-spec"} {
		rep := checkFixture(t, "conformant", name, ModeBlock)
		if rep.ExitCode != 0 {
			t.Errorf("conformant %s: exit %d, want 0", name, rep.ExitCode)
		}
		if rep.HasFail {
			t.Errorf("conformant %s: unexpected fail finding(s): %+v", name, rep.Results)
		}
	}
}

func TestC1BadJSON(t *testing.T) {
	rep := checkFixture(t, "failing", "bad-json", ModeBlock)
	r, ok := findResult(rep, "C1")
	if !ok || r.Status != "fail" {
		t.Fatalf("bad-json: expected C1 fail, got %+v", rep.Results)
	}
	if rep.ExitCode != 3 {
		t.Errorf("bad-json block exit = %d, want 3", rep.ExitCode)
	}
	// When C1 fails, C2–C5 MUST NOT run (changeOK guard). Only C1 (+ any C6) present.
	if _, has := findResult(rep, "C2a"); has {
		t.Errorf("bad-json: C2a should not run when C1 fails")
	}
}

func TestC2aIllegalStatus(t *testing.T) {
	rep := checkFixture(t, "failing", "illegal-status", ModeBlock)
	r, ok := findResult(rep, "C2a")
	if !ok || r.Status != "fail" {
		t.Fatalf("illegal-status: expected C2a fail, got %+v", rep.Results)
	}
	if rep.ExitCode != 3 {
		t.Errorf("illegal-status block exit = %d, want 3", rep.ExitCode)
	}
}

func TestC2bIllegalTier(t *testing.T) {
	rep := checkFixture(t, "failing", "illegal-tier", ModeBlock)
	r, ok := findResult(rep, "C2b")
	if !ok || r.Status != "fail" {
		t.Fatalf("illegal-tier: expected C2b fail, got %+v", rep.Results)
	}
	// C3 must record NOTHING for an illegal tier (the bash `*)` default).
	if _, has := findResult(rep, "C3"); has {
		t.Errorf("illegal-tier: C3 should record nothing for an illegal tier")
	}
	if rep.ExitCode != 3 {
		t.Errorf("illegal-tier block exit = %d, want 3", rep.ExitCode)
	}
}

func TestC3LiteMissingSpec(t *testing.T) {
	rep := checkFixture(t, "failing", "lite-missing-spec", ModeBlock)
	r, ok := findResult(rep, "C3")
	if !ok || r.Status != "fail" {
		t.Fatalf("lite-missing-spec: expected C3 fail, got %+v", rep.Results)
	}
}

func TestC3LiteEmptyAcceptance(t *testing.T) {
	rep := checkFixture(t, "failing", "lite-empty-acceptance", ModeBlock)
	r, ok := findResult(rep, "C3")
	if !ok || r.Status != "fail" {
		t.Fatalf("lite-empty-acceptance: expected C3 fail, got %+v", rep.Results)
	}
	if r.Reason == "" {
		t.Errorf("lite-empty-acceptance: expected a reason on C3")
	}
}

func TestC3FullMissingSpec(t *testing.T) {
	rep := checkFixture(t, "failing", "full-missing-spec", ModeBlock)
	r, ok := findResult(rep, "C3")
	if !ok || r.Status != "fail" {
		t.Fatalf("full-missing-spec: expected C3 fail, got %+v", rep.Results)
	}
}

func TestC4MakerEqualsChecker(t *testing.T) {
	rep := checkFixture(t, "failing", "maker-equals-checker", ModeBlock)
	r, ok := findResult(rep, "C4")
	if !ok || r.Status != "fail" {
		t.Fatalf("maker-equals-checker: expected C4 fail, got %+v", rep.Results)
	}
	if rep.ExitCode != 3 {
		t.Errorf("maker-equals-checker block exit = %d, want 3", rep.ExitCode)
	}
}

func TestC5ArchiveNoDrift(t *testing.T) {
	rep := checkFixture(t, "failing", "archive-no-drift", ModeBlock)
	r, ok := findResult(rep, "C5")
	if !ok || r.Status != "fail" {
		t.Fatalf("archive-no-drift: expected C5 fail, got %+v", rep.Results)
	}
}

func TestC6BadPerformative(t *testing.T) {
	rep := checkFixture(t, "failing", "bad-performative", ModeBlock)
	r, ok := findResult(rep, "C6")
	if !ok || r.Status != "fail" {
		t.Fatalf("bad-performative: expected C6 fail, got %+v", rep.Results)
	}
	if rep.ExitCode != 3 {
		t.Errorf("bad-performative block exit = %d, want 3", rep.ExitCode)
	}
}

func TestC6OneRecordPerEnvelope(t *testing.T) {
	// full-new-subsystem has 3 envelopes at depth 1 (propose, critique, verify);
	// the archive snapshot is at depth 2 and MUST NOT be counted (maxdepth 1).
	rep := checkFixture(t, "conformant", "full-new-subsystem", ModeBlock)
	if n := countID(rep, "C6"); n != 3 {
		t.Errorf("full-new-subsystem: C6 count = %d, want 3 (one per depth-1 envelope)", n)
	}
}

func TestWarnModeNeverExits3(t *testing.T) {
	// A failing fixture in warn mode must still exit 0 (advisory).
	rep := checkFixture(t, "failing", "maker-equals-checker", ModeWarn)
	if rep.ExitCode != 0 {
		t.Errorf("maker-equals-checker warn exit = %d, want 0", rep.ExitCode)
	}
	if !rep.HasFail {
		t.Errorf("maker-equals-checker warn: HasFail should be true (the violation is still reported)")
	}
}
