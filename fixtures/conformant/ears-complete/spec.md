# ears-complete — lite spec

> SPECTRA-owned one-page format (ESL only requires its presence for lite tier).
> This fixture proves C7 (EARS lint) emits `ok` when an acceptance_checks item
> carries all four EARS fields (given/when/then/verify_method).

## Behavior

GIVEN: the CLI is invoked with the new `--dry-run` flag
WHEN:  the command would otherwise mutate the working tree
THEN:  THE SYSTEM SHALL print the plan and exit 0 without writing

## Acceptance checks

- AC-1: EARS-complete — `given`/`when`/`then`/`verify_method` all present.
