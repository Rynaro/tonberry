// Package rightsizing implements the ESL v1.0 §4 mandatory, mechanical
// right-sizing gate. It classifies a change into trivial / lite / full from
// exactly three observable signals (ESL §4.2):
//
//	(a) files_touched   — an integer estimate from the proposal
//	(b) rubric_score    — the SPECTRA /12 complexity-matrix score (integer 0..12)
//	(c) tradeoff_present — true iff a competing-approach decision exists (the FORGE trigger)
//
// DETERMINISM (ESL §4.3): given the same three signals the gate MUST yield the
// identical tier. This package is pure arithmetic on the signals — no maps, no
// time, no randomness, no I/O — so the output is a deterministic function of the
// input. The thresholds are the §4.2 table constants.
package rightsizing

import "github.com/Rynaro/tonberry/internal/manifest"

// Signals are the three mechanical inputs to the gate (ESL §4.2).
type Signals struct {
	FilesTouched    int  `json:"files_touched"`
	RubricScore     int  `json:"rubric_score"`
	TradeoffPresent bool `json:"tradeoff_present"`
}

// Result is the gate output: the tier, the §4.2 route path string, the echoed
// signals, and the determinism marker (always true — the gate is pure).
type Result struct {
	Tier          manifest.Tier `json:"tier"`
	Route         string        `json:"route"`
	Signals       Signals       `json:"signals"`
	Deterministic bool          `json:"deterministic"`
}

// §4.2 table thresholds. These are the spec constants, not tunable knobs.
const (
	trivialMaxFiles  = 2 // files <= 2
	trivialMaxRubric = 4 // /12 <= 4
	liteMinRubric    = 5 // /12 in 5..6
	liteMaxRubric    = 6
	fullMinRubric    = 7 // /12 >= 7
)

// Classify applies the §4.2 / §4.3 precedence:
//
//	any `full` trigger wins  (/12 >= 7  OR  trade-off present);
//	else `trivial` if ALL trivial conditions hold (files <= 2 AND /12 <= 4);
//	else `lite`.
//
// The "no new behavior contract" trivial condition and the "system-wide blast
// radius" full condition are LLM/proposal-level judgments not expressible from
// the three mechanical signals; the mechanical gate uses the signal thresholds
// it can compute, and the proposal layer is responsible for the qualitative
// triggers upstream (ESL §4.2). The trade-off boolean already carries the FORGE
// trigger, which is the mechanical proxy for a competing-approach decision.
func Classify(s Signals) Result {
	tier := classifyTier(s)
	return Result{
		Tier:          tier,
		Route:         route(tier),
		Signals:       s,
		Deterministic: true,
	}
}

func classifyTier(s Signals) manifest.Tier {
	// Precedence 1: any full trigger wins.
	if s.RubricScore >= fullMinRubric || s.TradeoffPresent {
		return manifest.TierFull
	}
	// Precedence 2: trivial if ALL trivial conditions hold.
	if s.FilesTouched <= trivialMaxFiles && s.RubricScore <= trivialMaxRubric {
		return manifest.TierTrivial
	}
	// Precedence 3: otherwise lite.
	return manifest.TierLite
}

// route returns the §4.2 "Path" string for a tier.
func route(t manifest.Tier) string {
	switch t {
	case manifest.TierTrivial:
		return "bypass machine"
	case manifest.TierLite:
		return "0->2->3->4"
	case manifest.TierFull:
		return "0->1->2->3->4"
	}
	return ""
}
