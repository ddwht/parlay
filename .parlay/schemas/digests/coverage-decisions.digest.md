# Coverage Decisions Schema — authoring digest

Derived from `coverage-decisions.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

## Top level

| Field | Required | Description |
|---|---|---|
| `schema_version` | yes | `1`. Any other version is refused rather than read leniently. |
| `feature` | yes | The feature slug; refused on mismatch. |
| `criteria_hash` | yes | The standard these decisions were made against. |
| `granted_at` | yes | When this ledger was first opened. Per-decision timing lives on each entry. |
| `exceptions` | no | Decisions in force. |
| `retired_decisions` | no | Decisions withdrawn, kept rather than deleted. |
| `legacy_file_hash` | no | The version of the retired coverage-review that `reconciled_legacy` answers. |
| `reconciled_legacy` | no | Stranded legacy judgments that have been answered. |
| `deferred_legacy` | no | Review attempts that reached no decision. **Not answers.** |

---

## `exceptions`

| Field | Required | Description |
|---|---|---|
| `ref` | yes | The contract entry. |
| `text` | no | The exact criterion. Omitted means the exception is **entry-wide**, which is broader and warned. |
| `kind` | yes | `waived` or `state-only`. |
| `reason` | yes | Why. An exception nobody can review later is not one. |
| `at`, `by` | yes | Per decision, never file-level. |
| `entry_hash` | no | For an entry-wide exception: the bullet set it was granted over, so adding a bullet invalidates it. A bullet-specific exception needs none — its `(ref, text)` **is** its binding. |
| `suite`, `case` | `state-only` only | The case whose weaker observation is accepted. |
| `case_hash` | `state-only` only | What that case actually observed when approved. |

---

### Identity, by kind

A waiver is a claim about the criterion, so one criterion cannot be waived
twice: a second is refused as a duplicate that would shadow the first.

A downgrade is a claim about one case, and several cases may each observe one
criterion weakly for their own reasons. Downgrades are therefore identified by
`(ref, text, suite, case)`. Keying them on `(ref, text)` alone refuses the
second as a duplicate and leaves one case unreviewable.

---

## `reconciled_legacy` and `deferred_legacy`

Both concern exemptions stranded in a retired `coverage-review.yaml`.

| Field | Required | Description |
|---|---|---|
| `ref`, `criterion_text` | yes / no | What the legacy entry named. |
| `fingerprint` | yes | The EXACT legacy entry, hashed over its whole content including its reason. |
| `duplicate` | no | Index among entries identical in every field. |
| `reason`, `at`, `by` | yes | The decision, when, and what made it. |
| `disposition` | `reconciled_legacy` only | `recorded` or `dropped`. |
| `source_hash` | `deferred_legacy` only | The version of the legacy file this attempt was made against. |

---

## Errors

| Code | When |
|---|---|
| unsupported `schema_version` | Version this build does not implement. |
| missing reason | An entry records no `reason`. |
| missing attribution | An entry records no `by` or no `at`. |
| stale ledger | `criteria_hash` does not match the current standard. |
| stranded legacy exemptions | The retired review holds entries nothing has answered. |
| legacy file changed | `legacy_file_hash` no longer matches. |
| duplicate claim | Two waivers on one criterion, or two downgrades on one case. |
