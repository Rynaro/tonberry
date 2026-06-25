# trivial-typo-fix

A trivial-tier change: a single-typo fix routed to Kupo with **no spec**
(`spec_ref: null`). Demonstrates the right-sizing gate's no-spec bypass — the
state machine is skipped; the route is ECL `DELEGATE` → Kupo, verifier-backed.

maker = `kupo`, checker = `vigil` (distinct identities, so promotion to
`verified` is legal). Conformance passes in block mode because no spec file is
required for the trivial tier.
