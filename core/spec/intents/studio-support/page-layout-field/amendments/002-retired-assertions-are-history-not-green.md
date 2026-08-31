---
amendment: retired-assertions-are-history-not-green
date: 2026-08-31
trigger: "post-merge review found amendment 001's acceptance claiming the feature's suite is green with the Design-Loop block absent, while the frozen buildfile/testcases still assert the block's presence — no mechanism excludes those assertions from a literal run"
supersedes:
  - layout-schema-drops-design-loop-preservation
affects:
  - "@studio-support/page-layout-field/infrastructure:layout-tree-schema"
---

## Change

Amendment 001's decision stands: the Design-Loop/Figma preservation
obligation on layout.schema.md is retired, and the schema edit under the
2026-08-31 user authorization is correct. Its acceptance wording is corrected.

001 implied the feature's suite runs green with the block absent. It would
not: the frozen `buildfile.yaml` constraint and the
`layout-schema-doc-preserves-design-loop-figma-block-marker` testcase still
assert byte-equivalent presence, and nothing rewrites or filters them. The
truthful statement is: those generated assertions are RETIRED AS CURRENT
OBLIGATIONS by 001 and by the studio retirement's disposition record (which
acknowledges each occurrence individually, with rationale), while the
generated files themselves are preserved unchanged as frozen build history.
They are evidence of what the contract used to require, not tests anyone runs
against the current schema. Hand-editing or regenerating that history to
"make it pass" would falsify the record of what was built.

## Why

An acceptance criterion a reader can execute and watch fail is a defect in
the ledger even when the underlying decision is right. The distinction this
record draws — retired obligation versus preserved evidence — is the same one
the root-retirement machinery enforces mechanically (acknowledged references
carry the finding's full identity and a rationale; the artifacts stay
byte-identical in the archive), so the ledger and the tooling now say the
same thing.

## Acceptance

- layout.schema.md (embedded source and deployed copy) contains no
  Design-Loop/Figma block, marker, or design-spec relationship section.
- The frozen `core/.parlay/build/studio-support/page-layout-field/`
  buildfile/testcases still contain the preservation assertions,
  byte-unchanged — they are history, and no current check treats them as
  obligations (the studio retirement disposition record acknowledges each
  occurrence with its rationale).
- No tool, skill, or doc instructs running those retired assertions against
  the current schema.
