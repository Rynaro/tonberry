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

// conformantFixtures is every fixture under fixtures/conformant/: "conformant"
// means exit 0 even in --mode block — it does NOT mean zero findings (a
// SHOULD-level advisory fail, e.g. C7/C8, never blocks; see
// TestConformantZeroFindings below for the stronger, narrower claim).
var conformantFixtures = []string{
	"trivial-typo-fix", "lite-add-flag", "full-new-subsystem", "trivial-no-spec",
	"ears-complete", "lite-ears-complete",
	"fresh-context-attested", "fresh-context-no-envelope",
	"memory-preflight-recorded", "memory-preflight-skipped",
}

func TestConformantExitZeroInBlock(t *testing.T) {
	for _, name := range conformantFixtures {
		rep := checkFixture(t, "conformant", name, ModeBlock)
		if rep.ExitCode != 0 {
			t.Errorf("conformant %s: exit %d, want 0", name, rep.ExitCode)
		}
	}
}

// TestConformantZeroFindings is the STRONGER claim (zero findings of any kind,
// not just "no MUST fail") for the fixtures that genuinely predate no advisory
// checks OR were authored with v1.1 attestation in mind. `lite-add-flag`,
// `full-new-subsystem`, and `lite-ears-complete` are EXCLUDED here: they are
// eidolons-esl `examples/` fixtures authored before C8 existed (pre-v1.1) —
// their `verify.envelope.json` has no `ise.verification` sub-block, so C8
// legitimately records a SHOULD-level advisory fail on them now (still exit 0;
// see TestConformantExitZeroInBlock). This is the intended, additive-only
// behavior of a new advisory check landing on pre-existing conformant history,
// not a fixture bug — re-vendoring the upstream examples with an attestation
// is upstream's call, not tonberry's to fabricate.
func TestConformantZeroFindings(t *testing.T) {
	for _, name := range []string{
		"trivial-typo-fix", "trivial-no-spec", "ears-complete",
		"fresh-context-attested", "fresh-context-no-envelope",
		"memory-preflight-recorded", "memory-preflight-skipped",
	} {
		rep := checkFixture(t, "conformant", name, ModeBlock)
		if rep.HasFail {
			t.Errorf("conformant %s: unexpected fail finding(s): %+v", name, rep.Results)
		}
	}
}

// TestC7EARSComplete: an EARS-complete item (all 4 fields) → C7 ok, exit 0.
func TestC7EARSComplete(t *testing.T) {
	rep := checkFixture(t, "conformant", "ears-complete", ModeBlock)
	r, ok := findResult(rep, "C7")
	if !ok {
		t.Fatalf("ears-complete: expected a C7 finding, got %+v", rep.Results)
	}
	if r.Level != "SHOULD" {
		t.Errorf("ears-complete: C7 level = %q, want SHOULD", r.Level)
	}
	if r.Status != "ok" {
		t.Errorf("ears-complete: C7 status = %q, want ok", r.Status)
	}
	if rep.ExitCode != 0 {
		t.Errorf("ears-complete block exit = %d, want 0", rep.ExitCode)
	}
}

// TestC7EARSMissingFieldAdvisory is the LOAD-BEARING advisory proof: a genuine
// C7 fail must NOT escalate the exit code — exit 0 even in --mode block, because
// C7 is SHOULD-level and only the MUST checks C1–C6 block.
func TestC7EARSMissingFieldAdvisory(t *testing.T) {
	rep := checkFixture(t, "failing", "ears-missing-field", ModeBlock)
	r, ok := findResult(rep, "C7")
	if !ok || r.Status != "fail" {
		t.Fatalf("ears-missing-field: expected C7 fail, got %+v", rep.Results)
	}
	if r.Level != "SHOULD" {
		t.Errorf("ears-missing-field: C7 level = %q, want SHOULD", r.Level)
	}
	if rep.ExitCode != 0 {
		t.Errorf("ADVISORY VIOLATION: ears-missing-field block exit = %d, want 0 (C7 must never block)", rep.ExitCode)
	}
	if !rep.HasFail {
		t.Errorf("ears-missing-field: HasFail should be true (the C7 warning is still reported)")
	}
}

// TestC7PlainStringNoFinding: plain-string and minimal {id,verify_method} items
// produce NO C7 finding (existing examples stay clean).
func TestC7PlainStringNoFinding(t *testing.T) {
	for _, name := range []string{"trivial-typo-fix", "lite-add-flag", "full-new-subsystem"} {
		rep := checkFixture(t, "conformant", name, ModeBlock)
		if _, has := findResult(rep, "C7"); has {
			t.Errorf("%s: expected NO C7 finding (minimal-object acceptance form), got %+v", name, rep.Results)
		}
	}
}

// TestC8FreshContextAttested: a verify.envelope.json with a complete, honest
// ise.verification sub-block (fresh_context:true, checker!=maker,
// transcript_access:artifact-only) → C8 ok, exit 0.
func TestC8FreshContextAttested(t *testing.T) {
	rep := checkFixture(t, "conformant", "fresh-context-attested", ModeBlock)
	r, ok := findResult(rep, "C8")
	if !ok {
		t.Fatalf("fresh-context-attested: expected a C8 finding, got %+v", rep.Results)
	}
	if r.Level != "SHOULD" {
		t.Errorf("fresh-context-attested: C8 level = %q, want SHOULD", r.Level)
	}
	if r.Status != "ok" {
		t.Errorf("fresh-context-attested: C8 status = %q, want ok (reason=%q)", r.Status, r.Reason)
	}
	if rep.ExitCode != 0 {
		t.Errorf("fresh-context-attested block exit = %d, want 0", rep.ExitCode)
	}
}

// TestC8NoEnvelopeNoRecord: status=verified with NO verify.envelope.json at all
// produces NO C8 record (skip, not fail) — distinct from an absent
// ise.verification sub-block inside a present envelope, which IS a fail.
func TestC8NoEnvelopeNoRecord(t *testing.T) {
	rep := checkFixture(t, "conformant", "fresh-context-no-envelope", ModeBlock)
	if _, has := findResult(rep, "C8"); has {
		t.Errorf("fresh-context-no-envelope: expected NO C8 finding (no envelope present), got %+v", rep.Results)
	}
	if rep.ExitCode != 0 || rep.HasFail {
		t.Errorf("fresh-context-no-envelope: expected clean conformant report, got exit=%d hasFail=%v results=%+v", rep.ExitCode, rep.HasFail, rep.Results)
	}
}

// TestC8FreshContextSameSessionAdvisory is the LOAD-BEARING advisory proof for
// C8 (mirroring TestC7EARSMissingFieldAdvisory): a genuine C8 fail must NOT
// escalate the exit code — exit 0 even in --mode block, because C8 is
// SHOULD-level and only the MUST checks C1–C6 block. This fixture also proves
// C8 is a STRICTER check than C4: manifest-level maker!=checker holds (C4 ok),
// yet the attestation reveals same-session self-review (C8 fail).
func TestC8FreshContextSameSessionAdvisory(t *testing.T) {
	rep := checkFixture(t, "failing", "fresh-context-same-session", ModeBlock)
	c4, ok := findResult(rep, "C4")
	if !ok || c4.Status != "ok" {
		t.Fatalf("fresh-context-same-session: expected C4 ok (manifest-level maker!=checker), got %+v", rep.Results)
	}
	c8, ok := findResult(rep, "C8")
	if !ok || c8.Status != "fail" {
		t.Fatalf("fresh-context-same-session: expected C8 fail, got %+v", rep.Results)
	}
	if c8.Level != "SHOULD" {
		t.Errorf("fresh-context-same-session: C8 level = %q, want SHOULD", c8.Level)
	}
	if rep.ExitCode != 0 {
		t.Errorf("ADVISORY VIOLATION: fresh-context-same-session block exit = %d, want 0 (C8 must never block)", rep.ExitCode)
	}
	if !rep.HasFail {
		t.Errorf("fresh-context-same-session: HasFail should be true (the C8 warning is still reported)")
	}
}

// TestC8OnlyAppliesAtVerifiedOrArchived: a proposed-status change (even one that
// happens to carry an ise.verification-shaped file, which none of our fixtures
// do) must never record a C8 finding — the memory_preflight fixtures are
// proposed-status and have no verify envelope, so C8 must be silent.
func TestC8OnlyAppliesAtVerifiedOrArchived(t *testing.T) {
	for _, name := range []string{"memory-preflight-recorded", "memory-preflight-skipped"} {
		rep := checkFixture(t, "conformant", name, ModeBlock)
		if _, has := findResult(rep, "C8"); has {
			t.Errorf("%s: expected NO C8 finding (status=proposed), got %+v", name, rep.Results)
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
