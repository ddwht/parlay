# Features And Initiatives Renaming

> Commands for renaming entities that exist in parlay's three parallel trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`), keeping all three in lockstep and preserving git history. Initiative renaming is specified now; feature renaming is reserved for a later pass. Assumes the structural model defined in the `initiatives` feature.

---

## Rename an Initiative

**Goal**: Change an initiative's slug while keeping its member features and all their artifacts intact across the three parallel trees.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer named an initiative hastily, the project's scope shifted, or the initial name no longer describes the work well. Plain `mv` on `spec/intents/<old>/` is not enough — it would leave `spec/handoff/<old>/` and `.parlay/build/<old>/` stale, breaking qualified-path lookups for every member feature.
**Action**: Run `parlay rename-initiative <old-name> <new-name>`. Renames `spec/intents/<old>/` to `spec/intents/<new>/`, and the matching paths under `spec/handoff/` and `.parlay/build/`, atomically. Feature leaf slugs inside are preserved; only the parent portion of each member feature's qualified identifier changes.
**Objects**: initiative, directory-rename, command-argument

**Constraints**:
- The rename must be applied atomically across all three trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`). If any of the three fails partway, parlay must roll back, leaving the initiative at its original name on every tree.
- In a git repository, the command uses `git mv` on each of the three trees so history follows across the rename.
- The new name must not collide in the top-level namespace — if `spec/intents/<new>/` already exists (as another initiative, as an orphan feature, or in any non-deferred state), the command fails with a top-level-namespace collision error.
- If `<old>` does not resolve to an existing initiative, the command fails with an error naming the unresolved identifier. If `<old>` resolves to an orphan feature rather than an initiative, the command fails with a directive that feature rename is out of scope for this command.
- Running the command with `<old>` equal to `<new>` (after slugification) is a no-op — the command succeeds silently.
- The `<old>` and `<new>` arguments each accept raw human-readable names or already-slugified forms, and are slugified on input.
- Plain `mv` is not an alternative to this command. Parlay does not silently detect or reconcile bare filesystem renames performed outside parlay (`mv`, IDE refactors, file-manager drags). If a designer renames an initiative's directory externally, `spec/handoff/` and `.parlay/build/` are left stale, and any subsequent attempt to address a member feature under the new name will fail. Lockstep is restored by running `parlay repair` explicitly (see the `repair-project-state` feature), which will detect the mismatch, confirm the intent with the designer, and apply the corresponding three-tree rename.
- The command does not rename the feature leaf slugs inside the initiative; those are preserved byte-for-byte. Qualified identifiers for member features go from `<old>/<feature>` to `<new>/<feature>` automatically as a consequence of the parent rename.

**Verify**:
- `parlay rename-initiative auth-overhaul auth-redesign` renames `spec/intents/auth-overhaul/` to `spec/intents/auth-redesign/`, and the matching paths under `spec/handoff/` and `.parlay/build/`; every member feature's leaf slug is preserved
- Every qualified-identifier lookup that resolved to `@auth-overhaul/<feature>` before the rename now resolves to `@auth-redesign/<feature>` in all three trees
- `git log --follow` on a member feature's intents.md, specification.md, and buildfile.yaml each traverse the rename
- `parlay rename-initiative auth-overhaul some-existing-top-level-name` fails with a top-level-namespace collision error
- `parlay rename-initiative nonexistent auth-redesign` fails with an error naming the unresolved initiative
- `parlay rename-initiative some-orphan-feature auth-redesign` fails with a directive pointing to the out-of-scope feature-rename concern
- `parlay rename-initiative auth-overhaul auth-overhaul` succeeds and reports "no change"
- A mid-rename filesystem failure (simulated by making `.parlay/build/` read-only mid-command) rolls back the already-renamed `spec/intents/` and `spec/handoff/` entries so the initiative is fully back at its original name on every tree
- A plain `mv spec/intents/auth-overhaul spec/intents/auth-redesign` followed by any parlay command does not trigger silent repair; subsequent commands that try to resolve `@auth-redesign/<feature>` fail because `spec/handoff/auth-overhaul/` and `.parlay/build/auth-overhaul/` are stale. Running `parlay repair` then detects the rename, confirms it with the designer, and brings the parallel trees into lockstep as `auth-redesign`.

---
