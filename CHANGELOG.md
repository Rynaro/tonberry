# Changelog

All notable changes to **tonberry** are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); this project uses SemVer.

## [Unreleased]

### Fixed
- `mcpserver.New` doc comment claimed "all 8 tools registered" while 11 are;
  the comment is now count-free and the registered tool surface is pinned to
  `ToolNames` by a new in-memory MCP session test
  (`internal/mcpserver/mcpserver_test.go`), so the count can never silently
  drift again.

## [0.4.0] — 2026-06-25

Three evidence-backed lifecycle-op UX fixes surfaced by the ESL dogfood. The
`verify` parity surface is **FROZEN** — `internal/conformance` (C1–C6 + C7),
`parity/esl-conformance.sh`, and `fixtures/` are untouched and stay
byte-identical to the vendored bash oracle (the drift-guard re-proves it). These
fixes are lifecycle-OP behavior + manifest schema only. There are no existing
consumers, so the default flips are safe.

### Changed

- **`has_code` is read from the manifest (FIX 1).** `transition` previously
  defaulted `--has_code=false`, silently rejecting a code change out of
  `in_progress` unless the flag was re-passed every call. Now: `propose` and
  `compose_manifest` accept `has_code` and persist it into `change.json` (the new
  OPTIONAL `has_code` boolean from `eidolons-esl` `change.v1.json` §3.2);
  `transition` READS `has_code` from the manifest, and an explicit per-transition
  `--has_code` OVERRIDES the manifest value (back-compat). `internal/manifest.Change`
  gained `HasCode *bool` + `HasCodeTrue()`.
- **Persist-by-default for the lifecycle-advancing ops (FIX 2).** `transition`,
  `right_size`, and `drift_check` now **WRITE the manifest by default** when the
  action is allowed/valid (previously they defaulted `write_manifest=false` —
  "evaluate-only"). A new `--dry-run` flag (and the equivalent MCP `dry_run` arg)
  restores evaluate-only. `--write_manifest` still works (now `*bool`, nil =
  default); an explicit `--write_manifest=false` is honored. `right_size` with no
  `change_id` stays pure classification (no write, no error). Each output gained a
  `persisted` boolean.
- **`archive` MOVES the change folder (FIX 3).** `archive` now `os.Rename`s the
  change folder into `archive/<date>-<change_id>/` (with a copy+remove fallback for
  a cross-device move) instead of copying and leaving a stale original — the active
  `.spectra/changes/<change_id>/` no longer exists afterward (ESL §9.2 "a move").
  `status=archived` + `archive_path` are set on the moved manifest; the
  promotion-intent envelope is composed in the moved folder.
- **Counts survive the move (FIX 3).** `assess`'s `change_count` (and the
  `full_ratio` numerator/denominator) now count BOTH active (`.spectra/changes/*`)
  AND archived (`.spectra/changes/archive/*`) changes, so archiving never drops the
  escalation signal. `list` shows ACTIVE changes by default and gained `--all`
  (alias `--include-archived`) to include archived snapshots; each row carries an
  `archived` boolean.
- **Version bump to 0.4.0** (`cmd/tonberry` CLI `version`, `internal/mcpserver`
  MCP-reported version). MCP tool descriptions + README + CLI `--help` document the
  persist-by-default flip, the `has_code`-from-manifest read, the archive MOVE, and
  the `list --all` flag.

### Tests

- `internal/manifest` — `has_code` round-trips (absent reads false).
- `internal/ops` — has_code read-from-manifest (+ control: no hint blocks
  `in_progress`); the explicit-flag override in both directions; persist-by-default
  for transition/right_size/drift_check (default persists, `--dry-run` does not,
  explicit `write_manifest=false` does not); `assess` counts the archived change;
  `list` active-default vs `--all`.
- `internal/archive` — archive MOVES (active folder gone, content + manifest under
  `archive/`, `status=archived` on the moved manifest).

### Unchanged (parity surface frozen)

- `internal/conformance` (C1–C6 + C7), `parity/esl-conformance.sh`, and
  `fixtures/` are untouched; the byte-identical parity against the vendored bash
  oracle (both directions) is re-proven green (`TONBERRY_REQUIRE_PARITY=1`), and
  the vendored-oracle drift-guard stays identical.

[0.4.0]: https://github.com/Rynaro/tonberry/releases/tag/v0.4.0

## [0.3.1] — 2026-06-25

Hardening + hygiene release. No change to the `verify` 6-check semantics or the
parity surface (`internal/conformance` + `parity/esl-conformance.sh` stay
byte-identical to the canonical bash oracle). CI, release-metadata, and CLI
arg-parsing only.

### Added

- **Release-manifest index digest** — `release.yml` now captures the multi-arch
  **manifest-list (index) digest** and stamps it into `dist/release-manifest.json`
  as `manifest_sha256` (previously hard-coded `null`). The merge job already emits
  the index digest as a job output; a new `Resolve multi-arch index digest` step in
  the `github-release` job consumes it. Works for BOTH the tag-triggered build path
  AND the `skip_image` re-attach path (re-attach queries the **already-published**
  image's index digest via `docker buildx imagetools inspect` rather than a freshly
  built one). errexit-safe; `packages: read` added to the release job for the query.
- **Vendored-oracle drift guard** (`conformance.yml`) — mechanizes the ESL §9.3
  reversal condition. A new CI step fetches the canonical
  `eidolons-esl@main conformance/esl-conformance.sh` and compares its **body**
  (comment/header lines stripped) against the vendored `parity/esl-conformance.sh`.
  A confirmed body divergence **FAILS** the job (vendored oracle is stale →
  re-vendor); a fetch failure emits a **loud WARN** and does not hard-fail (so a
  transient network flake never reds the build). A weekly `schedule:` cron catches
  upstream checker revisions even when tonberry isn't being pushed.
- **gofmt gate** (`conformance.yml`) — a CI step fails the job if `gofmt -l .`
  lists any file, so the tree stays formatted. errexit-safe.

### Changed

- **gofmt-clean the tree** — `internal/ops/ops.go`, `internal/archive/archive.go`,
  and `internal/rightsizing/rightsizing_test.go` were re-formatted (struct field
  alignment only; no behavior change) so the new gofmt gate passes.
- **Consistent `list`/`status`/`assess` CLI args** — all three one-shot CLI ops
  now share one convention: a **positional changes-dir path** AND the
  `--changes_dir` flag are accepted uniformly (default `.spectra/changes`,
  relative to `--project_root`); `--changes_dir` wins if both are given. Previously
  `list <dir>` ignored the positional and `assess` only honored `--changes_dir`.
  README usage examples updated. The MCP tool input schemas are **unchanged** —
  only the CLI arg parsing in `cmd/tonberry/main.go`.
- **Version bump to 0.3.1** in `cmd/tonberry` (CLI `version`) and
  `internal/mcpserver` (MCP-reported version), which were both still `0.2.0`.

### Unchanged (parity surface frozen)

- `internal/conformance` (the 6 checks C1–C6 + C7), `parity/esl-conformance.sh`,
  and `fixtures/` are untouched; the parity invariant (byte-identical to the
  vendored bash oracle, both directions) is re-proven green this phase.

[0.3.1]: https://github.com/Rynaro/tonberry/releases/tag/v0.3.1

## [0.3.0] — 2026-06-25

EARS-structured acceptance checks + the advisory **C7** lint, re-vendored and
re-proven byte-identical against the bash oracle. The parity surface CHANGES this
phase (C7 is added to both implementations); the load-bearing parity invariant
(FORGE Decision 2) is re-proven with the new EARS fixtures in both directions.
Tool surface stays at **11 tools** (no new op — C7 rides `verify`).

### Added

- **Optional EARS acceptance form** — `acceptance_checks[]` items are now
  `oneOf:[string, object]` (ESL §2.5). An item MAY be a plain string OR a
  structured object; a structured object MAY adopt the EARS form
  `{id, given, when, then, verify_method}`. `internal/manifest.AcceptanceCheck`
  gained custom `MarshalJSON`/`UnmarshalJSON` so all three forms (plain-string,
  minimal `{id, verify_method}`, full EARS) round-trip; `Validate` accepts the
  plain-string form (no `id`); new `AcceptanceCheck.IsEARS()` predicate.
- **Check C7 (SHOULD, advisory)** in `internal/conformance` — a faithful Go port
  of the bash oracle's C7. For any EARS-form acceptance item (an object declaring
  ≥1 of `given`/`when`/`then`), it warns if any of `given`/`when`/`then`/
  `verify_method` is missing or empty. **C7 NEVER changes the exit code** — a
  C7-only failure stays exit 0 even under `--mode block` (only the MUST checks
  C1–C6 block). Plain-string and minimal-object items produce no C7 finding.
- **Parity corpus** — new EARS fixtures under `fixtures/`: `conformant/ears-complete`
  and `conformant/lite-ears-complete` (C7 `ok`), and `failing/ears-missing-field`
  (C7 `fail` + **exit 0** in block — the advisory proof). The parity test now
  covers C7 in BOTH directions, byte-identical to the bash oracle.

### Changed

- **Re-vendored the oracle** — `parity/esl-conformance.sh` re-synced from the
  UPDATED `eidolons-esl/conformance/esl-conformance.sh` (the deliberate, controlled
  reversal-condition re-sync per ESL §9.3). The canonical-source header note is
  preserved.
- `verify`'s check family is now **C1–C6 (MUST) + C7 (SHOULD)**; the README +
  fixtures README document the EARS form and the advisory exit-code contract. The
  byte-identical parity against the vendored bash oracle still holds (incl. C7).

## [0.2.0] — 2026-06-25

Project-scope lifecycle observability + the escalation assessment. Three new
read-only ops bring the tool surface to **11 tools**. The `verify` parity surface
(the six checks C1–C6) is **UNCHANGED** — `internal/conformance`,
`parity/esl-conformance.sh`, and the fixtures are frozen this phase; the
byte-identical parity against the vendored bash oracle still holds.

### Added

- **`list`** — enumerate change folders under `.spectra/changes/` (default;
  `--changes_dir` / `--project_root` overridable). Returns
  `[{change_id,status,tier,drift_checked}]` read straight from each `change.json`,
  **sorted by `change_id`** (deterministic). Skips the `archive/` snapshot subdir
  and any folder without a readable manifest; an absent changes directory lists
  zero changes (not an error).
- **`status`** — for one `change_id`: the manifest summary + the **`verify`
  verdict** (calls the EXISTING `verify` logic — the same six checks, not
  duplicated) + the legal next lifecycle transitions (from `internal/lifecycle`).
- **`assess`** — project-scope **escalation assessment** (FORGE Decision 3 /
  `eidolons-esl` `docs/escalation.md`). Aggregates the §4.2 right-sizing signal
  family to project scope — `change_count` (number of changes), `full_ratio`
  (`full`-tier / total), and `repo_loc` (a `--repo_loc` override, else a
  deterministic text-line walk skipping `.git`, vendor/build dirs, and obvious
  binaries) — and compares them to thresholds (`--n`/`--l`/`--r`; seed defaults
  `N=10` / `L=50000` / `R=0.4`, tunable). Returns
  `{signals, thresholds, tripped[], recommended_mode}`; `recommended_mode` is
  `block` if ANY threshold trips, else `advisory`. **Deterministic** (property-
  tested). tonberry ships the assessment + the lever; the *flip recording* is
  nexus-side (`eidolons.mcp.lock`) and deferred — tonberry never writes a lock.
- **`lifecycle.LegalNextStatuses`** — enumerates the legal forward/escalate edges
  from a status (reuses the `Transition` predicate; single-sourced, deterministic),
  backing the `status` op.

### Changed

- **11 tools** (`tools/list`): the v0.1 eight + `list`, `status`, `assess`. Wired
  into `internal/mcpserver` (manifest + dispatch), the one-shot CLI
  (`tonberry list|status|assess ...`), and `internal/ops`.
- README: **8 tools → 11 tools**; documents the three new ops + the escalation
  lever/assessment, linking the ESL `docs/escalation.md` concept.

### Unchanged (frozen this phase)

- The `verify` parity surface: `internal/conformance`, `parity/esl-conformance.sh`,
  and `fixtures/` are untouched; the six checks C1–C6 and the exit-code contract
  (0/1/3, 2 reserved) are byte-identical to the vendored bash oracle.

[0.2.0]: https://github.com/Rynaro/tonberry/releases/tag/v0.2.0

## [0.1.0] — 2026-06-25

First release. tonberry is the official, Eidolons-backed implementation of ESL
(Eidolons Spec Lifecycle), targeting `ESL_VERSION` `1.0`.

### Added

- **8 MCP tools** under the `mcp__tonberry__*` namespace (bare op names):
  `propose`, `right_size`, `transition`, `compose_manifest`, `compose_envelope`,
  `verify`, `drift_check`, `archive`. `maker_checker` is folded into `verify` as
  check C4.
- **`verify` — the parity surface.** A faithful Go port of the normative
  `esl-conformance.sh` (the six checks C1–C6, exit codes 0/1/3 with 2 reserved,
  `--mode warn|block`, `--json`→stdout / findings→stderr, the maxdepth-1
  `LC_ALL=C` envelope glob). Locked by a shared-fixture parity test against the
  vendored bash oracle (`parity/esl-conformance.sh`); the bash checker is
  authoritative on any divergence.
- **`right_size` — the deterministic ESL §4 gate.** Three mechanical signals
  → `trivial`/`lite`/`full` with the §4.3 precedence; pure arithmetic, no maps,
  no time — identical input always yields the identical tier (property-tested).
- **`transition` — the ESL §3 state machine** with skip-rules: lite/trivial skip
  `deliberated`; code-states require code; `archived` requires `verified` +
  `drift_checked==true`; the `verify_fail` ESCALATE return to `in_progress`.
- **`archive` — snapshot + promotion-intent** (FORGE Decision 4 / GAP-D): writes
  `archive/<date>-<change_id>/`, sets `archive_path`, enforces
  `drift_checked==true`, and composes an on-disk `INFORM(promotion)` ECL
  envelope. tonberry **never** imports or calls CRYSTALIUM — the parent/cortex
  routes the sidecar to `mcp__crystalium__ingest`.
- **Dual-mode single binary:** `serve` (stdio MCP via the official
  `modelcontextprotocol/go-sdk`), one-shot CLI per op, and a `verify` CI/standalone
  checker — all in one CGO-off static binary.
- **Distroless image:** `gcr.io/distroless/static-debian12:nonroot`, ~13 MB,
  multi-arch (amd64 + arm64) native-runner release matrix; Eidolon-release asset
  contract (`release-manifest.json` + `SHA256SUMS` + build-provenance attestation).
- **CI:** `go vet` + `go test` + the enforced parity gate + an anti-scope gate +
  the oracle tripwire, on ubuntu + macos.

### Anti-scope

The only ESL-owned schema is the `status`/`tier` enums. tonberry references
`spec_ref` (SPECTRA), names ECL performatives (the closed-10 set), and references
the CRYSTALIUM Semantic layer — it re-declares none of them.

[0.1.0]: https://github.com/Rynaro/tonberry/releases/tag/v0.1.0
