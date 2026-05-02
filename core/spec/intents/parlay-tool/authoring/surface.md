# Authoring — Surface

---

## Feature Scaffold Confirmation

**Shows**: status, message, data-list
**Actions**: invoke
**Source**: @parlay-tool/authoring/author-intents

**Page**: add-feature
**Region**: main
**Order**: 1

**Notes**:
- Output of `parlay add-feature <name>`.
- `status` confirms creation. `message` shows the created path. `data-list` shows the scaffolded files (intents.md, dialogs.md).
- `invoke` directs the designer to start authoring intents.md.

---

## Dialog Generation Report

**Shows**: status, message, summary, data-list
**Actions**: select-one, invoke
**Source**: @parlay-tool/authoring/generate-dialogs-from-intents

**Page**: create-dialogs
**Region**: main
**Order**: 1

**Notes**:
- Output of the generation phase of `/parlay-scaffold-dialogs`.
- When intents lack dialogs: `summary` reports how many intents were found, `data-list` shows each generated dialog with its branch count (e.g., "Group Features — trigger, happy path, 4 branches").
- The generated dialogs are complete — full triggers, user/system turns, edge case branches from Constraints and Verify items. Not empty templates.
- If ambiguities are detected in intents BEFORE generation: `select-one` presents the clarifying question. The agent does not generate until ambiguities are resolved.
- When all intents already have dialogs: transitions to the Dialog Update Proposals fragment.
- `invoke` directs the designer to review the generated dialogs.

---

## Dialog Update Proposals

**Shows**: message, diff, data-value, status
**Actions**: select-one
**Flow**: review-and-approve
**Source**: @parlay-tool/authoring/update-dialogs-from-changed-intents

**Page**: create-dialogs
**Region**: main
**Order**: 2

**Notes**:
- Output of the update phase of `/parlay-scaffold-dialogs` (runs when all intents already have dialogs).
- For each meaningful intent change: `message` names the triggering change, `diff` or text shows the complete proposed dialog content (full branches with turns, not stubs), and `select-one` offers accept/skip/edit.
- `data-value` shows the intent field that changed and the proposed dialog section.
- `status` per update: [OK] for accepted, "Skipped" for rejected.
- Cosmetic intent changes are silently skipped — not shown.
- Final `message`: "Applied N updates, skipped M. Dialogs are up to date." or "No updates needed."

---

## Coverage Report

**Shows**: summary, data-list, status, message
**Actions**: select-one, invoke
**Source**: @parlay-tool/authoring/sync-intents-and-dialogs

**Page**: sync
**Region**: main
**Order**: 1

**Notes**:
- Output of `/parlay-sync`.
- `summary` reports coverage stats — covered intents, uncovered intents, orphan dialogs.
- `data-list` shows each category with specific items.
- `status` shows drift detection results — intents changed since last build, with specific fields that changed.
- `message` for drift: names downstream artifacts that may need updating.
- `select-one` for uncovered intents: generate complete dialogs for all, pick specific ones, or just report.
- `invoke` for drift: offers to walk through each stale artifact.

---
