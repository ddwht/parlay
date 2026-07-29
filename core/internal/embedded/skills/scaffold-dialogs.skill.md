---
name: scaffold-dialogs
description: "Scaffold dialog templates from intents"
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

4. **Update existing dialogs** — For intents that already have dialogs, compare each dialog against its current intent:
   - For new Constraints: generate a complete `#### Branch:` section with user turn, system response, and any sub-branches
   - For new Verify edge cases: generate a complete branch showing the edge case flow
   - For renamed intents: propose updating the dialog heading
   - Skip cosmetic changes (rewording that preserves meaning)
   - Raise one decision request carrying every proposed update: the triggering intent change and the complete proposed content for each, with a per-update option id
   - `dialogs.md` is designer-authored — modify it only after that decision comes back

5. **Report** — Summarize: how many dialogs were generated, how many updates were proposed, how many were accepted/skipped. If everything is current: "Dialogs are up to date — no updates needed."
