// Package fsdiag turns a bare filesystem permission error into an actionable
// diagnostic. tonberry runs almost exclusively as a containerized MCP server
// bind-mounting a host workspace; when a write op fails with EACCES/EPERM the
// raw OS message ("permission denied") gives the operator nothing to act on,
// while reads keep working — a quietly asymmetric failure that is expensive to
// diagnose from the MCP client side (see tonberry#3, tonberry#4).
//
// Explain inspects a permission error and appends host-aware hints. Each hint
// source registers a Detector; Explain runs them all and joins the non-empty
// hints onto the wrapped error. Non-permission (or nil) errors pass through
// unchanged, so callers can wrap unconditionally.
package fsdiag

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Detector inspects the target path of a failed write and returns a one-line
// remediation hint, or "" when its condition does not apply on this host.
type Detector func(path string) string

// detectors is the ordered registry of hint sources. Hint files register via
// init(); order is registration order (deterministic within a build).
var detectors []Detector

// register adds a detector to the registry. Called from hint files' init().
func register(d Detector) { detectors = append(detectors, d) }

// IsPermission reports whether err is (or wraps) a filesystem permission error
// — EACCES or EPERM surface as fs.ErrPermission through *os.PathError.
func IsPermission(err error) bool {
	return err != nil && errors.Is(err, fs.ErrPermission)
}

// Explain returns err unchanged unless it is a permission error, in which case
// it appends every applicable host hint. path is the write target that failed
// (used by detectors to stat ownership, resolve the mount, etc.).
func Explain(err error, path string) error {
	if !IsPermission(err) {
		return err
	}
	var hints []string
	for _, d := range detectors {
		if h := d(path); h != "" {
			hints = append(hints, "hint: "+h)
		}
	}
	if len(hints) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(hints, "\n"))
}
