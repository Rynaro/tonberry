// Package conformance is a faithful Go port of the normative ESL conformance
// checker, conformance/esl-conformance.sh from Rynaro/eidolons-esl.
//
// THE PARITY INVARIANT (FORGE Decision 2 — LOAD-BEARING): for every change-folder
// input, `tonberry verify` (this package) and `bash esl-conformance.sh` MUST
// agree on the checks C1–C6 (MUST) and C7/C8 (SHOULD, the advisory EARS lint and
// the fresh-context verification attestation) — ids, MUST/SHOULD level, ok/fail
// verdict, and semantics — the exit codes (0/1/3; 2 reserved), and the --json
// shape. C7/C8 are advisory: a C7- or C8-only failure NEVER changes the exit code
// (only the MUST checks C1–C6 block), in BOTH the bash oracle and this port. The
// bash checker is AUTHORITATIVE on any divergence; a divergence is a
// release-blocking reversal condition. parity/esl-conformance.sh is the vendored
// oracle and parity_test.go is the locking gate.
//
// To stay faithful, this port reads change.json as untyped JSON (like jq's
// `-r '<filter> // empty'`) rather than through the typed manifest.Change struct,
// so an illegal status/tier or wrong-typed value is observed exactly as the bash
// checker observes it, not rejected at decode.
//
// ANTI-SCOPE (ESL §1.3): the closed ECL ten-performative set below is a REFERENCE
// to ECL (schemas/performative.v1.json in Rynaro/eidolons-ecl), vendored verbatim
// from the bash checker as a constant — NOT an ESL-owned enumeration.
package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CheckerVersion mirrors ESL_CHECKER_VERSION in esl-conformance.sh.
const CheckerVersion = "1.1.0"

// eclPerformatives is the closed ECL v1.0 ten-performative set, vendored as a
// constant from esl-conformance.sh:36 (ESL_ECL_PERFORMATIVES). A REFERENCE to
// ECL, not an ESL-owned enum.
var eclPerformatives = []string{
	"REQUEST", "INFORM", "PROPOSE", "CRITIQUE", "DECIDE",
	"DELEGATE", "ACKNOWLEDGE", "ESCALATE", "RESUME", "REFUSE",
}

// Result is one finding, matching the bash esl_record fields.
type Result struct {
	ID     string `json:"id"`
	Level  string `json:"level"`  // MUST | SHOULD
	Status string `json:"status"` // ok | fail
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

// Report is the full check outcome. The JSON shape matches esl-conformance.sh's
// --json output: {target_basename, mode, results[], exit_code}.
type Report struct {
	TargetBasename string   `json:"target_basename"`
	Mode           string   `json:"mode"`
	Results        []Result `json:"results"`
	ExitCode       int      `json:"exit_code"`
	HasFail        bool     `json:"-"`
}

// Mode is the warn/block enforcement mode.
type Mode string

const (
	ModeWarn  Mode = "warn"
	ModeBlock Mode = "block"
)

// recorder accumulates findings append-only, mirroring the bash parallel arrays.
type recorder struct {
	results []Result
}

func (r *recorder) record(id, level, status, name, reason string) {
	r.results = append(r.results, Result{ID: id, Level: level, Status: status, Name: name, Reason: reason})
}

// jget mimics `jq -r '<filter> // empty'` for a top-level string-ish field: it
// returns the value coerced to a string the way `jq -r` would, or "" if the key
// is absent/null/false-ish-empty. jq's `// empty` treats JSON null AND false AND
// the absent key as empty; we replicate that for the string fields the checker
// reads (.status, .tier, .maker, .checker, .performative, .from.eidolon).
func jget(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	return jqStringOrEmpty(v)
}

// jqStringOrEmpty replicates `jq -r (.x // empty)` value coercion: null/false ->
// "" (the `// empty` alternative fires on null and false); strings pass through;
// numbers/bools(true) render like jq -r; objects/arrays render as compact JSON.
func jqStringOrEmpty(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		if !t {
			// jq: false // empty -> empty
			return ""
		}
		return "true"
	case string:
		return t
	case float64:
		// jq -r prints integers without a decimal point.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// nestedFromEidolon replicates `jq -r '.from.eidolon // empty'`.
func nestedFromEidolon(m map[string]any) string {
	from, ok := m["from"].(map[string]any)
	if !ok {
		return ""
	}
	return jget(from, "eidolon")
}

// illegalStatusDetail builds the C2a failure detail (esl-conformance.sh:167
// parity: the bash oracle emits the single shape `illegal status: '$M_STATUS'`
// in every case, and the parity gate (parity_test.go) only compares {id,status}
// pairs — never the reason text — so enriching the Go-side detail below is not
// a parity divergence).
//
// For a NON-EMPTY but illegal status (e.g. "in-review") this keeps that exact
// original shape unchanged, so existing fixtures/tests are unaffected.
//
// For an EMPTY/absent status this adds a DIAGNOSTIC-ONLY detail naming the
// missing field explicitly, and — per Rynaro/tonberry#9 — additionally
// inspects the already-parsed raw manifest map for a non-canonical top-level
// `stage` key (the shape observed in the `ariramba/esl-change.compat.v1`
// project schema, which is NOT the ESL-owned change.v1 schema). This is
// diagnostic only: tonberry does NOT read `stage` as a status value anywhere
// else, and the verdict stays "fail" either way — ESL anti-scope §1.3
// forbids tonberry from adopting or aliasing a non-canonical field into the
// ESL-owned status enum.
func illegalStatusDetail(m map[string]any, mStatus string) string {
	if mStatus != "" {
		return fmt.Sprintf("illegal status: '%s'", mStatus)
	}
	if stage := jget(m, "stage"); stage != "" {
		return fmt.Sprintf(
			"illegal status: '' — no top-level 'status' field; found non-canonical 'stage'='%s'. "+
				"ESL change.v1 requires a top-level 'status' (proposed|deliberated|in_progress|verified|archived); "+
				"this manifest looks like a non-canonical compat schema and must be migrated.",
			stage,
		)
	}
	return "illegal status: '' — no top-level 'status' field"
}

// Check runs the six mechanical checks against a change folder and returns the
// Report. It NEVER returns a usage error itself — the caller (verify command)
// validates the target exists and is a directory and emits the usage exit 1.
// `mode` decides whether HasFail maps to exit 3 (block) or 0 (warn).
func Check(targetAbs string, mode Mode) Report {
	rec := &recorder{}
	changeJSON := filepath.Join(targetAbs, "change.json")

	// -- Check 1: change.json is valid JSON --------------------------------- //
	changeOK := false
	var m map[string]any
	var mStatus, mTier, mMaker, mChecker string
	mDrift := "" // "true" | "false" | ""
	mACCount := 0

	if !fileExists(changeJSON) {
		rec.record("C1", "MUST", "fail", "change_json_present", "no change.json in folder")
	} else if data, err := os.ReadFile(changeJSON); err != nil || !validJSONObject(data, &m) {
		rec.record("C1", "MUST", "fail", "change_json_valid_json", "jq parse failed")
	} else {
		rec.record("C1", "MUST", "ok", "change_json_valid_json", "")
		changeOK = true
		mStatus = jget(m, "status")
		mTier = jget(m, "tier")
		mMaker = jget(m, "maker")
		mChecker = jget(m, "checker")
		mDrift = driftTriState(m)
		mACCount = acceptanceCount(m)
	}

	// -- Check 2: status / tier are legal enum values ----------------------- //
	if changeOK {
		switch mStatus {
		case "proposed", "deliberated", "in_progress", "verified", "archived":
			rec.record("C2a", "MUST", "ok", "status_enum_legal", "")
		default:
			rec.record("C2a", "MUST", "fail", "status_enum_legal", illegalStatusDetail(m, mStatus))
		}
		switch mTier {
		case "trivial", "lite", "full":
			rec.record("C2b", "MUST", "ok", "tier_enum_legal", "")
		default:
			rec.record("C2b", "MUST", "fail", "tier_enum_legal", fmt.Sprintf("illegal tier: '%s'", mTier))
		}
	}

	// -- Check 3: tier-appropriate artifacts present ------------------------ //
	if changeOK {
		specMD := filepath.Join(targetAbs, "spec.md")
		specYAML := filepath.Join(targetAbs, "spec.yaml")
		switch mTier {
		case "trivial":
			rec.record("C3", "MUST", "ok", "tier_artifacts_present", "trivial: no spec required")
		case "lite":
			if !fileExists(specMD) {
				rec.record("C3", "MUST", "fail", "tier_artifacts_present", "lite: one-page spec.md missing")
			} else if mACCount < 1 {
				rec.record("C3", "MUST", "fail", "tier_artifacts_present", "lite: acceptance_checks is empty")
			} else {
				rec.record("C3", "MUST", "ok", "tier_artifacts_present", "")
			}
		case "full":
			missing := ""
			if !fileExists(specMD) {
				missing += "spec.md "
			}
			if !fileExists(specYAML) {
				missing += "spec.yaml "
			}
			if missing != "" {
				rec.record("C3", "MUST", "fail", "tier_artifacts_present", "full: missing "+trimTrailingSpace(missing))
			} else {
				rec.record("C3", "MUST", "ok", "tier_artifacts_present", "")
			}
		default:
			// tier already flagged illegal by C2b — record nothing (matches bash `*)`).
		}
	}

	// -- Check 4: maker != checker when status in {verified, archived} ------- //
	if changeOK {
		if mStatus == "verified" || mStatus == "archived" {
			if mMaker == mChecker {
				rec.record("C4", "MUST", "fail", "maker_distinct_from_checker",
					fmt.Sprintf("maker == checker ('%s') at status=%s", mMaker, mStatus))
			} else {
				verifyAuthor := ""
				vePath := filepath.Join(targetAbs, "verify.envelope.json")
				if fileExists(vePath) {
					if vd, err := os.ReadFile(vePath); err == nil {
						var vm map[string]any
						if validJSONObject(vd, &vm) {
							verifyAuthor = nestedFromEidolon(vm)
						}
					}
				}
				if verifyAuthor != "" && verifyAuthor == mMaker {
					rec.record("C4", "MUST", "fail", "maker_distinct_from_checker",
						fmt.Sprintf("verify envelope from.eidolon == maker ('%s')", mMaker))
				} else {
					rec.record("C4", "MUST", "ok", "maker_distinct_from_checker", "")
				}
			}
		}
	}

	// -- Check 5: drift_checked == true before archived --------------------- //
	if changeOK {
		if mStatus == "archived" {
			if mDrift == "true" {
				rec.record("C5", "MUST", "ok", "drift_checked_before_archive", "")
			} else {
				got := mDrift
				if got == "" {
					got = "unset"
				}
				rec.record("C5", "MUST", "fail", "drift_checked_before_archive",
					fmt.Sprintf("status=archived requires drift_checked=true (got '%s')", got))
			}
		}
	}

	// -- Check 6: ECL envelope sidecars well-formed + performative in 10-set - //
	for _, envf := range envelopeFiles(targetAbs) {
		base := filepath.Base(envf)
		ed, err := os.ReadFile(envf)
		var em map[string]any
		if err != nil || !validJSONObject(ed, &em) {
			rec.record("C6", "MUST", "fail", "envelope_well_formed", base+": jq parse failed")
			continue
		}
		perf := jget(em, "performative")
		if inSet(perf, eclPerformatives) {
			rec.record("C6", "MUST", "ok", "envelope_performative_in_ecl_set", base)
		} else {
			rec.record("C6", "MUST", "fail", "envelope_performative_in_ecl_set",
				fmt.Sprintf("%s: performative '%s' not in ECL closed-10 set", base, perf))
		}
	}

	// -- Check 7: EARS-structured acceptance_checks complete (SHOULD) -------- //
	//
	// C7 is ADVISORY (SHOULD-level), a faithful port of esl-conformance.sh's C7.
	// An acceptance_checks item is in the EARS form iff it is an object that
	// declares at least one EARS-specific key (given|when|then). For each EARS
	// item, warn if any of given|when|then|verify_method is absent or not a
	// non-empty string. A C7 fail NEVER changes the exit code (only MUST checks
	// C1–C6 can block) — see the BLOCKING_FAIL split below.
	if changeOK && mACCount > 0 {
		items, _ := m["acceptance_checks"].([]any)
		for i := 0; i < mACCount; i++ {
			it, isObj := items[i].(map[string]any)
			if !isObj {
				continue // plain string (or other non-object) → no C7 finding
			}
			if !earsForm(it) {
				continue // legacy {id, verify_method} object → no C7 finding
			}
			acID := jget(it, "id")
			if acID == "" {
				acID = fmt.Sprintf("#%d", i)
			}
			missing := earsMissing(it)
			if missing != "" {
				rec.record("C7", "SHOULD", "fail", "ears_acceptance_complete",
					fmt.Sprintf("%s: EARS item missing/empty: %s", acID, missing))
			} else {
				rec.record("C7", "SHOULD", "ok", "ears_acceptance_complete", acID)
			}
		}
	}

	// -- Check 8: fresh-context verification attestation (SHOULD, NEW in v1.1) //
	//
	// C8 extends C4 from identity-inequality to context-separation (ESL v1.1
	// §5.4), a faithful port of esl-conformance.sh's C8. It ONLY evaluates when
	// status is verified/archived AND a verify.envelope.json sidecar is present
	// in the change folder -- no envelope means no attestation to check yet, so
	// C8 produces NO record at all (skip, not fail). A malformed envelope is
	// already reported by C6; C8 also produces no record in that case (nothing
	// reliable to read). When the envelope exists and parses, it MAY carry an
	// `ise.verification` sub-block {fresh_context, checker, transcript_access}
	// (a forward reference to an anticipated ECL extension -- see spec §5.4's
	// caveat). C8 warns if the sub-block is absent, or if fresh_context != true,
	// transcript_access is not one of {none, artifact-only}, or the sub-block's
	// checker == change.json.maker. A C8 fail NEVER changes the exit code (only
	// C1-C6 MUST checks can block) -- see the blockingFail split below.
	if changeOK && (mStatus == "verified" || mStatus == "archived") {
		vePath := filepath.Join(targetAbs, "verify.envelope.json")
		if fileExists(vePath) {
			if vd, err := os.ReadFile(vePath); err == nil {
				var vm map[string]any
				if validJSONObject(vd, &vm) {
					iseObj, isePresent := iseVerification(vm)
					if !isePresent {
						rec.record("C8", "SHOULD", "fail", "fresh_context_verification_attested",
							"ise.verification sub-block absent (advisory; SHOULD be present at verified/archived)")
					} else {
						fresh := ""
						if v, ok := iseObj["fresh_context"]; ok {
							fresh = boolTriState(v)
						}
						checker := jget(iseObj, "checker")
						transcript := jget(iseObj, "transcript_access")

						var issues []string
						if fresh != "true" {
							got := fresh
							if got == "" {
								got = "unset"
							}
							issues = append(issues, fmt.Sprintf("fresh_context!=true(got '%s')", got))
						}
						switch transcript {
						case "none", "artifact-only":
							// ok
						default:
							got := transcript
							if got == "" {
								got = "unset"
							}
							issues = append(issues, fmt.Sprintf("transcript_access invalid(got '%s')", got))
						}
						if checker == "" {
							issues = append(issues, "checker missing")
						} else if checker == mMaker {
							issues = append(issues, fmt.Sprintf("checker==maker('%s')", mMaker))
						}

						if len(issues) > 0 {
							rec.record("C8", "SHOULD", "fail", "fresh_context_verification_attested", strings.Join(issues, " "))
						} else {
							rec.record("C8", "SHOULD", "ok", "fresh_context_verification_attested", "")
						}
					}
				}
				// else: malformed envelope, already reported by C6; no C8 record.
			}
			// else: unreadable file; no C8 record (matches the bash `jq empty` failure path).
		}
	}

	// -- summarise ---------------------------------------------------------- //
	// hasFail reflects any fail (human "warnings present"); blockingFail is the
	// exit-code lever — ONLY MUST-level fails block. A SHOULD-level fail (C7/C8 —
	// advisory EARS lint / fresh-context attestation) never changes the exit code.
	hasFail := false
	blockingFail := false
	for _, r := range rec.results {
		if r.Status == "fail" {
			hasFail = true
			if r.Level == "MUST" {
				blockingFail = true
			}
		}
	}
	exit := 0
	if blockingFail && mode == ModeBlock {
		exit = 3
	}

	return Report{
		TargetBasename: filepath.Base(targetAbs),
		Mode:           string(mode),
		Results:        rec.results,
		ExitCode:       exit,
		HasFail:        hasFail,
	}
}

// envelopeFiles enumerates *.envelope.json files with the SAME glob scope as the
// bash checker: `find <target> -maxdepth 1 -type f -name '*.envelope.json' |
// LC_ALL=C sort` (esl-conformance.sh:244-246). maxdepth 1 + a byte-wise (C-locale)
// sort — a depth or ordering mismatch is a silent parity break.
func envelopeFiles(targetAbs string) []string {
	entries, err := os.ReadDir(targetAbs)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) >= len(".envelope.json") && hasSuffix(name, ".envelope.json") {
			// Resolve to a regular file (not a symlink-to-dir etc.). os.ReadDir
			// gives DirEntry; -type f means regular file.
			info, ierr := e.Info()
			if ierr != nil || !info.Mode().IsRegular() {
				continue
			}
			out = append(out, filepath.Join(targetAbs, name))
		}
	}
	// LC_ALL=C sort == byte-wise sort of the FULL paths. Since the directory
	// prefix is identical for all entries, byte-sorting the joined paths equals
	// byte-sorting the basenames.
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// driftTriState replicates the bash:
//
//	jq -r 'if .drift_checked == true then "true" elif .drift_checked == false then "false" else "" end'
func driftTriState(m map[string]any) string {
	v, ok := m["drift_checked"]
	if !ok {
		return ""
	}
	return boolTriState(v)
}

// boolTriState replicates the bash tri-state pattern used for both
// drift_checked and ise.verification.fresh_context:
//
//	jq -r 'if .x == true then "true" elif .x == false then "false" else "" end'
//
// i.e. JSON true -> "true", JSON false -> "false", anything else (missing,
// null, non-bool) -> "".
func boolTriState(v any) string {
	b, isBool := v.(bool)
	if !isBool {
		return ""
	}
	if b {
		return "true"
	}
	return "false"
}

// iseVerification replicates the bash jq filter
// `.ise.verification? != null` used to gate C8: it returns the
// ise.verification sub-object and true iff .ise is an object AND its
// "verification" key exists and is not JSON null. If "verification" is
// present but is not itself a JSON object (an edge case no known fixture
// exercises), the second return is still true (mirroring `!= null`) but the
// returned map is nil, so subsequent jget/boolTriState reads on it come back
// empty/"" — matching jq's behavior of a failed (and suppressed) index
// expression yielding an empty capture.
func iseVerification(vm map[string]any) (map[string]any, bool) {
	ise, ok := vm["ise"].(map[string]any)
	if !ok {
		return nil, false
	}
	v, exists := ise["verification"]
	if !exists || v == nil {
		return nil, false
	}
	obj, _ := v.(map[string]any)
	return obj, true
}

// acceptanceCount replicates:
//
//	jq -r 'if (.acceptance_checks | type) == "array" then (.acceptance_checks | length) else 0 end'
func acceptanceCount(m map[string]any) int {
	v, ok := m["acceptance_checks"]
	if !ok {
		return 0
	}
	arr, isArr := v.([]any)
	if !isArr {
		return 0
	}
	return len(arr)
}

// earsForm reports whether an acceptance_checks object is in the EARS form,
// replicating the bash jq predicate:
//
//	($it | type) == "object" and (has("given") or has("when") or has("then"))
//
// The caller has already asserted $it is an object; here we test the key set.
func earsForm(it map[string]any) bool {
	if _, ok := it["given"]; ok {
		return true
	}
	if _, ok := it["when"]; ok {
		return true
	}
	if _, ok := it["then"]; ok {
		return true
	}
	return false
}

// earsMissing returns the comma-joined list of EARS fields (in the fixed order
// given,when,then,verify_method) that are absent or NOT a non-empty string,
// replicating the bash jq:
//
//	["given","when","then","verify_method"]
//	| map(select((($it[.]?) | (type == "string" and length > 0)) | not))
//	| join(",")
func earsMissing(it map[string]any) string {
	var miss []string
	for _, f := range []string{"given", "when", "then", "verify_method"} {
		v, ok := it[f]
		if !ok {
			miss = append(miss, f)
			continue
		}
		s, isStr := v.(string)
		if !isStr || len(s) == 0 {
			miss = append(miss, f)
		}
	}
	return joinComma(miss)
}

// joinComma joins with a literal comma (no spaces), matching jq's join(",").
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// validJSONObject parses data as JSON. The bash checker uses `jq empty`, which
// accepts ANY valid JSON (object, array, scalar). To then read top-level fields
// it uses `jq -r '.status'`, which on a non-object input yields an error -> empty
// via `// empty` / `2>/dev/null`. We model that: if data is valid JSON but not an
// object, we still treat C1 as ok and leave m as an empty map so field reads
// return "". out receives the decoded object (or stays empty for non-objects).
func validJSONObject(data []byte, out *map[string]any) bool {
	// First: is it valid JSON at all (jq empty)?
	var any2 any
	if err := json.Unmarshal(data, &any2); err != nil {
		return false
	}
	if obj, ok := any2.(map[string]any); ok {
		*out = obj
	} else {
		*out = map[string]any{}
	}
	return true
}

func inSet(s string, set []string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}
	return false
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func trimTrailingSpace(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
