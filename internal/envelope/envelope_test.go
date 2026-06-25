package envelope

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComposeValidPerformative(t *testing.T) {
	e, err := Compose("PROPOSE", "spectra", "vivi", Options{Objective: "hand off"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Performative != "PROPOSE" {
		t.Errorf("performative = %q", e.Performative)
	}
	if e.EnvelopeVersion != DefaultEnvelopeVersion {
		t.Errorf("envelope_version = %q, want %q", e.EnvelopeVersion, DefaultEnvelopeVersion)
	}
}

func TestComposeRejectsOutOfSetPerformative(t *testing.T) {
	if _, err := Compose("ACCEPT", "a", "b", Options{}); err == nil {
		t.Errorf("ACCEPT must be rejected (not in the closed ten-set)")
	}
}

func TestEnvelopeVersionOverride(t *testing.T) {
	if got := EnvelopeVersion("2.0"); got != "2.0" {
		t.Errorf("override = %q, want 2.0", got)
	}
	t.Setenv("TONBERRY_ECL_ENVELOPE_VERSION", "9.9")
	if got := EnvelopeVersion(""); got != "9.9" {
		t.Errorf("env override = %q, want 9.9", got)
	}
}

func TestWriteAndShape(t *testing.T) {
	dir := t.TempDir()
	e, _ := Compose("INFORM", "vigil", "orchestrator", Options{
		Objective:    "verify_pass",
		ContextDelta: map[string]interface{}{"intent": "promotion"},
	})
	p, err := Write(dir, "verify.envelope.json", e)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("written envelope is not valid JSON: %v", err)
	}
	if m["performative"] != "INFORM" {
		t.Errorf("performative field = %v", m["performative"])
	}
	if m["envelope_version"] != "1.0" {
		t.Errorf("envelope_version field = %v", m["envelope_version"])
	}
	from, _ := m["from"].(map[string]any)
	if from == nil || from["eidolon"] != "vigil" {
		t.Errorf("from.eidolon missing/wrong: %v", m["from"])
	}
	if filepath.Base(p) != "verify.envelope.json" {
		t.Errorf("basename = %s", filepath.Base(p))
	}
}

func TestAllClosedTenAccepted(t *testing.T) {
	for _, p := range []string{"REQUEST", "INFORM", "PROPOSE", "CRITIQUE", "DECIDE", "DELEGATE", "ACKNOWLEDGE", "ESCALATE", "RESUME", "REFUSE"} {
		if !ValidPerformative(p) {
			t.Errorf("%s should be in the closed-10 set", p)
		}
	}
	if ValidPerformative("ACCEPT") {
		t.Errorf("ACCEPT must NOT be valid")
	}
}
