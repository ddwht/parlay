# Authoring

> The design authoring loop — capturing user goals as intents, generating complete dialogs from intents, updating dialogs when intents change, and checking coverage.

---

## Author Intents

**Goal**: Describe what users need using simple, human-readable documents that capture goals, context, and constraints.
**Persona**: UX Designer
**Priority**: P0
**Context**: Starting a new feature — the designer needs to capture user goals before any dialogs, surfaces, or code exist.
**Action**: Write markdown documents following the intent schema.
**Objects**: intent, feature

**Constraints**:
- The designer must never need to touch generated code or final specifications
- The only documents the designer works with are intents, dialogs, and surfaces
- The format must support quick iteration on different ideas
- A new intent should be writable in under 5 minutes

**Verify**:
- An intent with only Goal and Persona fields is valid
- An intent with all fields (Goal, Persona, Priority, Context, Action, Objects, Constraints, Verify, Questions) is valid
- Intents are separated by `---` and each starts with `## `

**Questions**:
- What if the designer wants to revise intents after dialogs and surfaces already reference them?
- Should the format support inline comments or annotations for collaboration?

---

## Generate Dialogs from Intents

**Goal**: Generate complete, ready-to-review dialogs from authored intents — full dialog flows with triggers, user/system turns, and edge case branches derived from the intent's Constraints and Verify items — not empty templates.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer has finished authoring intents for a feature. Each intent contains enough information (Goal, Context, Action, Constraints, Verify, Objects) for the agent to produce complete dialog flows that the designer can review and refine — rather than empty templates that require writing from scratch.
**Action**: Run `/parlay-scaffold-dialogs @feature`. The agent reads each intent, identifies ambiguities that would block dialog generation, asks the designer to resolve them first, then generates complete dialogs. For intents that already have dialogs, the agent compares the existing dialog against the current intent and proposes updates for meaningful changes.
**Objects**: intent, dialog, feature

**Constraints**:
- Generated dialogs must be complete — full trigger, happy path with user/system turns, branches for edge cases (derived from Constraints and Verify items), and error handling flows. Not placeholder templates with `==fill in==` markers.
- The agent reads each intent's Goal (what the user wants), Context (when this happens), Action (how it's done), Constraints (rules and edge cases), and Verify (expected outcomes) to produce the dialog content.
- **Ambiguity-first**: if an intent has ambiguity that would affect the dialog (unclear Goal, contradictory Constraints, missing Context for a branching decision), the agent asks the designer to clarify BEFORE generating. The agent does not produce dialogs with gaps — it resolves gaps first, then generates.
- Must not overwrite existing designer-authored dialogs. For intents that already have dialogs, the agent runs the update pass instead of regenerating.
- Each Constraint that implies a user-visible behavior should produce a dialog branch or system turn. Each Verify item that describes an edge case should produce a dialog branch showing that edge case.
- Generated dialogs follow the dialog schema: `### ` heading, `**Trigger**:` field, `User:`/`System:` turns, `#### Branch:` sections for alternatives and edge cases.
- The designer reviews and can edit the generated dialogs — they are high-quality starting points, not final drafts. But they should be complete enough that most dialogs need only minor refinement, not rewriting.

**Verify**:
- Running scaffold-dialogs on a feature with 3 intents and no dialogs produces 3 complete dialog sections with triggers, happy paths, and branches — not 3 empty templates
- Each generated dialog has branches derived from the intent's Constraints (e.g., a constraint about collision detection produces a "Branch: Scope collision" in the dialog)
- Each generated dialog has branches derived from the intent's Verify items that describe edge cases
- If an intent's Goal is ambiguous, the agent asks the designer before generating (not after)
- Existing designer-authored dialogs are never overwritten — only updated via the update pass
- The generated dialogs are complete enough that a designer can review and approve them with minor edits

---

## Update Dialogs from Changed Intents

**Goal**: When intents change after dialogs exist, detect the meaningful differences and generate complete dialog updates — new branches with full content, updated system turns, corrected triggers — presenting each for the designer's approval.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer revised intents — added constraints, changed verify items, renamed an intent, added a new edge case. The corresponding dialogs need updating. The agent compares each dialog against its current intent, generates complete content for any new branches or turns needed, and presents the changes for approval.
**Action**: The update pass runs automatically when `/parlay-scaffold-dialogs @feature` is invoked on a feature where all intents already have dialogs. For each meaningful intent change, the agent generates the complete dialog content (not just a flag or placeholder) and presents it for approval.
**Objects**: intent, dialog, dialog-update, feature

**Constraints**:
- The update pass runs AFTER the generation pass — first generate dialogs for intents that lack them, then update existing dialogs for intents that changed
- For new Constraints: generate a complete dialog branch with trigger, user turn, system response, and any sub-branches — not just "add a branch for X"
- For new Verify edge cases: generate a complete `#### Branch:` section showing the edge case flow
- For renamed intents: propose updating the dialog heading to match
- Cosmetic changes (rewording that preserves meaning) are silently skipped
- Each proposed update is presented with the triggering intent change and the complete proposed dialog content, so the designer can accept, reject, or edit
- The designer's existing dialog prose is preserved — the agent proposes additions alongside existing content, never rewrites what the designer wrote
- Dialogs.md is designer-authored. The agent asks permission before every modification.

**Verify**:
- After adding a new Constraint about error handling, the agent generates a complete dialog branch showing the error flow — user action, system error message, suggested resolution
- After adding a new Verify edge case, the agent generates a complete branch (not just a stub)
- The designer can accept, reject, or modify each proposed update independently
- Running when nothing has changed reports "Dialogs are up to date"

---

## Sync Intents and Dialogs

**Goal**: Identify gaps and drift across the full artifact chain — intents without dialogs, stale downstream artifacts, broken references — and help the designer bring everything back in sync.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer has been authoring or editing intents and dialogs and wants to check that everything is covered and consistent before generating a surface or prototype.
**Action**: AI agent scans all intents and dialogs in a feature, checks coverage and full-chain traceability, detects content drift in intents that changed since the last build, and produces a report with actionable next steps.
**Objects**: intent, dialog, coverage-report, dialog-template, baseline

**Constraints**:
- Generated dialog templates must follow the dialog schema and be immediately editable by the designer
- The sync must not modify existing human-authored files without permission
- The coverage report must clearly distinguish between missing dialogs and dialogs that exist but may not fully cover an intent
- Content drift detection must compare current intents against a stored baseline from the last build
- The agent must review drifted intents against downstream artifacts and flag meaningful mismatches

**Verify**:
- Covered intents are correctly identified (structural + semantic matching)
- Uncovered intents are listed with an option to generate dialog templates
- Orphan dialogs (no matching intent) are identified and reported
- Intents modified since the last build are flagged with the specific fields that changed
- The agent reviews drifted intents against surface/buildfile/testcases and suggests updates
- Existing human-authored dialogs are never modified without permission

---
