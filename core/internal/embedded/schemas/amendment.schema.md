<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/ledger-and-contract
parlay-extends: parlay-tool/intent-supersession/supersede-a-founding-intent-through-the-amendment-ledger
parlay-extends: parlay-tool/intent-supersession/refuse-a-supersession-that-abandons-work-rather-than-replacing-it
-->

# Amendment Schema

File: `spec/intents/<feature>/amendments/NNN-<slug>.md`. The append-only change ledger of the ledger-and-contract model: one file per change, written once and **never edited** — a later change that alters the same ground is a NEW amendment that names the old one in `supersedes:`. A standard zone of every feature (since v0.4 the ledger model is the only regime); a feature that has never been refined simply has no `amendments/` directory yet.

An amendment records a refinement to an existing feature: what changed, why, and — machine-readably — which contract artifact entries it touches. The apply step splices the delta into those artifacts and lands the `## Acceptance` bullets as `verify:` entries on them. The founding documents (intents.md, dialogs.md) are frozen at feature birth and are never modified by an amendment.

## Structure
<!-- parlay:normative -->



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
| `supersedes_intents` | No | Founding intent slugs **in this amendment's own feature** that this decision replaces. Bare slugs — a qualified `@feature/slug` is refused, because one feature may never retire another's founding promise (record cross-feature pressure in `trigger:`). Naming one makes this a **governance amendment**: `affects:` may then be empty, and `## Why` and `## Acceptance` become required. The superseded `intents.md` is never modified. |
| `## Change` | Yes | The delta in prose. Delta-shaped by rule: an amendment describes a change, never restates a feature. |
| `## Why` | No (strongly encouraged) | The reasoning. |
| `## Acceptance` | Behavior changes: yes | Criteria the apply step lands as `verify:` on the affected entries. Legitimately absent for renames and pure-prose changes. |

<!-- /parlay:normative -->

## Filename and sequence
<!-- parlay:normative -->



`NNN-<slug>.md`, three digits, sequential from `001`. Lexical order equals ledger order in every directory listing. A gap in the sequence is what a compacted ledger legitimately looks like (`amendments/archive/` holds the pre-compaction files); a duplicate is a collision. Files not matching the pattern are invisible to the ledger and reported by `check-amendments`.

<!-- /parlay:normative -->

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
| `amendment-supersedes-intent-malformed` | A `supersedes_intents:` entry is empty. |
| `amendment-supersedes-intent-foreign` | A `supersedes_intents:` entry is qualified (`@feature/slug` or `feature/slug`). One feature may not retire another's founding promise; record cross-feature pressure in `trigger:`. |
| `amendment-supersedes-intent-unknown` | A `supersedes_intents:` entry names no intent in this feature's `intents.md`. |
| `amendment-supersedes-intent-forked` | Two live amendments supersede the same intent with no ordering between them. Naming the earlier in `supersedes:` settles it. |
| `amendment-supersedes-last-intent` | The ledger would retire every founding intent of the feature. Retiring a whole feature is a lifecycle operation with its own dependency checks. |
| `amendment-supersession-no-successor` | A superseding amendment has no `## Acceptance`. The rename/pure-prose exemption does not apply: retiring a promise with nothing in its place is deletion. |
| `amendment-supersession-no-rationale` | A superseding amendment has no `## Why`. The frozen intent cannot record why it stopped being true, so this is the only place that reasoning will exist. |
| `intent-supersession-unaccounted-affect` | A contract entry whose `source:` names a superseded intent has no disposition in `affects:`. |
| `amendment-affects-unresolved` | An `affects:` ref names an operation/fragment/entity that does not exist in the referenced feature's contract artifacts. |
| `amendment-scope-overlap` (warning) | A later amendment's `affects:` intersects an earlier amendment's, and the earlier one is not named in the later's `supersedes:`. Two amendments editing the same contract entry with no ordering between them. Naming the earlier in `supersedes:` — the declaration that this change replaces it — silences the warning. |

## Superseding a founding intent

The founding documents are frozen at feature birth and no amendment modifies
them. That freezes the *file*, and for a long time it also froze the *promise*:
`affects:` resolves contract entries, so an amendment had nothing to name in a
feature that owns no contract artifact. Such features are not rare — in parlay's
own tree, 18 of 27 `parlay-tool` features carry no feature-local contract
artifact, and four of them exist to deprecate something and could not themselves
be revised. Their only options were to edit a frozen document, which
`ledger_integrity` correctly reports, or to let the spec contradict the code
indefinitely.

`supersedes_intents:` closes that. It names founding intents in the amendment's
own feature that this decision replaces, and it makes `affects:` optional — an
amendment naming only superseded intents is a **governance amendment**, which
changes what a feature promises without claiming to splice an artifact. This is
the same "this replaces that" relation `supersedes:` uses one level down, and
that `supersedes:` on a surface fragment uses one level down again; `surface.schema.md`
calls it "deliberately the same concept ... one 'this replaces that' model at
every level". Intent level was the level it was missing from.

Four rules keep supersession from becoming deletion:

- **A successor is required.** `## Why` and `## Acceptance` are both mandatory
  here, with no rename or pure-prose exemption, and the Acceptance becomes the
  replacement's active criteria. The field is named `supersedes_intents` rather
  than `retires` for this reason: a commitment may be replaced, not dropped.
- **Scope is accounted for.** Every contract entry whose `source:` names a
  superseded intent must appear in `affects:` — replaced, removed or retained.
  A feature with no contract artifact satisfies this with an empty set; a
  feature with artifacts must say what becomes of each, or the generated scope
  outlives the promise that justified it.
- **One decision at a time.** Two live amendments superseding the same intent
  fork the ledger and block, unless the later names the earlier in `supersedes:`.
- **A feature must still promise something.** Retiring the last live intent
  fails; whole-feature retirement is a lifecycle operation with broader
  dependency checks.

Supersession takes effect only once **applied**. An authored but unapplied
supersession is proposed specification and blocks the boundary — the artifacts
and the code still reflect the promise it proposes to withdraw. And it grants no
exemption from byte-integrity: editing a superseded `intents.md` still raises
`ledger_integrity`, because the file is never touched. What changes is semantic
authority, not the hash. `check-amendments` reports the link in
`superseded_intents`, since the forward direction cannot live in the frozen file.

## Reading the ledger

An amendment file alone is not current truth: files are immutable, so a superseded amendment
still asserts its retired behavior and carries only whatever `supersedes:` links existed when
it was written — the forward direction is never in the file. Before relying on any amendment's
content, run `parlay internal check-amendments <@feature>` and consult its `superseded_by`
map; the handoff projection renders the same links in its History section. An amendment with a
`superseded_by` entry is history, not specification.

## The dirty set

`check-amendments` emits `dirty_set` — the resolvable `affects:` refs of the **unapplied tail** only: amendments whose sequence exceeds the feature baseline's `last-applied-amendment`. Everything at or below that sequence was already folded into generated code when the baseline was saved, so it is not what a rebuild must touch. This is the **declared** counterpart of what `parlay internal diff` **infers** by hashing: consumers scope rebuilds, prompts, and (later) test selection with it, while the hash comparison remains as trust-but-verify. A disagreement between the declared set and the observed diff is a bug signal — in the amendment or in the apply — not something to proceed past.

*Which reading won and why (L7):* the tail, not the cumulative union. `dirty_set` names what has changed since the last build — the same question `parlay internal diff` answers — so the two agree. The union kept naming long-applied refs as dirty forever and never converged with the observed diff. The full union is still available, honestly named, as `all_affects` (every amendment's resolvable refs regardless of application state) for consumers that want the whole ledger footprint rather than the rebuild-scoping tail. With no baseline (never built, or pre-v3) `last-applied-amendment` reads as 0, so `dirty_set` equals `all_affects` — the conservative from-scratch reading.

## Forward links: `superseded_by`

`supersedes:` points backward — a later amendment names the earlier ones it replaces. The forward link ("which later amendment replaced me?") cannot live in the earlier file, because an amendment is immutable once written: nothing may edit a landed ledger entry to record something that happened after it. So `check-amendments` computes the reverse at read time and emits it as `superseded_by` — a map from each superseded slug to the slugs of the later amendments that supersede it. The map is always present (possibly empty); consumers index it without a nil check. This is the same "compute the forward link rather than mutate the immutable record" move the composition vocabulary uses for surface `supersedes:` (F18).
