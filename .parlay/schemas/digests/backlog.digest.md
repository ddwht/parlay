# Backlog Item Schema — authoring digest

Derived from `backlog.schema.md` at deploy time. Never hand-edit: edit the schema's
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
| `schema_version` | yes | `2`. Outside the readable range is **refused**, not read leniently. A v1 file is read through the chain and upgraded on its next explicit write, never on read. |
| `id` | yes | Timestamp, random suffix, slug. **Lexically time-sortable**, therefore approximately chronological — not a total capture order. Not a sequential `NNN`. |
| `kind` | yes | Closed at `defect \| gap \| debt \| idea`. |
| `priority` | no | `P0 \| P1 \| P2`. **Absent means untriaged**, never a default. |
| `title` | yes | One line a reader can recognise in a listing. |
| `body` | no | The observation in full. |
| `about` | no | Semantic parlay refs this concerns. **Shape-validated** — see below. |
| `captured` | yes | Immutable provenance — see below. |
| `evidence` | no | Filesystem locations: `path`, optional `line`, optional `detail`. |
| `history` | no | Append-only dispositions. |

## `captured` — immutable once written

| Field | Required | Description |
|---|---|---|
| `at` | yes | RFC3339. |
| `by` | yes | Who or what observed it. An observation nobody can attribute is one nobody can follow up. |
| `run` | no | `PARLAY_RUN_ID`, so an item ties back to the pipeline run that produced it. |
| `feature` | no | The feature being worked on when it was found. |
| `phase` | no | The pipeline phase it was found in. |
| `origin_root` | no | Where the discovery happened, which is not always where the work belongs. |

## `history` entries

| Field | Required | Description |
|---|---|---|
| `event` | yes | Seven values: `deferred` is nonterminal; `promoted`, `amended`, `folded`, `declined`, `obsolete` and `fixed` are terminal. |
| `reason` | on `deferred`, `declined`, `obsolete`, `fixed` | Why, or for `fixed` what was done. A disposition nobody can review later is not one. |
| `becomes` | on `promoted`, `amended`, `folded` | What the work became. **Forbidden** on `declined`, `obsolete` and `fixed` — nothing became of those. |
| `at`, `by` | yes | Per decision, never file-level. |

## Derived state, never stored

There is no `state:` field. State is computed from `history`:

- **open** — no terminal event.
- **promoted / amended / folded / declined / obsolete / fixed** — *the* terminal event.

**At most one terminal event, and it must be last.** No event may follow it.
Deriving from "the latest terminal event" would quietly admit a deferral
recorded after a promotion.

`deferred` is **non-terminal**. Deferral attempts accumulate and never change
open state: two people independently unable to decide is a different fact from
one attempt overwritten twice.

## Mutability

The item is **mutable** — `title`, `body`, `priority`, `about` and `evidence`
can be corrected and enriched. Amendment immutability is priced for authority
mutation; a low-authority inbox note needs typo correction, and demanding a
supersession record for a fixed typo is ceremony the act does not warrant.

Not mutable: the `captured` block, and `history`, which is append-only. Both are
enforced **at the mutation commands**, by snapshotting the immutable parts before
the mutation runs and comparing afterwards — so the guarantee holds against any
caller rather than resting on the fact that no current caller has a line that
could reach them. A validator reading one current file has
no prior value to compare against, so it cannot detect a hand edit — and a
self-contained hash chain would not close that either, since an editor who can
change a field can recompute the chain. Detection would need a separate trusted
baseline, which this design deliberately does not build for low-authority notes.

## Refusals

Parsing refuses, before any state is derived:

- a `schema_version` outside the readable range
- a `kind` or `event` outside its closed vocabulary
- unknown fields at any level — `histroy:` would otherwise parse as an empty
  history and `reasno:` as a disposition with no reason, turning a typo into a
  different, well-formed record
- more than one YAML document
