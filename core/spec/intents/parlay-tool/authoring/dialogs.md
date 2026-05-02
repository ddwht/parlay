# Authoring — Dialogs

---

### Add Feature

**Trigger**: /parlay-add-feature ==feature-name==

User: /parlay-add-feature upgrade plan creation
System (background): Runs `parlay add-feature upgrade plan creation`.
System: I've added the upgrade-plan-creation feature to /spec/intents/upgrade-plan-creation/.
System: Start by filling out intents.md. When your intents are ready, use /parlay-scaffold-dialogs.

---

### Generate Dialogs from Intents

**Trigger**: /parlay-scaffold-dialogs @feature-name

User: /parlay-scaffold-dialogs @initiatives
System (background): Reads intents.md for @initiatives. Found 3 intents.
System (background): Checks for ambiguities that would block dialog generation.
System (condition: ambiguity found): Before I generate dialogs, I need to clarify something in intent "Group Features under an Initiative":
System: The Constraints mention both "per-scope uniqueness" and "top-level namespace collision" — should these be separate dialog branches, or one combined error flow?
  A: Separate branches — they're different error types
  B: Combined — show both in one error handling branch
User: A
System (background): Generates complete dialogs for all 3 intents. Each dialog includes: trigger, happy path with user/system turns, branches for edge cases from Constraints and Verify items.
System: Generated 3 complete dialogs in dialogs.md:
System: - **Group Features under an Initiative** — trigger, orientation flow, 4 branches (qualified addressing, hybrid error, sub-initiative error, external tools)
System: - **Create a Feature Inside an Initiative** — trigger, happy path, 4 branches (initiative exists, scope collision, top-level collision, partial failure)
System: - **Create an Empty Initiative** — trigger, happy path, 3 branches (idempotent, top-level collision, nested feature allowed)
System: Review the dialogs. They're complete — edit anything that doesn't match the real conversation you envision.

#### Branch: Some intents already have dialogs

User: /parlay-scaffold-dialogs @initiatives
System (background): Found 3 intents. 2 already have dialogs, 1 is new.
System (background): Generates a complete dialog for the new intent.
System: Generated 1 new dialog for the new intent. 2 existing dialogs unchanged.
System: Checking existing dialogs for updates from intent changes...
System (background): Compares 2 existing dialogs against their intents — no meaningful drift.
System: Existing dialogs are up to date. No updates needed.

---

### Update Dialogs from Changed Intents

**Trigger**: /parlay-scaffold-dialogs @feature-name (when dialogs already exist)

User: /parlay-scaffold-dialogs @initiatives
System (background): Reads intents.md and dialogs.md for @initiatives.
System (background): Checks for intents without dialogs — none found (all covered).
System (background): Compares each dialog against its matched intent. Detects 2 meaningful differences.
System: All intents have dialogs. Checking for updates...
System: Found 2 dialog updates based on intent changes:
System: **Update 1:** Intent "Group Features under an Initiative" added a new Constraint about hybrid directory detection.
System:   Proposed: add a new `#### Branch: Hybrid directory detected` to the dialog with a User turn asking about the error and a System turn explaining the mutual exclusion rule.
System:   ==shows proposed branch content==
  A: Accept this update
  B: Skip — my dialog already covers this
  C: Let me edit the proposed text
User: A
System (background): Appends the new branch to the dialog in dialogs.md.
System: [OK] Added branch "Hybrid directory detected" to dialog "Group Features under an Initiative".
System: **Update 2:** Intent "Create a Feature Inside an Initiative" changed a Verify item — added a test for the `--initiative` flag accepting raw names.
System:   Proposed: no dialog change needed — this is a CLI flag behavior already covered by the existing dialog flow.
System: Skipped — cosmetic, no dialog impact.
System: Done. Applied 1 update, skipped 1. Dialogs are up to date.

#### Branch: No updates needed

User: /parlay-scaffold-dialogs @initiatives
System (background): Checks for gaps — none. Compares dialogs against intents — no meaningful differences.
System: All intents have dialogs. No updates needed — dialogs are up to date.

#### Branch: Intent renamed

User: /parlay-scaffold-dialogs @initiatives
System (background): Detects that intent "Create an Initiative" was renamed to "Create an Empty Initiative". Dialog heading still says "Create an Initiative".
System: Found 1 dialog update:
System: **Intent renamed:** "Create an Initiative" → "Create an Empty Initiative"
System:   Proposed: update dialog heading `### Create an Initiative` → `### Create an Empty Initiative`
  A: Accept
  B: Skip — I prefer the shorter name
User: A
System (background): Updates the dialog heading.
System: [OK] Renamed dialog heading. Dialogs are up to date.

#### Branch: New edge case from Verify

User: /parlay-scaffold-dialogs @move-feature
System (background): Detects that a new Verify item was added: "Moving the last feature out of an initiative leaves the initiative empty."
System: Found 1 dialog update:
System: **New edge case:** Intent added a Verify item about empty initiatives after the last feature is moved out.
System:   Proposed: add a new `#### Branch: Empty initiative left behind` dialog branch.
System:   ==shows proposed branch==
  A: Accept
  B: Skip
  C: Edit
User: A
System: [OK] Added branch. Dialogs are up to date.

---

### Sync Intents and Dialogs

**Trigger**: /parlay-sync @feature-name

User: /parlay-sync @upgrade-plan-creation
System (background): Runs `parlay check-coverage @upgrade-plan-creation` to get coverage and drift data.
System (background): Runs `parlay collect-questions @upgrade-plan-creation` to check for open questions.
System: Here's the sync report for @upgrade-plan-creation:
System: **Coverage:**
System: - ==intent A== — matched by dialog ==dialog title==
System: - ==intent B== — matched by dialog ==dialog title==
System (condition: uncovered intents exist): **Intents without dialogs:**
System: - ==intent C== — no matching dialog found
System (condition: orphan dialogs exist): **Orphan dialogs** (no matching intent):
System: - ==dialog title== — doesn't trace to any intent
System (condition: open questions exist): **Open questions:** ==number== unresolved across ==number== intents
System (condition: drift detected): **Drift detected:** ==number== intents changed since last build:
System: - ==intent title== — ==fields that changed==
System (condition: uncovered intents exist): Want me to generate complete dialogs for the uncovered intents?
  A: Yes, generate dialogs for all
  B: Let me pick which ones
  C: No, just the report is enough

---
