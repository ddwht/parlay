# Authoring — Infrastructure

---

## Scaffold-Dialogs Skill Update for Complete Dialog Generation

**Affects**: skill deployment, dialog generation pipeline
**Behavior**: Extend the scaffold-dialogs skill from template scaffolding to complete dialog generation. The skill should read each intent's Goal, Context, Action, Constraints, and Verify items and produce full dialog flows — triggers, happy path user/system turns, branches for every Constraint that implies user-visible behavior, and branches for every Verify item that describes an edge case. When ambiguities are detected in intents, the skill asks the designer to resolve them BEFORE generating. When all intents already have dialogs, the skill compares each dialog against its current intent and proposes complete updates (full branches with content, not stubs) for meaningful changes. The designer approves each change before dialogs.md is modified.
**Invariants**:
- Running scaffold-dialogs on a feature with intents but no dialogs produces complete dialog flows, not empty templates
- Each generated dialog has branches derived from the intent's Constraints and Verify edge cases
- Ambiguities in intents trigger clarifying questions before dialog generation begins
- Running on a feature where all intents have dialogs triggers the update pass
- Each update proposal includes complete dialog content (full branches with turns)
- The designer can accept, reject, or edit each proposed change independently
- Running when nothing has changed reports "Dialogs are up to date"
**Source**: @parlay-tool/authoring/generate-dialogs-from-intents, @parlay-tool/authoring/update-dialogs-from-changed-intents
**Backward-Compatible**: no

**Notes**:
- This is a behavioral change to the skill — it replaces template scaffolding with full dialog generation
- The CLI `parlay create-dialogs` command still handles the mechanical scaffolding fallback when no agent is available; the skill layer adds the intelligence

---
