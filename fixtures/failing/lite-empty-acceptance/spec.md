# lite-empty-acceptance — fixture spec

> spec.md is present so C3 reaches the empty-acceptance branch; the isolated
> violation is C3 "lite: acceptance_checks is empty".

## Behavior

GIVEN: a lite change with spec.md present but acceptance_checks == []
WHEN:  the conformance checker reads it
THEN:  C3 fails (acceptance_checks is empty)
