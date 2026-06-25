# tonberry

**The official, Eidolons-backed implementation of ESL** (the [Eidolons Spec
Lifecycle](https://github.com/Rynaro/eidolons-esl)). A thin, single-binary Go MCP
that **composes** ESL change folders and **enforces** the lifecycle — the
programmatic sibling of the ESL spec + its bash conformance checker.

`ESL_VERSION` = `1.0` · sibling of ECL / EIIS · opt-in.

---

## What tonberry is

ESL is a thin coordination grammar over the Eidolons: a `change.json` manifest, a
five-state lifecycle, a mechanical right-sizing gate, a maker≠checker rule, a
drift gate, and a standalone **bash** conformance checker. tonberry is the
in-process, fast path that the Eidolons call directly — it scaffolds manifests,
runs the right-sizing gate, advances the state machine, composes ECL sidecars,
archives, and runs the same six conformance checks the bash checker runs.

It is **thin by design**: a ~13 MB distroless image, no ML deps, no network, no
state beyond the change folders it reads and writes under `.spectra/changes/`.

### Anti-scope (load-bearing)

ESL owns exactly four things — the `status`/`tier` enums, the `change_id`/
supersede grammar, the right-sizing gate, and the drift-check transition. tonberry
**REFERENCES** everything else by version and **NEVER re-declares** it:

- the SPECTRA spec artifact (`spec.{md,yaml}`) via `spec_ref` — never inlined;
- the ECL envelope (the closed ten-performative set) — **named**, never
  re-declared;
- the CRYSTALIUM Semantic layer — **referenced** at archive, never imported or
  called.

The only schema tonberry owns is the `status`/`tier` enums in
`change.v1.json` (referenced from `Rynaro/eidolons-esl`).

---

## The 8 tools (`mcp__tonberry__*`)

| Tool | Purpose |
|------|---------|
| `propose` | Scaffold a `change.json` (status `proposed`) under `.spectra/changes/<change_id>/`. tier is null until `right_size`. |
| `right_size` | Deterministic ESL §4 gate: `(files_touched, rubric_score/12, tradeoff_present)` → `trivial`/`lite`/`full`. Same signals always yield the same tier. |
| `transition` | Advance `status` honoring §3 skip-rules (lite/trivial skip `deliberated`; code-states require code; `archived` requires `drift_checked`). |
| `compose_manifest` | Write/validate `change.json` against the ESL-owned `change.v1.json` (references `spec_ref`; never inlines the SPECTRA schema). |
| `compose_envelope` | Emit an ECL sidecar `*.envelope.json` naming the §7.2 performative for a transition. |
| `verify` | Run the six mechanical conformance checks **C1–C6** (incl. maker≠checker as C4). `--mode warn\|block`, `--json`. **Parity-locked** to `esl-conformance.sh`. |
| `drift_check` | Identity-distinct checker (checker ≠ maker) records the drift verdict; mismatch → `ESCALATE` to `in_progress`; match → `drift_checked=true`. |
| `archive` | Snapshot folder → `archive/<date>-<change_id>/`, set `status=archived` + `archive_path`, compose the **promotion-intent** ECL envelope. Requires `drift_checked=true`. Never calls CRYSTALIUM. |

`maker_checker` is **not** a separate tool — maker ≠ checker is **check C4 of
`verify`** (where the normative bash checker keeps it).

---

## The parity invariant

`tonberry verify` is a behavioral re-implementation of the normative
`conformance/esl-conformance.sh` from `Rynaro/eidolons-esl`. For every change
folder, the two MUST agree on the six checks (ids, MUST/SHOULD level, ok/fail
verdict, semantics), the exit codes (`0` conformant / `1` usage / `3` block; `2`
reserved), and the `--json` shape. **The bash checker is authoritative; on any
disagreement tonberry is the bug.**

This is locked mechanically:

- `parity/esl-conformance.sh` vendors the canonical oracle (re-sync on any ESL
  checker revision — a divergence is a release-blocking reversal condition);
- `internal/conformance/parity_test.go` runs every fixture through both
  implementations and asserts structural equality of the `--json` summaries
  (the set of `{id,status}` + exit code), in both `warn` and `block` modes;
- CI runs the parity test with `TONBERRY_REQUIRE_PARITY=1`.

---

## Dual-mode usage

One static binary, three entry points:

```sh
# 1. stdio MCP server (the .mcp.json entry runs this)
tonberry serve

# 2. one-shot CLI for any op (JSON result to stdout)
tonberry propose      --change_id add-flag --maker vivi --spec_ref spec.md --checker vigil
tonberry right_size   --change_id add-flag --files_touched 1 --rubric_score 5 --tradeoff_present false --write_manifest true
tonberry transition   --change_id add-flag --to_status in_progress --has_code true --write_manifest true
tonberry archive      --change_id add-flag

# 3. CI / standalone conformance checker (no MCP host needed)
tonberry verify .spectra/changes/add-flag --mode block --json
#   exit 0 conformant / 1 usage error / 3 hard violation
```

### Container

```sh
# stdio MCP, project tree mounted read-write at /workspace
docker run --rm -i \
  -v "$PROJECT_ROOT":/workspace -w /workspace \
  --cap-drop ALL --security-opt no-new-privileges \
  ghcr.io/rynaro/tonberry@<digest> serve

# CI checker
docker run --rm -v "$PROJECT_ROOT":/workspace -w /workspace \
  ghcr.io/rynaro/tonberry@<digest> verify .spectra/changes/<id> --mode block
```

The ECL envelope stamp defaults to `envelope_version: "1.0"` (matching the
eidolons-esl examples) and is overridable via `TONBERRY_ECL_ENVELOPE_VERSION` or
the `--envelope_version` flag — tonberry tracks the ecosystem stamp, it does not
resolve the ECL version ambiguity.

---

## Build / test (containerized)

Go is not required locally — everything runs in `golang:1.23`:

```sh
docker run --rm -v "$PWD":/src -w /src golang:1.23 go vet ./...
docker run --rm -v "$PWD":/src -w /src golang:1.23 go test ./...   # parity needs jq + bash
docker build -t tonberry:dev .                                     # ~13 MB distroless
```

---

## Layout

```
cmd/tonberry/main.go       arg dispatch: serve / verify / <op>
internal/lifecycle/        §3 state machine + skip-rules (transition)
internal/rightsizing/      §4 deterministic 3-signal gate (right_size)
internal/conformance/      the 6 checks C1–C6 — the bash-parity surface (verify)
internal/manifest/         change.json read/write + change.v1.json validation
internal/envelope/         ECL sidecar compose (name performatives; no schema re-decl)
internal/archive/          snapshot + promotion-intent compose
internal/mcpserver/        stdio server: 8-tool manifest + call dispatch
internal/ops/              the op business logic shared by MCP + CLI
fixtures/                  shared parity corpus (conformant + each-check-failing)
parity/esl-conformance.sh  vendored normative oracle (canonical: Rynaro/eidolons-esl)
```

Each `internal/*` maps to an ESL section, keeping the anti-scope boundary visible
in the tree.

## License

MIT — see [LICENSE](LICENSE).
