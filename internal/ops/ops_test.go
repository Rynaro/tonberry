package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Rynaro/tonberry/internal/manifest"
)

// truePtr / falsePtr build *bool flag values for the persist/has_code inputs.
func truePtr() *bool  { b := true; return &b }
func falsePtr() *bool { b := false; return &b }

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

	// right_size (full: rubric 9) — persists the tier by default (v0.4.0)
	rs, err := RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "feature-x", FilesTouched: 6, RubricScore: 9})
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

	// transition proposed -> deliberated (full tier allows it); persists by default
	tr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "feature-x", ToStatus: "deliberated", HasCode: truePtr()})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Allowed {
		t.Errorf("proposed->deliberated (full) should be allowed: %s", tr.Reason)
	}

	// deliberated -> in_progress (code)
	if tr, _ := Transition(TransitionInput{ProjectRoot: root, ChangeID: "feature-x", ToStatus: "in_progress", HasCode: truePtr()}); !tr.Allowed {
		t.Errorf("deliberated->in_progress should be allowed: %s", tr.Reason)
	}
	// in_progress -> verified
	if tr, _ := Transition(TransitionInput{ProjectRoot: root, ChangeID: "feature-x", ToStatus: "verified", HasCode: truePtr()}); !tr.Allowed {
		t.Errorf("in_progress->verified should be allowed: %s", tr.Reason)
	}

	// drift_check by a distinct checker, no mismatches -> drift_checked=true (persists by default)
	dc, err := DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "feature-x", Checker: "vigil"})
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

	// archive MOVES the folder (FIX 3): the active folder is gone, and the final
	// manifest lives under the archive path.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("active folder should be gone after archive (move), stat err=%v", err)
	}
	archDir := filepath.Join(root, ChangesRoot, "archive", "2026-06-25-feature-x")
	c, err := manifest.Read(archDir)
	if err != nil {
		t.Fatalf("archived manifest unreadable: %v", err)
	}
	if c.Status != manifest.StatusArchived || !c.DriftCheckedTrue() {
		t.Errorf("final manifest not archived/drift_checked: %+v", c)
	}
}

func TestTransitionRejectsIllegalSkip(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "lite-x", Maker: "vivi", SpecRef: "spec.md", Checker: "vigil"})
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "lite-x", FilesTouched: 1, RubricScore: 5})

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
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "nodrift", FilesTouched: 6, RubricScore: 9})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "nodrift", ToStatus: "in_progress", HasCode: truePtr()})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "nodrift", ToStatus: "verified", HasCode: truePtr()})

	// transition to archived must be rejected (drift not set)
	tr, _ := Transition(TransitionInput{ProjectRoot: root, ChangeID: "nodrift", ToStatus: "archived", HasCode: truePtr()})
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

// -- FIX 1: has_code read from the manifest (+ flag override) --------------- //

// TestTransitionReadsHasCodeFromManifest: a change proposed with has_code=true
// can advance proposed->in_progress WITHOUT passing has_code on the transition
// (the manifest hint is read). The dual proves the friction: no has_code anywhere
// blocks in_progress.
func TestTransitionReadsHasCodeFromManifest(t *testing.T) {
	root := t.TempDir()
	// has_code declared at propose.
	if _, err := Propose(ProposeInput{ProjectRoot: root, ChangeID: "code-x", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md", HasCode: truePtr()}); err != nil {
		t.Fatal(err)
	}
	if _, err := RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "code-x", FilesTouched: 6, RubricScore: 9}); err != nil {
		t.Fatal(err)
	}
	// transition WITHOUT a per-call has_code flag — the manifest hint must drive it.
	tr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "code-x", ToStatus: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Allowed {
		t.Errorf("proposed->in_progress should be allowed from the manifest has_code hint: %s", tr.Reason)
	}

	// Control: a sibling with no has_code hint is BLOCKED from in_progress.
	if _, err := Propose(ProposeInput{ProjectRoot: root, ChangeID: "nocode-x", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md"}); err != nil {
		t.Fatal(err)
	}
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "nocode-x", FilesTouched: 1, RubricScore: 5})
	ntr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "nocode-x", ToStatus: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	if ntr.Allowed {
		t.Errorf("no has_code hint should block in_progress, got allowed")
	}
}

// TestTransitionHasCodeFlagOverridesManifest: an explicit per-transition has_code
// flag wins over the manifest hint in BOTH directions (back-compat).
func TestTransitionHasCodeFlagOverridesManifest(t *testing.T) {
	root := t.TempDir()
	// Manifest says has_code=false, but the explicit flag true must allow in_progress.
	if _, err := Propose(ProposeInput{ProjectRoot: root, ChangeID: "ovr-x", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md", HasCode: falsePtr()}); err != nil {
		t.Fatal(err)
	}
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "ovr-x", FilesTouched: 6, RubricScore: 9})
	tr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "ovr-x", ToStatus: "in_progress", HasCode: truePtr()})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Allowed {
		t.Errorf("explicit has_code=true must override manifest false: %s", tr.Reason)
	}

	// Manifest says has_code=true, but the explicit flag false must BLOCK in_progress.
	if _, err := Propose(ProposeInput{ProjectRoot: root, ChangeID: "ovr-y", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md", HasCode: truePtr()}); err != nil {
		t.Fatal(err)
	}
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "ovr-y", FilesTouched: 6, RubricScore: 9})
	tr2, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "ovr-y", ToStatus: "in_progress", HasCode: falsePtr(), DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if tr2.Allowed {
		t.Errorf("explicit has_code=false must override manifest true (block in_progress)")
	}
}

// TestProposePersistsHasCode: propose --has_code persists has_code into change.json.
func TestProposePersistsHasCode(t *testing.T) {
	root := t.TempDir()
	if _, err := Propose(ProposeInput{ProjectRoot: root, ChangeID: "hc", Maker: "vivi", Checker: "vigil", HasCode: truePtr()}); err != nil {
		t.Fatal(err)
	}
	c, err := manifest.Read(filepath.Join(root, ChangesRoot, "hc"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.HasCodeTrue() {
		t.Errorf("propose --has_code should persist has_code=true, got %+v", c.HasCode)
	}
}

// -- FIX 2: persist-by-default for transition / right_size / drift_check ----- //

// TestTransitionPersistsByDefault: a transition with no --write_manifest now
// writes the new status; --dry-run leaves the manifest at the old status.
func TestTransitionPersistsByDefault(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "p", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md", HasCode: truePtr()})
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "p", FilesTouched: 6, RubricScore: 9})
	dir := filepath.Join(root, ChangesRoot, "p")

	// Default: persist.
	tr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "p", ToStatus: "in_progress"})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Allowed || !tr.Persisted {
		t.Fatalf("transition should persist by default: %+v", tr)
	}
	if c, _ := manifest.Read(dir); c.Status != manifest.StatusInProgress {
		t.Errorf("manifest status = %q, want in_progress (persisted by default)", c.Status)
	}

	// --dry-run: allowed but NOT persisted; status stays in_progress.
	dr, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "p", ToStatus: "verified", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dr.Allowed {
		t.Errorf("dry-run transition should still evaluate allowed: %s", dr.Reason)
	}
	if dr.Persisted || dr.ManifestPath != "" {
		t.Errorf("dry-run must not persist: %+v", dr)
	}
	if c, _ := manifest.Read(dir); c.Status != manifest.StatusInProgress {
		t.Errorf("dry-run must not advance status; got %q, want in_progress", c.Status)
	}

	// Explicit --write_manifest=false (back-compat) also does not persist.
	wf, err := Transition(TransitionInput{ProjectRoot: root, ChangeID: "p", ToStatus: "verified", WriteManifest: falsePtr()})
	if err != nil {
		t.Fatal(err)
	}
	if wf.Persisted {
		t.Errorf("explicit write_manifest=false must not persist: %+v", wf)
	}
}

// TestRightSizePersistsByDefault: right_size with a change_id now writes the tier;
// --dry-run does not; no change_id is still pure classification (no error, no write).
func TestRightSizePersistsByDefault(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "rs", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md"})
	dir := filepath.Join(root, ChangesRoot, "rs")

	rs, err := RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "rs", FilesTouched: 6, RubricScore: 9})
	if err != nil {
		t.Fatal(err)
	}
	if !rs.Persisted || rs.Tier != "full" {
		t.Fatalf("right_size should persist tier by default: %+v", rs)
	}
	if c, _ := manifest.Read(dir); c.Tier != manifest.TierFull {
		t.Errorf("tier not persisted: %q", c.Tier)
	}

	// dry-run on a lite reclassification must NOT overwrite the persisted full tier.
	dr, err := RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "rs", FilesTouched: 1, RubricScore: 5, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dr.Persisted || dr.Tier != "lite" {
		t.Errorf("dry-run right_size should classify lite without persisting: %+v", dr)
	}
	if c, _ := manifest.Read(dir); c.Tier != manifest.TierFull {
		t.Errorf("dry-run must not overwrite tier; got %q, want full", c.Tier)
	}

	// No change_id: pure classification, no error, no persistence.
	pc, err := RightSize(RightSizeInput{FilesTouched: 6, RubricScore: 9})
	if err != nil {
		t.Fatalf("classification without change_id must not error: %v", err)
	}
	if pc.Persisted || pc.Tier != "full" {
		t.Errorf("classification-only result wrong: %+v", pc)
	}
}

// TestDriftCheckPersistsByDefault: a clean drift_check now writes drift_checked;
// --dry-run does not.
func TestDriftCheckPersistsByDefault(t *testing.T) {
	root := t.TempDir()
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "dc", Maker: "vivi", Checker: "vigil"})
	dir := filepath.Join(root, ChangesRoot, "dc")

	dc, err := DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "dc", Checker: "vigil"})
	if err != nil {
		t.Fatal(err)
	}
	if !dc.DriftChecked || !dc.Persisted {
		t.Fatalf("drift_check should persist by default: %+v", dc)
	}
	if c, _ := manifest.Read(dir); !c.DriftCheckedTrue() {
		t.Errorf("drift_checked not persisted")
	}

	// Reset + dry-run: verdict drift_checked true but NOT persisted.
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "dc2", Maker: "vivi", Checker: "vigil"})
	dir2 := filepath.Join(root, ChangesRoot, "dc2")
	dr, err := DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "dc2", Checker: "vigil", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dr.DriftChecked || dr.Persisted {
		t.Errorf("dry-run drift_check verdict should be clean but not persisted: %+v", dr)
	}
	if c, _ := manifest.Read(dir2); c.DriftCheckedTrue() {
		t.Errorf("dry-run must not persist drift_checked")
	}
}

// -- FIX 3: assess counts active + archived; list --all --------------------- //

// TestAssessCountsArchived: after a full lifecycle ends in archive (the active
// folder is MOVED away), assess's change_count + full_ratio still count the
// archived change — archiving must NOT drop the escalation signal.
func TestAssessCountsArchived(t *testing.T) {
	root := t.TempDir()
	// One full change, drive it all the way to archived.
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "fa", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md", HasCode: truePtr()})
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "fa", FilesTouched: 6, RubricScore: 9})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "fa", ToStatus: "in_progress"})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "fa", ToStatus: "verified"})
	_, _ = DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "fa", Checker: "vigil"})
	if _, err := Archive(ArchiveInput{ProjectRoot: root, ChangeID: "fa", Date: "2026-06-25"}); err != nil {
		t.Fatal(err)
	}

	// Active folder is gone (moved).
	if _, err := os.Stat(filepath.Join(root, ChangesRoot, "fa")); !os.IsNotExist(err) {
		t.Errorf("active change folder should be gone after archive (move), stat err=%v", err)
	}

	a, err := Assess(AssessInput{ProjectRoot: root, RepoLOC: 100})
	if err != nil {
		t.Fatal(err)
	}
	if a.Signals.ChangeCount != 1 {
		t.Errorf("change_count = %d, want 1 (archived change must be counted)", a.Signals.ChangeCount)
	}
	if a.Signals.FullRatio != 1.0 {
		t.Errorf("full_ratio = %v, want 1.0 (archived full change counts)", a.Signals.FullRatio)
	}
}

// TestListActiveDefaultAndIncludeArchived: list shows active by default; an
// archived change appears only with IncludeArchived.
func TestListActiveDefaultAndIncludeArchived(t *testing.T) {
	root := t.TempDir()
	// An active lite change.
	seedChange(t, root, "active-1", 1, 5, false)
	// An archived full change (full lifecycle -> archive moves it).
	_, _ = Propose(ProposeInput{ProjectRoot: root, ChangeID: "arch-1", Maker: "vivi", Checker: "vigil", SpecRef: "spec.md", HasCode: truePtr()})
	_, _ = RightSize(RightSizeInput{ProjectRoot: root, ChangeID: "arch-1", FilesTouched: 6, RubricScore: 9})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "arch-1", ToStatus: "in_progress"})
	_, _ = Transition(TransitionInput{ProjectRoot: root, ChangeID: "arch-1", ToStatus: "verified"})
	_, _ = DriftCheck(DriftCheckInput{ProjectRoot: root, ChangeID: "arch-1", Checker: "vigil"})
	if _, err := Archive(ArchiveInput{ProjectRoot: root, ChangeID: "arch-1", Date: "2026-06-25"}); err != nil {
		t.Fatal(err)
	}

	// Default: only the active change.
	def, err := List(ListInput{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if def.Count != 1 || def.Changes[0].ChangeID != "active-1" {
		t.Errorf("default list should show only active-1, got %+v", def.Changes)
	}

	// IncludeArchived: both, with the archived row flagged.
	all, err := List(ListInput{ProjectRoot: root, IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	if all.Count != 2 {
		t.Fatalf("include-archived list count = %d, want 2", all.Count)
	}
	var archived *ChangeSummary
	for i := range all.Changes {
		if all.Changes[i].ChangeID == "arch-1" {
			archived = &all.Changes[i]
		}
	}
	if archived == nil || !archived.Archived || archived.Status != "archived" {
		t.Errorf("arch-1 should appear archived with status=archived: %+v", all.Changes)
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
