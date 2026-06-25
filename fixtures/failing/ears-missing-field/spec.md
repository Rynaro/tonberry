# ears-missing-field — lite spec

> SPECTRA-owned one-page format (ESL only requires its presence for lite tier).
> This fixture proves C7 (EARS lint) emits `fail` when a structured EARS item is
> missing a field (`then`), YET the checker still exits 0 in `--mode block`
> because C7 is SHOULD-level (advisory) — only the MUST checks C1–C6 block.

## Behavior

GIVEN: the CLI is invoked with the new `--dry-run` flag
WHEN:  the command would otherwise mutate the working tree
THEN:  THE SYSTEM SHALL print the plan and exit 0 without writing

## Acceptance checks

- AC-1: EARS item declares `given`/`when` (so C7 applies) but omits `then`
  (so C7 fails). Advisory only — exit code unaffected.
