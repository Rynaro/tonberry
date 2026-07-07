package fsdiag

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// withEnforcePath points selinuxEnforcePath at path for the duration of the
// test, restoring the original value on cleanup.
func withEnforcePath(t *testing.T, path string) {
	t.Helper()
	orig := selinuxEnforcePath
	selinuxEnforcePath = path
	t.Cleanup(func() { selinuxEnforcePath = orig })
}

func writeEnforceFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "enforce")
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("writeEnforceFile: %v", err)
	}
	return p
}

func permissionErr(path string) error {
	return &os.PathError{Op: "mkdir", Path: path, Err: syscall.EACCES}
}

func TestExplainNilError(t *testing.T) {
	if err := Explain(nil, "/x"); err != nil {
		t.Fatalf("Explain(nil, ...) = %v, want nil", err)
	}
}

func TestExplainNonPermissionErrorPassesThroughUnchanged(t *testing.T) {
	orig := errors.New("boom")
	got := Explain(orig, "/x")
	if got != orig {
		t.Fatalf("Explain(non-permission err) = %v, want unchanged %v", got, orig)
	}
	if !errors.Is(got, orig) {
		t.Fatalf("Explain(non-permission err) lost identity: %v", got)
	}
}

func TestExplainAppendsSELinuxHintWhenEnforcing(t *testing.T) {
	withEnforcePath(t, writeEnforceFile(t, "1"))

	path := "/workspace/.spectra/changes/foo"
	err := Explain(permissionErr(path), path)
	if err == nil {
		t.Fatal("Explain returned nil, want a wrapped permission error")
	}
	msg := err.Error()
	if !strings.Contains(msg, ":z") {
		t.Errorf("expected hint to mention :z, got: %s", msg)
	}
	if !strings.Contains(msg, "label=disable") {
		t.Errorf("expected hint to mention label=disable, got: %s", msg)
	}
	if !IsPermission(err) {
		t.Errorf("wrapped error should still satisfy IsPermission: %v", err)
	}
}

func TestExplainNoHintWhenNotEnforcing(t *testing.T) {
	withEnforcePath(t, writeEnforceFile(t, "0"))

	path := "/workspace/.spectra/changes/foo"
	orig := permissionErr(path)
	got := Explain(orig, path)
	if got.Error() != orig.Error() {
		t.Errorf("expected no hint appended, got: %s", got.Error())
	}
}

func TestExplainNoHintWhenEnforcePathMissing(t *testing.T) {
	withEnforcePath(t, filepath.Join(t.TempDir(), "does-not-exist"))

	path := "/workspace/.spectra/changes/foo"
	orig := permissionErr(path)
	got := Explain(orig, path)
	if got.Error() != orig.Error() {
		t.Errorf("expected no hint appended when enforce path missing, got: %s", got.Error())
	}
}

func TestIsPermission(t *testing.T) {
	if IsPermission(nil) {
		t.Error("IsPermission(nil) = true, want false")
	}
	if IsPermission(errors.New("boom")) {
		t.Error("IsPermission(non-permission err) = true, want false")
	}
	if !IsPermission(permissionErr("/x")) {
		t.Error("IsPermission(EACCES path error) = false, want true")
	}
	if !IsPermission(fs.ErrPermission) {
		t.Error("IsPermission(fs.ErrPermission) = false, want true")
	}
}
