# stage-not-status — fixture spec

> Present so C3 (lite tier-artifacts) passes; the isolated violation is C2a
> (no top-level `status` field — this manifest uses the non-canonical
> `ariramba/esl-change.compat.v1` shape, which stores lifecycle state under a
> top-level `stage` field instead). See Rynaro/tonberry#9. The verdict stays
> FAIL — tonberry does NOT adopt/alias `stage` as a status — this fixture
> only proves the C2a failure DETAIL is diagnosable.

## Behavior

GIVEN: a change manifest with no top-level `status` field but a non-canonical
       top-level `stage` field
WHEN:  the conformance checker reads it
THEN:  C2a fails (illegal status: empty) with a detail that names the missing
       `status` field and flags the non-canonical `stage` field, while other
       checks pass
