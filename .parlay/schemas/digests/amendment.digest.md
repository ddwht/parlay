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
supersedes_intents:
  - <founding intent slug in THIS feature that this decision replaces — omit when none>
retires_feature: <true when this record closes the whole feature — omit otherwise>
outcome: <replaced | obsolete — required with retires_feature>
replacement_feature: <@feature that carries this work now — required by replaced, forbidden by obsolete>
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
| `affects` | Yes, unless `supersedes_intents` is non-empty | The declared dirty set: contract entries this amendment changes, as `@<feature>/<kind>:<name>` refs with kind one of `operation` (capabilities.yaml id), `surface` (fragment name slug), `infrastructure` (fragment heading slug), `domain` (root-model entity name). Never an intent ref: this field routes contract splices and drives the dirty set, and retiring a founding intent is a different act with its own field (`supersedes_intents`). |
| `supersedes` | No | Earlier amendment slugs in the same ledger that this one replaces. What compaction and contradiction detection walk. |
| `retires_feature` | No | `true` marks this the feature's **terminal record**: it closes the feature rather than changing it. Declared, never inferred — an amendment that merely names every live intent stays the `amendment-supersedes-last-intent` error it is today, because it carries none of retirement's obligations and a lifecycle transition nobody chose is not one to infer. Permits retiring every live intent, which no other record may. |
| `outcome` | With `retires_feature` | Closed at `replaced \| obsolete`. A reader months later cannot recover from silence whether the work moved or stopped mattering, and that difference is the whole content of the decision. |
| `replacement_feature` | With `outcome: replaced` | The feature that carries this work now. Forbidden by `obsolete`. Must exist, must not be the retiring feature, and must not itself be retired. It is metadata about the outcome and **never permission**: a reference aimed at the retiring feature does not begin aiming at the replacement by being told about it, so `replaced` faces the same zero-inbound rule as `obsolete`. |
| `supersedes_intents` | No | Founding intent slugs **in this amendment's own feature** that this decision replaces. Bare slugs — a qualified `@feature/slug` is refused, because one feature may never retire another's founding promise (record cross-feature pressure in `trigger:`). Naming one makes this a **governance amendment**: `affects:` may then be empty, and `## Why` and `## Acceptance` become required. The superseded `intents.md` is never modified. |
| `amends_intents` | No | The **evolution vocabulary**, and the field new records use. One entry per founding promise: `intent:` (the lineage slug, bare and same-feature), `mode:` (closed at `extend \| revise \| narrow \| retire`), and — for every mode but `retire` — a `version:` block holding the promise's **complete** new text. A promise is a durable decision **lineage**; an amendment creates its next version. See *Intent evolution* below. |
| `## Change` | Yes | The delta in prose. Delta-shaped by rule: an amendment describes a change, never restates a feature. |
| `## Why` | No (strongly encouraged) | The reasoning. |
| `## Acceptance` | Behavior changes: yes | Criteria the apply step lands as `verify:` on the affected entries. Legitimately absent for renames and pure-prose changes. |

---

`NNN-<slug>.md`, three digits, sequential from `001`. Lexical order equals ledger order in every directory listing. A gap in the sequence is what a compacted ledger legitimately looks like (`amendments/archive/` holds the pre-compaction files); a duplicate is a collision. Files not matching the pattern are invisible to the ledger and reported by `check-amendments`.
