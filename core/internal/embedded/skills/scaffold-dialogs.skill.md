---
name: scaffold-dialogs
description: "Scaffold dialog templates from intents"
surface: module
---

# Scaffold Dialogs

Generate complete dialogs from authored intents, and update existing dialogs when intents change.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

<!-- parlay:expand-decision-protocol -->

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

## Steps

1. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md` (may not exist yet)

2. **Check for ambiguities** — Before generating, scan each intent for ambiguities that would affect the dialog (unclear Goal, contradictory Constraints, missing Context for branching decisions). If found, raise an `ambiguity` decision request BEFORE generating anything (see **Asking the user**). Do not generate dialogs with gaps, and do not resolve an ambiguity by picking the reading that is easiest to write a dialog for.

3. **Generate dialogs for uncovered intents** — For each intent that has no matching dialog:
   - Read the intent's Goal, Context, Action, Constraints, Verify, and Objects
   - Generate a complete dialog flow: trigger, happy path with user/system turns, branches for each Constraint that implies user-visible behavior, branches for each Verify item that describes an edge case
   - The generated dialogs should be complete enough that the designer can review and approve with minor edits — not empty templates requiring rewriting
   - Run `parlay create-dialogs @{feature}` for the mechanical scaffolding, then enrich each template with full content

4. **Never re-sync existing dialogs** — dialogs freeze at first build as
   founding documents. Intents cannot have changed underneath them
   (check-drift enforces the freeze), and post-birth change is recorded in
   the amendment ledger instead of re-synced into the transcript. This
   skill's job is step 3 only, at birth. If a dialog looks out of date with
   the current behavior, that is the ledger working as designed — the
   transcript records the founding conversation, and the contract artifacts
   carry current truth.

5. **Report** — Summarize: how many dialogs were generated. If every intent is covered: "Dialogs are up to date — no updates needed."

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
