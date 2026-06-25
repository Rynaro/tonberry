// Package ops is the operations layer: the business logic behind the 11 tonberry
// tools (FORGE Decision 1). Each op has a typed Input and Output struct so the
// same logic serves both the stdio MCP (internal/mcpserver) and the one-shot CLI
// (cmd/tonberry). maker_checker is folded INTO verify as check C4 (Decision 1) —
// there is no standalone maker_checker op.
//
// The 11 ops: Propose, RightSize, Transition, ComposeManifest, ComposeEnvelope,
// Verify, DriftCheck, Archive (the v0.1 lifecycle surface) + List, Status, Assess
// (the v0.2 read-only project-scope observability, in project.go).
//
// ANTI-SCOPE: ops compose/reference; they never re-declare the SPECTRA spec
// schema, the ECL envelope schema, or CRYSTALIUM layer shapes. Only the ESL-owned
// status/tier enums are declared (in internal/manifest).
package ops

import (
	"fmt"
	"path/filepath"

	"github.com/Rynaro/tonberry/internal/archive"
	"github.com/Rynaro/tonberry/internal/conformance"
	"github.com/Rynaro/tonberry/internal/envelope"
	"github.com/Rynaro/tonberry/internal/lifecycle"
	"github.com/Rynaro/tonberry/internal/manifest"
	"github.com/Rynaro/tonberry/internal/rightsizing"
)

// ChangesRoot is the per-project changes directory ESL §9.2 mandates.
const ChangesRoot = ".spectra/changes"

// changeDirFor resolves the change folder path for a change_id under root.
func changeDirFor(root, changeID string) string {
	return filepath.Join(root, ChangesRoot, changeID)
}

// -- propose ---------------------------------------------------------------- //

// ProposeInput scaffolds a change.json (status=proposed). tier is left empty
// (null-until-right_size); the caller runs right_size next.
type ProposeInput struct {
	ProjectRoot string `json:"project_root,omitempty"`
	ChangeID    string `json:"change_id"`
	Maker       string `json:"maker"`
	SpecRef     string `json:"spec_ref,omitempty"`
	Checker     string `json:"checker,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Supersedes  string `json:"supersedes,omitempty"`
}

// ProposeOutput is the propose result.
type ProposeOutput struct {
	ChangeDir    string `json:"change_dir"`
	ManifestPath string `json:"manifest_path"`
	Status       string `json:"status"`
	Tier         string `json:"tier"` // empty until right_size
}

// Propose scaffolds the change folder and a proposed manifest. tier is set to ""
// (unclassified) — right_size writes it. spec_ref defaults to null.
func Propose(in ProposeInput) (*ProposeOutput, error) {
	if in.ChangeID == "" {
		return nil, fmt.Errorf("change_id is required")
	}
	if in.Maker == "" {
		return nil, fmt.Errorf("maker is required")
	}
	root := in.ProjectRoot
	dir := changeDirFor(root, in.ChangeID)

	var specRef *string
	if in.SpecRef != "" {
		s := in.SpecRef
		specRef = &s
	}
	checker := in.Checker
	if checker == "" {
		checker = "REPLACE-distinct-checker-identity"
	}
	c := &manifest.Change{
		ESLVersion:       manifest.ESLVersion,
		ChangeID:         in.ChangeID,
		Status:           manifest.StatusProposed,
		Tier:             "", // null-until-right_size
		Maker:            in.Maker,
		Checker:          checker,
		AcceptanceChecks: []manifest.AcceptanceCheck{},
		SpecRef:          specRef,
		CreatedAt:        in.CreatedAt,
	}
	if in.Supersedes != "" {
		s := in.Supersedes
		c.Supersedes = &s
	}
	p, err := manifest.Write(dir, c)
	if err != nil {
		return nil, err
	}
	return &ProposeOutput{
		ChangeDir:    dir,
		ManifestPath: p,
		Status:       string(manifest.StatusProposed),
		Tier:         "",
	}, nil
}

// -- right_size ------------------------------------------------------------- //

// RightSizeInput carries the 3 mechanical signals and an OPTIONAL change_id.
// change_id is only required when WriteManifest is true (to locate the manifest);
// for pure classification it may be omitted, so it is not a required schema field.
type RightSizeInput struct {
	ProjectRoot     string `json:"project_root,omitempty"`
	ChangeID        string `json:"change_id,omitempty"`
	FilesTouched    int    `json:"files_touched"`
	RubricScore     int    `json:"rubric_score"`
	TradeoffPresent bool   `json:"tradeoff_present"`
	// WriteManifest, if true, persists the tier into the change's manifest.
	WriteManifest bool `json:"write_manifest,omitempty"`
}

// RightSizeOutput echoes the deterministic classification.
type RightSizeOutput struct {
	Tier          string              `json:"tier"`
	Route         string              `json:"route"`
	Signals       rightsizing.Signals `json:"signals"`
	Deterministic bool                `json:"deterministic"`
	ManifestPath  string              `json:"manifest_path,omitempty"`
}

// RightSize runs the deterministic gate and (optionally) writes the tier into
// the manifest. The classification is a pure function of the signals.
func RightSize(in RightSizeInput) (*RightSizeOutput, error) {
	res := rightsizing.Classify(rightsizing.Signals{
		FilesTouched:    in.FilesTouched,
		RubricScore:     in.RubricScore,
		TradeoffPresent: in.TradeoffPresent,
	})
	out := &RightSizeOutput{
		Tier:          string(res.Tier),
		Route:         res.Route,
		Signals:       res.Signals,
		Deterministic: res.Deterministic,
	}
	if in.WriteManifest {
		if in.ChangeID == "" {
			return nil, fmt.Errorf("change_id is required to write the manifest")
		}
		dir := changeDirFor(in.ProjectRoot, in.ChangeID)
		c, err := manifest.Read(dir)
		if err != nil {
			return nil, err
		}
		c.Tier = res.Tier
		p, err := manifest.Write(dir, c)
		if err != nil {
			return nil, err
		}
		out.ManifestPath = p
	}
	return out, nil
}

// -- transition ------------------------------------------------------------- //

// TransitionInput advances status honoring §3 skip-rules.
type TransitionInput struct {
	ProjectRoot string `json:"project_root,omitempty"`
	ChangeID    string `json:"change_id"`
	ToStatus    string `json:"to_status"`
	Actor       string `json:"actor,omitempty"`
	// HasCode declares whether the change contains code (the code-states require
	// it; no-code changes skip in_progress). Defaults false.
	HasCode bool `json:"has_code,omitempty"`
	// WriteManifest, if true, persists the new status when the transition is allowed.
	WriteManifest bool `json:"write_manifest,omitempty"`
}

// TransitionOutput is the lifecycle decision.
type TransitionOutput struct {
	FromStatus       string `json:"from_status"`
	ToStatus         string `json:"to_status"`
	Allowed          bool   `json:"allowed"`
	Reason           string `json:"reason,omitempty"`
	NextPerformative string `json:"next_performative,omitempty"`
	ManifestPath     string `json:"manifest_path,omitempty"`
}

// Transition reads the manifest, evaluates the requested transition, and
// (optionally, when allowed) writes the new status.
func Transition(in TransitionInput) (*TransitionOutput, error) {
	if in.ChangeID == "" {
		return nil, fmt.Errorf("change_id is required")
	}
	dir := changeDirFor(in.ProjectRoot, in.ChangeID)
	c, err := manifest.Read(dir)
	if err != nil {
		return nil, err
	}
	to := manifest.Status(in.ToStatus)

	// Pre-check the §6.4 archive precondition: archived requires drift_checked.
	d := lifecycle.Transition(c.Status, to, c.Tier, in.HasCode)
	if d.Allowed && to == manifest.StatusArchived && !c.DriftCheckedTrue() {
		d.Allowed = false
		d.Reason = "archived requires drift_checked=true (ESL §6.4)"
	}

	out := &TransitionOutput{
		FromStatus:       string(d.From),
		ToStatus:         string(d.To),
		Allowed:          d.Allowed,
		Reason:           d.Reason,
		NextPerformative: d.NextPerformative,
	}
	if d.Allowed && in.WriteManifest {
		c.Status = to
		p, werr := manifest.Write(dir, c)
		if werr != nil {
			return nil, werr
		}
		out.ManifestPath = p
	}
	return out, nil
}

// -- compose_manifest ------------------------------------------------------- //

// ComposeManifestInput writes/validates change.json against the ESL-owned schema.
// Patch is a partial set of manifest fields to merge into the existing manifest
// (or to seed a new one).
type ComposeManifestInput struct {
	ProjectRoot string           `json:"project_root,omitempty"`
	ChangeID    string           `json:"change_id"`
	Patch       *manifest.Change `json:"patch,omitempty"`
}

// ComposeManifestOutput reports the path + validation result.
type ComposeManifestOutput struct {
	ManifestPath string   `json:"manifest_path"`
	Valid        bool     `json:"valid"`
	SchemaErrors []string `json:"schema_errors"`
}

// ComposeManifest merges the patch onto the existing manifest (if any), writes
// it, and validates the result against change.v1.json's ESL-owned constraints.
func ComposeManifest(in ComposeManifestInput) (*ComposeManifestOutput, error) {
	if in.ChangeID == "" {
		return nil, fmt.Errorf("change_id is required")
	}
	dir := changeDirFor(in.ProjectRoot, in.ChangeID)

	c, err := manifest.Read(dir)
	if err != nil {
		// No existing manifest: start from the patch (or an empty change).
		c = &manifest.Change{ESLVersion: manifest.ESLVersion, ChangeID: in.ChangeID}
	}
	if in.Patch != nil {
		mergeChange(c, in.Patch)
	}
	if c.ChangeID == "" {
		c.ChangeID = in.ChangeID
	}
	if c.ESLVersion == "" {
		c.ESLVersion = manifest.ESLVersion
	}
	if c.AcceptanceChecks == nil {
		c.AcceptanceChecks = []manifest.AcceptanceCheck{}
	}

	errs := manifest.Validate(c)
	p, werr := manifest.Write(dir, c)
	if werr != nil {
		return nil, werr
	}
	if errs == nil {
		errs = []string{}
	}
	return &ComposeManifestOutput{
		ManifestPath: p,
		Valid:        len(errs) == 0,
		SchemaErrors: errs,
	}, nil
}

// mergeChange merges non-zero fields of patch onto base.
func mergeChange(base, patch *manifest.Change) {
	if patch.ESLVersion != "" {
		base.ESLVersion = patch.ESLVersion
	}
	if patch.ChangeID != "" {
		base.ChangeID = patch.ChangeID
	}
	if patch.Status != "" {
		base.Status = patch.Status
	}
	if patch.Tier != "" {
		base.Tier = patch.Tier
	}
	if patch.Maker != "" {
		base.Maker = patch.Maker
	}
	if patch.Checker != "" {
		base.Checker = patch.Checker
	}
	if patch.AcceptanceChecks != nil {
		base.AcceptanceChecks = patch.AcceptanceChecks
	}
	if patch.SpecRef != nil {
		base.SpecRef = patch.SpecRef
	}
	if patch.Supersedes != nil {
		base.Supersedes = patch.Supersedes
	}
	if patch.SupersededBy != nil {
		base.SupersededBy = patch.SupersededBy
	}
	if patch.CreatedAt != "" {
		base.CreatedAt = patch.CreatedAt
	}
	if patch.DriftChecked != nil {
		base.DriftChecked = patch.DriftChecked
	}
	if patch.ArchivePath != nil {
		base.ArchivePath = patch.ArchivePath
	}
}

// -- compose_envelope ------------------------------------------------------- //

// ComposeEnvelopeInput emits an ECL sidecar naming the §7.2 performative.
type ComposeEnvelopeInput struct {
	ProjectRoot string `json:"project_root,omitempty"`
	ChangeID    string `json:"change_id"`
	// Performative may be given explicitly; otherwise it is derived from
	// Transition (the entering-state performative).
	Performative string `json:"performative,omitempty"`
	Transition   string `json:"transition,omitempty"` // target status
	From         string `json:"from"`
	To           string `json:"to"`
	// Basename for the sidecar (default derived from performative, e.g. "propose.envelope.json").
	Basename                string                 `json:"basename,omitempty"`
	ContextDelta            map[string]interface{} `json:"context_delta,omitempty"`
	Objective               string                 `json:"objective,omitempty"`
	EnvelopeVersionOverride string                 `json:"envelope_version,omitempty"`
}

// ComposeEnvelopeOutput reports the path + performative + validity.
type ComposeEnvelopeOutput struct {
	EnvelopePath string `json:"envelope_path"`
	Performative string `json:"performative"`
	Valid        bool   `json:"valid"`
}

// ComposeEnvelope writes an ECL sidecar to the change folder. The performative is
// the explicit one, or derived from the named target transition status.
func ComposeEnvelope(in ComposeEnvelopeInput) (*ComposeEnvelopeOutput, error) {
	if in.ChangeID == "" {
		return nil, fmt.Errorf("change_id is required")
	}
	perf := in.Performative
	if perf == "" && in.Transition != "" {
		perf = lifecycle.EnteringPerformative(manifest.Status(in.Transition))
	}
	if perf == "" {
		return nil, fmt.Errorf("performative or transition is required")
	}
	if in.From == "" || in.To == "" {
		return nil, fmt.Errorf("from and to identities are required")
	}
	env, err := envelope.Compose(perf, in.From, in.To, envelope.Options{
		EnvelopeVersionOverride: in.EnvelopeVersionOverride,
		Objective:               in.Objective,
		ContextDelta:            in.ContextDelta,
	})
	if err != nil {
		return nil, err
	}
	basename := in.Basename
	if basename == "" {
		basename = defaultEnvelopeBasename(perf)
	}
	dir := changeDirFor(in.ProjectRoot, in.ChangeID)
	p, err := envelope.Write(dir, basename, env)
	if err != nil {
		return nil, err
	}
	return &ComposeEnvelopeOutput{
		EnvelopePath: p,
		Performative: perf,
		Valid:        true,
	}, nil
}

func defaultEnvelopeBasename(perf string) string {
	switch perf {
	case "PROPOSE":
		return "propose.envelope.json"
	case "CRITIQUE":
		return "critique.envelope.json"
	case "DECIDE":
		return "decide.envelope.json"
	case "DELEGATE":
		return "delegate.envelope.json"
	case "ACKNOWLEDGE":
		return "acknowledge.envelope.json"
	case "INFORM":
		return "verify.envelope.json"
	case "ESCALATE":
		return "escalate.envelope.json"
	case "RESUME":
		return "resume.envelope.json"
	case "REFUSE":
		return "refuse.envelope.json"
	default:
		return "envelope.json"
	}
}

// -- verify ----------------------------------------------------------------- //

// VerifyInput runs the 6 conformance checks against a change folder.
type VerifyInput struct {
	// ChangeDir is a direct path to the change folder (absolute or relative to cwd).
	ChangeDir string `json:"change_dir"`
	Mode      string `json:"mode,omitempty"` // warn (default) | block
}

// VerifyOutput mirrors the conformance.Report (the parity surface).
type VerifyOutput struct {
	TargetBasename string               `json:"target_basename"`
	Mode           string               `json:"mode"`
	Results        []conformance.Result `json:"results"`
	ExitCode       int                  `json:"exit_code"`
	HasFail        bool                 `json:"has_fail"`
}

// Verify resolves the change dir, runs the checks, and returns the report.
// It returns a usage error (the caller maps to exit 1) for a bad/missing folder.
func Verify(in VerifyInput, resolveAbs func(string) (string, error)) (*VerifyOutput, error) {
	if in.ChangeDir == "" {
		return nil, fmt.Errorf("change_dir is required")
	}
	mode := conformance.ModeWarn
	switch in.Mode {
	case "", "warn":
		mode = conformance.ModeWarn
	case "block":
		mode = conformance.ModeBlock
	default:
		return nil, fmt.Errorf("invalid mode: %s (expected warn or block)", in.Mode)
	}
	abs, err := resolveAbs(in.ChangeDir)
	if err != nil {
		return nil, err
	}
	rep := conformance.Check(abs, mode)
	return &VerifyOutput{
		TargetBasename: rep.TargetBasename,
		Mode:           rep.Mode,
		Results:        rep.Results,
		ExitCode:       rep.ExitCode,
		HasFail:        rep.HasFail,
	}, nil
}

// -- drift_check ------------------------------------------------------------ //

// DriftCheckInput re-derives acceptance_checks vs the spec-of-record and tree.
type DriftCheckInput struct {
	ProjectRoot     string `json:"project_root,omitempty"`
	ChangeID        string `json:"change_id"`
	Checker         string `json:"checker"`
	SpecOfRecordRef string `json:"spec_of_record_ref,omitempty"`
	TreeRoot        string `json:"tree_root,omitempty"`
	// Mismatches, if provided by the caller's re-derivation, drive the result.
	// tonberry does not itself re-run tests; the identity-distinct checker reports
	// the mismatches it found, and tonberry records the verdict deterministically.
	Mismatches []string `json:"mismatches,omitempty"`
	// WriteManifest, if true and no mismatches, sets drift_checked=true.
	WriteManifest bool `json:"write_manifest,omitempty"`
}

// DriftCheckOutput reports the verdict.
type DriftCheckOutput struct {
	DriftChecked bool     `json:"drift_checked"`
	Mismatches   []string `json:"mismatches"`
	Escalate     bool     `json:"escalate"`
	NextStatus   string   `json:"next_status,omitempty"`
	ManifestPath string   `json:"manifest_path,omitempty"`
}

// DriftCheck enforces the §5/§6.4 identity rule (checker != maker) and records
// the verdict. On mismatch it signals ESCALATE back to in_progress; on match it
// (optionally) sets drift_checked=true.
func DriftCheck(in DriftCheckInput) (*DriftCheckOutput, error) {
	if in.ChangeID == "" {
		return nil, fmt.Errorf("change_id is required")
	}
	if in.Checker == "" {
		return nil, fmt.Errorf("checker is required")
	}
	dir := changeDirFor(in.ProjectRoot, in.ChangeID)
	c, err := manifest.Read(dir)
	if err != nil {
		return nil, err
	}
	// Identity rule (ESL §5.1/§6.4): the checker MUST be distinct from the maker.
	if in.Checker == c.Maker {
		return nil, fmt.Errorf("drift_check checker (%q) must be distinct from maker (%q)", in.Checker, c.Maker)
	}

	out := &DriftCheckOutput{Mismatches: in.Mismatches}
	if out.Mismatches == nil {
		out.Mismatches = []string{}
	}
	if len(out.Mismatches) > 0 {
		// Mismatch -> verify_fail -> ESCALATE -> in_progress.
		out.DriftChecked = false
		out.Escalate = true
		out.NextStatus = string(manifest.StatusInProgress)
		return out, nil
	}
	out.DriftChecked = true
	out.Escalate = false
	if in.WriteManifest {
		t := true
		c.DriftChecked = &t
		p, werr := manifest.Write(dir, c)
		if werr != nil {
			return nil, werr
		}
		out.ManifestPath = p
	}
	return out, nil
}

// -- archive ---------------------------------------------------------------- //

// ArchiveInput snapshots the folder + composes the promotion-intent envelope.
type ArchiveInput struct {
	ProjectRoot             string `json:"project_root,omitempty"`
	ChangeID                string `json:"change_id"`
	Date                    string `json:"date,omitempty"`
	EnvelopeVersionOverride string `json:"envelope_version,omitempty"`
}

// ArchiveOutput is the archive result.
type ArchiveOutput struct {
	ArchivePath           string `json:"archive_path"`
	Status                string `json:"status"`
	PromotionEnvelopePath string `json:"promotion_envelope_path"`
	PromotionPerformative string `json:"promotion_performative"`
}

// Archive runs the snapshot + promotion-intent compose (Decision 4).
func Archive(in ArchiveInput) (*ArchiveOutput, error) {
	if in.ChangeID == "" {
		return nil, fmt.Errorf("change_id is required")
	}
	dir := changeDirFor(in.ProjectRoot, in.ChangeID)
	res, err := archive.Archive(dir, in.Date, in.EnvelopeVersionOverride)
	if err != nil {
		return nil, err
	}
	return &ArchiveOutput{
		ArchivePath:           res.ArchivePath,
		Status:                res.Status,
		PromotionEnvelopePath: res.PromotionEnvelopePath,
		PromotionPerformative: res.PromotionPerformative,
	}, nil
}
