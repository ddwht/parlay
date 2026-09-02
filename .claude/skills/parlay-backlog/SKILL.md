---
name: parlay-backlog
description: "Parlay: Work through what was noticed and not done — one item at a time, with a decision at the end"
---

# Backlog

Run a triage sitting. One item, one decision, then the next.

**Why this is a session and not a listing.** A backlog that only grows is a
graveyard, and a graveyard is worse than nothing because the absence of
tracking looks like tracking. A wall of items converts into decisions at
roughly the rate of zero; one question with its evidence attached converts at
the rate people actually answer questions. So this hands over one at a time.

This is the other half of `parlay note`. Capture is deliberately cheap — no
prompt, no triage, no priority unless somebody gave one — because a capture
that costs a round trip gets skipped in exactly the situation it exists for.
Closing is deliberately not cheap: a closed vocabulary and an attribution,
because closing is where things get silently lost. Beyond that the requirement
splits — `decline`, `obsolete` and `fix` each carry a **reason**, while `fold`,
`promote` and `amend` each carry a **typed destination** instead, because what
the work became is the record and a reason would only restate it. (`defer` also
carries a reason, though it closes nothing.)

## Arguments

- `feature` (optional): limit the sitting to one feature's items, in standard
  parlay form — `{feature}` or `@{initiative}/{feature}`. Omit for everything.

<!-- parlay:active-root-aware -->

## 1. Report the shape of it

```
parlay backlog list
```

Untriaged first, because an unranked item is one nobody has considered and
that is the work triage exists to do. Read three things before you start:

- **`root_errors`** (or the `could not be enumerated` lines). A root that could
  not be read has not been checked, and nothing you find today covers it. Say
  so before reporting any count as the project's.
- **`record(s) could not be read`**. An item nobody can parse still needs a
  person. It is NOT in the counts.
- **`findings`** — cross-file faults: a `becomes:` that stopped resolving, an
  amendment whose trigger no longer names the item, a target nobody could read,
  an item open past the age bucket. These concern items that are mostly already
  CLOSED, so nothing else will ever surface them. Report the code and the fix.
- **counts vs project_totals** when you pass `--json`. `counts` describes the
  listing in front of you; `project_totals` describes the project. Reporting
  one as the other is the trap those two fields exist to keep apart.

Every row carries its root. Two roots may hold the same id prefix or the same
feature slug, so a row without its root is one nobody can act on — always
carry it into whatever you run next.

Parked features are in the same listing, deliberately. Somebody asking "what
are we not doing?" should not have to remember the answer lives in two shapes.
They are different records — an item has no home yet, a parked feature has one
— and the listing keeps them apart without making you run a second command.

## 2. Walk it, one item at a time

```
parlay internal next-backlog-review
```

One subject with its capture provenance, its evidence, and — the part that
matters — **any prior deferrals**. Somebody may already have looked and been
unable to decide; the next reviewer starts from that rather than from nothing,
which is the whole reason a deferral is recorded rather than the item simply
being skipped.

Present the subject as given and build the chooser from its `options`. Do not
invent options and do not add a default: a default would let an unattended run
close somebody's observation, which is precisely the judgment this exists to
route to a person.

Each option carries both `command` (pasteable) and `argv` (already split).
**Use `argv`** when you are running it yourself — a root name or a reason
containing a space does not survive splitting the shell text.

Re-run with `--exclude {exclude}` per item handled, using the token the
subject gives you rather than a bare id: it is root-qualified, and an
unqualified exclusion can skip the wrong item.

Stop when `subject` is absent — but read `note` first. It distinguishes
"nothing left" from "everything was excluded this sitting", from "a root could
not be read", from "items could not be parsed". Only the first is done.

## 3. The decisions, and what they mean

| Verb | When | Records |
|---|---|---|
| `defer` | Looked, cannot decide | The attempt. **The item stays open.** |
| `decline` | We are choosing not to do this | A reason |
| `obsolete` | The condition that produced it is gone | A reason |
| `fold` | It is really the same as another item | Which **item** it merged into |
| `fix` | The work was done directly | A reason saying what was done |
| `amend` | An amendment landed and carries it | Which **amendment** carries it |
| `promote` | It should become work | A feature, or an amendment |

**`fold` and `amend` are not interchangeable either.** `fold --into` takes
another backlog item id; `amend --into` takes an amendment ref
(`@feature/NNN-slug`) and refuses unless that amendment is on disk. Folded
means the work merged into another observation; amended means it landed as a
change to a promise the project had already made. Recording one as the other
loses the difference permanently, because the item is closed and nobody
revisits it.

**Three endings produce nothing the pipeline carries**, and they are not interchangeable:
`declined` is a choice not to do it, `obsolete` is the condition going away, and
`fix` is somebody having changed the system so it no longer holds. Reach for
`fix` when a defect, gap or debt was corrected directly — not for an idea that
was implemented, which should prompt the question of whether it bypassed the
promise model.

**`declined` and `obsolete` are not the same**, and the difference is the whole
content of the decision: one is a choice not to do it, the other is the
condition having gone away. A reader months later cannot recover that from
silence.

**A deferral is not an answer.** Attempts accumulate and never close the item.
Two people independently unable to decide is a different fact from one attempt
overwritten twice, and recording it is what stops the next reviewer starting
from nothing.

**Ranking is not a disposition.** `parlay backlog edit <id> --priority P1`
leaves the item open. The review offers it only on items nobody has ranked,
because offering it on a ranked one would suggest re-ranking is part of triage.
Never rank an item yourself: absent means untriaged, and a guessed priority is
worse than an absent one because it looks like a judgment and is not.

## 4. Promotion — the only exit that produces something

```
parlay promote <item> --as-feature <name> [--initiative X] --by <who>
parlay promote <item> --as-amendment @feature --by <who>
```

Two genuinely different acts. A **feature** is a promise the project has not
made yet; an **amendment** changes one it already has. If the target feature
does not exist, the act is `--as-feature`.

**One feature, one promotion.** If two items name the same feature, one wins
and the other **stays open** — whether two observations are really the same
work is a judgment for a person, not for whichever call arrived second. Decide
it, then either `fold` the loser into the winner or promote it to its own
feature.

**An interrupted promotion resumes.** The scaffold is a sequence of writes, not
a transaction, so a run can die with the feature half-created and the item
still open. Just run the same command again: promotion records a durable claim
on the target before it creates anything, so a re-run completes its own partial
work instead of mistaking it for somebody else's feature. It never writes a
second origin link, never overwrites a file that is already there, and still
refuses a feature carrying a different item's origin. If it reports the target
belongs to another item, that is the real answer, not a stuck state.

`--as-feature` writes the standard zero-intent scaffold and **seeds no Goal**.
An implementation observation is usually not a user-world outcome and has no
Persona, so seeding one manufactures exactly the malformed intent the scaffold
warns against — and the feature would then parse as having an intent it does
not have, which is the state `planned` exists to distinguish. The observation
lands as a non-parsing `backlog-origin` link; the intents phase translates it
into a real promise.

`--as-amendment` takes a **feature**, not a contract entry — `@widget`, not
`@widget/operation:rename`. The amendment is authored against the feature and
declares the entries it affects itself.

It writes nothing. It emits the pre-filled `trigger: backlog:<id>` for
`/parlay-refine`, because an amendment is authored with a person in the loop
and a command that wrote one alone would be recording a decision nobody made.

**The item stays OPEN until the amendment exists.** Once `/parlay-refine` has
written it to disk, close the item with — application is NOT a prerequisite,
for the reason stated below:

```
parlay backlog amend <item> --into @feature/NNN-slug --by <who>
```

That command refuses unless the amendment exists **and carries
`trigger: backlog:<id>` for this item**. Existence alone would let any
amendment close any item, manufacturing the causal link the trigger exists to
record — so the amendment has to name the item itself.

**"Lands" means authored on disk, not applied.** An amendment is a decision the
moment it is written; application is a separate later act with its own gate,
and an item held open until then would keep being handed to reviewers with
nothing left to decide.

**A priority travels as a PROPOSAL, never as a decided rank.** A `debt` item
ranked for its blast radius in the codebase is exactly the case the intents
phase must re-judge against the user outcome rather than copy.

The item is **retained as provenance**, never moved and never duplicated into
active requirements. Months later, "where did this feature come from" has an
answer nobody had to remember to keep.

## 5. Ending the sitting

Report what was decided and what remains. **A sitting that resolves four of
seventeen is four more than none** — do not push for the whole queue, and do
not close items to reach the end of it.

Name anything you could not reach: unreadable records, roots that failed,
unresolved `findings`, items excluded but not decided. A sitting that reported "done" while any of
those stood would be making the same false claim the backlog exists to prevent.

## What does not belong here

The backlog admits work that could become a feature, an amendment or an intent — a defect found
while generating code, a gap the dialogs never covered, a deliberate shortcut
with a known better shape. It excludes a flaky CI job or a dependency bump:
anything that could never resolve into a feature, an amendment or an intent. If the tool has no way to represent it, it does not belong here.

And backlog entries stay capture-and-triage notes. Rich intents and dialogs
belong in features — **the backlog is not a second requirements system.** If an
item is growing a specification, that is the signal to promote it.
