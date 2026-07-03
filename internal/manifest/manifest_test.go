package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	specRef := "spec.md"
	drift := true
	in := &Change{
		ESLVersion:       ESLVersion,
		ChangeID:         "round-trip",
		Status:           StatusVerified,
		Tier:             TierLite,
		Maker:            "vivi",
		Checker:          "kupo-verifier",
		AcceptanceChecks: []AcceptanceCheck{{ID: "AC-1", VerifyMethod: "bats"}},
		SpecRef:          &specRef,
		DriftChecked:     &drift,
	}
	p, err := Write(dir, in)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != ManifestFile {
		t.Errorf("wrote %s, want change.json", p)
	}
	out, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out.ChangeID != in.ChangeID || out.Status != in.Status || out.Tier != in.Tier {
		t.Errorf("round-trip mismatch: %+v vs %+v", out, in)
	}
	if out.SpecRef == nil || *out.SpecRef != specRef {
		t.Errorf("spec_ref lost in round-trip")
	}
	if !out.DriftCheckedTrue() {
		t.Errorf("drift_checked lost in round-trip")
	}
}

// TestHasCodeRoundTrip proves the OPTIONAL has_code hint (ESL §3.2) survives a
// Write/Read round-trip and that an absent has_code reads as false.
func TestHasCodeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	specRef := "spec.md"
	hc := true
	in := &Change{
		ESLVersion:       ESLVersion,
		ChangeID:         "hascode-rt",
		Status:           StatusProposed,
		Tier:             TierFull,
		Maker:            "vivi",
		Checker:          "vigil",
		AcceptanceChecks: []AcceptanceCheck{{ID: "AC-1"}},
		SpecRef:          &specRef,
		HasCode:          &hc,
	}
	if _, err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasCodeTrue() {
		t.Errorf("has_code lost in round-trip: %+v", out.HasCode)
	}

	// Absent has_code reads as false.
	in.HasCode = nil
	if _, err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	out2, _ := Read(dir)
	if out2.HasCodeTrue() {
		t.Errorf("absent has_code should read as false, got %+v", out2.HasCode)
	}
}

// TestMemoryPreflightRoundTrip proves the OPTIONAL v1.1 memory_preflight
// record (ESL §2.6) survives a Write/Read round-trip, including the
// records:0 graceful-skip case, and that an absent memory_preflight is nil
// (not a zero-value struct) after round-trip.
func TestMemoryPreflightRoundTrip(t *testing.T) {
	dir := t.TempDir()
	specRef := "spec.md"
	in := &Change{
		ESLVersion:       ESLVersion,
		ChangeID:         "preflight-rt",
		Status:           StatusProposed,
		Tier:             TierTrivial,
		Maker:            "vivi",
		Checker:          "vigil",
		AcceptanceChecks: []AcceptanceCheck{{ID: "AC-1"}},
		SpecRef:          &specRef,
		MemoryPreflight:  &MemoryPreflight{Ran: true, Records: 3},
	}
	if _, err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out.MemoryPreflight == nil || !out.MemoryPreflight.Ran || out.MemoryPreflight.Records != 3 {
		t.Errorf("memory_preflight lost in round-trip: %+v", out.MemoryPreflight)
	}

	// The graceful-skip form: ran:false, records:0 is a distinct, valid,
	// explicitly-recorded state — NOT the same as an absent field.
	in.MemoryPreflight = &MemoryPreflight{Ran: false, Records: 0}
	if _, err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	out2, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out2.MemoryPreflight == nil || out2.MemoryPreflight.Ran || out2.MemoryPreflight.Records != 0 {
		t.Errorf("memory_preflight graceful-skip form lost in round-trip: %+v", out2.MemoryPreflight)
	}

	// Absence: nil stays nil (the field is omitempty and fully conformant absent).
	in.MemoryPreflight = nil
	if _, err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	out3, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out3.MemoryPreflight != nil {
		t.Errorf("absent memory_preflight should read as nil, got %+v", out3.MemoryPreflight)
	}
}

// TestValidateRejectsNegativeMemoryPreflightRecords proves the schema's
// `records: minimum 0` constraint (ESL §2.6) is enforced by Validate.
func TestValidateRejectsNegativeMemoryPreflightRecords(t *testing.T) {
	specRef := "spec.md"
	c := &Change{
		ESLVersion:       ESLVersion,
		ChangeID:         "bad-preflight",
		Status:           StatusProposed,
		Tier:             TierTrivial,
		Maker:            "vivi",
		Checker:          "vigil",
		AcceptanceChecks: []AcceptanceCheck{{ID: "AC-1"}},
		SpecRef:          &specRef,
		MemoryPreflight:  &MemoryPreflight{Ran: true, Records: -1},
	}
	errs := Validate(c)
	found := false
	for _, e := range errs {
		if contains(e, "memory_preflight") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a memory_preflight.records validation error, got %v", errs)
	}
}

// TestAcceptanceCheckOneOfRoundTrip exercises the oneOf:[string, object]
// acceptance_checks item shape (ESL §2.5): plain-string, minimal-object, and
// EARS-object items must all round-trip through Read/Write unchanged.
func TestAcceptanceCheckOneOfRoundTrip(t *testing.T) {
	dir := t.TempDir()
	specRef := "spec.md"
	in := &Change{
		ESLVersion: ESLVersion,
		ChangeID:   "oneof-round-trip",
		Status:     StatusProposed,
		Tier:       TierLite,
		Maker:      "vivi",
		Checker:    "vigil",
		AcceptanceChecks: []AcceptanceCheck{
			{Raw: "AC-0: plain string criterion"},
			{ID: "AC-1", VerifyMethod: "bats: x"},
			{ID: "AC-2", Given: "g", When: "w", Then: "t", VerifyMethod: "bats: y"},
		},
		SpecRef: &specRef,
	}
	if _, err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.AcceptanceChecks) != 3 {
		t.Fatalf("expected 3 acceptance_checks, got %d", len(out.AcceptanceChecks))
	}
	if out.AcceptanceChecks[0].Raw != "AC-0: plain string criterion" {
		t.Errorf("plain-string item lost: %+v", out.AcceptanceChecks[0])
	}
	if out.AcceptanceChecks[0].IsEARS() {
		t.Errorf("plain-string item must not be EARS")
	}
	if out.AcceptanceChecks[1].ID != "AC-1" || out.AcceptanceChecks[1].IsEARS() {
		t.Errorf("minimal-object item wrong: %+v", out.AcceptanceChecks[1])
	}
	ac2 := out.AcceptanceChecks[2]
	if ac2.ID != "AC-2" || ac2.Given != "g" || ac2.When != "w" || ac2.Then != "t" || !ac2.IsEARS() {
		t.Errorf("EARS item lost fields: %+v", ac2)
	}
}

// TestValidateAcceptsPlainStringAcceptance proves a plain-string acceptance item
// (no id) is valid — the oneOf:[string,...] backward-compat form.
func TestValidateAcceptsPlainStringAcceptance(t *testing.T) {
	specRef := "spec.md"
	c := &Change{
		ESLVersion:       "1.0",
		ChangeID:         "plain-ac",
		Status:           StatusProposed,
		Tier:             TierLite,
		Maker:            "vivi",
		Checker:          "vigil",
		AcceptanceChecks: []AcceptanceCheck{{Raw: "AC-1: dry-run exits 0"}},
		SpecRef:          &specRef,
	}
	if errs := Validate(c); len(errs) != 0 {
		t.Errorf("plain-string acceptance item should validate, got %v", errs)
	}
}

func TestValidateRejectsIllegalEnums(t *testing.T) {
	c := &Change{
		ESLVersion:       "1.0",
		ChangeID:         "x",
		Status:           "in-review", // illegal
		Tier:             "medium",    // illegal
		Maker:            "vivi",
		Checker:          "vigil",
		AcceptanceChecks: []AcceptanceCheck{},
	}
	errs := Validate(c)
	if len(errs) < 2 {
		t.Errorf("expected status+tier enum errors, got %v", errs)
	}
}

func TestValidateRequiresFields(t *testing.T) {
	errs := Validate(&Change{})
	// esl_version, change_id, status, tier, maker, checker, acceptance_checks
	if len(errs) < 6 {
		t.Errorf("expected several required-field errors, got %v", errs)
	}
}

func TestValidateAcceptsConformant(t *testing.T) {
	specRef := "spec.md"
	c := &Change{
		ESLVersion:       "1.0",
		ChangeID:         "ok-change",
		Status:           StatusProposed,
		Tier:             TierLite,
		Maker:            "vivi",
		Checker:          "vigil",
		AcceptanceChecks: []AcceptanceCheck{{ID: "AC-1"}},
		SpecRef:          &specRef,
	}
	if errs := Validate(c); len(errs) != 0 {
		t.Errorf("conformant change should validate, got %v", errs)
	}
}

func TestValidateChangeIDPattern(t *testing.T) {
	c := &Change{
		ESLVersion:       "1.0",
		ChangeID:         "Not_Kebab",
		Status:           StatusProposed,
		Tier:             TierTrivial,
		Maker:            "kupo",
		Checker:          "vigil",
		AcceptanceChecks: []AcceptanceCheck{{ID: "AC-1"}},
	}
	found := false
	for _, e := range Validate(c) {
		if filepath.Base(e) != "" && contains(e, "change_id") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected change_id pattern error")
	}
}

func TestReadMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(dir); err == nil {
		t.Errorf("Read of empty dir should error")
	}
	_ = os.WriteFile(filepath.Join(dir, "change.json"), []byte("{not json"), 0o644)
	if _, err := Read(dir); err == nil {
		t.Errorf("Read of invalid JSON should error")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
