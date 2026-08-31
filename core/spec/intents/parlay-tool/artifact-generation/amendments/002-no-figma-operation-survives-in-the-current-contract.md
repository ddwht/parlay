---
amendment: no-figma-operation-survives-in-the-current-contract
date: 2026-08-31
trigger: "review found a contradiction 001 could not have named: minutes before 001 was authored, the domain-model operations migration wrote a kind: unknown stub for generate-surface-from-figma into this feature's capabilities.yaml — a live current-contract entry naming a Figma path, while 001 states no such path remains"
supersedes:
  - design-spec-surface-retired
supersedes_intents:
  - reference-design-spec-from-figma
---

## Change

Amendment 001 stands whole; this record adds the disposition it could not
have known to name. The migrated operation stub
`unknown.generate-surface-from-figma` is REMOVED from this feature's
capabilities.yaml rather than retained for later review: build ordering
treats capabilities as current truth, and a current contract entry naming a
Figma-driven surface path would contradict 001's central claim the moment it
was read. The operation's existence is not erased — it remains recorded in
the domain model's git history and in the migration commit — but it has no
place in the current contract of a feature whose Figma intent is retired.
The twelve other migrated stubs across the project are unaffected and await
ordinary kind: review.

## Why

Two same-day records must not leave the contract saying both "no Figma path
remains" and "here is a Figma operation". The stub was a faithful migration
of a legacy declaration, and the declaration's subject was retired by 001;
carrying the stub forward would have re-created, in the capabilities layer,
exactly the promise 001 closed in the intent layer.

## Acceptance

- capabilities.yaml in this feature contains no operation whose id, subject
  or notes reference Figma or a design-spec.
- The remaining migrated stubs in this feature (generate-surface, view-page,
  lock-page) are untouched.
