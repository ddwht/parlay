# Amendment Schema — authoring digest

Derived from `amendment.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

```markdown
---
amendment: <slug — must equal the filename's <slug> part>
date: <YYYY-MM-DD>
trigger: <one line: what prompted this — a user ask, another feature's need (@feature), a defect>
affects:
  - "@<feature>/<kind>:<name>"
supersedes:
  - <earlier amendment slug this replaces — omit or empty when none>
---

## Change
<the delta, in prose — what is different after this amendment>

## Why
<the reasoning — recorded here because nothing else records it>

## Acceptance
- <criterion the apply step lands as a verify: entry on the affected entries>
```

| Field | Required | Description |
|---|---|---|
| `amendment` | Yes | Slug identity. Must match the filename slug — a file may not disagree with its own name. |
| `date` | Yes | ISO date the amendment was decided. |
| `trigger` | No | What prompted the change. Cross-feature pressure (`@export needs …`) is exactly what this field exists to record — the causal link that previously lived nowhere. |
| `affects` | Yes | The declared dirty set: contract entries this amendment changes, as `@<feature>/<kind>:<name>` refs with kind one of `operation` (capabilities.yaml id), `surface` (fragment name slug), `infrastructure` (fragment heading slug), `domain` (root-model entity name). Never an intent ref — amendments change the contract, not the frozen founding docs. |
| `supersedes` | No | Earlier amendment slugs in the same ledger that this one replaces. What compaction and contradiction detection walk. |
| `## Change` | Yes | The delta in prose. Delta-shaped by rule: an amendment describes a change, never restates a feature. |
| `## Why` | No (strongly encouraged) | The reasoning. |
| `## Acceptance` | Behavior changes: yes | Criteria the apply step lands as `verify:` on the affected entries. Legitimately absent for renames and pure-prose changes. |

---

`NNN-<slug>.md`, three digits, sequential from `001`. Lexical order equals ledger order in every directory listing. A gap in the sequence is what a compacted ledger legitimately looks like (`amendments/archive/` holds the pre-compaction files); a duplicate is a collision. Files not matching the pattern are invisible to the ledger and reported by `check-amendments`.
