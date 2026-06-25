# lite-ears-complete — lite spec (optional EARS acceptance form)

> SPECTRA-owned one-page format (ESL only requires its presence for lite tier).
> This worked example uses the OPTIONAL EARS structured acceptance form
> (`{id, given, when, then, verify_method}`) — the advisory C7 lint emits `ok`
> because all four EARS fields are present. The plain-string and `{id,
> verify_method}` forms remain equally valid (see `../lite-add-flag/`).

## Behavior (EARS: WHEN [event] THE SYSTEM SHALL [action])

GIVEN: the CLI is invoked with the new `--dry-run` flag
WHEN:  the command would otherwise mutate the working tree
THEN:  THE SYSTEM SHALL print the plan it would execute and exit 0 without writing

## Acceptance checks

- AC-1: EARS-complete — `given`/`when`/`then`/`verify_method` all present, so
  the advisory C7 lint passes. Verified via `bats`.
