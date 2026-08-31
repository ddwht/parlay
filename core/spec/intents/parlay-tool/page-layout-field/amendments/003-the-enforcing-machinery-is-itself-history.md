---
amendment: the-enforcing-machinery-is-itself-history
date: 2026-08-31
trigger: "amendment 002 described the root-retirement machinery in the present tense (it 'enforces mechanically'); that machinery was decommissioned the same day, after its one execution, so the sentence is now false as written"
supersedes:
  - retired-assertions-are-history-not-green
  - layout-schema-drops-design-loop-preservation
affects:
  - "@parlay-tool/page-layout-field/infrastructure:layout-tree-schema"
---

## Change

Amendment 002's substance stands whole: the Design-Loop preservation
obligations on layout.schema.md are retired as current obligations, and the
generated buildfile/testcases assertions are preserved unchanged as frozen
build history — evidence of what the contract used to require, never
regenerated or hand-edited to "pass".

One sentence is corrected from present to past tense. 002 said the
retired-obligation-versus-preserved-evidence distinction is "the same one
the root-retirement machinery enforces mechanically". That machinery DID
enforce it mechanically — at execution time, the studio retirement's
disposition record acknowledged each retired occurrence individually with
its rationale, and the archive preserved the artifacts byte-identically —
and was then decommissioned as a spent one-time migrator (see
docs/plans/root-retirement-decommission.md). What remains is the evidence
the enforcement produced: the acknowledgments in the preserved disposition
record at .parlay/retired/studio/dispositions.yaml, the archived artifacts,
and the standard-tool verification recipe. The ledger and that evidence say
the same thing; no running machinery is claimed.

## Why

A ledger sentence that names a mechanism as current outlives the mechanism
at its peril: a reader sent to look for the enforcing tool would find it
gone and doubt the record. The enforcement happened, once, on the only
occasion it was ever needed; the record should say exactly that.

## Acceptance

- The frozen page-layout-field build artifacts still contain the retired
  preservation assertions, byte-unchanged.
- The preserved disposition record at
  .parlay/retired/studio/dispositions.yaml acknowledges the retired
  occurrences with their rationales.
- No document in this feature claims a currently-running enforcement
  mechanism for the retired assertions.
