---
amendment: correct-the-json-exit-and-code-claims
date: 2026-08-31
trigger: "post-merge review found amendment 001 asserting behavior the code contradicts: it claimed --json exits 0 with findings and named domain-operations-deprecated, while the live path exits 1 on blocking findings and emits domain-operations-unsupported"
supersedes:
  - cli-only-role-after-studio-teardown
affects:
  - "@studio-support/structured-domain-model-validation/infrastructure:json-validation-mode-for-domain-model-validate"
  - "@studio-support/structured-domain-model-validation/infrastructure:emit-domain-operations-deprecated-in-authoring-mode"
---

## Change

Amendment 001's recast of this feature into its CLI-only role stands; two of
its factual claims are corrected here.

First, the exit-code claim. 001 said `validate --type domain-model --json`
prints the findings array and exits 0. It does not, and did not at the time
001 was written: blocking findings exit 1, on the same rule as every other
validate path — the R4-22 correction ("rendering does not decide verdicts")
predates the teardown. The JSON output is unchanged (the bare findings array,
`[]` when clean); only 001's description of the exit contract was wrong.

Second, the code name. 001 referred to `domain-operations-deprecated`
surfacing at warning severity. That code no longer exists: the emitted code is
`domain-operations-unsupported`, an error in both validation modes, because
the `operations:` field was removed in v0.3 rather than deprecated. No
severity downgrade for it survives anywhere in the rule table.

## Why

An amendment is authority: later readers reconstruct the contract from it
without re-reading the code, so an amendment that asserts observably false
behavior is worse than no record — it certifies the wrong contract. 001's
substantive decision (this feature is the CLI validation surface, with no
Studio parity obligation) was and remains correct; its collateral description
of the surface it was recasting was stale on both points. Correcting by
supersession rather than editing preserves what the ledger is for: 001 still
shows what was believed when the teardown was recorded, and this record shows
what is true.

## Acceptance

- `parlay validate --type domain-model --json <path>` with blocking findings
  prints the findings array and exits 1; with warnings only, or clean, it
  exits 0.
- No spec, amendment, or code comment in this feature names
  `domain-operations-deprecated` as a live code; the live code is
  `domain-operations-unsupported`, error severity in both modes.
