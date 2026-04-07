# Create Surface

Generate a surface.md file for a feature by analyzing its intents and dialogs.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Load schemas** — Read these files before generating:
   - `.parlay/schemas/surface.schema.md`
   - `.parlay/schemas/intent.schema.md`
   - `.parlay/schemas/dialog.schema.md`

2. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/disambiguation.yaml` (if exists — contains prior decisions, skip resolved issues)

3. **Analyze for ambiguities** — Read the intents and dialogs carefully. Identify any:
   - Ambiguities: where the same intent could be interpreted multiple ways
   - Conflicts: where intents and dialogs contradict each other
   - Missing information: where there's not enough detail to determine UI fragments

4. **If ambiguities found** — Present each one to the user:
   - Quote the relevant text from intents or dialogs
   - Explain what's ambiguous
   - Offer lettered options (A, B, C) with a recommended choice
   - Wait for the user's response
   - Ask if they want the source file updated to reflect the decision
   - Save the decision to `spec/intents/{feature}/disambiguation.yaml`

5. **Generate surface.md** — For each distinct UI piece implied by the intents and dialogs:
   - Create a fragment with a descriptive `## Name` heading
   - `**Shows**:` what the user sees (derived from intent Goal and dialog system turns)
   - `**Actions**:` what the user can do (derived from dialog options and user turns)
   - `**Source**:` `@{feature}/{intent-slug}` reference (required — every fragment must trace to its source intent)
   - If a surface.md already exists, preserve existing fragments and only add new ones
   - Use intent Priority to guide fragment importance — P0 intents should produce primary fragments

6. **Check readiness** — Before generation, run: `parlay check-readiness @{feature} --stage create-surface`
   - If errors are reported, present them to the user with fixes and stop
   - If warnings are reported (e.g., no dialogs), inform the user and ask whether to proceed

7. **Validate** — Run: `parlay validate --type surface --json spec/intents/{feature}/surface.md`
   - If validation fails, parse the JSON error output and apply the fix from each error's `fix` field, then retry

8. **Report** — Tell the user what was generated and remind them to add Page and Region targets.

## Error Handling

When `parlay check-readiness --stage create-surface` returns errors:
- `intents-not-readable` — intents.md is missing or malformed. Ask user to fix it before retrying.
- `no-intents` — intents.md is empty. Tell the user to author at least one intent.
- `missing-goal` / `missing-persona` — required field missing on a specific intent. Show which intent and ask user to fill it in.

When `parlay validate --type surface --json` returns errors:
- `schema-validation-failed` — the generated surface.md is malformed. This is likely a generation bug; review what you wrote and regenerate.

When the user provides ambiguous input during disambiguation:
- Offer to defer the decision and proceed with a sensible default
- Save the deferred status to disambiguation.yaml so it can be revisited later
