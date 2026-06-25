package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rynaro/tonberry/internal/manifest"
)

func writeVerifiedChange(t *testing.T, changeDir string, drift bool) {
	t.Helper()
	specRef := "spec.md"
	c := &manifest.Change{
		ESLVersion:       manifest.ESLVersion,
		ChangeID:         "arch-me",
		Status:           manifest.StatusVerified,
		Tier:             manifest.TierFull,
		Maker:            "vivi",
		Checker:          "vigil",
		AcceptanceChecks: []manifest.AcceptanceCheck{{ID: "AC-1"}},
		SpecRef:          &specRef,
	}
	if drift {
		d := true
		c.DriftChecked = &d
	}
	if _, err := manifest.Write(changeDir, c); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(changeDir, "spec.md"), []byte("# spec"), 0o644)
	_ = os.WriteFile(filepath.Join(changeDir, "spec.yaml"), []byte("k: v"), 0o644)
}

func TestArchiveRequiresDrift(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, ".spectra", "changes", "arch-me")
	writeVerifiedChange(t, changeDir, false) // drift NOT set
	if _, err := Archive(changeDir, "2026-06-25", ""); err == nil {
		t.Fatalf("archive without drift_checked must fail")
	}
}

func TestArchiveSnapshotAndPromotion(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, ".spectra", "changes", "arch-me")
	writeVerifiedChange(t, changeDir, true)

	res, err := Archive(changeDir, "2026-06-25", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivePath != "archive/2026-06-25-arch-me" {
		t.Errorf("archive_path = %q", res.ArchivePath)
	}
	if res.Status != "archived" {
		t.Errorf("status = %q", res.Status)
	}
	if res.PromotionPerformative != "INFORM" {
		t.Errorf("promotion performative = %q, want INFORM", res.PromotionPerformative)
	}

	// Snapshot folder exists with change.json + the promotion envelope.
	snapDir := filepath.Join(root, ".spectra", "changes", "archive", "2026-06-25-arch-me")
	if _, err := os.Stat(filepath.Join(snapDir, "change.json")); err != nil {
		t.Errorf("snapshot change.json missing: %v", err)
	}
	promo := filepath.Join(snapDir, PromotionEnvelopeName)
	data, err := os.ReadFile(promo)
	if err != nil {
		t.Fatalf("promotion envelope missing: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("promotion envelope not valid JSON: %v", err)
	}
	if m["performative"] != "INFORM" {
		t.Errorf("promotion performative field = %v", m["performative"])
	}
	cd, _ := m["context_delta"].(map[string]any)
	if cd == nil || cd["intent"] != "promotion" {
		t.Errorf("promotion intent must ride context_delta, got %v", m["context_delta"])
	}
	if cd["change_id"] != "arch-me" {
		t.Errorf("context_delta.change_id = %v", cd["change_id"])
	}

	// Snapshot manifest is status=archived with archive_path set.
	sc, err := manifest.Read(snapDir)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Status != manifest.StatusArchived || sc.ArchivePath == nil {
		t.Errorf("snapshot manifest not archived: %+v", sc)
	}

	// GRACEFUL DEGRADATION (FORGE acceptance gate): no CRYSTALIUM is involved at
	// all — archive completed and left the promotion intent on disk un-routed.
	// (There is no crystalium import/call anywhere; this test proves archive is
	// self-contained.)
}
