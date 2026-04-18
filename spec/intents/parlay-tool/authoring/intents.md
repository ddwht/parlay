# Authoring

> The design authoring loop — capturing user goals as intents, scaffolding dialog templates, and checking coverage between intents and dialogs.

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

## Scaffold Dialogs from Intents

**Goal**: Generate dialog templates from authored intents so the designer has a starting point for writing user-system conversations.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer has finished authoring intents for a feature and is ready to describe the user-system interactions.
**Action**: Run a command that reads intents and generates one dialog template per intent, pre-filled from Goal and Action fields.
**Objects**: intent, dialog, dialog-template, feature

**Constraints**:
- Generated templates must follow the dialog schema and be immediately editable
- Templates are a starting point — the designer owns and rewrites them
- Must not overwrite existing dialogs if some have already been authored

**Verify**:
- Each generated template has a `### ` heading and `**Trigger**:` field
- Existing dialogs in the file are preserved when new templates are appended
- Templates pre-fill `User:` and `System:` turns from the intent's Goal and Action

**Questions**:
- Should the command generate one dialog per intent, or group related intents into a single dialog?
- What if some intents are too abstract to produce a useful dialog template?

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
