# Sync Intents and Dialogs

Check coverage between intents and dialogs, using AI for semantic matching. Optionally check the full traceability chain (intents → dialogs → surface → buildfile → testcases).

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Get structural coverage** — Run: `parlay check-coverage @{feature}`
   - This outputs JSON with covered intents, uncovered intents, and orphan dialogs based on title/word matching

2. **Enhance with semantic matching** — Review the uncovered intents and orphan dialogs from the JSON output:
   - Check if any "uncovered" intent is actually covered by an orphan dialog with a different name
   - For example: intent "Configure Project Tools" might match dialog "Bootstrap Project" — these are semantically related even though the titles don't overlap
   - Present any suspected matches to the user for confirmation

3. **Collect open questions** — Scan all intents for `**Questions**:` items and report them:
   - Show the count of open questions per intent
   - If questions exist, note that they should be resolved before build-feature

4. **Report the final coverage** — Show:
   - Covered intents (structural + semantic matches)
   - Truly uncovered intents (no matching dialog at all)
   - True orphan dialogs (no matching intent)
   - Open questions count

5. **Check full chain (if downstream artifacts exist)** — If surface.md, buildfile.yaml, or testcases.yaml exist, also report:
   - Intents with no surface fragment (check `**Source**:` references in surface.md)
   - Surface fragments with no buildfile component (check `source:` in buildfile.yaml)
   - Buildfile components with no test suite (check `component:` in testcases.yaml)
   - Orphaned artifacts referencing intents that no longer exist

6. **Offer template generation** — If uncovered intents exist:
   - A: Generate dialog templates for all uncovered
   - B: Let the user pick which ones
   - C: Just the report
   - If the user chooses A or B, run `parlay scaffold-dialogs @{feature}` or generate templates inline
