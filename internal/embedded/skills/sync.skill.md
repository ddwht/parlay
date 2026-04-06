# Sync Intents and Dialogs

Check coverage between intents and dialogs, using AI for semantic matching.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Get structural coverage** — Run: `parlay check-coverage @{feature}`
   - This outputs JSON with covered intents, uncovered intents, and orphan dialogs based on title/word matching

2. **Enhance with semantic matching** — Review the uncovered intents and orphan dialogs from the JSON output:
   - Check if any "uncovered" intent is actually covered by an orphan dialog with a different name
   - For example: intent "Configure Project Tools" might match dialog "Bootstrap Project" — these are semantically related even though the titles don't overlap
   - Present any suspected matches to the user for confirmation

3. **Report the final coverage** — Show:
   - Covered intents (structural + semantic matches)
   - Truly uncovered intents (no matching dialog at all)
   - True orphan dialogs (no matching intent)

4. **Offer template generation** — If uncovered intents exist:
   - A: Generate dialog templates for all uncovered
   - B: Let the user pick which ones
   - C: Just the report
   - If the user chooses A or B, run `parlay scaffold-dialogs @{feature}` or generate templates inline
