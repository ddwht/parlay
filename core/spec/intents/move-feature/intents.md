# Move Feature

> A command for relocating an existing feature between initiatives — or in and out of top-level orphan state — while keeping all three parallel trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`) in lockstep and preserving git history. Assumes the structural model defined in the `initiatives` feature: qualified identifiers, per-scope uniqueness, three-tree lockstep maintained only by parlay commands.

---

## Move a Feature Between Locations

**Goal**: Move an existing feature into an initiative, between initiatives, or back out to top-level orphan state, keeping its leaf slug and all its artifacts intact across the three parallel trees.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer already has orphan features — or features in the wrong initiative — and wants to reorganize without re-creating anything, and without losing git history on the feature's files or orphaning its build or handoff artifacts.
**Action**: Run `parlay move-feature @feature --to <initiative>` to relocate a feature into an initiative (creating the initiative if it doesn't yet exist), or `parlay move-feature @feature --out` to move it back to top-level orphan state. The `@feature` argument is a qualified identifier — `@feature` for an orphan, `@initiative/feature` for a nested feature. The command performs a three-tree directory move (across `spec/intents/`, `spec/handoff/`, and `.parlay/build/`) — using `git mv` when the project is in a git repository — and reports the before-and-after qualified paths.
**Objects**: feature, initiative, directory-move, command-argument

**Constraints**:
- The feature's leaf slug never changes during a move. The qualified identifier changes because the parent portion changes, but the leaf is preserved.
- The move must be applied atomically across all three trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`). If any of the three fails partway, parlay must roll back any successful renames, leaving the feature in its original qualified location on every tree. No half-moved state must persist across the trees.
- If the target initiative does not exist, it is auto-created — matching the auto-create behavior of `add-feature --initiative` for consistency. Auto-creation happens on all three trees.
- In a git repository, the command uses `git mv` on each of the three trees so history follows the files; outside a git repository, it falls back to a plain move.
- Scope-based collision — if the target parent directory (the destination initiative for `--to`, or the top level for `--out`) already contains a feature with the same leaf slug, the command fails with a scope-collision error naming the conflicting path. The designer must resolve by renaming one feature before retrying.
- Moving a feature to its current qualified location is a no-op — the command succeeds silently, does not error
- Passing `--out` on a feature that is already at the top level is a no-op — succeeds, does not error
- The command fails with a clear error if the qualified feature identifier does not resolve to any existing feature
- The `@slug` argument must resolve to a **feature**, not an initiative. If the identifier resolves to an initiative, the command fails with an error explaining that only features can be moved via this command; initiatives are renamed by `parlay rename-initiative`, not `move-feature`.
- The `--to` and `--out` flags are mutually exclusive. Passing both is an argument-parsing error; passing neither is also an argument-parsing error.
- The command output names both the before qualified path and the after qualified path so the designer can see exactly what happened
- The `--to` value must be an initiative slug (at the top level); if it resolves to an existing orphan feature at the top level, the command fails with the same top-level-namespace collision error as `add-feature --initiative`
- The `--to` argument accepts either a raw human-readable name (`"auth overhaul"`) or an already-slugified form (`auth-overhaul`); both are slugified on input and resolve to the same initiative.
- Moving the last feature out of an initiative leaves the now-empty initiative directory in place on all three trees (in deferred-classification state per the `initiatives` spec). Parlay does not auto-delete empty initiative directories; cleanup is the designer's responsibility via `rm` or `rmdir` on each tree, or by creating a new feature inside the empty initiative.

**Verify**:
- `parlay move-feature @password-reset --to auth-overhaul` relocates `spec/intents/password-reset/` to `spec/intents/auth-overhaul/password-reset/`, and the matching paths under `spec/handoff/` and `.parlay/build/`, all in lockstep; qualified-identifier lookups for `@auth-overhaul/password-reset` now resolve to the new paths on every tree
- `parlay move-feature @sso-setup --to auth-redesign` when `auth-redesign` does not exist creates the initiative directory on all three trees and moves the feature into it; output distinguishes "initiative auth-redesign created" from "feature sso-setup moved"
- `parlay move-feature @auth-overhaul/password-reset --out` moves the feature from `spec/intents/auth-overhaul/password-reset/` back to `spec/intents/password-reset/` (and corresponding paths on the other two trees)
- `parlay move-feature @auth-overhaul/password-reset --to billing` when `billing` already has a `password-reset` feature fails with a scope-collision error naming `spec/intents/billing/password-reset/`
- In a git repository, `git log --follow` on any moved file under `spec/intents/`, `spec/handoff/`, or `.parlay/build/` traverses the move without losing history
- `parlay move-feature @auth-overhaul/password-reset --to auth-overhaul` run when the feature already lives there succeeds and reports "no change"
- `parlay move-feature @nonexistent --to auth-overhaul` fails with an error naming the unresolved identifier
- `parlay move-feature @password-reset --to some-existing-orphan-feature-slug` fails with a top-level-namespace collision error
- `parlay move-feature @auth-overhaul --to other-initiative` when `auth-overhaul` is an initiative (not a feature) fails with an error explaining that only features can be moved; the designer is directed to `parlay rename-initiative` if they wanted to rename
- `parlay move-feature @password-reset --to auth-overhaul --out` fails with an argument-parsing error for mutually exclusive flags
- Moving the only remaining feature out of `spec/intents/auth-overhaul/` leaves empty `spec/intents/auth-overhaul/`, `spec/handoff/auth-overhaul/`, and `.parlay/build/auth-overhaul/` directories on disk; subsequent `parlay add-feature --initiative auth-overhaul` reuses them without re-creating
- A mid-move filesystem failure (simulated by making `.parlay/build/` read-only mid-command) rolls back the already-renamed `spec/intents/` and `spec/handoff/` entries so the feature is fully back at its original qualified path on every tree

---
