package lifecycle

import (
	"testing"

	"github.com/Rynaro/tonberry/internal/manifest"
)

func TestSkipRuleDeliberatedForLiteTrivial(t *testing.T) {
	// lite/trivial MUST NOT enter deliberated (ESL §3.1 skip-rule).
	for _, tier := range []manifest.Tier{manifest.TierLite, manifest.TierTrivial} {
		d := Transition(manifest.StatusProposed, manifest.StatusDeliberated, tier, false)
		if d.Allowed {
			t.Errorf("tier %q: proposed->deliberated should be REJECTED, got allowed", tier)
		}
	}
	// full MAY enter deliberated.
	d := Transition(manifest.StatusProposed, manifest.StatusDeliberated, manifest.TierFull, false)
	if !d.Allowed {
		t.Errorf("full: proposed->deliberated should be allowed, got %q", d.Reason)
	}
}

func TestArchivedRequiresVerified(t *testing.T) {
	// archived may only be entered from verified (no skipping verified).
	for _, from := range []manifest.Status{manifest.StatusProposed, manifest.StatusInProgress} {
		d := Transition(from, manifest.StatusArchived, manifest.TierFull, true)
		if d.Allowed {
			t.Errorf("%s->archived should be REJECTED (must pass verified), got allowed", from)
		}
	}
	d := Transition(manifest.StatusVerified, manifest.StatusArchived, manifest.TierFull, true)
	if !d.Allowed {
		t.Errorf("verified->archived should be allowed, got %q", d.Reason)
	}
}

func TestInProgressRequiresCode(t *testing.T) {
	// in_progress requires code; a no-code change skips it.
	dNoCode := Transition(manifest.StatusProposed, manifest.StatusInProgress, manifest.TierLite, false)
	if dNoCode.Allowed {
		t.Errorf("no-code proposed->in_progress should be rejected (skip the code state), got allowed")
	}
	dCode := Transition(manifest.StatusProposed, manifest.StatusInProgress, manifest.TierLite, true)
	if !dCode.Allowed {
		t.Errorf("code proposed->in_progress should be allowed, got %q", dCode.Reason)
	}
}

func TestNoCodeSkipsToVerified(t *testing.T) {
	// A no-code lite change may go proposed->verified directly.
	d := Transition(manifest.StatusProposed, manifest.StatusVerified, manifest.TierLite, false)
	if !d.Allowed {
		t.Errorf("no-code proposed->verified should be allowed, got %q", d.Reason)
	}
}

func TestCodeChangeMustPassInProgress(t *testing.T) {
	// A code change MUST pass through in_progress before verified.
	d := Transition(manifest.StatusProposed, manifest.StatusVerified, manifest.TierFull, true)
	if d.Allowed {
		t.Errorf("code proposed->verified (skipping in_progress) should be rejected, got allowed")
	}
}

func TestEscalateBackToInProgress(t *testing.T) {
	// verify_fail ESCALATE: verified/archived -> in_progress is the only legal backward edge.
	for _, from := range []manifest.Status{manifest.StatusVerified, manifest.StatusArchived} {
		d := Transition(from, manifest.StatusInProgress, manifest.TierFull, true)
		if !d.Allowed {
			t.Errorf("%s->in_progress (ESCALATE) should be allowed, got %q", from, d.Reason)
		}
		if d.NextPerformative != "ESCALATE" {
			t.Errorf("%s->in_progress should name ESCALATE, got %q", from, d.NextPerformative)
		}
	}
}

func TestNoBackwardExceptEscalate(t *testing.T) {
	// e.g. verified->proposed is illegal.
	d := Transition(manifest.StatusVerified, manifest.StatusProposed, manifest.TierFull, true)
	if d.Allowed {
		t.Errorf("verified->proposed should be rejected, got allowed")
	}
}

func TestNoOp(t *testing.T) {
	d := Transition(manifest.StatusProposed, manifest.StatusProposed, manifest.TierFull, true)
	if d.Allowed {
		t.Errorf("proposed->proposed should be rejected (no-op)")
	}
}

func TestEnteringPerformatives(t *testing.T) {
	cases := map[manifest.Status]string{
		manifest.StatusProposed:    "PROPOSE",
		manifest.StatusDeliberated: "CRITIQUE",
		manifest.StatusInProgress:  "DELEGATE",
		manifest.StatusVerified:    "INFORM",
		manifest.StatusArchived:    "ACKNOWLEDGE",
	}
	for s, want := range cases {
		if got := EnteringPerformative(s); got != want {
			t.Errorf("EnteringPerformative(%s) = %q, want %q", s, got, want)
		}
	}
}
