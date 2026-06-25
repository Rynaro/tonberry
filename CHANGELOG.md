# Changelog

All notable changes to **tonberry** are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/); this project uses SemVer.

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
