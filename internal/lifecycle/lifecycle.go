// Package lifecycle implements the ESL v1.0 §3 state machine and its
// right-sizing skip-rules (the `transition` operation).
//
// The five states are proposed → [deliberated] → in_progress → verified →
// archived. The ONLY mandatory spine is proposed → (in_progress if code) →
// verified → archived. `deliberated` is conditional on a real trade-off (and is
// SKIPPED for lite/trivial tiers); the code-states are conditional on there
// being code (ESL §3.2).
//
// Transition-to-performative mapping is ESL §7.2 — this package NAMES the ECL
// performative for a transition but does NOT re-declare the ECL envelope schema
// (anti-scope, ESL §1.3); composing the envelope is the envelope package's job.
package lifecycle

import (
	"fmt"

	"github.com/Rynaro/tonberry/internal/manifest"
)

// Decision is the result of evaluating a requested transition.
type Decision struct {
	From             manifest.Status `json:"from_status"`
	To               manifest.Status `json:"to_status"`
	Allowed          bool            `json:"allowed"`
	Reason           string          `json:"reason,omitempty"`
	NextPerformative string          `json:"next_performative,omitempty"`
}

// rank gives each status its position in the lifecycle spine for ordering checks.
func rank(s manifest.Status) int {
	switch s {
	case manifest.StatusProposed:
		return 0
	case manifest.StatusDeliberated:
		return 1
	case manifest.StatusInProgress:
		return 2
	case manifest.StatusVerified:
		return 3
	case manifest.StatusArchived:
		return 4
	}
	return -1
}

// EnteringPerformative returns the ECL performative NAMED for entering a state
// (ESL §3 table "Entering ECL performative" + §7.2). It is a name only; the
// envelope package composes the actual sidecar.
//
// Note: verification enters `verified` via INFORM(verify_pass) — the wire
// performative is INFORM (the verify_pass is the objective/context, not a
// performative; the closed-10 set has INFORM, not INFORM(verify_pass)). The
// returned name is the closed-10 performative.
func EnteringPerformative(to manifest.Status) string {
	switch to {
	case manifest.StatusProposed:
		return "PROPOSE"
	case manifest.StatusDeliberated:
		return "CRITIQUE"
	case manifest.StatusInProgress:
		return "DELEGATE"
	case manifest.StatusVerified:
		return "INFORM"
	case manifest.StatusArchived:
		return "ACKNOWLEDGE"
	}
	return ""
}

// Transition evaluates whether a change at `from` (tier `tier`, hasCode flag)
// may advance to `to`, applying the §3 skip-rules. It is a pure predicate: it
// computes the Decision and never mutates anything.
//
// hasCode signals "this change contains code" — the code-states (in_progress)
// are required if any code, skippable if none (ESL §3.2). For non-code changes
// (e.g. a lite spec-only doc change) the machine MAY skip in_progress.
//
// Skip-rules enforced:
//   - deliberated is SKIPPED for lite/trivial tiers: a lite/trivial change MUST
//     NOT enter `deliberated` (ESL §3.1 "SKIP for lite/trivial", §4.2).
//   - archived requires the §6.4 precondition (drift_checked==true). That
//     precondition is a manifest-data check enforced by the caller / verify C5;
//     here we reject the *transition* if the change is not at least `verified`.
//   - no backward jumps except the explicit verify_fail ESCALATE return to
//     in_progress (verified/archived → in_progress is allowed as the escalate
//     path, ESL §5.3/§6.4).
//   - no skipping `verified` on the way to `archived`.
func Transition(from, to manifest.Status, tier manifest.Tier, hasCode bool) Decision {
	d := Decision{From: from, To: to, NextPerformative: EnteringPerformative(to)}

	rf, rt := rank(from), rank(to)
	if rf < 0 {
		d.Allowed = false
		d.Reason = fmt.Sprintf("illegal from-status %q", from)
		return d
	}
	if rt < 0 {
		d.Allowed = false
		d.Reason = fmt.Sprintf("illegal to-status %q", to)
		return d
	}
	if from == to {
		d.Allowed = false
		d.Reason = "no-op transition (from == to)"
		return d
	}

	// Skip-rule: deliberated is for full tier only (real trade-off). lite/trivial
	// MUST NOT enter deliberated.
	if to == manifest.StatusDeliberated && tier != manifest.TierFull {
		d.Allowed = false
		d.Reason = fmt.Sprintf("tier %q skips `deliberated` (only `full` deliberates on a trade-off)", tier)
		return d
	}

	// The verify_fail ESCALATE return path: verified/archived → in_progress is
	// the only legal backward edge (ESL §5.3, §6.4).
	if to == manifest.StatusInProgress && (from == manifest.StatusVerified || from == manifest.StatusArchived) {
		d.Allowed = true
		d.Reason = "verify_fail ESCALATE: return to in_progress"
		d.NextPerformative = "ESCALATE"
		return d
	}

	// Otherwise only forward motion is allowed.
	if rt <= rf {
		d.Allowed = false
		d.Reason = fmt.Sprintf("illegal backward/static transition %s -> %s", from, to)
		return d
	}

	// Forward edges, honoring legal skips:
	switch to {
	case manifest.StatusDeliberated:
		// Only reachable from proposed (already gated to full tier above).
		if from != manifest.StatusProposed {
			d.Allowed = false
			d.Reason = "deliberated may only be entered from proposed"
			return d
		}
	case manifest.StatusInProgress:
		// Reachable from proposed (skipping deliberated for non-trade-off) or
		// deliberated. Requires code.
		if !hasCode {
			d.Allowed = false
			d.Reason = "in_progress requires code (no-code changes skip the code states)"
			return d
		}
	case manifest.StatusVerified:
		// Reachable from in_progress (the normal path) or, for no-code changes
		// that skipped in_progress, from proposed/deliberated.
		if hasCode && from != manifest.StatusInProgress {
			d.Allowed = false
			d.Reason = "a code change must pass through in_progress before verified"
			return d
		}
	case manifest.StatusArchived:
		// MUST NOT skip verified.
		if from != manifest.StatusVerified {
			d.Allowed = false
			d.Reason = "archived may only be entered from verified (verified is required to promote)"
			return d
		}
	}

	d.Allowed = true
	return d
}
