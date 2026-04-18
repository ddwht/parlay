# Repair Project State

> A command for validating and reconciling parlay's three parallel trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`) after external filesystem operations that bypass parlay have left them out of sync. Uses interactive questions when the correct reconciliation is ambiguous. Assumes the structural model defined in the `initiatives` feature, and reuses the three-tree operations defined in `move-feature` and `features-and-initiatives-renaming`.

---

## Repair Project State

**Goal**: Detect inconsistencies across the three parallel trees — caused by external filesystem operations that bypass parlay (`mv`, IDE refactors, file-manager drags, hand-edits) — and restore lockstep, asking the designer questions when the correct reconciliation is ambiguous.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer renamed an initiative folder in their IDE, or moved a feature with `mv`, or restored one of the parallel trees from a backup, and now `spec/intents/`, `spec/handoff/`, and `.parlay/build/` are no longer in lockstep. Parlay commands that resolve qualified identifiers start failing. The designer needs a way to put the project back into a consistent state without manually touching three trees, and without parlay silently guessing their intent.
**Action**: Run `parlay repair`. Parlay scans all three trees, identifies mismatches, and for each mismatch: if the correct reconciliation is unambiguous, applies it and prints a summary; if ambiguous, asks the designer an interactive question and applies whichever answer is chosen. At the end, parlay reports what was repaired and verifies that the three trees are in lockstep.
**Objects**: project, initiative, feature, directory-diff, command-argument

**Constraints**:
- `parlay repair` is the only supported way to restore lockstep after external operations. Parlay commands themselves never silently run repair on your behalf — repair happens only when the designer invokes it explicitly.
- The detection pass compares the set of initiative and feature directories across `spec/intents/`, `spec/handoff/`, and `.parlay/build/`. Every classification-rule-qualifying directory (per the `initiatives` feature) should exist in all three trees with the same qualified path; any deviation is a mismatch.
- Detected mismatches fall into a small set of categories, each with a defined resolution strategy:
  - **Unambiguous initiative rename** — `spec/intents/` has a new initiative name; `spec/handoff/` and `.parlay/build/` still have the old name; no other changes. Parlay reports the detected rename, asks the designer to confirm (`Rename <old> → <new> across spec/handoff/ and .parlay/build/? [Y/n]`), and applies it via the same logic as `parlay rename-initiative`.
  - **Unambiguous feature move** — `spec/intents/` shows a feature at a new qualified path; `spec/handoff/` and `.parlay/build/` still have it at the old path. Parlay reports the detected move, asks to confirm, and applies it via the same logic as `parlay move-feature`.
  - **Ambiguous rename or move** — multiple plausible pairings (e.g., two initiatives were renamed simultaneously before running repair). Parlay presents each candidate pair and asks the designer to pick the correct pairing, or to choose "none of these" (in which case that mismatch is left for manual resolution).
  - **Missing directory** — a directory exists on one or two trees but not the others. Parlay asks whether to recreate the missing directory on the other trees (bringing them up to lockstep with the tree that has it) or to delete the existing one (removing the orphan). The designer picks.
  - **Extra directory** — a directory exists on `spec/handoff/` or `.parlay/build/` that has no matching entry in `spec/intents/`. Parlay asks whether to delete the extra directory (it is likely stale output) or to preserve it (if the designer intends to re-create the source later).
  - **Genuinely unknown state** — parlay cannot match the observed state to any of the categories above. Parlay reports the unresolved mismatch, names the offending paths, and instructs the designer to resolve manually. Repair continues with the remaining mismatches and exits with a non-zero status if any remain unresolved.
- The interactive question layer is always skippable: a `--dry-run` flag reports detected mismatches and proposed repairs without applying any changes or asking any questions; a `--yes` flag auto-confirms all unambiguous mismatches (but still pauses on ambiguous ones). `--yes --dry-run` is an argument-parsing error.
- Repair operations are applied atomically per-mismatch. If applying a confirmed repair fails partway (filesystem error, permission change mid-command), parlay rolls back that single repair but continues processing the remaining mismatches.
- If no mismatches are detected, the command is a successful no-op and reports "Project is in lockstep."
- Questions asked by repair must name the detected mismatch concretely, using full paths, so the designer can verify the diagnosis is correct before confirming. Generic "something is off" prompts are forbidden.
- Repair is not destructive by default. Any operation that deletes a directory or its contents is proposed as a question with the explicit path and file count (e.g., `Delete spec/handoff/auth-overhaul/ (12 files)? [y/N]`); the designer must opt in affirmatively. Defaults on delete prompts are always "no".

**Verify**:
- `parlay repair` on a project already in lockstep exits with status 0 and reports "Project is in lockstep."
- After `mv spec/intents/auth-overhaul spec/intents/auth-redesign`, `parlay repair` detects the rename, asks the designer to confirm, and on confirmation applies the corresponding rename across `spec/handoff/` and `.parlay/build/`
- After `mv spec/intents/password-reset spec/intents/auth-overhaul/password-reset`, `parlay repair` detects the feature move and applies the matching move across the other two trees on confirmation
- After two simultaneous external renames (`mv a x; mv b y` on initiative dirs), `parlay repair` presents both candidate pairings and asks the designer to disambiguate
- `parlay repair --dry-run` reports detected mismatches and proposed repairs without applying changes, and without prompting
- `parlay repair --yes` auto-confirms unambiguous repairs but still pauses on ambiguous ones
- A destructive proposal (e.g., "delete `spec/handoff/auth-overhaul/` (12 files)") defaults to "no" and requires an explicit `y` to proceed
- An unrecognized mismatch (no category applies) is reported clearly, left unresolved, and causes `parlay repair` to exit non-zero while still applying every repair the designer did confirm
- After `parlay repair` succeeds on all detected mismatches, a subsequent `parlay repair` reports "Project is in lockstep" with no further questions

---
