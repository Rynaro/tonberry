package rightsizing

import (
	"testing"

	"github.com/Rynaro/tonberry/internal/manifest"
)

func TestClassifyTable(t *testing.T) {
	cases := []struct {
		name   string
		s      Signals
		want   manifest.Tier
	}{
		// full triggers (precedence 1)
		{"rubric>=7 forces full", Signals{FilesTouched: 1, RubricScore: 7}, manifest.TierFull},
		{"rubric=12 forces full", Signals{FilesTouched: 1, RubricScore: 12}, manifest.TierFull},
		{"tradeoff forces full even at low rubric", Signals{FilesTouched: 1, RubricScore: 1, TradeoffPresent: true}, manifest.TierFull},
		{"tradeoff forces full even trivial-looking", Signals{FilesTouched: 0, RubricScore: 0, TradeoffPresent: true}, manifest.TierFull},
		// trivial (precedence 2): files<=2 AND rubric<=4 AND no tradeoff
		{"files=2 rubric=4 -> trivial", Signals{FilesTouched: 2, RubricScore: 4}, manifest.TierTrivial},
		{"files=0 rubric=0 -> trivial", Signals{FilesTouched: 0, RubricScore: 0}, manifest.TierTrivial},
		// lite (precedence 3): the residual
		{"rubric=5 -> lite", Signals{FilesTouched: 1, RubricScore: 5}, manifest.TierLite},
		{"rubric=6 -> lite", Signals{FilesTouched: 1, RubricScore: 6}, manifest.TierLite},
		{"files=3 rubric=4 -> lite (files break trivial)", Signals{FilesTouched: 3, RubricScore: 4}, manifest.TierLite},
		{"files=2 rubric=5 -> lite (rubric breaks trivial)", Signals{FilesTouched: 2, RubricScore: 5}, manifest.TierLite},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.s).Tier
			if got != c.want {
				t.Errorf("Classify(%+v).Tier = %q, want %q", c.s, got, c.want)
			}
		})
	}
}

// TestDeterminism: identical signals MUST yield byte-identical Result across many
// repeated runs (ESL §4.3). This is the determinism gate as a property test.
func TestDeterminism(t *testing.T) {
	signalSets := []Signals{
		{FilesTouched: 1, RubricScore: 3},
		{FilesTouched: 5, RubricScore: 6, TradeoffPresent: false},
		{FilesTouched: 9, RubricScore: 9, TradeoffPresent: true},
		{FilesTouched: 2, RubricScore: 4},
		{FilesTouched: 2, RubricScore: 5},
	}
	for _, s := range signalSets {
		first := Classify(s)
		for i := 0; i < 1000; i++ {
			got := Classify(s)
			if got != first {
				t.Fatalf("non-deterministic for %+v: run %d = %+v, first = %+v", s, i, got, first)
			}
		}
	}
}

func TestRouteStrings(t *testing.T) {
	if Classify(Signals{FilesTouched: 0, RubricScore: 0}).Route != "bypass machine" {
		t.Error("trivial route")
	}
	if Classify(Signals{FilesTouched: 1, RubricScore: 5}).Route != "0->2->3->4" {
		t.Error("lite route")
	}
	if Classify(Signals{FilesTouched: 1, RubricScore: 9}).Route != "0->1->2->3->4" {
		t.Error("full route")
	}
}

func TestDeterministicFlagAlwaysTrue(t *testing.T) {
	if !Classify(Signals{}).Deterministic {
		t.Error("Deterministic must be true")
	}
}
