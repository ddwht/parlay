<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/ledger-and-contract
-->

# Amendment Schema

File: `spec/intents/<feature>/amendments/NNN-<slug>.md`. The append-only change ledger of the ledger-and-contract model: one file per change, written once and **never edited** — a later change that alters the same ground is a NEW amendment that names the old one in `supersedes:`. Active only in projects with `parlay.ledger: true` in `.parlay/config.yaml`; without the flag the directory is inert.

An amendment records a refinement to an existing feature: what changed, why, and — machine-readably — which contract artifact entries it touches. The apply step splices the delta into those artifacts and lands the `## Acceptance` bullets as `verify:` entries on them. The founding documents (intents.md, dialogs.md) are frozen at feature birth and are never modified by an amendment.

## Structure

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

## Filename and sequence

`NNN-<slug>.md`, three digits, sequential from `001`. Lexical order equals ledger order in every directory listing. A gap in the sequence is what a compacted ledger legitimately looks like (`amendments/archive/` holds the pre-compaction files); a duplicate is a collision. Files not matching the pattern are invisible to the ledger and reported by `check-amendments`.

## Versioning

No `schema_version:` field (see `schema-versioning.schema.md`). Amendments are immutable once written, so there is nothing to migrate in place — a shape change to this schema applies to new files only, and the parser stays tolerant of the old shape, which remains on disk forever by design.

## Diagnostics

Single-file (`parlay validate --type amendment`):

| Code | When it fires |
|---|---|
| `amendment-not-parseable` | Missing or unterminated frontmatter, or invalid YAML in it. |
| `amendment-frontmatter-incomplete` | `amendment:` or `date:` missing. |
| `amendment-affects-missing` | `affects:` is empty — an amendment that names no contract entry cannot be applied or scoped. |
| `amendment-affects-malformed` | An `affects:` entry is not `@<feature>/<kind>:<name>` with a known kind. |
| `amendment-missing-change` | No `## Change` section. |
| `amendment-missing-acceptance` (warning) | No `## Acceptance` bullets — fine for renames and pure-prose changes, wrong for anything testable. |

Ledger-level (`parlay internal check-amendments <@feature>` — JSON, also emits `dirty_set`):

| Code | When it fires |
|---|---|
| `amendment-slug-mismatch` | Frontmatter `amendment:` disagrees with the filename slug. |
| `amendment-out-of-sequence` | Two files share a sequence number, or a file in `amendments/` matches no `NNN-<slug>.md` shape and is invisible to the ledger. |
| `amendment-sequence-gap` (warning) | Sequence numbers jump — expected after compaction, otherwise a numbering mistake. |
| `amendment-supersedes-unknown` | `supersedes:` names no earlier amendment in this ledger. |
| `amendment-affects-unresolved` | An `affects:` ref names an operation/fragment/entity that does not exist in the referenced feature's contract artifacts. |

## The dirty set

`check-amendments` emits `dirty_set` — the resolvable `affects:` refs of the **unapplied tail** only: amendments whose sequence exceeds the feature baseline's `last-applied-amendment`. Everything at or below that sequence was already folded into generated code when the baseline was saved, so it is not what a rebuild must touch. This is the **declared** counterpart of what `parlay internal diff` **infers** by hashing: consumers scope rebuilds, prompts, and (later) test selection with it, while the hash comparison remains as trust-but-verify. A disagreement between the declared set and the observed diff is a bug signal — in the amendment or in the apply — not something to proceed past.

*Which reading won and why (L7):* the tail, not the cumulative union. `dirty_set` names what has changed since the last build — the same question `parlay internal diff` answers — so the two agree. The union kept naming long-applied refs as dirty forever and never converged with the observed diff. The full union is still available, honestly named, as `all_affects` (every amendment's resolvable refs regardless of application state) for consumers that want the whole ledger footprint rather than the rebuild-scoping tail. With no baseline (never built, or pre-v3) `last-applied-amendment` reads as 0, so `dirty_set` equals `all_affects` — the conservative from-scratch reading.
