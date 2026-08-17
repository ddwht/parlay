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
