package fsdiag

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func init() { register(ownershipHint) }

// ownershipHint fires when the process UID differs from the owner of the
// nearest existing ancestor of the failed path. The tonberry image runs as
// distroless nonroot UID 65532, but a host workspace is typically owned by the
// developer's UID (e.g. 1000) at mode 0755 — granting o+rx (reads) but no write
// (tonberry#4).
func ownershipHint(path string) string {
	owner, ok := ownerUID(path)
	return ownershipHintFor(owner, ok, os.Geteuid())
}

// ownershipHintFor is the pure core: it renders the hint iff the owner is known
// and differs from the process UID. proc < 0 (unknown) or a match yields "".
func ownershipHintFor(owner uint32, hasOwner bool, proc int) string {
	if !hasOwner || proc < 0 || uint32(proc) == owner {
		return ""
	}
	return fmt.Sprintf(
		"the workspace is owned by UID %d but this process runs as UID %d — "+
			"re-run the container as the mount owner: add --user \"$(id -u):$(id -g)\" "+
			"to the docker run args (or chown the workspace to UID %d)",
		owner, proc, proc,
	)
}

// ownerUID returns the owning UID of the nearest existing ancestor of path.
func ownerUID(path string) (uint32, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for {
		if fi, err := os.Stat(abs); err == nil {
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				return st.Uid, true
			}
			return 0, false
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return 0, false
		}
		abs = parent
	}
}
