# Activity Declaration Schema — authoring digest

Derived from `activity.schema.md` at deploy time. Never hand-edit: edit the schema's
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
| `schema_version` | yes | `2`. Versions 1–2 are readable through the migrator chain; anything outside that range is **refused**, not read leniently — a binary that cannot know a version's semantics must decline rather than guess. |
| `history` | yes | Append-only list of declarations, oldest first. At least one entry: a declaration with no events declares nothing. |

There is no `state:` field. Current activity is **derived** from `history` — the
latest event wins — so a stored status can never disagree with the events that
produced it.

## `history` entries

| Field | Required | Description |
|---|---|---|
| `event` | yes | Closed at `parked \| unparked \| activated`. Any other value is refused. `activated` requires `schema_version: 2` — a v1 file carrying it is invalid, which is the whole content of the bump. |
| `reason` | on `parked` | Why the work was paused. Required on `parked`: a pause with no stated reason is indistinguishable from neglect, which is the state this file exists to replace. |
| `until` | no, `parked` only | Free text describing the condition that would end the pause (`"after adapter-set v2 lands"`). Forbidden on any other event. |
| `at` | yes | RFC3339 timestamp. |
| `by` | yes | Who decided. A declaration nobody can attribute tells the next reader nothing they did not already know. |

## Derived state

| History | Reported |
|---|---|
| Latest event `parked` | `parked` |
| Latest event `unparked` | `active` — a pause ended |
| Latest event `activated` | `active` — there was never a pause |
| No file | see below |

The file alone cannot report the third state. `unclassified` is resolved by the
caller from two facts together: no declaration **and** no observed pipeline
boundary. A feature that has passed or been blocked at a boundary is
demonstrably being worked on, and reporting a missing disposition for work whose
activity is already evident is the permanent non-problem this artifact exists to
remove.

**A declaration outranks observation.** A parked feature that still passes a gate
reads `parked`: somebody said so, and the gate did not.

## Ordering

`history` is read in **append order**, not timestamp order. Nothing depends on
clocks being monotonic or even correct; `at` is attribution, not sequencing.

## `unparked` and `activated` are not the same event

`unparked` ends a pause. `activated` says somebody looked at a feature nobody
had classified and confirmed it is live.

Both derive `active`, and the distinction still matters. Writing `unparked` for
a feature that was never parked puts a false statement into the one record whose
value is being literally true months later — and `LatestParking` walks backwards
looking for exactly that event as evidence a pause existed. Two different facts
get two different records.

It is also what lets a triage finish. Without `activated` the only declarable
state is `parked`, so a feature somebody has examined and judged active has
nowhere to record that, stays `unclassified`, and comes back every sitting.

## Refusals

Parsing refuses, before any state is derived:

- an unknown `schema_version`
- an `event` outside the closed vocabulary
- unknown fields at any level — a misspelled `histroy:` would otherwise parse as
  an empty history and `reasno:` as a parking with no reason, turning a typo
  into a different, well-formed record
- more than one YAML document

A file that exists but cannot be read or parsed is reported as an unavailable
declaration, never as `unclassified` and never as its last known state.
