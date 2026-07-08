package fsdiag

import (
	"os"
	"strings"
)

// selinuxEnforcePath is the kernel's SELinux enforce toggle ("1" == enforcing).
// A package var so tests can point it at a fixture.
var selinuxEnforcePath = "/sys/fs/selinux/enforce"

func init() { register(selinuxHint) }

// selinuxHint fires on SELinux-enforcing hosts, where a workspace bind mount
// without a label option (:z / :Z) stays labeled user_home_t and container_t is
// denied writes while reads still pass (tonberry#3). It cannot distinguish a
// label denial from other MAC denials, so it stays advisory — it only speaks up
// when the policy is actually enforcing.
func selinuxHint(path string) string {
	if !selinuxEnforcing() {
		return ""
	}
	return "on an SELinux-enforcing host the workspace bind mount needs a label " +
		"option: add :z to the volume flag (-v <path>:/workspace:z) so Docker " +
		"relabels the tree shared (container_file_t), or use " +
		"--security-opt label=disable"
}

func selinuxEnforcing() bool {
	b, err := os.ReadFile(selinuxEnforcePath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(b)) == "1"
}
