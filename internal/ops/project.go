// Project-scope ops (tonberry v0.2.0): list, status, assess.
//
// These three ops are READ-ONLY observability over a project's change folders.
// They add NO new ESL-owned schema — list/status echo the existing change.json
// fields and the existing verify/lifecycle surfaces; assess computes a
// project-aggregate of the §4.2 right-sizing signal family (change_count /
// repo_loc / full_ratio) for the advisory→block escalation assessment described
// in eidolons-esl docs/escalation.md (FORGE Decision 3). assess REUSES the §4.2
// signals — no new vocabulary, mechanical not LLM-judged — and is deterministic.
//
// The verify 6-check surface is UNCHANGED: `status` calls the EXISTING Verify
// logic; it does not re-implement any check.
package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Rynaro/tonberry/internal/lifecycle"
	"github.com/Rynaro/tonberry/internal/manifest"
)

// archiveDirName is the snapshot subdir skipped when enumerating live changes
// (ESL §9.2 snapshots archived content under archive/<date>-<change_id>/).
const archiveDirName = "archive"

// -- list ------------------------------------------------------------------- //

// ListInput enumerates change folders under a changes directory.
type ListInput struct {
	// ChangesDir is the directory holding change folders. Default .spectra/changes
	// (resolved relative to ProjectRoot when ProjectRoot is set).
	ChangesDir  string `json:"changes_dir,omitempty"`
	ProjectRoot string `json:"project_root,omitempty"`
	// IncludeArchived, if true, ALSO enumerates archived changes under the
	// archive/<date>-<change_id>/ snapshot subdir (ESL §9.2). Default false: list
	// shows ACTIVE changes only.
	IncludeArchived bool `json:"include_archived,omitempty"`
}

// ChangeSummary is one row of the list output: the manifest's identity fields.
// It carries no new schema — every field is read straight from change.json.
type ChangeSummary struct {
	ChangeID     string `json:"change_id"`
	Status       string `json:"status"`
	Tier         string `json:"tier"`
	DriftChecked bool   `json:"drift_checked"`
	// Archived is true for a row read from the archive/ snapshot subdir.
	Archived bool `json:"archived"`
}

// ListOutput is the deterministic, change_id-sorted list of changes.
type ListOutput struct {
	ChangesDir string          `json:"changes_dir"`
	Count      int             `json:"count"`
	Changes    []ChangeSummary `json:"changes"`
}

// resolveChangesDir picks the changes directory: an explicit ChangesDir wins;
// otherwise ChangesRoot under ProjectRoot.
func resolveChangesDir(projectRoot, changesDir string) string {
	if changesDir != "" {
		return changesDir
	}
	return filepath.Join(projectRoot, ChangesRoot)
}

// List enumerates immediate child folders of the changes directory, reads each
// change.json, and returns a change_id-sorted summary. The archive/ snapshot
// subdir and any folder without a readable change.json are skipped by default;
// IncludeArchived ALSO enumerates the archived snapshots (ESL §9.2). Any folder
// without a readable manifest is skipped. Ordering is deterministic (sort by
// change_id; archived rows sort alongside active ones).
func List(in ListInput) (*ListOutput, error) {
	dir := resolveChangesDir(in.ProjectRoot, in.ChangesDir)
	out := &ListOutput{ChangesDir: dir, Changes: []ChangeSummary{}}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// No changes directory yet is a valid empty project, not an error.
			return out, nil
		}
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == archiveDirName {
			continue
		}
		c, rerr := manifest.Read(filepath.Join(dir, e.Name()))
		if rerr != nil {
			// A folder with no readable manifest is not a change; skip it.
			continue
		}
		out.Changes = append(out.Changes, ChangeSummary{
			ChangeID:     c.ChangeID,
			Status:       string(c.Status),
			Tier:         string(c.Tier),
			DriftChecked: c.DriftCheckedTrue(),
		})
	}

	if in.IncludeArchived {
		out.Changes = append(out.Changes, listArchived(dir)...)
	}

	sort.Slice(out.Changes, func(i, j int) bool {
		return out.Changes[i].ChangeID < out.Changes[j].ChangeID
	})
	out.Count = len(out.Changes)
	return out, nil
}

// listArchived enumerates the archived snapshot folders under
// <changesDir>/archive/<date>-<change_id>/, reading each change.json. Returns the
// summaries (Archived=true) or an empty slice if no archive/ dir exists. Folders
// without a readable manifest are skipped. Order is left to the caller's sort.
func listArchived(changesDir string) []ChangeSummary {
	rows := []ChangeSummary{}
	archiveDir := filepath.Join(changesDir, archiveDirName)
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return rows // no archive dir yet (or unreadable) => zero archived changes
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, rerr := manifest.Read(filepath.Join(archiveDir, e.Name()))
		if rerr != nil {
			continue
		}
		rows = append(rows, ChangeSummary{
			ChangeID:     c.ChangeID,
			Status:       string(c.Status),
			Tier:         string(c.Tier),
			DriftChecked: c.DriftCheckedTrue(),
			Archived:     true,
		})
	}
	return rows
}

// -- status ----------------------------------------------------------------- //

// StatusInput targets one change_id.
type StatusInput struct {
	ProjectRoot string `json:"project_root,omitempty"`
	ChangesDir  string `json:"changes_dir,omitempty"`
	ChangeID    string `json:"change_id"`
	// Mode for the embedded verify (warn default | block).
	Mode string `json:"mode,omitempty"`
	// HasCode is passed to the legal-next-transition enumeration (code-states
	// require code; defaults false).
	HasCode bool `json:"has_code,omitempty"`
}

// StatusOutput bundles the manifest summary, the conformance verdict (from the
// EXISTING verify logic — not re-implemented), and the legal next transitions.
type StatusOutput struct {
	ChangeID        string                     `json:"change_id"`
	ChangeDir       string                     `json:"change_dir"`
	Manifest        ChangeSummary              `json:"manifest"`
	Verify          *VerifyOutput              `json:"verify"`
	NextTransitions []lifecycle.NextTransition `json:"next_transitions"`
}

// Status returns the manifest summary + the conformance verdict (via the
// existing Verify) + the legal next transitions for one change. The verify call
// is the SAME 6-check surface; this op duplicates none of it.
func Status(in StatusInput) (*StatusOutput, error) {
	if in.ChangeID == "" {
		return nil, fmt.Errorf("change_id is required")
	}
	dir := filepath.Join(resolveChangesDir(in.ProjectRoot, in.ChangesDir), in.ChangeID)

	c, err := manifest.Read(dir)
	if err != nil {
		return nil, err
	}

	v, err := Verify(VerifyInput{ChangeDir: dir, Mode: in.Mode}, filepath.Abs)
	if err != nil {
		return nil, err
	}

	return &StatusOutput{
		ChangeID:  in.ChangeID,
		ChangeDir: dir,
		Manifest: ChangeSummary{
			ChangeID:     c.ChangeID,
			Status:       string(c.Status),
			Tier:         string(c.Tier),
			DriftChecked: c.DriftCheckedTrue(),
		},
		Verify:          v,
		NextTransitions: lifecycle.LegalNextStatuses(c.Status, c.Tier, in.HasCode),
	}, nil
}

// -- assess ----------------------------------------------------------------- //

// Default escalation thresholds (eidolons-esl docs/escalation.md seed defaults).
// These are policy knobs, overridable per call — NOT baked constants.
const (
	DefaultThresholdN = 10    // change_count threshold
	DefaultThresholdL = 50000 // repo_loc threshold
	DefaultThresholdR = 0.4   // full_ratio threshold
)

// AssessInput computes the project-aggregate escalation assessment.
type AssessInput struct {
	ProjectRoot string `json:"project_root,omitempty"`
	ChangesDir  string `json:"changes_dir,omitempty"`
	// RepoLOC overrides the deterministic line count when >= 0 (default -1 = walk).
	RepoLOC int `json:"repo_loc,omitempty"`
	// Threshold overrides; <=0 (or, for R, <0) means "use the default".
	N int     `json:"n,omitempty"`
	L int     `json:"l,omitempty"`
	R float64 `json:"r,omitempty"`
}

// AssessSignals is the project-aggregate of the §4.2 right-sizing signal family.
type AssessSignals struct {
	ChangeCount int     `json:"change_count"`
	RepoLOC     int     `json:"repo_loc"`
	FullRatio   float64 `json:"full_ratio"`
}

// AssessThresholds echoes the thresholds used (after applying defaults).
type AssessThresholds struct {
	N int     `json:"n"`
	L int     `json:"l"`
	R float64 `json:"r"`
}

// AssessOutput is the read-only escalation assessment. recommended_mode is
// "block" if ANY threshold trips, else "advisory".
type AssessOutput struct {
	Signals         AssessSignals    `json:"signals"`
	Thresholds      AssessThresholds `json:"thresholds"`
	Tripped         []string         `json:"tripped"`
	RecommendedMode string           `json:"recommended_mode"`
}

// Assess computes change_count / repo_loc / full_ratio over a project and
// compares them to the (overridable) thresholds. It is deterministic: identical
// inputs over an identical tree always yield the identical result. tonberry only
// produces the assessment — it does NOT write any lock (that flip is nexus-side
// and deferred; see eidolons-esl docs/escalation.md).
func Assess(in AssessInput) (*AssessOutput, error) {
	changesDir := resolveChangesDir(in.ProjectRoot, in.ChangesDir)

	// change_count + full_ratio from the change manifests, counting BOTH active
	// AND archived changes (ESL §9.2) so archiving never drops the escalation
	// signal — an archived `full` change still contributes to change_count and the
	// full_ratio numerator/denominator.
	ls, err := List(ListInput{ChangesDir: changesDir, IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	changeCount := ls.Count
	fullCount := 0
	for _, ch := range ls.Changes {
		if ch.Tier == string(manifest.TierFull) {
			fullCount++
		}
	}
	fullRatio := 0.0
	if changeCount > 0 {
		fullRatio = float64(fullCount) / float64(changeCount)
	}

	// repo_loc: a positive override wins; otherwise (<=0, the default) walk the
	// tree for a deterministic text-line count.
	repoLOC := in.RepoLOC
	if repoLOC <= 0 {
		root := in.ProjectRoot
		if root == "" {
			root = "."
		}
		repoLOC, err = countTextLines(root)
		if err != nil {
			return nil, err
		}
	}

	thr := AssessThresholds{
		N: orDefaultInt(in.N, DefaultThresholdN),
		L: orDefaultInt(in.L, DefaultThresholdL),
		R: orDefaultFloat(in.R, DefaultThresholdR),
	}

	tripped := []string{}
	if changeCount >= thr.N {
		tripped = append(tripped, "change_count")
	}
	if repoLOC >= thr.L {
		tripped = append(tripped, "repo_loc")
	}
	if fullRatio >= thr.R {
		tripped = append(tripped, "full_ratio")
	}

	mode := "advisory"
	if len(tripped) > 0 {
		mode = "block"
	}

	return &AssessOutput{
		Signals:         AssessSignals{ChangeCount: changeCount, RepoLOC: repoLOC, FullRatio: fullRatio},
		Thresholds:      thr,
		Tripped:         tripped,
		RecommendedMode: mode,
	}, nil
}

func orDefaultInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func orDefaultFloat(v, def float64) float64 {
	if v > 0 {
		return v
	}
	return def
}

// binaryExts is a deny-list of obvious binary/asset extensions skipped by the
// LOC walk. The list is conservative; the count is a deterministic proxy, not an
// exact SLOC measure.
var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".ico": true, ".bmp": true, ".tiff": true, ".svg": true,
	".pdf": true, ".zip": true, ".gz": true, ".tar": true, ".tgz": true,
	".bz2": true, ".xz": true, ".7z": true, ".rar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true,
	".o": true, ".bin": true, ".class": true, ".jar": true, ".wasm": true,
	".mp3": true, ".mp4": true, ".wav": true, ".mov": true, ".avi": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".db": true, ".sqlite": true, ".lock": true,
}

// skipDirs is a deny-list of directory names not counted (VCS, vendored deps,
// build output). Deterministic and host-independent.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	".idea": true, ".vscode": true,
}

// countTextLines walks root and counts newline-delimited lines across text
// files, skipping .git, common vendor/build dirs, and obvious binaries. The walk
// order does not affect the total, so the count is deterministic for a given tree.
func countTextLines(root string) (int, error) {
	total := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// An unreadable entry is skipped, not fatal — the count is a proxy.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if binaryExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if isBinary(data) {
			return nil
		}
		total += countLines(data)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

// isBinary reports whether data looks binary (contains a NUL in the first 8 KiB).
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// countLines counts newline-delimited lines; a non-empty final line with no
// trailing newline counts as one line.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}
