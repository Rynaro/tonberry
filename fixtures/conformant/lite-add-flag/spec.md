# lite-add-flag — lite spec

> SPECTRA-owned one-page format (ESL only requires its presence for lite tier).

## Behavior

GIVEN: the CLI is invoked with the new `--dry-run` flag
WHEN:  the command would otherwise mutate the working tree
THEN:  it prints the plan it would execute and exits 0 without writing

## Acceptance checks

- AC-1: `--dry-run` prints the plan and exits 0, leaving the tree unchanged —
  verify via `bats` (`--dry-run flag prints plan and exits 0 without writing`)
