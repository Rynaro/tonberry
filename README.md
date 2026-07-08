# tonberry

**The official, Eidolons-backed implementation of ESL** (the [Eidolons Spec
Lifecycle](https://github.com/Rynaro/eidolons-esl)). A thin, single-binary Go MCP
that **composes** ESL change folders and **enforces** the lifecycle — the
programmatic sibling of the ESL spec + its bash conformance checker.

`ESL_VERSION` = `1.1` · sibling of ECL / EIIS · opt-in.

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

## The 11 tools (`mcp__tonberry__*`)

**Per-change lifecycle (8):**

| Tool | Purpose |
|------|---------|
| `propose` | Scaffold a `change.json` (status `proposed`) under `.spectra/changes/<change_id>/`. tier is null until `right_size`. Pass `--has_code` to persist the §3.2 lifecycle hint so `transition` reads it without a per-call flag. Pass BOTH `--memory_preflight_ran` and `--memory_preflight_records` to persist the OPTIONAL v1.1 recall-before-authoring record (§2.6); omit both to skip it (graceful-skip, still conformant). |
| `right_size` | Deterministic ESL §4 gate: `(files_touched, rubric_score/12, tradeoff_present)` → `trivial`/`lite`/`full`. Same signals always yield the same tier. **PERSISTS the tier by default** when a `change_id` is given (`--dry-run` classifies only). |
| `transition` | Advance `status` honoring §3 skip-rules (lite/trivial skip `deliberated`; code-states require code; `archived` requires `drift_checked`). `has_code` is **read from the manifest** hint (override with an explicit `--has_code`). **PERSISTS by default** when allowed (`--dry-run` evaluates only). |
| `compose_manifest` | Write/validate `change.json` against the ESL-owned `change.v1.json` (references `spec_ref`; never inlines the SPECTRA schema). |
| `compose_envelope` | Emit an ECL sidecar `*.envelope.json` naming the §7.2 performative for a transition. |
| `verify` | Run the six MUST conformance checks **C1–C6** (incl. maker≠checker as C4) plus the SHOULD advisory **C7** (EARS acceptance lint) and **C8** (fresh-context verification attestation, v1.1). `--mode warn\|block`, `--json`. **Parity-locked** to `esl-conformance.sh`. C7/C8 are advisory — they never change the exit code (only C1–C6 block). |
| `drift_check` | Identity-distinct checker (checker ≠ maker) records the drift verdict; mismatch → `ESCALATE` to `in_progress`; match → `drift_checked=true`. **PERSISTS by default** on a clean verdict (`--dry-run` evaluates only). |
| `archive` | **MOVE** the change folder → `archive/<date>-<change_id>/` (the active folder no longer exists afterward), set `status=archived` + `archive_path`, compose the **promotion-intent** ECL envelope. Requires `drift_checked=true`. Never calls CRYSTALIUM. |

**Project-scope observability (3, read-only — v0.2.0):**

| Tool | Purpose |
|------|---------|
| `list` | Enumerate **active** change folders under `.spectra/changes/`; returns `[{change_id,status,tier,drift_checked,archived}]` sorted by `change_id`. `--all` (alias `--include-archived`) ALSO lists archived snapshots under `archive/`. |
| `status` | For one `change_id`: the manifest summary + the **`verify` verdict** (the same six checks — not re-implemented) + the legal next lifecycle transitions. |
| `assess` | Project-scope **escalation assessment**: aggregate the §4.2 signals (`change_count` / `repo_loc` / `full_ratio`, counting **both active AND archived** changes per §9.2) vs thresholds → `recommended_mode` `advisory`\|`block`. Deterministic. See the escalation lever below. |

`maker_checker` is **not** a separate tool — maker ≠ checker is **check C4 of
`verify`** (where the normative bash checker keeps it).

### EARS acceptance form + the advisory C7 lint (v0.3.0)

`acceptance_checks[]` items are `oneOf:[string, object]` (ESL §2.5). An item MAY
be a plain string (minimal/free-form) OR a structured object; a structured object
MAY adopt the optional **EARS** form (`{id, given, when, then, verify_method}` —
*WHEN [event] THE SYSTEM SHALL [action]*). `verify`'s check **C7** advisory-lints
EARS-form items (those declaring ≥1 of `given`/`when`/`then`) for completeness:
it warns if any of `given`/`when`/`then`/`verify_method` is missing or empty. C7
is **SHOULD-level** — a C7-only failure stays **exit 0 even under `--mode block`**
(only C1–C6 block). Plain-string and minimal `{id, verify_method}` items produce
**no** C7 finding. C7 is byte-identically parity-locked to the bash oracle.

### Fresh-context verification attestation + `memory_preflight` (v0.5.0, ESL v1.1)

`verify`'s check **C8** extends C4 from *identity*-inequality (maker ≠ checker)
to *context*-separation: when `status` ∈ `{verified, archived}` **AND** a
`verify.envelope.json` sidecar is present, its `ise.verification` sub-block
(`{fresh_context, checker, transcript_access}`) SHOULD show
`fresh_context == true`, `transcript_access ∈ {none, artifact-only}`, and
`checker != maker`. No envelope at all → **no C8 record** (skip, not fail) — C8
only evaluates once verification is claimed. Like C7, C8 is **SHOULD-level** and
**never changes the exit code**, and is byte-identically parity-locked to the
bash oracle.

`change.json` MAY also carry an OPTIONAL `memory_preflight: {ran, records}`
object (ESL §2.6), recorded when a change enters `proposed`: `ran` records that
a CRYSTALIUM (or equivalent memory-MCP) recall-before-authoring query executed;
`records` is the count recalled (`0` is a valid, conformant result). It is
schema-validated when present but gates no C1–C6 MUST check; absence is fully
conformant. `propose` accepts `--memory_preflight_ran` + `--memory_preflight_records`
(given together) to persist it; `compose_manifest --patch` also carries it
through the generic manifest-merge path.

### The escalation lever (advisory → forced)

ESL is **advisory by default** (`verify --mode warn`) and **forced on demand**
(`--mode block`). A project escalates when its project-aggregate right-sizing
numbers cross mechanical thresholds — `change_count ≥ N`, `repo_loc ≥ L`, or
`full_ratio ≥ R` (seed defaults `N=10` / `L=50000` / `R=0.4`, all tunable). The
signals REUSE the §4.2 family (no new vocabulary, mechanical not LLM-judged).
`assess` computes the aggregate and recommends a mode; tonberry ships the
**assessment + the lever** — the *flip* is recorded **nexus-side** in
`eidolons.mcp.lock`, NOT in `esl-1.0.md` (ESL stays opt-in). See
[`eidolons-esl/docs/escalation.md`](https://github.com/Rynaro/eidolons-esl/blob/main/docs/escalation.md).

---

## The parity invariant

`tonberry verify` is a behavioral re-implementation of the normative
`conformance/esl-conformance.sh` from `Rynaro/eidolons-esl`. For every change
folder, the two MUST agree on the eight checks — C1–C6 (MUST) plus the SHOULD
advisory C7/C8 (ids, MUST/SHOULD level, ok/fail verdict, semantics), the exit
codes (`0` conformant / `1` usage / `3` block; `2` reserved), and the `--json`
shape. **The bash checker is authoritative; on any disagreement tonberry is the
bug.**

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
# right_size / transition / drift_check PERSIST by default (v0.4.0); add --dry-run to
# evaluate-only. propose --has_code persists the §3.2 hint; transition then reads it.
# propose --memory_preflight_ran + --memory_preflight_records (given together) persist
# the OPTIONAL v1.1 recall-before-authoring record (§2.6).
tonberry propose      --change_id add-flag --maker vivi --spec_ref spec.md --checker vigil --has_code \
                      --memory_preflight_ran --memory_preflight_records 2
tonberry right_size   --change_id add-flag --files_touched 1 --rubric_score 5 --tradeoff_present false
tonberry transition   --change_id add-flag --to_status in_progress      # reads has_code from the manifest; persists
tonberry transition   --change_id add-flag --to_status verified --dry-run   # evaluate-only, no write
tonberry archive      --change_id add-flag   # MOVES the folder into archive/<date>-add-flag/

# project-scope observability (read-only)
# list/status/assess share one convention: a positional changes-dir path and
# --changes_dir are equivalent (default .spectra/changes; --changes_dir wins if both given).
tonberry list         .spectra/changes                  # active only; add --all to include archived
tonberry status       .spectra/changes --change_id add-flag --mode warn
tonberry assess       .spectra/changes --repo_loc 60000  # or omit repo_loc to walk the tree

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

#### SELinux-enforcing hosts

Without a label option, a bind-mounted workspace stays labeled `user_home_t`
and container writes fail with a bare `EACCES` (`propose`, `right_size`,
`transition`, `archive`, `compose_*`) while reads (`list`/`status`/`verify`)
keep working — a confusing asymmetric failure. Fix it by adding `:z` to the
volume flag:

```sh
-v "$PROJECT_ROOT":/workspace:z
```

Docker relabels the tree shared (`container_file_t`); `:z` is inert on
non-SELinux hosts (macOS, default Ubuntu), so it is safe to include by
default. `--security-opt label=disable` is the alternative when relabeling is
undesirable. tonberry itself now detects an SELinux-enforcing host and
appends this hint to permission-denied write errors.

#### Host-owned workspaces (container UID)

The tonberry image runs as distroless nonroot **UID 65532**. A host workspace
owned by your own UID (e.g. `1000`) at the common `0755` mode grants `o+rx` —
reads succeed — but no `o+w`, so write ops (`propose`, `transition`, `archive`,
`compose_*`) fail with `EACCES` while reads keep working, a confusing
asymmetry. Fix it by running the container as the mount owner:

```sh
docker run --rm -i --user "$(id -u):$(id -g)" \
  -v "$PROJECT_ROOT":/workspace -w /workspace \
  ghcr.io/rynaro/tonberry@<digest> serve
```

(or `chown` the workspace to `65532`). tonberry now surfaces this as a hint —
naming both UIDs — appended to the `permission denied` error on a failed write.

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
internal/conformance/      C1–C6 (MUST) + C7/C8 (SHOULD) — the bash-parity surface (verify)
internal/manifest/         change.json read/write + change.v1.json validation
internal/envelope/         ECL sidecar compose (name performatives; no schema re-decl)
internal/archive/          snapshot + promotion-intent compose
internal/mcpserver/        stdio server: 11-tool manifest + call dispatch
internal/ops/              the op business logic shared by MCP + CLI
                           (project.go = list/status/assess observability)
fixtures/                  shared parity corpus (conformant + each-check-failing)
parity/esl-conformance.sh  vendored normative oracle (canonical: Rynaro/eidolons-esl)
```

Each `internal/*` maps to an ESL section, keeping the anti-scope boundary visible
in the tree.

## License

MIT — see [LICENSE](LICENSE).
