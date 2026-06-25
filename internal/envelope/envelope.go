// Package envelope composes ECL sidecar envelopes on disk for ESL lifecycle
// transitions (ESL §7).
//
// ANTI-SCOPE (ESL §1.3, §7.1): this package NAMES a performative from the closed
// ECL ten-set and emits a sidecar whose SHAPE MIRRORS the eidolons-esl example
// envelopes (envelope_version "1.0"). It MUST NOT re-declare the ECL envelope
// schema — the authoritative schema lives in Rynaro/eidolons-ecl. The struct
// below is the minimal on-disk projection needed to be conformant with the
// checker's C6 (valid JSON + performative in the closed-10 set) and to match the
// examples; ECL owns the canonical definition.
//
// The ECL envelope_version stamp is configurable (default "1.0" to MATCH the
// eidolons-esl examples). tonberry does NOT resolve the ECL spec(1.0)-vs-wire(2.0)
// ambiguity; it stamps whatever DefaultEnvelopeVersion / the caller specifies.
package envelope

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultEnvelopeVersion matches the eidolons-esl example envelopes. Overridable
// via the TONBERRY_ECL_ENVELOPE_VERSION env var or an explicit option, so the
// stamp tracks whatever the ecosystem's eidolons-ecl ships without tonberry
// unilaterally resolving the version ambiguity.
const DefaultEnvelopeVersion = "1.0"

// closed ECL ten-performative set (REFERENCE to ECL, not ESL-owned).
var performativeSet = map[string]bool{
	"REQUEST": true, "INFORM": true, "PROPOSE": true, "CRITIQUE": true, "DECIDE": true,
	"DELEGATE": true, "ACKNOWLEDGE": true, "ESCALATE": true, "RESUME": true, "REFUSE": true,
}

// ValidPerformative reports whether p is in the closed ECL ten-set.
func ValidPerformative(p string) bool { return performativeSet[p] }

// EnvelopeVersion resolves the stamp: explicit override > env var > default.
func EnvelopeVersion(override string) string {
	if override != "" {
		return override
	}
	if v := os.Getenv("TONBERRY_ECL_ENVELOPE_VERSION"); v != "" {
		return v
	}
	return DefaultEnvelopeVersion
}

// Party is an envelope sender/receiver identity (ECL-shaped; referenced).
type Party struct {
	Eidolon string `json:"eidolon"`
	Version string `json:"version,omitempty"`
}

// Envelope is the minimal on-disk ECL sidecar projection. Field names/order
// mirror the eidolons-esl example *.envelope.json files. ECL owns the canonical
// schema; this is a conformant projection, not a re-declaration.
type Envelope struct {
	EnvelopeVersion string                 `json:"envelope_version"`
	From            Party                  `json:"from"`
	To              Party                  `json:"to"`
	Performative    string                 `json:"performative"`
	Objective       string                 `json:"objective,omitempty"`
	ContextDelta    map[string]interface{} `json:"context_delta,omitempty"`
}

// Options configure envelope composition.
type Options struct {
	// EnvelopeVersionOverride, if non-empty, sets the stamp (else env/default).
	EnvelopeVersionOverride string
	// Objective is a one-line human-readable purpose.
	Objective string
	// ContextDelta carries the per-transition delta. Per ESL §7.3, memory-ingest
	// intent rides this FIELD, not a performative.
	ContextDelta map[string]interface{}
}

// Compose builds an Envelope for a transition. performative MUST be in the
// closed-10 set (validated). from/to are eidolon identities.
func Compose(performative, from, to string, opts Options) (*Envelope, error) {
	if !ValidPerformative(performative) {
		return nil, fmt.Errorf("performative %q is not in the closed ECL ten-set", performative)
	}
	return &Envelope{
		EnvelopeVersion: EnvelopeVersion(opts.EnvelopeVersionOverride),
		From:            Party{Eidolon: from, Version: ""},
		To:              Party{Eidolon: to, Version: ""},
		Performative:    performative,
		Objective:       opts.Objective,
		ContextDelta:    opts.ContextDelta,
	}, nil
}

// Write marshals the envelope to <changeDir>/<basename> (e.g. "propose.envelope.json").
// Returns the absolute-or-relative path written.
func Write(changeDir, basename string, e *Envelope) (string, error) {
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", changeDir, err)
	}
	p := filepath.Join(changeDir, basename)
	data, err := MarshalIndent(e)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", p, err)
	}
	return p, nil
}

// MarshalIndent renders an Envelope as deterministic pretty JSON with a trailing newline.
func MarshalIndent(e *Envelope) ([]byte, error) {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	return append(data, '\n'), nil
}
