# Sync Intents and Dialogs

Check coverage, traceability, and drift between intents and downstream artifacts.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

## Steps

1. **Get coverage, chain, and drift data** — Run: `parlay check-coverage @{feature}`
   - This outputs JSON with:
     - Covered/uncovered intents and orphan dialogs (title/word matching)
     - Full-chain traceability gaps (if downstream artifacts exist)
     - Drift detection (if a baseline exists from a previous build)

2. **Collect open questions** — Run: `parlay collect-questions @{feature}`
   - Reports open questions per intent with priority

3. **Enhance with semantic matching** — Review the uncovered intents and orphan dialogs:
   - Check if any "uncovered" intent is actually covered by an orphan dialog with a different name
   - For example: intent "Configure Project Tools" might match dialog "Bootstrap Project" — semantically related even though titles don't overlap
   - Present any suspected matches to the user for confirmation

4. **Report** — Present to the user:
   - **Coverage**: covered intents, uncovered intents, orphan dialogs
   - **Open questions**: count and list (note they should be resolved before build-feature)
   - **Chain gaps** (if present): intents without surface, fragments without buildfile, components without tests, orphaned references
   - **Drift** (if present): intents that changed since last build, with specific fields that changed (Goal, Constraints, Verify, Objects)

5. **Handle drift** — If drifted intents are detected:
   - Read the downstream artifacts (surface.md, buildfile.yaml, testcases.yaml) for the drifted intents
   - Compare the changed intent fields against what the artifacts expect
   - Flag meaningful mismatches (e.g., Goal changed but surface Shows still reflects the old Goal)
   - Ask the user how to proceed:
     - A: Walk through each mismatch and update downstream artifacts
     - B: Flag them for manual update
     - C: Ignore — the current artifacts are fine

6. **Offer template generation** — If uncovered intents exist:
   - A: Generate dialog templates for all uncovered
   - B: Let the user pick which ones
   - C: Just the report
   - If the user chooses A or B, run `parlay scaffold-dialogs @{feature}` or generate templates inline
