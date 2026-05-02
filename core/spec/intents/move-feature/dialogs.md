# Move Feature — Dialogs

---

### Move a Feature Between Locations

**Trigger**: /parlay-move-feature @==feature== --to ==initiative== | --out

User: /parlay-move-feature @password-reset --to auth-overhaul
System (background): Parses `@password-reset` — resolves to orphan feature at `spec/intents/password-reset/`.
System (background): Slugifies `--to` value → `auth-overhaul`. Checks the top-level namespace for collisions.
System (background): Finds initiative `auth-overhaul` already exists. Checks for scope collision — no existing `password-reset` inside `auth-overhaul`.
System (background): Performs three-tree `git mv`:
System (background):   spec/intents/password-reset/ → spec/intents/auth-overhaul/password-reset/
System (background):   spec/handoff/password-reset/ → spec/handoff/auth-overhaul/password-reset/
System (background):   .parlay/build/password-reset/ → .parlay/build/auth-overhaul/password-reset/
System: Feature moved:
System:   Before: @password-reset (spec/intents/password-reset/)
System:   After:  @auth-overhaul/password-reset (spec/intents/auth-overhaul/password-reset/)
System: All three trees updated in lockstep. Git history preserved via `git mv`.

#### Branch: Auto-create initiative on move

User: /parlay-move-feature @sso-setup --to auth-redesign
System (background): Resolves `@sso-setup` to orphan feature. Slugifies `--to` → `auth-redesign`.
System (background): Initiative `auth-redesign` does not exist. Creates `spec/intents/auth-redesign/`, `spec/handoff/auth-redesign/`, `.parlay/build/auth-redesign/` in lockstep.
System (background): Performs three-tree `git mv` of `sso-setup/` into the new initiative.
System: Initiative auth-redesign created.
System: Feature moved:
System:   Before: @sso-setup (spec/intents/sso-setup/)
System:   After:  @auth-redesign/sso-setup (spec/intents/auth-redesign/sso-setup/)

#### Branch: Move feature out of initiative (--out)

User: /parlay-move-feature @auth-overhaul/password-reset --out
System (background): Resolves `@auth-overhaul/password-reset` to nested feature at `spec/intents/auth-overhaul/password-reset/`.
System (background): Checks for scope collision at the top level — no existing orphan `password-reset`.
System (background): Performs three-tree `git mv`:
System (background):   spec/intents/auth-overhaul/password-reset/ → spec/intents/password-reset/
System (background):   spec/handoff/auth-overhaul/password-reset/ → spec/handoff/password-reset/
System (background):   .parlay/build/auth-overhaul/password-reset/ → .parlay/build/password-reset/
System: Feature moved:
System:   Before: @auth-overhaul/password-reset (spec/intents/auth-overhaul/password-reset/)
System:   After:  @password-reset (spec/intents/password-reset/)

#### Branch: Move between initiatives

User: /parlay-move-feature @auth-overhaul/password-reset --to billing
System (background): Resolves `@auth-overhaul/password-reset` to nested feature. Target initiative `billing` exists. No `password-reset` inside `billing`.
System (background): Performs three-tree `git mv` from `auth-overhaul/password-reset/` to `billing/password-reset/`.
System: Feature moved:
System:   Before: @auth-overhaul/password-reset (spec/intents/auth-overhaul/password-reset/)
System:   After:  @billing/password-reset (spec/intents/billing/password-reset/)

#### Branch: Scope collision — feature already exists at target

User: /parlay-move-feature @auth-overhaul/password-reset --to billing
System (background): Resolves feature. Checks target scope — `spec/intents/billing/password-reset/` already exists.
System: [ERR] Feature `password-reset` already exists inside initiative `billing` at spec/intents/billing/password-reset/. Rename one of the features before retrying the move.

#### Branch: Top-level namespace collision (--to targets orphan feature)

User: /parlay-move-feature @login-flow --to password-reset
System (background): Slugifies `--to` → `password-reset`. Finds `spec/intents/password-reset/` exists as an orphan feature (has intents.md directly).
System: [ERR] `password-reset` exists at the top level as a feature, not an initiative. A feature and an initiative can't share a top-level slug. Either pick a different initiative name, or first move the existing `password-reset` feature into an initiative with /parlay-move-feature.

#### Branch: Top-level collision on --out

User: /parlay-move-feature @auth-overhaul/password-reset --out
System (background): Resolves feature. Checks top level — `spec/intents/password-reset/` already exists as an orphan feature.
System: [ERR] Feature `password-reset` already exists at the top level at spec/intents/password-reset/. Rename one of the features before moving this one out.

#### Branch: Non-existent feature

User: /parlay-move-feature @nonexistent --to auth-overhaul
System (background): Attempts to resolve `@nonexistent`. No directory found at `spec/intents/nonexistent/`.
System: [ERR] Feature `nonexistent` not found. No directory exists at spec/intents/nonexistent/. Check the spelling, or use a qualified identifier (@initiative/feature) if the feature is inside an initiative.

#### Branch: Moving an initiative instead of a feature

User: /parlay-move-feature @auth-overhaul --to other-initiative
System (background): Resolves `@auth-overhaul`. Finds `spec/intents/auth-overhaul/` — classifies as an initiative (contains subdirectories with intents.md), not a feature.
System: [ERR] `auth-overhaul` is an initiative, not a feature. Only features can be moved with /parlay-move-feature. To rename an initiative, use parlay rename-initiative.

#### Branch: Mutually exclusive flags

User: /parlay-move-feature @password-reset --to auth-overhaul --out
System: [ERR] `--to` and `--out` are mutually exclusive. Use `--to <initiative>` to move into an initiative, or `--out` to move to the top level — not both.

User: /parlay-move-feature @password-reset
System: [ERR] Missing destination. Use `--to <initiative>` to move into an initiative, or `--out` to move to the top level.

#### Branch: No-op — already in target location

User: /parlay-move-feature @auth-overhaul/password-reset --to auth-overhaul
System (background): Resolves feature. Target is `auth-overhaul` — feature already lives there.
System: Feature @auth-overhaul/password-reset is already in initiative auth-overhaul — no change.

User: /parlay-move-feature @password-reset --out
System (background): Resolves `@password-reset` — already at the top level.
System: Feature @password-reset is already at the top level — no change.

#### Branch: Empty initiative left behind

User: /parlay-move-feature @auth-overhaul/login-flow --out
System (background): `login-flow` is the only feature inside `auth-overhaul`. Moves it out across all three trees.
System: Feature moved:
System:   Before: @auth-overhaul/login-flow
System:   After:  @login-flow
System: Note: initiative auth-overhaul is now empty (deferred classification). It remains on disk across all three trees. To remove it, delete the directories manually, or reuse it with /parlay-add-feature --initiative auth-overhaul.

#### Branch: Rollback on partial failure

User: /parlay-move-feature @password-reset --to auth-overhaul
System (background): Moves spec/intents/password-reset/ → spec/intents/auth-overhaul/password-reset/ — success.
System (background): Moves spec/handoff/password-reset/ → spec/handoff/auth-overhaul/password-reset/ — success.
System (background): Moves .parlay/build/password-reset/ → .parlay/build/auth-overhaul/password-reset/ — fails (permissions error).
System (background): Rolls back: moves spec/intents/auth-overhaul/password-reset/ back to spec/intents/password-reset/, moves spec/handoff/auth-overhaul/password-reset/ back to spec/handoff/password-reset/.
System: [ERR] Move failed on .parlay/build/ tree: permission denied. Rolled back all trees — feature remains at @password-reset. Fix the permissions and retry.

---
