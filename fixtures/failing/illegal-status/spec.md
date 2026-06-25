# illegal-status — fixture spec

> Present so C3 (lite tier-artifacts) passes; the isolated violation is C2a
> (status `in-review` is not in the ESL status enum).

## Behavior

GIVEN: a change manifest with an out-of-enum status
WHEN:  the conformance checker reads it
THEN:  C2a fails (illegal status) while other checks pass
