---
name: create-intents
description: "Guide the authoring and revision of a feature's intents.md, including the soft boundaries that catch an intent drifting toward the solution"
surface: module
---

# Create Intents

Guide the authoring or revision of `spec/intents/{feature}/intents.md`.

Read `intent.schema.md` (or its digest) for the shape: the ten fields, which are
required, and the parse rules. This module is about the part the schema cannot
check — whether what the intent SAYS is an intent at all.

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

<!-- parlay:expand-decision-protocol -->

## Why this module exists

An intent has almost no content rules. Six of its ten fields — Context, Action,
Objects, Constraints, Verify, Questions — carry no structural validation at
all, nothing anywhere forbids interface language, and the whole file freezes at
the feature's first green build.

The founding text is then immutable, but the project is not stuck: current
authority moves on through the amendment ledger, and a founding intent can be
retired with `supersedes_intents:`. What a drifted intent costs is therefore
not permanence — it is that every artifact derived at birth inherits the drift,
and correcting it later means amendments against a contract that was wrong from
the start rather than a conversation before anything was built. Catching it
here costs one exchange. Missing it costs the ledger.

## Freeze pre-flight — run this first

These boundaries are an authoring-time aid, and `intents.md` freezes at the
feature's first green build. Before reviewing anything, run:

```
parlay internal check-applied @{feature}
```

- `has_baseline: true` — the founding docs are frozen. **Do not review the
  prose and do not raise a single wording finding.** There is no legal edit, so
  a finding could only nag. If something about the founding text genuinely
  looks wrong, the question is whether the CONTRACT is wrong today: check the
  amendment ledger and the contract artifacts, and route a real problem to
  `/parlay-refine`. Clumsy founding wording with a correct contract is left
  alone — history is allowed to be imperfect.
- `has_baseline: false` and an empty `amendments` ledger — the feature is at
  birth. Review normally.
- The probe cannot be read or does not parse — **stop and say so**. Do not
  infer freeze from whether a buildfile or artifacts happen to exist: guessing
  wrong in the permissive direction means advising edits to a frozen document,
  which is the one outcome this pre-flight exists to prevent.

## The one failure mode

Intents drift toward the **solution**. Not toward being vague — toward being
specific about the wrong thing: the screen that will show it, the control that
will trigger it, the record the system will write.

This happens because the author usually knows what they are going to build, and
describing the build is easier than describing the need. `Action` is where it
enters most often, since it is the field that asks *how*.

## Two rules that govern every boundary

**Route, do not delete.** Drifted content is usually misplaced, not wrong. A
technical mechanism belongs in `infrastructure.md`; an interaction belongs in
`dialogs.md`; a rendering belongs in the surface. Always say where it goes —
an author who is told only that something is wrong will delete real
information.

**Domain is per product.** "Interface noun" is not a fixed list. `Component`,
`Route` and `Screen` are domain concepts for a UI builder; `command-argument`
and `generated-file` are domain concepts for a CLI toolkit, and parlay's own
intents use them correctly. The test is whether a term is meaningful in THIS
product's domain, never whether it sounds like a widget. Getting this wrong
produces confident, wrong corrections on exactly the projects whose domain IS
software.

## Read the backlog for this feature — ON ENTRY, and only when invoked standalone

```
parlay backlog list --related @{feature} --open --json
```

**Before any phase work below.** This section sits above the procedure because
that is when it runs — the contract is that the USER decides whether a hit
enters scope, and a read performed after you have already written has nothing
left to decide about. It said so from the bottom of the file once, which
asserted an order it did not have.

**Only when you were invoked on your own.** Inside a chained designer run the
phase-group has already done this once for the whole group, and repeating it
per phase is the cost the scoped read-set exists to remove.

Report what it returns — titles and ids — and let the user decide whether any
of it enters scope. Never fold a backlog item into the work on your own
initiative: cheap capture only stays safe if capture cannot silently become
commitment. Read `counts` for this listing, not `project_totals`, which
describes the whole project regardless of the filter.

A failed read must never fail the phase.

## Working the boundaries

`intent.schema.md` § Soft boundaries carries the full table, marked
**Obligation** or **Heuristic**. Apply them like this:

1. Read the block as a whole first, for cohesion, before looking at fields.
   Every field can be individually clean and still be stitched from different
   intents. Read it as a sentence: *this role, in this situation, does this to
   these things, so that this outcome holds, and here is how you would know.*
   If that sentence does not survive, the intent is describing more than one
   thing — and that is worth raising before any wording note.

2. Then the two critical fields, `Action` and `Objects`. These carry the drift
   most often and are the hardest to repair later, because dialogs and surface
   derive from them directly.

3. Then the rest.

## Worked examples

Drawn from a tax-filing domain. The bad column is not a strawman — each is the
shape an author reaches for when they already know the screen.

| Field | Drifted | Repaired |
|---|---|---|
| Goal | "create an object in the accounting system" | "be in good standing with the tax authorities" |
| Persona | "accountant" | "a person sending the tax report" |
| Context | "the user opens the reports page" | "the quarterly reporting period has closed" |
| Action | "upload the tax report to the system" | "fill in the tax report and send it to the tax authority" |
| Objects | "tax modal window, send report button" | "tax report, tax number" |
| Constraints | "the screen fits only 10 tax entities" | "the tax report must be sent before 1 March" |

Note what the `Action` repair keeps: it is still an action performed largely in
software. The correction is task-level versus control-level, NOT outside versus
inside the product. "Approve the invoice" and "reconcile the account" are
proper actions. What drifts is "click Approve", "open the reconciliation
screen", "select from the dropdown".

Note what the `Constraints` repair loses: nothing. "The screen fits only 10
entries" is a real constraint — it belongs in `infrastructure.md`. Say so.

## How to present findings

Never rewrite an intent silently, and never present a finding as a verdict.
Offer three responses per finding:

- **Revise** — the suggested wording, so the author can accept it directly.
- **Keep, with reason** — the author knows their domain. `Objects: Route` is
  correct for a UI builder, and being asked twice is worse than not being
  asked. **This suppression lasts for the current authoring conversation
  only** — there is nowhere durable to record it, and claiming otherwise would
  promise something no later session can honour. If an exception is worth
  keeping permanently, fold the reason into the wording so the intent explains
  itself to the next reader (`Objects: Route (a routing entry the user
  authors, not a navigation target)`). That survives; a remembered decision
  does not.
- **Reroute** — the content is real and belongs elsewhere. Name the artifact
  and carry the text over rather than making the author retype it.
  **Never remove the only copy before the destination exists.** Dialogs and
  artifacts are later phases, and a user may stop after intents; a phase
  decision is not durable workflow state. So either complete the move within
  this session, or leave the text in place and record the intended destination
  in the phase-boundary `context:`. "Route, do not delete" must not itself
  delete information between sittings.

Fold the findings into the intents phase-boundary decision's `context:`,
alongside the existing gap analysis. They are advisory, and that has a concrete
consequence for the decision you raise: a soft-boundary finding never becomes
the `question:`, never removes the advancing option, and never changes the
`default:`. It is context the user reads while deciding, not a reason the
phase cannot end. A run under `--non-interactive` therefore advances past every
finding here, which is correct — wording is not a gate.

## What not to do

- **Do not lint by word list.** These are semantic judgments. A word list fires
  on "form" in a medical product and misses "the place where they pick the
  thing" in any product.
- **Do not check `Objects` against `domain-model.yaml` here.** The dependency
  runs the other way at birth: `create-domain-model` derives entities FROM
  Objects, so a missing entry means "not modelled yet" far more often than it
  means drift. And plenty of legitimate Objects — documents, policies, events,
  contextual nouns — never become entities at all.
- **Do not apply any of this to a frozen `intents.md`.** After the first green
  build there is no legal edit, so a finding could only nag. If a concern
  surfaces later in `/parlay-doctor` or `/parlay-refine`, route it to current
  authority: if the CONTRACT is wrong today, record an amendment or supersede
  the intent; if only the founding wording is clumsy, do nothing. History is
  allowed to be imperfect.

## Capturing what you notice and do not do

Mid-phase you will find things: a defect, a gap the spec never covered, a
shortcut you took knowingly. Record them instead of mentioning them.

```
parlay note --kind gap --title "..." --by <you> [--feature @{feature}] [--phase {phase}] \
            [--evidence path:line] [--about @{feature}/operation:x]
```

**What to capture.** A concrete, evidenced piece of undone work, or an explicit
later/defer/out-of-scope statement from the user. Never speculation, never a
generic suggestion, never work already recorded. Every phase boundary reports
the ids captured during it, so noise is visible rather than silent — an
over-eager capture is caught at the next confirmation, not three weeks later.

**Never guess a priority.** Pass `--priority` only when a person actually ranked
it. Absent means untriaged, which is a fact about the record; a guessed rank
looks like a judgment and is not.

**A failed capture must never fail your phase.** The writer is strict — malformed
input is refused rather than written as a corrupt record — but that refusal is
yours to absorb. Report it in `notes:` and carry on.

**Note vs. `decisions:`.** If you wrote it into a file, it is a decision. If you
walked past it, it is a note.
