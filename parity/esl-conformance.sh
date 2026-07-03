#!/usr/bin/env bash
#
# VENDORED ORACLE — DO NOT EDIT HERE.
# Canonical source: Rynaro/eidolons-esl  conformance/esl-conformance.sh
# This is the normative parity oracle for `tonberry verify` (FORGE Decision 2):
# tonberry's internal/conformance is a faithful Go port, locked by
# internal/conformance/parity_test.go against this exact script. On any ESL
# checker revision, RE-SYNC this file from eidolons-esl and re-run the parity
# test; a divergence is a RELEASE-BLOCKING reversal condition (ESL §9.3). The
# bash checker is AUTHORITATIVE on any disagreement.
#
# ESL conformance checker.
#
# Usage:
#   bash esl-conformance.sh <change-folder> [options]
#
# <change-folder> is a directory containing a change.json manifest and,
# tier-dependent, a SPECTRA spec.{md,yaml} and ECL *.envelope.json sidecars.
#
# Options:
#   --mode warn     Report findings; exit 0 even on violations (default).
#   --mode block    Exit 3 on any hard violation.
#   --json          Emit a machine-readable JSON summary on stdout.
#   -h, --help      Print this help and exit 0.
#   --version       Print the checker version and exit 0.
#
# Exit codes (ESL v1.1 §8.3; unchanged since v1.0):
#   0  Conformant, or warnings-only under --mode warn.
#   1  Usage error (bad args, missing change folder).
#   3  Hard violation under --mode block.
#   (2 is RESERVED.)
#
# All human-readable findings go to stderr. The --json summary is the ONLY
# thing written to stdout, and is deterministic (sorted, no timestamps).
#
# Bash 3.2 compatible. No LLM, no network. Hard deps: jq, POSIX coreutils.

set -u
# We deliberately do not `set -e`; we collect every finding before deciding.

ESL_CHECKER_VERSION="1.1.0"

# The closed ECL v1.0 ten-performative set, vendored as a constant. This is a
# REFERENCE to ECL, NOT an ESL-owned enumeration: see schemas/performative.v1.json
# in Rynaro/eidolons-ecl (ECL v1.0 §2). ESL does not extend or redefine it.
ESL_ECL_PERFORMATIVES="REQUEST INFORM PROPOSE CRITIQUE DECIDE DELEGATE ACKNOWLEDGE ESCALATE RESUME REFUSE"

# -- option parsing --------------------------------------------------------- #

TARGET=""
MODE="warn"
OUTPUT="human"

print_help() {
  sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
}

usage_err() {
  # usage_err <message>
  echo "$1" >&2
  echo "Usage: bash $(basename "$0") <change-folder> [--mode warn|block] [--json]" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --mode)        [ $# -ge 2 ] || usage_err "Missing value for --mode"; MODE="$2"; shift 2 ;;
    --mode=*)      MODE="${1#--mode=}"; shift ;;
    --json)        OUTPUT="json"; shift ;;
    -h|--help)     print_help; exit 0 ;;
    --version)     echo "$ESL_CHECKER_VERSION"; exit 0 ;;
    --)            shift; break ;;
    -*)            usage_err "Unknown option: $1" ;;
    *)
      if [ -z "$TARGET" ]; then
        TARGET="$1"
      else
        usage_err "Unexpected extra argument: $1"
      fi
      shift
      ;;
  esac
done

case "$MODE" in
  warn|block) : ;;
  *)          usage_err "Invalid --mode: $MODE (expected warn or block)" ;;
esac

if [ -z "$TARGET" ]; then
  usage_err "Missing <change-folder>."
fi

if [ ! -d "$TARGET" ]; then
  usage_err "Change folder not found: $TARGET"
fi

if ! command -v jq >/dev/null 2>&1; then
  usage_err "Required dependency not found: jq"
fi

# Normalise to an absolute path.
TARGET_ABS="$(cd "$TARGET" && pwd)"
CHANGE_JSON="$TARGET_ABS/change.json"

# -- result accumulator ----------------------------------------------------- #

# Parallel arrays (bash 3.2 has no associative arrays); append-only.
N_RESULTS=0
RIDS=()
RLEVELS=()   # MUST | SHOULD
RSTATUSES=() # ok | fail
RNAMES=()
RREASONS=()

esl_record() {
  # esl_record <id> <level> <status> <name> <reason?>
  RIDS[$N_RESULTS]="$1"
  RLEVELS[$N_RESULTS]="$2"
  RSTATUSES[$N_RESULTS]="$3"
  RNAMES[$N_RESULTS]="$4"
  RREASONS[$N_RESULTS]="${5:-}"
  N_RESULTS=$((N_RESULTS + 1))
}

# -- check helpers ---------------------------------------------------------- #

jget() {
  # jget <jq-filter> <file>  -> raw value or empty string
  jq -r "$1 // empty" "$2" 2>/dev/null
}

# Read manifest fields once (empty string if unreadable / absent).
CHANGE_JSON_OK=0
M_STATUS=""
M_TIER=""
M_MAKER=""
M_CHECKER=""
M_DRIFT=""
M_AC_COUNT=0

# -- Check 1: change.json is valid JSON ------------------------------------- #

if [ ! -f "$CHANGE_JSON" ]; then
  esl_record "C1" "MUST" "fail" "change_json_present" "no change.json in folder"
elif ! jq empty "$CHANGE_JSON" >/dev/null 2>&1; then
  esl_record "C1" "MUST" "fail" "change_json_valid_json" "jq parse failed"
else
  esl_record "C1" "MUST" "ok" "change_json_valid_json" ""
  CHANGE_JSON_OK=1
  M_STATUS="$(jget '.status' "$CHANGE_JSON")"
  M_TIER="$(jget '.tier' "$CHANGE_JSON")"
  M_MAKER="$(jget '.maker' "$CHANGE_JSON")"
  M_CHECKER="$(jget '.checker' "$CHANGE_JSON")"
  M_DRIFT="$(jq -r 'if .drift_checked == true then "true" elif .drift_checked == false then "false" else "" end' "$CHANGE_JSON" 2>/dev/null)"
  M_AC_COUNT="$(jq -r 'if (.acceptance_checks | type) == "array" then (.acceptance_checks | length) else 0 end' "$CHANGE_JSON" 2>/dev/null)"
  if [ -z "$M_AC_COUNT" ]; then M_AC_COUNT=0; fi
fi

# -- Check 2: status / tier are legal enum values --------------------------- #

if [ "$CHANGE_JSON_OK" -eq 1 ]; then
  case "$M_STATUS" in
    proposed|deliberated|in_progress|verified|archived)
      esl_record "C2a" "MUST" "ok" "status_enum_legal" "" ;;
    *)
      esl_record "C2a" "MUST" "fail" "status_enum_legal" "illegal status: '$M_STATUS'" ;;
  esac
  case "$M_TIER" in
    trivial|lite|full)
      esl_record "C2b" "MUST" "ok" "tier_enum_legal" "" ;;
    *)
      esl_record "C2b" "MUST" "fail" "tier_enum_legal" "illegal tier: '$M_TIER'" ;;
  esac
fi

# -- Check 3: tier-appropriate artifacts present ---------------------------- #

if [ "$CHANGE_JSON_OK" -eq 1 ]; then
  SPEC_MD="$TARGET_ABS/spec.md"
  SPEC_YAML="$TARGET_ABS/spec.yaml"
  case "$M_TIER" in
    trivial)
      # No spec required. Absence of a spec file is NOT a violation.
      esl_record "C3" "MUST" "ok" "tier_artifacts_present" "trivial: no spec required" ;;
    lite)
      if [ ! -f "$SPEC_MD" ]; then
        esl_record "C3" "MUST" "fail" "tier_artifacts_present" "lite: one-page spec.md missing"
      elif [ "$M_AC_COUNT" -lt 1 ]; then
        esl_record "C3" "MUST" "fail" "tier_artifacts_present" "lite: acceptance_checks is empty"
      else
        esl_record "C3" "MUST" "ok" "tier_artifacts_present" ""
      fi ;;
    full)
      MISSING=""
      [ -f "$SPEC_MD" ]   || MISSING="${MISSING}spec.md "
      [ -f "$SPEC_YAML" ] || MISSING="${MISSING}spec.yaml "
      if [ -n "$MISSING" ]; then
        esl_record "C3" "MUST" "fail" "tier_artifacts_present" "full: missing ${MISSING% }"
      else
        esl_record "C3" "MUST" "ok" "tier_artifacts_present" ""
      fi ;;
    *)
      : # tier already flagged illegal by C2b
      ;;
  esac
fi

# -- Check 4: maker != checker when status in {verified, archived} ---------- #

if [ "$CHANGE_JSON_OK" -eq 1 ]; then
  case "$M_STATUS" in
    verified|archived)
      # Manifest-level inequality.
      if [ "$M_MAKER" = "$M_CHECKER" ]; then
        esl_record "C4" "MUST" "fail" "maker_distinct_from_checker" \
          "maker == checker ('$M_MAKER') at status=$M_STATUS"
      else
        # Cross-check the verify envelope author, if a verify envelope exists.
        VERIFY_AUTHOR=""
        if [ -f "$TARGET_ABS/verify.envelope.json" ]; then
          VERIFY_AUTHOR="$(jget '.from.eidolon' "$TARGET_ABS/verify.envelope.json")"
        fi
        if [ -n "$VERIFY_AUTHOR" ] && [ "$VERIFY_AUTHOR" = "$M_MAKER" ]; then
          esl_record "C4" "MUST" "fail" "maker_distinct_from_checker" \
            "verify envelope from.eidolon == maker ('$M_MAKER')"
        else
          esl_record "C4" "MUST" "ok" "maker_distinct_from_checker" ""
        fi
      fi ;;
    *)
      : # maker/checker split not yet required pre-verification
      ;;
  esac
fi

# -- Check 5: drift_checked == true before archived ------------------------- #

if [ "$CHANGE_JSON_OK" -eq 1 ]; then
  if [ "$M_STATUS" = "archived" ]; then
    if [ "$M_DRIFT" = "true" ]; then
      esl_record "C5" "MUST" "ok" "drift_checked_before_archive" ""
    else
      esl_record "C5" "MUST" "fail" "drift_checked_before_archive" \
        "status=archived requires drift_checked=true (got '${M_DRIFT:-unset}')"
    fi
  fi
fi

# -- Check 6: ECL envelope sidecars are well-formed + performative in 10-set  #

# Collect *.envelope.json via a while-read loop (bash 3.2 has no array-fill builtin).
ENVELOPES=()
while IFS= read -r f; do
  [ -n "$f" ] && ENVELOPES[${#ENVELOPES[@]}]="$f"
done < <(find "$TARGET_ABS" -maxdepth 1 -type f -name '*.envelope.json' | LC_ALL=C sort)

ei=0
while [ "$ei" -lt "${#ENVELOPES[@]}" ]; do
  envf="${ENVELOPES[$ei]}"
  base="$(basename "$envf")"
  if ! jq empty "$envf" >/dev/null 2>&1; then
    esl_record "C6" "MUST" "fail" "envelope_well_formed" "$base: jq parse failed"
  else
    perf="$(jget '.performative' "$envf")"
    found=0
    for p in $ESL_ECL_PERFORMATIVES; do
      if [ "$perf" = "$p" ]; then found=1; break; fi
    done
    if [ "$found" -eq 1 ]; then
      esl_record "C6" "MUST" "ok" "envelope_performative_in_ecl_set" "$base"
    else
      esl_record "C6" "MUST" "fail" "envelope_performative_in_ecl_set" \
        "$base: performative '$perf' not in ECL closed-10 set"
    fi
  fi
  ei=$((ei + 1))
done

# -- Check 7: EARS-structured acceptance_checks are complete (SHOULD) -------- #
#
# C7 is ADVISORY (SHOULD-level). An acceptance_checks item MAY be EITHER a plain
# string (legacy/minimal — no C7 finding) OR a structured object. An item is in
# the EARS form iff it is an object that declares at least one EARS-specific key
# (given|when|then). For each EARS item, warn if any of given|when|then|
# verify_method is absent or not a non-empty string. A C7 fail NEVER changes the
# exit code (only the MUST checks C1–C6 can block) — EARS is opt-in polish.
if [ "$CHANGE_JSON_OK" -eq 1 ] && [ "$M_AC_COUNT" -gt 0 ]; then
  aci=0
  while [ "$aci" -lt "$M_AC_COUNT" ]; do
    # Is item aci an EARS-structured object (declares given|when|then)?
    is_ears="$(jq -r --argjson i "$aci" '
      (.acceptance_checks[$i]) as $it
      | if ($it | type) == "object"
          and (($it | has("given")) or ($it | has("when")) or ($it | has("then")))
        then "1" else "0" end
    ' "$CHANGE_JSON" 2>/dev/null)"
    if [ "$is_ears" = "1" ]; then
      # Identify the item (its id, if any) for a readable reason.
      ac_id="$(jq -r --argjson i "$aci" '.acceptance_checks[$i].id // empty' "$CHANGE_JSON" 2>/dev/null)"
      [ -n "$ac_id" ] || ac_id="#$aci"
      # Which of the four EARS fields are missing or not a non-empty string?
      missing="$(jq -r --argjson i "$aci" '
        (.acceptance_checks[$i]) as $it
        | ["given","when","then","verify_method"]
        | map(select((($it[.]? ) | (type == "string" and length > 0)) | not))
        | join(",")
      ' "$CHANGE_JSON" 2>/dev/null)"
      if [ -n "$missing" ]; then
        esl_record "C7" "SHOULD" "fail" "ears_acceptance_complete" \
          "$ac_id: EARS item missing/empty: $missing"
      else
        esl_record "C7" "SHOULD" "ok" "ears_acceptance_complete" "$ac_id"
      fi
    fi
    aci=$((aci + 1))
  done
fi

# -- Check 8: fresh-context verification attestation (SHOULD, NEW in v1.1) -- #
#
# C8 extends C4 from identity-inequality to context-separation (ESL v1.1 §5.4).
# It ONLY evaluates when status is verified/archived AND a verify.envelope.json
# sidecar is present in the change folder -- no envelope means no attestation
# to check yet, so C8 produces NO record at all (skip, not fail). When the
# envelope exists, it MAY carry an `ise.verification` sub-block
# {fresh_context, checker, transcript_access} (a forward reference to an
# anticipated ECL extension -- see spec §5.4's caveat). C8 warns if the
# sub-block is absent, or if fresh_context != true, transcript_access is not
# one of {none, artifact-only}, or the sub-block's checker == change.json.maker.
# A C8 fail NEVER changes the exit code (only C1-C6 MUST checks can block).
VERIFY_ENVELOPE="$TARGET_ABS/verify.envelope.json"
if [ "$CHANGE_JSON_OK" -eq 1 ] && [ -f "$VERIFY_ENVELOPE" ]; then
  case "$M_STATUS" in
    verified|archived)
      if ! jq empty "$VERIFY_ENVELOPE" >/dev/null 2>&1; then
        : # malformed envelope already reported by C6; nothing reliable to read
      else
        ISE_PRESENT="$(jq -r 'if (.ise.verification? != null) then "1" else "0" end' "$VERIFY_ENVELOPE" 2>/dev/null)"
        if [ "$ISE_PRESENT" != "1" ]; then
          esl_record "C8" "SHOULD" "fail" "fresh_context_verification_attested" \
            "ise.verification sub-block absent (advisory; SHOULD be present at verified/archived)"
        else
          ISE_FRESH="$(jq -r 'if .ise.verification.fresh_context == true then "true" elif .ise.verification.fresh_context == false then "false" else "" end' "$VERIFY_ENVELOPE" 2>/dev/null)"
          ISE_CHECKER="$(jget '.ise.verification.checker' "$VERIFY_ENVELOPE")"
          ISE_TRANSCRIPT="$(jget '.ise.verification.transcript_access' "$VERIFY_ENVELOPE")"

          C8_ISSUES=""
          if [ "$ISE_FRESH" != "true" ]; then
            C8_ISSUES="${C8_ISSUES}fresh_context!=true(got '${ISE_FRESH:-unset}') "
          fi
          case "$ISE_TRANSCRIPT" in
            none|artifact-only) : ;;
            *) C8_ISSUES="${C8_ISSUES}transcript_access invalid(got '${ISE_TRANSCRIPT:-unset}') " ;;
          esac
          if [ -z "$ISE_CHECKER" ]; then
            C8_ISSUES="${C8_ISSUES}checker missing "
          elif [ "$ISE_CHECKER" = "$M_MAKER" ]; then
            C8_ISSUES="${C8_ISSUES}checker==maker('$M_MAKER') "
          fi

          if [ -n "$C8_ISSUES" ]; then
            esl_record "C8" "SHOULD" "fail" "fresh_context_verification_attested" "${C8_ISSUES% }"
          else
            esl_record "C8" "SHOULD" "ok" "fresh_context_verification_attested" ""
          fi
        fi
      fi ;;
    *)
      : # C8 only applies once verification is claimed (status verified/archived)
      ;;
  esac
fi

# -- summarise -------------------------------------------------------------- #

# HAS_FAIL reflects any fail (used only for the human "warnings present" line).
# BLOCKING_FAIL is the exit-code lever: ONLY MUST-level fails can block. A
# SHOULD-level fail (C7 — advisory EARS lint) never changes the exit code.
HAS_FAIL=0
BLOCKING_FAIL=0
ri=0
while [ "$ri" -lt "$N_RESULTS" ]; do
  if [ "${RSTATUSES[$ri]}" = "fail" ]; then
    HAS_FAIL=1
    if [ "${RLEVELS[$ri]}" = "MUST" ]; then BLOCKING_FAIL=1; fi
  fi
  ri=$((ri + 1))
done

EXIT_CODE=0
if [ "$BLOCKING_FAIL" -eq 1 ] && [ "$MODE" = "block" ]; then
  EXIT_CODE=3
fi

# -- emit ------------------------------------------------------------------- #

# Human findings -> stderr (deterministic order: the order checks were run).
{
  echo "ESL conformance check"
  echo "Target: $TARGET_ABS"
  echo "Mode:   $MODE"
  echo "----"
  ri=0
  while [ "$ri" -lt "$N_RESULTS" ]; do
    st="${RSTATUSES[$ri]}"
    case "$st" in
      ok)   tag="[OK]  " ;;
      fail) tag="[FAIL]" ;;
      *)    tag="[?]   " ;;
    esac
    line="${tag} ${RIDS[$ri]} ${RLEVELS[$ri]} ${RNAMES[$ri]}"
    if [ -n "${RREASONS[$ri]}" ]; then line="${line} (${RREASONS[$ri]})"; fi
    echo "$line"
    ri=$((ri + 1))
  done
  echo "----"
  case "$EXIT_CODE" in
    0) if [ "$HAS_FAIL" -eq 1 ]; then
         echo "Result: WARN — violations present, exit 0 (--mode warn)"
       else
         echo "Result: OK (exit 0)"
       fi ;;
    3) echo "Result: BLOCK — hard violation (exit 3)" ;;
  esac
} >&2

# Minimal deterministic JSON escaper.
json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/	/\\t/g'
}

if [ "$OUTPUT" = "json" ]; then
  printf '{'
  printf '"target_basename":"%s",' "$(json_escape "$(basename "$TARGET_ABS")")"
  printf '"mode":"%s",' "$(json_escape "$MODE")"
  printf '"results":['
  ri=0
  first=1
  while [ "$ri" -lt "$N_RESULTS" ]; do
    if [ "$first" -eq 0 ]; then printf ','; fi
    first=0
    printf '{"id":"%s","level":"%s","status":"%s","name":"%s"' \
      "$(json_escape "${RIDS[$ri]}")" "$(json_escape "${RLEVELS[$ri]}")" \
      "$(json_escape "${RSTATUSES[$ri]}")" "$(json_escape "${RNAMES[$ri]}")"
    if [ -n "${RREASONS[$ri]}" ]; then
      printf ',"reason":"%s"' "$(json_escape "${RREASONS[$ri]}")"
    fi
    printf '}'
    ri=$((ri + 1))
  done
  printf '],"exit_code":%d}\n' "$EXIT_CODE"
fi

exit "$EXIT_CODE"
