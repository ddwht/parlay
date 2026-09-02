<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/backlog-and-activity
-->

# Backlog Item Schema

`<activeRoot>/spec/backlog/<id>.yaml` — one piece of work that was observed and
not done.

Parlay records the code, the decisions enforced in the code, the drift, the
coverage judgments and the amendment history. It records nothing about work it
*noticed and walked past*. Mid-build an agent finds a defect, a gap the spec
never covered, a shortcut taken knowingly — says so, and the session ends. This
file is where that goes instead.

In `spec/` rather than `.parlay/`: an item is design intent a person reads and
decides on, and the `.parlay/` zone rule is explicit that it is never
user-facing.

<!-- parlay:normative -->
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

<!-- /parlay:normative -->

## `folded` and `amended` are not the same, and neither are their commands

`folded` names another **backlog item**; `amended` names an **amendment**. The
work merging into another observation and the work landing as a change to an
existing promise are different outcomes, and a reader months later cannot
recover which from silence.

The commands enforce it. `parlay backlog fold --into` resolves a backlog item
id and rejects an amendment ref; `parlay backlog amend --into` takes
`@feature/NNN-slug` and refuses unless that amendment exists **and its own
`trigger:` reads exactly `backlog:<id>` for the item being closed.**

Existence alone is not enough, and this is the reason: without the trigger check
any amendment on the feature could close any open item, manufacturing exactly
the causal link the field exists to record. The amendment has to say so itself.
A missing trigger, a trigger naming a different item, and an amendment that
will not parse are all refused with the item untouched.

**The boundary is AUTHORED-ON-DISK, not applied.** An amendment is a decision
the moment it is written — it is append-only and never edited — whereas
application is a separate later act with its own gate. An item that stayed open
until application would keep being handed to reviewers who have nothing left to
decide about it. Stated here so nobody later assumes the stronger meaning.

`parlay promote --as-amendment` therefore closes nothing and leaves the item
open until the amendment exists.

## Promotion is exclusive per target, and resumable

Two items cannot both promote to one feature name: one wins, the loser stays
**open**, because deciding whether two observations are the same work belongs
to a person.

That exclusivity must not cost the repair/retry property. The scaffold is a
sequence of writes rather than a transaction, so dying between creating the
feature and recording `promoted` is ordinary — and directory existence alone
cannot tell *our interrupted scaffold* from *somebody else's feature*,
especially when the interruption happened before anything identifying was
written. So promotion records a durable reservation naming target and item
before it creates anything, under `.parlay/`, which is tool-internal.

The reservation is written through the same store the ledger uses, which holds
the rename through a directory fsync — `atomicfile` syncs the temp file but not
the parent directory, so a reservation written that way could be missing after a
power loss, which is the one failure it exists to survive.

A re-run then resumes: it re-runs the whole conditional scaffold (no single file
marks it complete — a crash between `intents.md` and `dialogs.md`, or before the
handoff and build trees exist, leaves a tree that must still be finished),
writes the origin only if absent, never overwrites an existing file, and appends
exactly one terminal event.

A reservation is stale only when its holder is **readably** gone or closed. An
unparseable or unreadable holder record makes promotion unavailable and the
reservation stands: handing the target away is exactly wrong at the moment
ownership cannot be verified. A feature carrying a different item's origin is
still refused.

## Three endings that produce no parlay object

`declined`, `obsolete` and `fixed` all close an item without it becoming a
feature or an amendment, and they are kept apart by the same recoverability test
that separates the first two: months later, **"we chose not to do this"**, **"the
condition disappeared"** and **"somebody changed the system so the condition no
longer holds"** are three different facts, and prose should not have to
reconstruct which one applied.

`fixed` is deliberately **narrow**. `resolved` or `completed` would fit more
endings and become a catch-all that pulls the model toward generic task
tracking — which §16 excludes along with owners, estimates and sprints. It suits
a `defect`, a `gap` or a `debt` corrected directly. An `idea` implemented without
becoming a feature or an amendment should prompt the question of whether it
bypassed the promise model, not be hidden under a broader word.

It requires a `reason` (what was done) and **forbids `becomes:`**. `becomes` is a
typed lifecycle edge to a parlay object or another item, not a place for a
commit, a path or free text; overloading it would weaken every cross-file check
that resolves it. If event-specific evidence is ever needed it gets its own
structured field.

## `declined` and `obsolete` are not the same

Kept distinct for the reason `amendment.schema.md` keeps `replaced` and
`obsolete` distinct: *a reader months later cannot recover from silence whether
the work moved or stopped mattering, and that difference is the whole content of
the decision.* `declined` is a choice not to do it; `obsolete` is the condition
that produced it having gone away.

## `about` holds refs, and the vocabulary is closed

Two shapes, both validated on write:

- a **contract ref** — `@feature/<kind>:<name>`, kind one of
  `operation | surface | infrastructure | domain`, the same grammar amendments
  use and parsed by the same parser
- a **bare feature ref** — `@feature`, or `@initiative/feature`

Closed rather than forward-compatible, deliberately. `about` was free text, and
the cost was not cosmetic: a scoped read resolves these refs to decide what an
item concerns, so a misspelled or invented ref became **durable state the read
silently missed** — the item stored, listed under no feature, and nothing
reporting that it went nowhere. An unknown kind is refused at the boundary
because storing it for a future reader buys nothing a future reader can use.

When a fifth kind joins the contract vocabulary it is added to the canonical
parser and to this check together. That is a smaller cost than a class of
items that are invisible to every query that would surface them.

## Priority uses the intent scale, with the intent meaning

`P0 | P1 | P2`, the same closed vocabulary as `intents.md`.

Reusing the tokens is only safe because the meaning transfers.
`intent.schema.md` defines Priority as ranking *"the cost of leaving the USER
OUTCOME unmet — not implementation order, technical risk, or ease."* A backlog
item is observed work whose absence has a cost, and it is that cost the rank
orders. The danger was never the tokens; it was attaching *scheduling* semantics
to a scale the project has already defined as impact.

The mapping is direct for `gap` and most `defect` items. It is looser for
`debt`, whose cost reaches a user only through the code carrying it — so a rank
carries into promotion as a **proposal** the intents phase confirms, never an
inheritance. A `debt` item ranked P0 for its blast radius in the codebase is
exactly the case that must be re-judged rather than copied.

**Never inferred.** An agent sets a priority only when a person actually ranked
it. A guessed priority is worse than an absent one: it looks like a judgment and
is not.

## Why ids are not sequential

`amendments/` uses `NNN-`, and this deliberately does not. Phase agents record
mid-run and in parallel, so sequential allocation is a race — two agents pick
the same number and one record is lost. Approximate order with no lost records
beats exact order that drops one.

**What the id guarantees, precisely.** It is lexically time-sortable, so sorting
a listing puts earlier captures first *whenever their timestamps differ*. It is
NOT a total capture order: timestamps can tie, wall clocks can repeat or step
backwards, and two concurrent processes have no shared ordering to observe at
all. For equal timestamps the random suffix decides, which is arbitrary rather
than chronological. Microsecond precision makes ties rarer; rarer is not never.

The id's timestamp and `captured.at` are the same instant, taken once — so an
item cannot disagree with itself about when it was observed. The slug makes the
filename legible without opening it.

## Scope — what belongs here

> **Parlay tracks work that has been observed but not yet expressed as intent.**

Admits: a defect found while generating code, a gap the dialogs never covered,
an idea nobody has asked for, a deliberate shortcut with a known better shape.

Excludes: a flaky CI job, a dependency bump — anything that could never resolve
into a feature, an amendment or an intent. If it can never become a parlay
object, it does not belong here.

Backlog descriptions stay capture-and-triage notes. Rich intents and dialogs
belong only in features: **the backlog is not a second requirements system.**

## Versioning

`schema_version: 2`, **migrator chain** policy (see
`schema-versioning.schema.md`). Hand-authorable and unregenerable — nothing
upstream can reproduce an observation somebody made — which is the
migrator-chain profile. The walk refuses a missing link, a link that does not
advance exactly one version, and a walk that ends anywhere but the current
version.

**v2 admits `fixed`**, changing the domain of an existing field. The v1→v2 link
is structurally an identity — no existing value changes meaning — and exists so
the walk has no hole and a v1 file upgrades on its next explicit write.

The event vocabulary is checked against the **file's declared version, before
migration**. A v1 file carrying `fixed` is refused rather than migrated: the
version field is a claim about which vocabulary the file was written against,
and a file contradicting its own claim is not an old file to upgrade, it is one
whose meaning is not established.

## Diagnostics

**These codes supersede §14 of `docs/plans/backlog-support-proposal.md`.** The
proposal named `backlog-item-frontmatter-incomplete`, which is renamed here
because a YAML item has no frontmatter. The other three it named —
`backlog-fold-dangling`, `backlog-promotion-dangling` and `backlog-item-stale`
— are **cross-file**: they ask whether a ref resolves against the project, or
how long an item has been open, and a validator reading one current file can
answer neither. They are emitted by `parlay backlog list` and
`parlay internal next-backlog-review`, which hold the whole inventory.

Two more, `backlog-captured-update-forbidden` and
`backlog-history-update-forbidden`, are emitted by the **mutation commands**
and cannot be emitted by the validator either: it has no prior value to compare
against. See *Mutability* above for the exact extent of that guarantee.

| Code | When it fires |
|---|---|
| `backlog-item-not-parseable` | Invalid YAML, unknown field, unknown `schema_version`, unknown `kind` or `event`, or more than one document |
| `backlog-item-incomplete` | `id`, `title` or `kind` missing, or a wrong `schema_version` on a constructed item |
| `backlog-item-capture-incomplete` | `captured.at`/`captured.by` missing, or a history event missing `at`/`by` |
| `backlog-item-priority-invalid` | `priority` is present but not `P0`, `P1` or `P2` |
| `backlog-about-ref-invalid` | An `about` entry is not a parlay ref |
| `backlog-timestamp-not-rfc3339` | A timestamp does not parse |
| `backlog-disposition-incomplete` | A terminal event missing its `becomes`, carrying one it must not, or missing its `reason` |
| `backlog-terminal-event-not-last` | An event follows a terminal one |

### Cross-file — from `backlog list` and `next-backlog-review`

| Code | When it fires |
|---|---|
| `backlog-fold-dangling` | `folded` names an item that is not in the root |
| `backlog-promotion-dangling` | `promoted` names a missing feature, or `amended` names an amendment that is missing or no longer names this item |
| `backlog-item-stale` (warning) | Open and undecided for 90 days or more |
| `backlog-promotion-target-unavailable` | A `becomes:` target exists but could not be read, or exists twice |

An item is closed exactly once and never revisited, so a `becomes:` that has
stopped resolving is a permanently wrong answer to "what did this become" that
nothing else would surface. The mutation commands prevent *creating* one —
`fold` resolves its destination and requires it open, `promote` scaffolds before
it records — but a feature can be deleted or renamed afterwards.

**The event kind decides which object is looked for.** `promoted` resolves a
FEATURE ref and only a feature; `amended` resolves an AMENDMENT ref and only an
amendment. They are not interchangeable, and a shared lookup that tried one then
the other let the wrong object mask a dangling ref — an item amended into
`@widget/001-filter` would resolve clean whenever an initiative-qualified
feature happened to be named `widget/001-filter`.

**Unavailable is reported, not swallowed.** A target that cannot be read gets
its own code rather than no finding at all. The tri-state is only honest if it
reaches the surface: a corrupt amendment that produced no output was worse than
one mislabelled as deleted, because nothing said anything was wrong. Each
resolution also carries **its own remedy** — deletion gets removal guidance,
unreadability gets repair guidance and accuses the record of nothing, and
trigger drift says the causal link changed. A finding whose message and fix
disagree is worse than no finding.

Neither remedy tells anyone to edit their way out. Amendments and `history` are
both append-only, so a broken provenance link needs repair carrying the same
authority the closure did — not an ordinary mutation.

**Split history is unavailable, never resolved.** A record present in both
`amendments/` and `amendments/archive/` is duplicate history, which the ledger
treats as an integrity fault. Both candidates are examined; resolving on the
first success would let a valid archived copy mask an unreadable or conflicting
live duplicate — clean against exactly the state that is wrong.

**An amendment must still name the item.** Existence is not enough: the record
is parsed, and its `trigger:` must still read `backlog:<id>` for this item. That
was required at the moment the item was closed, so a trigger that has since
changed is exactly the post-mutation drift these checks exist to catch, and it
is reported distinctly from a deletion. An amendment that exists and will not
parse is **unavailable**, never missing — `os.Stat` proves a file is there, not
that it is a readable amendment.

**Compaction is not deletion.** `parlay internal compact` moves applied records
into `amendments/archive/`, which is retained ledger history, so amendment
resolution looks in both directories. Looking only in `amendments/` would turn
every routine compaction into a dangling link on every item closed against a
record it archived.

**Absence and unavailability are different answers.** A MISSING target fires on a
positive finding of absence — we looked where it would be and it is not there.
Causal-link drift is a separate outcome under the same code: the amendment
exists and readably no longer names this item, which is not absence at all but a
link that stopped holding, and the finding says so in its own words. A target
nobody could read is neither, and has its own code. A destination file that exists and will not parse, or a target that
cannot be stat'ed, produces no dangling finding: it is already reported under
its own honest diagnosis, and adding "removed or renamed" on top would be one
command making two contradictory claims about one file in a single run.

The age bucket is **90 days**, a chosen default rather than a derived constant:
a quarter with no disposition is where "we have not decided yet" has become
"nobody is going to". It is always a warning and never a refusal — age is
evidence that an item has been waiting, not that it is wrong.

**Prior deferrals do not reset it.** A deferral is review context for the next
reviewer, not a fresh lease; treating it as one would let an item stay
permanently invisible by being repeatedly not-decided, which is the failure the
age signal exists to surface.

### Mutation-time — from the commands that write

| Code | When it fires |
|---|---|
| `backlog-captured-update-forbidden` | A mutation would change any `captured` field |
| `backlog-history-update-forbidden` | A mutation would edit or remove an existing history event |
