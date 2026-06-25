package ops

import (
	"path/filepath"
	"testing"

	"github.com/Rynaro/tonberry/internal/manifest"
)

// TestFullLifecycle exercises propose -> right_size -> compose -> transition ->
// drift_check -> archive end to end against a temp project root.
func TestFullLifecycle(t *testing.T) {
	root := t.TempDir()

	// propose
	pr, err := Propose(ProposeInput{ProjectRoot: root, ChangeID: "feature-x", Maker: "vivi", SpecRef: "spec.md", Checker: "vigil"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Status != "proposed" || pr.Tier != "" {
		t.Errorf("propose: status=%q tier=%q (tier must be empty until right_size)", pr.Status, pr.Tier)
	}

	// right_size (full: rubric 9) and write the tier
	rs, err := RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "feature-x", FilesTouched: 6, RubricScore: 9, WriteManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if rs.Tier != "full" {
		t.Errorf("right_size tier = %q, want full", rs.Tier)
	}

	// add acceptance_checks + specs via compose_manifest patch
	specRef := "spec.md"
	cm, err := ComposeManifest(ComposeManifestInput{
		ProjectRoot: root, ChangeID: "feature-x",
		Patch: &manifest.Change{
			AcceptanceChecks: []manifest.AcceptanceCheck{{ID: "AC-1", VerifyMethod: "bats"}},
			SpecRef:          &specRef,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cm.Valid {
		t.Errorf("compose_manifest invalid: %v", cm.SchemaErrors)
	}

	dir := filepath.Join(root, ChangesRoot, "feature-x")

	// transition proposed -> deliberated (full tier allows it)
	tr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "feature-x", ToStatus: "deliberated", HasCode: true, WriteManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Allowed {
		t.Errorf("proposed->deliberated (full) should be allowed: %s", tr.Reason)
	}

	// deliberated -> in_progress (code)
	if tr, _ := Transition(TransitionInput{ProjectRoot: root, ChangeID: "feature-x", ToStatus: "in_progress", HasCode: true, WriteManifest: true}); !tr.Allowed {
		t.Errorf("deliberated->in_progress should be allowed: %s", tr.Reason)
	}
	// in_progress -> verified
	if tr, _ := Transition(TransitionInput{ProjectRoot: root, ChangeID: "feature-x", ToStatus: "verified", HasCode: true, WriteManifest: true}); !tr.Allowed {
		t.Errorf("in_progress->verified should be allowed: %s", tr.Reason)
	}

	// drift_check by a distinct checker, no mismatches -> drift_checked=true
	dc, err := DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "feature-x", Checker: "vigil", WriteManifest: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dc.DriftChecked || dc.Escalate {
		t.Errorf("drift_check should pass: %+v", dc)
	}

	// archive
	ar, err := Archive(ArchiveInput{ProjectRoot: root, ChangeID: "feature-x", Date: "2026-06-25"})
	if err != nil {
		t.Fatal(err)
	}
	if ar.Status != "archived" || ar.PromotionPerformative != "INFORM" {
		t.Errorf("archive result: %+v", ar)
	}

	// final manifest is archived + drift_checked
	c, _ := manifest.Read(dir)
	if c.Status != manifest.StatusArchived || !c.DriftCheckedTrue() {
		t.Errorf("final manifest not archived/drift_checked: %+v", c)
	}
}

func TestTransitionRejectsIllegalSkip(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "lite-x", Maker: "vivi", SpecRef: "spec.md", Checker: "vigil"})
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "lite-x", FilesTouched: 1, RubricScore: 5, WriteManifest: true})

	// lite -> deliberated is illegal
	tr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "lite-x", ToStatus: "deliberated"})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Allowed {
		t.Errorf("lite proposed->deliberated should be rejected, got allowed")
	}
}

func TestArchiveBlockedWithoutDrift(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "nodrift", Maker: "vivi", SpecRef: "spec.md", Checker: "vigil"})
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "nodrift", FilesTouched: 6, RubricScore: 9, WriteManifest: true})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "nodrift", ToStatus: "in_progress", HasCode: true, WriteManifest: true})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "nodrift", ToStatus: "verified", HasCode: true, WriteManifest: true})

	// transition to archived must be rejected (drift not set)
	tr, _ := Transition(TransitionInput{ProjectRoot: root, ChangeID: "nodrift", ToStatus: "archived", HasCode: true})
	if tr.Allowed {
		t.Errorf("archived without drift_checked should be rejected")
	}
	// archive op must also fail
	if _, err := Archive(ArchiveInput{ProjectRoot: root, ChangeID: "nodrift"}); err == nil {
		t.Errorf("Archive without drift_checked should error")
	}
}

func TestDriftCheckRejectsSameIdentity(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "id-x", Maker: "vivi", Checker: "vigil"})
	if _, err := DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "id-x", Checker: "vivi"}); err == nil {
		t.Errorf("drift_check with checker==maker should error")
	}
}

func TestDriftCheckMismatchEscalates(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "drift-x", Maker: "vivi", Checker: "vigil"})
	dc, err := DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "drift-x", Checker: "vigil", Mismatches: []string{"AC-1 diverged"}})
	if err != nil {
		t.Fatal(err)
	}
	if dc.DriftChecked || !dc.Escalate || dc.NextStatus != "in_progress" {
		t.Errorf("mismatch should ESCALATE to in_progress: %+v", dc)
	}
}

func TestComposeEnvelopeDerivesPerformative(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "env-x", Maker: "spectra", Checker: "vigil"})
	ce, err := ComposeEnvelope(ComposeEnvelopeInput{
		ProjectRoot: root, ChangeID: "env-x",
		Transition: "proposed", From: "spectra", To: "vivi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ce.Performative != "PROPOSE" {
		t.Errorf("derived performative = %q, want PROPOSE", ce.Performative)
	}
	if filepath.Base(ce.EnvelopePath) != "propose.envelope.json" {
		t.Errorf("basename = %s", filepath.Base(ce.EnvelopePath))
	}
}
