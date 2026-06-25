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
