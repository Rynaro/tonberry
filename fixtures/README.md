# Parity corpus

The shared fixture set that locks the `tonberry verify` ↔ `esl-conformance.sh`
parity invariant (FORGE Decision 2). Both implementations run this corpus; the
parity test (`internal/conformance/parity_test.go`) asserts structural equality
of their `--json` summaries (the set of `{id,status}` + `exit_code`) for every
fixture. The vendored bash oracle is `../parity/esl-conformance.sh`; on any
divergence the bash checker is authoritative.

## `conformant/` — exit 0 even in `--mode block`

| Fixture | Source | Note |
|---|---|---|
| `trivial-typo-fix/` | eidolons-esl `examples/` | trivial tier, no spec, verified |
| `lite-add-flag/` | eidolons-esl `examples/` | lite tier, spec.md + acceptance_checks |
| `full-new-subsystem/` | eidolons-esl `examples/` | full tier, spec.{md,yaml}, archived + drift_checked |
| `trivial-no-spec/` | eidolons-esl `conformance/tests/` | trivial bypass: no spec is NOT a violation |
| `ears-complete/` | eidolons-esl `conformance/tests/` | EARS-complete acceptance item (all 4 fields) → C7 `ok` (advisory) |
| `lite-ears-complete/` | eidolons-esl `examples/` | worked EARS-form change; C7 `ok` |
| `fresh-context-attested/` | eidolons-esl `conformance/tests/` | verified, `verify.envelope.json` with `ise.verification{fresh_context:true, checker!=maker, transcript_access:artifact-only}` → C8 `ok` (advisory) |
| `fresh-context-no-envelope/` | eidolons-esl `conformance/tests/` | verified, no `verify.envelope.json` at all → C8 produces **no record** (skip, not fail) |
| `memory-preflight-recorded/` | eidolons-esl `conformance/tests/` | `memory_preflight:{ran:true,records:3}` — OPTIONAL v1.1 field, schema-valid, not a C1–C6 gate |
| `memory-preflight-skipped/` | eidolons-esl `conformance/tests/` | `memory_preflight:{ran:false,records:0}` — graceful-skip form, still schema-valid |

## `failing/` — hard violation, exit 3 in `--mode block` (EXCEPT the C7/C8 advisory cases)

| Fixture | Check | Source | Why |
|---|---|---|---|
| `maker-equals-checker/` | C4 | eidolons-esl | status=verified, maker==checker, verify env `from.eidolon`==maker |
| `archive-no-drift/` | C5 | eidolons-esl | status=archived with `drift_checked:false` |
| `full-missing-spec/` | C3 | eidolons-esl | tier=full, no `spec.{md,yaml}` |
| `lite-missing-spec/` | C3 | eidolons-esl | tier=lite, no one-page `spec.md` |
| `bad-json/` | C1 | tonberry | change.json is not valid JSON (missing comma) |
| `illegal-status/` | C2a | tonberry | `status: in-review` not in the ESL status enum |
| `illegal-tier/` | C2b | tonberry | `tier: medium` not in the ESL tier enum |
| `lite-empty-acceptance/` | C3 | tonberry | lite tier, spec.md present, `acceptance_checks: []` |
| `bad-performative/` | C6 | tonberry | sidecar performative `ACCEPT` not in the closed ECL ten-set |
| `malformed-envelope/` | C6 | tonberry | `*.envelope.json` sidecar is not valid JSON (the C6 well-formed branch) |
| `ears-missing-field/` | C7 | eidolons-esl `conformance/tests/` | EARS item missing `then` → C7 `fail` in `--json`, YET **exit 0** in block mode (C7 is SHOULD-level/advisory; only C1–C6 block — the load-bearing advisory proof) |
| `fresh-context-same-session/` | C8 | eidolons-esl `conformance/tests/` | manifest-level `maker!=checker` holds (C4 `ok`), but `ise.verification{fresh_context:false, checker==maker}` → C8 `fail` in `--json`, YET **exit 0** in block mode (C8 is SHOULD-level/advisory, same discipline as C7) |

The eidolons-esl fixtures are copied verbatim (canonical source =
`Rynaro/eidolons-esl`); re-sync them on an ESL checker/example revision — a
divergence is a release-blocking reversal condition (ESL §9.3).
