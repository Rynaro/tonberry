package conformance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// THE PARITY GATE (FORGE Decision 2, LOAD-BEARING).
//
// For every fixture, assert that `tonberry verify` (conformance.Check) and the
// vendored bash oracle (parity/esl-conformance.sh) agree on:
//   - the SET of {id,status} findings, and
//   - the exit_code,
// for BOTH --mode warn and --mode block. The bash checker is authoritative; a
// divergence is a release-blocking reversal condition.
//
// The test shells out to `bash <oracle> <fixture> --json --mode <m>` and parses
// its stdout JSON. It requires bash + jq. If either is missing it FAILS (not
// skips) when TONBERRY_REQUIRE_PARITY=1 (CI sets this); otherwise it skips so a
// jq-less dev box can still run the rest of the suite.

type bashResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type bashSummary struct {
	Results  []bashResult `json:"results"`
	ExitCode int          `json:"exit_code"`
}

// repoRoot walks up from this test file to the module root (where go.mod lives).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller")
	}
	// internal/conformance/parity_test.go -> up two dirs.
	dir := filepath.Dir(thisFile)
	root := filepath.Clean(filepath.Join(dir, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod not found at %s: %v", root, err)
	}
	return root
}

// fixtureDirs returns every change folder under fixtures/ (conformant + failing),
// EXCLUDING nested archive snapshots (those are not standalone parity targets,
// though they would pass; we test the top-level change folders).
func fixtureDirs(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, group := range []string{"conformant", "failing"} {
		base := filepath.Join(root, "fixtures", group)
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatalf("read %s: %v", base, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			out[group+"/"+e.Name()] = filepath.Join(base, e.Name())
		}
	}
	return out
}

// runBashOracle invokes the vendored checker and returns the {id,status} set key
// and exit code. ok=false means bash/jq are unavailable.
func runBashOracle(t *testing.T, root, fixture, mode string) (key string, exit int, ok bool) {
	t.Helper()
	bashBin, berr := exec.LookPath("bash")
	if berr != nil {
		return "", 0, false
	}
	if _, jerr := exec.LookPath("jq"); jerr != nil {
		return "", 0, false
	}
	oracle := filepath.Join(root, "parity", "esl-conformance.sh")
	cmd := exec.Command(bashBin, oracle, fixture, "--json", "--mode", mode)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = nil // findings go to stderr; we ignore them
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, isExit := err.(*exec.ExitError); isExit {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("running oracle on %s: %v", fixture, err)
		}
	}
	var sum bashSummary
	if jerr := json.Unmarshal([]byte(stdout.String()), &sum); jerr != nil {
		t.Fatalf("parse oracle JSON for %s: %v\nstdout=%q", fixture, jerr, stdout.String())
	}
	return setKey(toPairs(sum.Results)), exitCode, true
}

type idStatus struct{ id, status string }

func toPairs(rs []bashResult) []idStatus {
	out := make([]idStatus, 0, len(rs))
	for _, r := range rs {
		out = append(out, idStatus{r.ID, r.Status})
	}
	return out
}

func goPairs(rs []Result) []idStatus {
	out := make([]idStatus, 0, len(rs))
	for _, r := range rs {
		out = append(out, idStatus{r.ID, r.Status})
	}
	return out
}

// setKey normalizes a slice of {id,status} into a sorted, deterministic key.
// Multiple findings with the same id (e.g. several C6 envelopes) are preserved
// as a multiset (sorted), so a count mismatch is also a divergence.
func setKey(pairs []idStatus) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, p.id+"|"+p.status)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func requireParity() bool { return os.Getenv("TONBERRY_REQUIRE_PARITY") == "1" }

func TestParityAgainstBashOracle(t *testing.T) {
	root := repoRoot(t)
	fixtures := fixtureDirs(t, root)
	if len(fixtures) == 0 {
		t.Fatal("no fixtures found")
	}

	checked := 0
	for name, dir := range fixtures {
		for _, mode := range []string{"warn", "block"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				bashKey, bashExit, ok := runBashOracle(t, root, dir, mode)
				if !ok {
					if requireParity() {
						t.Fatal("bash and/or jq unavailable but TONBERRY_REQUIRE_PARITY=1")
					}
					t.Skip("bash/jq unavailable; skipping parity (set TONBERRY_REQUIRE_PARITY=1 to enforce)")
				}
				abs, _ := filepath.Abs(dir)
				rep := Check(abs, Mode(mode))
				goKey := setKey(goPairs(rep.Results))

				if goKey != bashKey {
					t.Errorf("PARITY DIVERGENCE on %s [--mode %s]\n  go  : %s\n  bash: %s\n(bash is authoritative)", name, mode, goKey, bashKey)
				}
				if rep.ExitCode != bashExit {
					t.Errorf("EXIT-CODE DIVERGENCE on %s [--mode %s]: go=%d bash=%d (bash is authoritative)", name, mode, rep.ExitCode, bashExit)
				}
				checked++
			})
		}
	}
	if checked == 0 && requireParity() {
		t.Fatal("parity enforced but no fixtures were actually compared")
	}
	t.Logf("parity compared %d fixture×mode combinations", checked)
}
