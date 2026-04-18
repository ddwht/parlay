# Authoring — Dialogs

---

### Add Feature

**Trigger**: /parlay-add-feature ==feature-name==

User: /parlay-add-feature upgrade plan creation
System (background): Runs `parlay add-feature upgrade plan creation`.
System: I've added the upgrade-plan-creation feature to /spec/intents/upgrade-plan-creation/.
System: Start by filling out intents.md. When your intents are ready, use /parlay-scaffold-dialogs.

---

### Scaffold Dialogs from Intents

**Trigger**: /parlay-scaffold-dialogs @feature-name

User: /parlay-scaffold-dialogs @upgrade-plan-creation
System (background): Runs `parlay create-dialogs @upgrade-plan-creation`.
System: I found ==number== intents. Generating dialog templates.
System (background): For each intent, generates a dialog template pre-filled from the Goal and Action fields.
System: I've added ==number== dialog templates to dialogs.md. Each one has a title, trigger, and placeholder turns based on your intents.
System: Review and rewrite them to capture the real user-system conversation. The templates are a starting point — make them yours.

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
System (condition: uncovered intents exist): Want me to generate dialog templates for the uncovered intents?
  A: Yes, generate templates for all
  B: Let me pick which ones
  C: No, just the report is enough

---
