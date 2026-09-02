<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/backlog-and-activity
-->

# Activity Declaration Schema

`spec/intents/<feature>/activity.yaml` — whether a feature's pause was chosen,
and by whom.

A feature that has stopped moving and a feature that was deliberately parked
look identical on disk. `parlay status` cannot tell them apart, so it reports
neither, and a listing that reports a permanent non-problem stops being read.
This file is the missing half: the declaration that somebody decided, rather
than that something lapsed.

Designer-authored, in `spec/` rather than `.parlay/`, because it is a decision a
person makes and reviews. It is written through `parlay park`,
`parlay activate` and `parlay unpark`, never by hand in the ordinary case.

<!-- parlay:normative -->
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

<!-- /parlay:normative -->

## Why the record is colocated

A central index keyed by qualified feature id goes stale the moment
`parlay move-feature` moves a feature between initiatives: the key changes and
the record does not follow. Colocation moves with the directory for free, keeps
unrelated parkings out of each other's merge conflicts, and makes "why is this
parked?" answerable by opening the feature.

The cost is that a project-wide view must walk the features rather than read one
file. That is the right trade for a record whose whole value is being correct
about one feature months later.

## Parking is not retirement

| | Parking | Retirement (`retires_feature:`) |
|---|---|---|
| Applies to | An unbuilt feature not yet finished | A built feature being closed |
| Says | Not now | Not ever, here |
| Record | `activity.yaml` | Terminal amendment |
| Reversible | Yes, `unpark` | No — supersession only |

`parlay park` refuses a built or retired feature for that reason: parking is a
pre-build act, and offering it after the fact would present a reversible pause
where the honest options are an amendment or nothing.

## Versioning

`schema_version: 2`, **migrator chain** policy (see `schema-versioning.schema.md`).
Hand-authorable and unregenerable — nothing upstream can reproduce a person's
decision to pause work — which is the migrator-chain profile, the same one
`coverage-decisions.yaml` and `authored.yaml` carry.

**v1 → v2** admits the `activated` event. The representation is unchanged, so
the migration is an identity; it is registered explicitly all the same, because
the chain is the thing that must exist and the next version that *does* change
shape will be added beside it.

A v1 file is read through the chain and **not rewritten on read** — a command
that promises not to write must not upgrade every declaration it looks at. The
upgrade lands on the next explicit mutation, which is a write somebody asked
for.

The bump is required by the house rule even though nothing outside this work
consumes the schema yet: admitting a new event value changes the domain of an
existing field, the same class recorded for `.code-hashes.yaml`. The reason is
not how many readers exist but what an OLD reader does when handed a new file —
under v1 semantics `activated` is outside the vocabulary and is correctly
refused, and left at v1 that refusal would look like corruption rather than a
version the reader cannot handle.

## Diagnostics

| Code | When it fires |
|---|---|
| `activity-declaration-not-parseable` | Invalid YAML, unknown field, unknown `schema_version`, unknown `event`, or more than one document |
| `activity-declaration-incomplete` | `history` is empty, or an entry is missing `at` or `by` |
| `activity-parked-without-reason` | A `parked` event carries no `reason` |
| `activity-until-on-unparked` | `until` appears on any event other than `parked` |
| `activity-timestamp-not-rfc3339` | `at` is present but does not parse |

## Diagnostics emitted outside the file validator

Two codes in this vocabulary cannot come from `validate --type activity`:

| Code | Emitted by | Why not the validator |
|---|---|---|
| `activity-undeclared` (warning) | `next-activity-review` | There is no file. The finding is the absence of one, and a validator reads what exists. |
| `activity-history-update-forbidden` | `park`, `unpark`, `activate` | A validator reading one current file has no prior value against which to see that history was edited. |

`activity-undeclared` is a warning rather than an error because an undeclared
feature is not malformed — nobody has said anything about it yet, which is the
fact this axis exists to make visible rather than a fault to repair.

History append-only is enforced by snapshotting before the append and comparing
after, so the guarantee holds against any future caller rather than resting on
there being exactly one `append` in the code today.
