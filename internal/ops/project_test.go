package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// seedChange writes a minimal proposed manifest, then right-sizes (writing the
// tier) so the fixture has a real tier for list/assess.
func seedChange(t *testing.T, root, id string, files, rubric int, tradeoff bool) {
	t.Helper()
	if _, err := Propose(ProposeInput{ProjectRoot: root, ChangeID: id, Maker: "vivi", Checker: "vigil", SpecRef: "spec.md"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RightSize(RightSizeInput{ProjectRoot: root, ChangeID: id, FilesTouched: files, RubricScore: rubric, TradeoffPresent: tradeoff, WriteManifest: true}); err != nil {
		t.Fatal(err)
	}
}

func TestListOrderingAndArchiveSkip(t *testing.T) {
	root := t.TempDir()
	// Seed out of order to prove the sort.
	seedChange(t, root, "zeta-change", 1, 5, false)  // lite
	seedChange(t, root, "alpha-change", 1, 1, false) // trivial
	seedChange(t, root, "mid-change", 6, 9, false)   // full

	// Create an archive/ snapshot subdir with its own change.json — it MUST be skipped.
	archDir := filepath.Join(root, ChangesRoot, archiveDirName, "2026-06-25-old-change")
	if err := os.MkdirAll(archDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archDir, "change.json"), []byte(`{"change_id":"old-change","status":"archived"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A junk folder with no manifest — also skipped.
	if err := os.MkdirAll(filepath.Join(root, ChangesRoot, "not-a-change"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := List(ListInput{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, c := range out.Changes {
		got = append(got, c.ChangeID)
	}
	want := []string{"alpha-change", "mid-change", "zeta-change"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("list order = %v, want %v (archive/ + manifestless dirs must be skipped)", got, want)
	}
	if out.Count != 3 {
		t.Errorf("count = %d, want 3", out.Count)
	}
	// tier echoed from the manifest, not recomputed.
	if out.Changes[1].Tier != "full" {
		t.Errorf("mid-change tier = %q, want full", out.Changes[1].Tier)
	}
}

func TestListEmptyProjectIsNotAnError(t *testing.T) {
	root := t.TempDir() // no .spectra/changes at all
	out, err := List(ListInput{ProjectRoot: root})
	if err != nil {
		t.Fatalf("empty project should not error: %v", err)
	}
	if out.Count != 0 || len(out.Changes) != 0 {
		t.Errorf("empty project should list zero changes, got %+v", out)
	}
}

func TestStatusReturnsVerifyAndNextTransitions(t *testing.T) {
	root := t.TempDir()
	// A conformant full change at status=proposed: spec.{md,yaml} present.
	seedChange(t, root, "feature-x", 6, 9, false) // full
	dir := filepath.Join(root, ChangesRoot, "feature-x")
	mustWrite(t, filepath.Join(dir, "spec.md"), "# spec\n")
	mustWrite(t, filepath.Join(dir, "spec.yaml"), "id: feature-x\n")

	st, err := Status(StatusInput{ProjectRoot: root, ChangeID: "feature-x", HasCode: true})
	if err != nil {
		t.Fatal(err)
	}
	if st.Manifest.Status != "proposed" || st.Manifest.Tier != "full" {
		t.Errorf("status manifest = %+v", st.Manifest)
	}
	if st.Verify == nil {
		t.Fatal("status must carry a verify verdict")
	}
	// The verify surface is the existing 6-check surface; proposed full with both
	// specs should be conformant (exit 0).
	if st.Verify.ExitCode != 0 {
		t.Errorf("verify exit = %d, want 0 (warn mode default); results=%+v", st.Verify.ExitCode, st.Verify.Results)
	}
	// From proposed (full, hasCode) the legal next states include deliberated and
	// in_progress (full tier deliberates; code requires in_progress).
	tos := map[string]bool{}
	for _, nt := range st.NextTransitions {
		tos[string(nt.ToStatus)] = true
	}
	if !tos["deliberated"] || !tos["in_progress"] {
		t.Errorf("next transitions from proposed/full/code = %+v, want deliberated+in_progress", st.NextTransitions)
	}
}

func TestStatusMissingChangeErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := Status(StatusInput{ProjectRoot: root, ChangeID: "nope"}); err == nil {
		t.Errorf("status on a missing change should error")
	}
	if _, err := Status(StatusInput{ProjectRoot: root}); err == nil {
		t.Errorf("status without change_id should error")
	}
}

func TestAssessThresholdsAndTrips(t *testing.T) {
	root := t.TempDir()
	// 2 changes, both full -> full_ratio 1.0. repo_loc overridden.
	seedChange(t, root, "c1", 6, 9, false)
	seedChange(t, root, "c2", 6, 9, false)

	// Defaults: N=10, L=50000, R=0.4. change_count=2 (<10), repo_loc override 100
	// (<50000), full_ratio=1.0 (>=0.4) -> only full_ratio trips -> block.
	a, err := Assess(AssessInput{ProjectRoot: root, RepoLOC: 100})
	if err != nil {
		t.Fatal(err)
	}
	if a.Signals.ChangeCount != 2 {
		t.Errorf("change_count = %d, want 2", a.Signals.ChangeCount)
	}
	if a.Signals.RepoLOC != 100 {
		t.Errorf("repo_loc = %d, want 100 (override)", a.Signals.RepoLOC)
	}
	if a.Signals.FullRatio != 1.0 {
		t.Errorf("full_ratio = %v, want 1.0", a.Signals.FullRatio)
	}
	if a.Thresholds.N != 10 || a.Thresholds.L != 50000 || a.Thresholds.R != 0.4 {
		t.Errorf("thresholds = %+v, want defaults 10/50000/0.4", a.Thresholds)
	}
	if !contains(a.Tripped, "full_ratio") || contains(a.Tripped, "change_count") || contains(a.Tripped, "repo_loc") {
		t.Errorf("tripped = %v, want only [full_ratio]", a.Tripped)
	}
	if a.RecommendedMode != "block" {
		t.Errorf("recommended_mode = %q, want block (a threshold tripped)", a.RecommendedMode)
	}
}

func TestAssessAdvisoryWhenNothingTrips(t *testing.T) {
	root := t.TempDir()
	// One lite change -> full_ratio 0; small repo_loc; change_count 1 -> nothing trips.
	seedChange(t, root, "only-lite", 1, 5, false) // lite
	a, err := Assess(AssessInput{ProjectRoot: root, RepoLOC: 10})
	if err != nil {
		t.Fatal(err)
	}
	if a.Signals.FullRatio != 0 {
		t.Errorf("full_ratio = %v, want 0", a.Signals.FullRatio)
	}
	if len(a.Tripped) != 0 {
		t.Errorf("tripped = %v, want empty", a.Tripped)
	}
	if a.RecommendedMode != "advisory" {
		t.Errorf("recommended_mode = %q, want advisory", a.RecommendedMode)
	}
}

func TestAssessThresholdOverrideTrips(t *testing.T) {
	root := t.TempDir()
	seedChange(t, root, "c1", 6, 9, false)
	seedChange(t, root, "c2", 6, 9, false)
	// Lower N to 2 so change_count (2) trips it too; raise R above 1 so full_ratio
	// does NOT trip -> isolates the N override.
	a, err := Assess(AssessInput{ProjectRoot: root, RepoLOC: 10, N: 2, R: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if a.Thresholds.N != 2 || a.Thresholds.R != 1.5 {
		t.Errorf("overrides not applied: %+v", a.Thresholds)
	}
	if !contains(a.Tripped, "change_count") {
		t.Errorf("change_count should trip at N=2: %v", a.Tripped)
	}
	if contains(a.Tripped, "full_ratio") {
		t.Errorf("full_ratio should NOT trip at R=1.5: %v", a.Tripped)
	}
}

// TestAssessDeterminism: identical inputs over an identical tree yield identical
// output across repeated runs (the §4.3 determinism property, project-scope).
func TestAssessDeterminism(t *testing.T) {
	root := t.TempDir()
	seedChange(t, root, "c1", 6, 9, false) // full
	seedChange(t, root, "c2", 1, 5, false) // lite
	seedChange(t, root, "c3", 1, 1, false) // trivial
	mustWrite(t, filepath.Join(root, "main.go"), "package x\n\nfunc F() {}\n")
	mustWrite(t, filepath.Join(root, "README.md"), "line1\nline2\nline3\n")

	first, err := Assess(AssessInput{ProjectRoot: root}) // walk repo_loc, default thresholds
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		next, err := Assess(AssessInput{ProjectRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("assess is not deterministic:\n run0 = %+v\n run%d = %+v", first, i+1, next)
		}
	}
	// The walk must have counted the seeded text files (deterministic, > 0).
	if first.Signals.RepoLOC == 0 {
		t.Errorf("repo_loc walk counted 0 lines; expected the seeded text files")
	}
}

func TestAssessLOCWalkSkipsBinaries(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "1\n2\n3\n") // 3 text lines
	// A NUL-bearing "binary" file and a .png by extension — both skipped.
	if err := os.WriteFile(filepath.Join(root, "blob.bin"), []byte{0, 1, 2, '\n', '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "image.png"), "fake\npng\ncontent\n")
	// .git contents must not be counted.
	mustWrite(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")

	a, err := Assess(AssessInput{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if a.Signals.RepoLOC != 3 {
		t.Errorf("repo_loc = %d, want 3 (only a.txt; binaries/.git skipped)", a.Signals.RepoLOC)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
