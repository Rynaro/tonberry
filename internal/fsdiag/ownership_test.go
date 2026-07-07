package fsdiag

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnershipHintFor_Mismatch(t *testing.T) {
	msg := ownershipHintFor(1000, true, 65532)
	if msg == "" {
		t.Fatal("expected a hint for mismatched UIDs, got empty string")
	}
	for _, want := range []string{"--user", "1000", "65532"} {
		if !strings.Contains(msg, want) {
			t.Errorf("hint %q does not contain %q", msg, want)
		}
	}
}

func TestOwnershipHintFor_MatchingUID(t *testing.T) {
	if got := ownershipHintFor(1000, true, 1000); got != "" {
		t.Errorf("expected no hint for matching UIDs, got %q", got)
	}
}

func TestOwnershipHintFor_OwnerUnknown(t *testing.T) {
	if got := ownershipHintFor(0, false, 65532); got != "" {
		t.Errorf("expected no hint when owner is unknown, got %q", got)
	}
}

func TestOwnershipHintFor_ProcUIDUnknown(t *testing.T) {
	if got := ownershipHintFor(1000, true, -1); got != "" {
		t.Errorf("expected no hint when process UID is unknown, got %q", got)
	}
}

func TestOwnerUID_TempDir(t *testing.T) {
	dir := t.TempDir()
	uid, ok := ownerUID(dir)
	if !ok {
		t.Fatal("expected ownerUID to resolve the temp dir owner")
	}
	if uid != uint32(os.Geteuid()) {
		t.Errorf("ownerUID(%q) = %d, want %d", dir, uid, os.Geteuid())
	}
}

func TestOwnerUID_NonExistentDeepPath(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "change.json")
	uid, ok := ownerUID(deep)
	if !ok {
		t.Fatal("expected ownerUID to walk up to the nearest existing ancestor")
	}
	if uid != uint32(os.Geteuid()) {
		t.Errorf("ownerUID(%q) = %d, want %d", deep, uid, os.Geteuid())
	}
}

func TestExplain_NilError(t *testing.T) {
	if err := Explain(nil, "/x"); err != nil {
		t.Errorf("Explain(nil, ...) = %v, want nil", err)
	}
}

func TestExplain_NonPermissionErrorPassesThrough(t *testing.T) {
	orig := errors.New("boom")
	got := Explain(orig, "/x")
	if got != orig {
		t.Errorf("Explain(%v, ...) = %v, want unchanged %v", orig, got, orig)
	}
}

func TestExplain_PermissionErrorAppendsHint(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "change.json")
	permErr := &os.PathError{Op: "write", Path: target, Err: os.ErrPermission}
	wrapped := fmt.Errorf("write %s: %w", target, permErr)

	got := Explain(wrapped, target)
	if got == nil {
		t.Fatal("expected a non-nil error")
	}
	if !errors.Is(got, os.ErrPermission) {
		t.Errorf("Explain result does not wrap the original permission error: %v", got)
	}
	// Same-UID process owns the temp dir, so no ownership mismatch hint is
	// expected here — but the error itself must still be non-nil and wrap the
	// original (covered above). This test primarily guards that Explain does
	// not panic or drop the original error for a real permission error.
}
