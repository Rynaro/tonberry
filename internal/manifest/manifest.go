// Package manifest reads, writes, and validates the per-change `change.json`
// manifest defined by ESL v1.0 (eidolons-esl spec/esl-1.0.md §2, schema/change.v1.json).
//
// ANTI-SCOPE (ESL v1.0 §1.3): the ONLY schema tonberry owns is the `status` and
// `tier` enums. This package REFERENCES spec_ref (a relative path to the
// SPECTRA-owned spec.{md,yaml}); it MUST NOT inline the SPECTRA spec schema. The
// acceptance_checks items carry an `id` that points INTO spec_ref — ESL does not
// re-declare SPECTRA's GIVEN/WHEN/THEN format.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ESLVersion is the ESL document version this build targets (the ESL_VERSION stamp).
const ESLVersion = "1.0"

// ManifestFile is the canonical change-manifest filename within a change folder.
const ManifestFile = "change.json"

// Status is the ESL-OWNED lifecycle state vocabulary (ESL v1.0 §3).
type Status string

const (
	StatusProposed    Status = "proposed"
	StatusDeliberated Status = "deliberated"
	StatusInProgress  Status = "in_progress"
	StatusVerified    Status = "verified"
	StatusArchived    Status = "archived"
)

// Statuses is the closed status enum, in lifecycle order.
var Statuses = []Status{StatusProposed, StatusDeliberated, StatusInProgress, StatusVerified, StatusArchived}

// Tier is the ESL-OWNED right-sizing tier (ESL v1.0 §4).
type Tier string

const (
	TierTrivial Tier = "trivial"
	TierLite    Tier = "lite"
	TierFull    Tier = "full"
)

// Tiers is the closed tier enum.
var Tiers = []Tier{TierTrivial, TierLite, TierFull}

// AcceptanceCheck carries one acceptance criterion. Per ESL §2.4–§2.5
// (schema/change.v1.json: acceptance_checks[].items is a
// oneOf:[string, object]), an item is EITHER:
//
//   - a plain string (Raw set; the minimal/legacy free-form form), OR
//   - a structured object {id, verify_method} plus the OPTIONAL EARS fields
//     {given, when, then} (ESL §2.5).
//
// ESL does NOT re-declare SPECTRA's GIVEN/WHEN/THEN format; the id points INTO
// spec_ref and the EARS fields are a thin reference shape (advisory-linted by C7).
//
// Raw is mutually exclusive with the object fields: when Raw != "" the item
// round-trips as a JSON string; otherwise it round-trips as an object.
type AcceptanceCheck struct {
	// Raw holds the plain-string form. Empty for the object form.
	Raw string `json:"-"`

	ID           string `json:"id,omitempty"`
	Given        string `json:"given,omitempty"`
	When         string `json:"when,omitempty"`
	Then         string `json:"then,omitempty"`
	VerifyMethod string `json:"verify_method,omitempty"`
}

// acceptanceCheckObj is the object-form alias used to (un)marshal without
// recursing through AcceptanceCheck's custom methods.
type acceptanceCheckObj struct {
	ID           string `json:"id,omitempty"`
	Given        string `json:"given,omitempty"`
	When         string `json:"when,omitempty"`
	Then         string `json:"then,omitempty"`
	VerifyMethod string `json:"verify_method,omitempty"`
}

// UnmarshalJSON accepts the oneOf:[string, object] acceptance_checks item shape.
// A JSON string lands in Raw; a JSON object decodes the id + optional EARS fields.
func (a *AcceptanceCheck) UnmarshalJSON(data []byte) error {
	trimmed := []byte(data)
	// Distinguish a JSON string from an object by the first non-space byte.
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t' || trimmed[0] == '\n' || trimmed[0] == '\r') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*a = AcceptanceCheck{Raw: s}
		return nil
	}
	var obj acceptanceCheckObj
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*a = AcceptanceCheck{
		ID:           obj.ID,
		Given:        obj.Given,
		When:         obj.When,
		Then:         obj.Then,
		VerifyMethod: obj.VerifyMethod,
	}
	return nil
}

// MarshalJSON renders the plain-string form (Raw) as a JSON string and the
// object form as a JSON object, preserving the oneOf shape on round-trip.
func (a AcceptanceCheck) MarshalJSON() ([]byte, error) {
	if a.Raw != "" {
		return json.Marshal(a.Raw)
	}
	return json.Marshal(acceptanceCheckObj{
		ID:           a.ID,
		Given:        a.Given,
		When:         a.When,
		Then:         a.Then,
		VerifyMethod: a.VerifyMethod,
	})
}

// IsEARS reports whether the item is in the EARS form (object declaring at least
// one of given/when/then) — the C7 advisory lint trigger (ESL §2.5).
func (a AcceptanceCheck) IsEARS() bool {
	return a.Raw == "" && (a.Given != "" || a.When != "" || a.Then != "")
}

// Change is the in-memory shape of change.json. Field tags mirror
// schema/change.v1.json exactly; tonberry does not add fields the schema forbids
// (the schema is additionalProperties:false).
//
// Pointer/omitempty handling: optional fields that may be explicitly `null` in
// the manifest (spec_ref, supersedes, superseded_by, archive_path) use *string;
// drift_checked uses *bool so "unset" (nil) is distinct from "false".
type Change struct {
	ESLVersion       string            `json:"esl_version"`
	ChangeID         string            `json:"change_id"`
	Status           Status            `json:"status"`
	Tier             Tier              `json:"tier"`
	Maker            string            `json:"maker"`
	Checker          string            `json:"checker"`
	AcceptanceChecks []AcceptanceCheck `json:"acceptance_checks"`
	SpecRef          *string           `json:"spec_ref"`

	Supersedes   *string `json:"supersedes,omitempty"`
	SupersededBy *string `json:"superseded_by,omitempty"`
	CreatedAt    string  `json:"created_at,omitempty"`
	DriftChecked *bool   `json:"drift_checked,omitempty"`
	ArchivePath  *string `json:"archive_path,omitempty"`
}

var changeIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var eslVersionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// ManifestPath returns the change.json path for a change directory.
func ManifestPath(changeDir string) string {
	return filepath.Join(changeDir, ManifestFile)
}

// Read loads and JSON-decodes the change.json in a change directory. It does NOT
// validate against the schema beyond JSON well-formedness; call Validate for that.
func Read(changeDir string) (*Change, error) {
	p := ManifestPath(changeDir)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var c Change
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &c, nil
}

// Write marshals a Change to change.json in the change directory (pretty,
// 2-space indent, trailing newline — matching the ESL example/template style).
// The directory is created if absent.
func Write(changeDir string, c *Change) (string, error) {
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", changeDir, err)
	}
	p := ManifestPath(changeDir)
	data, err := MarshalIndent(c)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	return p, nil
}

// MarshalIndent renders a Change as deterministic, pretty JSON with a trailing newline.
func MarshalIndent(c *Change) ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal change: %w", err)
	}
	return append(data, '\n'), nil
}

// ValidStatus reports whether s is a legal ESL status enum value.
func ValidStatus(s Status) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// ValidTier reports whether t is a legal ESL tier enum value.
func ValidTier(t Tier) bool {
	for _, v := range Tiers {
		if v == t {
			return true
		}
	}
	return false
}

// Validate checks a Change against the ESL-owned constraints of change.v1.json:
// required fields present + non-empty, status/tier enum legality, change_id and
// esl_version patterns, acceptance_checks item ids non-empty. It deliberately
// does NOT re-derive the maker!=checker rule (that is a conformance-checker rule,
// not a schema rule — schema/change.v1.json description, ESL §1.3).
//
// Returns a slice of human-readable schema errors (empty == valid).
func Validate(c *Change) []string {
	var errs []string
	if c == nil {
		return []string{"manifest is nil"}
	}
	if c.ESLVersion == "" {
		errs = append(errs, "esl_version is required")
	} else if !eslVersionRe.MatchString(c.ESLVersion) {
		errs = append(errs, fmt.Sprintf("esl_version %q does not match MAJOR.MINOR", c.ESLVersion))
	}
	if c.ChangeID == "" {
		errs = append(errs, "change_id is required")
	} else if !changeIDRe.MatchString(c.ChangeID) {
		errs = append(errs, fmt.Sprintf("change_id %q is not a kebab identifier", c.ChangeID))
	}
	if c.Status == "" {
		errs = append(errs, "status is required")
	} else if !ValidStatus(c.Status) {
		errs = append(errs, fmt.Sprintf("status %q is not a legal enum value", c.Status))
	}
	if c.Tier == "" {
		errs = append(errs, "tier is required")
	} else if !ValidTier(c.Tier) {
		errs = append(errs, fmt.Sprintf("tier %q is not a legal enum value", c.Tier))
	}
	if c.Maker == "" {
		errs = append(errs, "maker is required and non-empty")
	}
	if c.Checker == "" {
		errs = append(errs, "checker is required and non-empty")
	}
	if c.AcceptanceChecks == nil {
		errs = append(errs, "acceptance_checks is required")
	} else {
		for i, ac := range c.AcceptanceChecks {
			// Plain-string items (Raw set) carry no id — that is the legal
			// oneOf:[string,...] form (ESL §2.5). Only the object form requires a
			// non-empty id (schema/change.v1.json: required ["id"]).
			if ac.Raw == "" && ac.ID == "" {
				errs = append(errs, fmt.Sprintf("acceptance_checks[%d].id is required and non-empty (object form)", i))
			}
		}
	}
	// spec_ref is required by the schema (string or null); a missing key decodes
	// to nil here, which is the legal `null`. There is no further constraint.
	return errs
}

// DriftCheckedTrue reports whether drift_checked is present and true.
func (c *Change) DriftCheckedTrue() bool {
	return c.DriftChecked != nil && *c.DriftChecked
}
