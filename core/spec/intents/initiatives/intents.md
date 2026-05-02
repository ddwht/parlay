# Initiatives

> A directory-based umbrella that groups related features, so large projects stay organized and each feature's intents and dialogs stay focused — without a flat wall of sibling folders. Feature slugs are unique within their parent directory; the full path through the hierarchy is the feature's identity. See also: `move-feature` for relocating features between initiatives, `features-and-initiatives-renaming` for renaming initiatives (and, later, features), and `repair-project-state` for reconciling the three trees after external operations.

---

## Group Features under an Initiative

**Goal**: Define the structural model that lets multiple related features be grouped under a single initiative — identity, per-scope uniqueness, classification rules, three-tree lockstep — so that grouping becomes a first-class concept rather than a naming convention, and each feature's intents.md and dialogs.md stay focused and small.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer is planning a big project that will ship as several features over time. The feature folder gets crowded, and after a year a flat directory of dozens of sibling folders becomes unmanageable. The system needs a structural way to express "these features belong together" that (a) preserves existing per-feature conventions, (b) lets orphan features keep working unchanged, and (c) keeps slug-keyed lookups across the three trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`) coherent.
**Action**: Introduce a directory-based hierarchy — an initiative is a direct-child directory of `spec/intents/` that contains member feature directories. Feature identity becomes a qualified path through that hierarchy. The three parallel trees mirror each other. Orphan features (not in any initiative) live at the top level alongside initiative directories, working unchanged.
**Objects**: initiative, feature, project

**Constraints**:
- A feature's identity is its **qualified path** through the hierarchy: `<feature-slug>` for an orphan at the top level, `<initiative-slug>/<feature-slug>` for a feature inside an initiative. The bare slug alone is not a unique identifier once initiatives exist.
- Slugs are unique per parent directory, not globally. At the top level (`spec/intents/`), initiative slugs and orphan feature slugs share one namespace — two entities at the top level cannot share a slug. Inside each initiative (`spec/intents/<initiative>/`), member feature slugs share one namespace — two features within the same initiative cannot share a slug. Across different parents, the same slug is allowed — `spec/intents/auth-overhaul/password-reset/` and `spec/intents/billing/password-reset/` may coexist, as may `spec/intents/password-reset/` (orphan) and `spec/intents/auth-overhaul/password-reset/` (nested).
- Command-line addressing uses `@feature` for orphans and `@initiative/feature` for nested features. The `@` introduces the identifier; the `/` mirrors the filesystem layout.
- Qualified identifiers are parsed by splitting on `/`. Each component is slugified independently. Two components address a nested feature (`<initiative>/<feature>`); one component addresses an orphan feature or — for commands that take an initiative argument like `new-initiative` and `rename-initiative` — an initiative. Empty components (leading, trailing, or a double `/`) are argument-parsing errors. Because slugification never produces `/`, the parse is unambiguous.
- A directory is recognized as an initiative when it contains **direct-child** subdirectories that each contain an `intents.md` file — not recursively. A subdirectory whose `intents.md` is more than one level deep does not qualify; any such structure violates the flat-hierarchy constraint and is invalid. A directory is recognized as a feature when it contains an `intents.md` directly. No marker file is required.
- Initiative directories live only at `spec/intents/` depth-1 (direct children of `spec/intents/`). A directory at depth 2 or deeper that would otherwise classify as an initiative — for example, `spec/intents/auth-overhaul/password-reset/v1/intents.md`, which would make `password-reset/` look like an initiative at depth 2 — represents a sub-initiative and is invalid. Parlay must error when it encounters such a structure, naming the offending path and citing the flat-hierarchy rule.
- A directory is either a feature or an initiative — never both. A directory containing both a top-level `intents.md` and subdirectories that themselves contain `intents.md` is a hybrid and is invalid. Parlay commands detecting a hybrid directory must error with a message that names the offending path and explains the mutual exclusion.
- A directory under `spec/intents/` that matches neither classification rule — no `intents.md` at its top level, and no subdirectory containing `intents.md` — is in **deferred classification**, regardless of whether it is literally empty or contains only unrelated files (images, loose notes, stray designer artifacts). Deferred directories are valid, must not cause errors, and are invisible to any future listing command (`parlay list-features`, `parlay list-initiatives`, or equivalent enumeration commands). As soon as a qualifying file or subdirectory arrives, the classification rule applies.
- An initiative has no required metadata file; an optional `README.md` inside the initiative directory captures narrative (the "why", out-of-scope notes, links to tickets or design files). A `README.md`, or any other arbitrary file, is narrative — not a classification signal. A directory containing only a `README.md` and no feature subdirectories remains in deferred classification until it gains its first feature.
- An initiative directory may contain arbitrary additional files alongside its feature subdirectories and optional `README.md` — images referenced by the README, design exports, loose notes, TODO lists. Parlay ignores anything it does not recognize; the classification rule considers only whether subdirectories with `intents.md` are present.
- The initiative hierarchy is mirrored across the three companion trees: `spec/intents/<initiative>/<feature>/`, `spec/handoff/<initiative>/<feature>/`, and `.parlay/build/<initiative>/<feature>/`. Orphan features live at `spec/intents/<feature>/`, `spec/handoff/<feature>/`, and `.parlay/build/<feature>/`. The three trees must stay in lockstep — every qualified feature identifier resolves to the same relative path in each tree.
- The mirroring applies to every direct-child directory under `spec/intents/`, including directories in deferred classification. The parallel dirs in `spec/handoff/` and `.parlay/build/` inherit the same classification state as their `spec/intents/` counterpart — an empty or deferred parent in `spec/intents/` is matched by empty or deferred parallel dirs on the other two trees.
- Lockstep is maintained by parlay commands that mutate the structure — creations, moves, and renames all update the three trees immediately and atomically in one operation. No parallel directory is ever lazy-created or left for later.
- Parlay has no visibility into external filesystem operations (plain `mv`, IDE refactors, file-manager drags, hand-edits); these can leave the three trees out of sync. Parlay does not silently detect or auto-repair such deltas. Restoring lockstep after external operations is the responsibility of the `repair-project-state` feature.
- A feature belongs to at most one initiative at a time
- Existing features not in any initiative must continue to work unchanged; the initiative layer is optional per-feature
- Every lookup keyed by a qualified feature identifier (`config.FeaturePath`, `config.BuildPath`, `config.HandoffPath`, `@feature` / `@initiative/feature` addressing in commands) must resolve correctly in all three parallel trees, regardless of whether the feature lives at the top level or inside an initiative
- The feature resolver must accept a qualified identifier (`<feature>` or `<initiative>/<feature>`) and return the matching path in whichever tree is requested. A bare slug always addresses the top-level (orphan) scope; it never matches a nested feature. The resolver must defensively detect duplicate-slug states at the same parent level — which the per-scope uniqueness rule forbids, but which can occur via external corruption (a manually copied directory, a botched restore) — and fail with a clear error listing both paths, rather than silently picking one.
- Any command that enumerates features across the project must walk both top-level feature directories and initiative-nested feature directories, so that placing a feature inside an initiative never hides it from a global scan. A shared helper must centralize this traversal — returning qualified identifiers — so the behavior stays consistent as new commands are added, and the same helper must work across the parallel `spec/handoff/` and `.parlay/build/` trees.

**Verify**:
- A designer can create an initiative directory, place three or more feature directories inside it, and every per-feature command (`create-dialogs`, `create-surface`, `build-feature`, `generate-code`, `generate-enggspec`) continues to work using `@feature` for orphans and `@initiative/feature` for nested features
- `config.FeaturePath("password-reset")` resolves to `spec/intents/password-reset/` when `password-reset` is an orphan, and `config.FeaturePath("auth-overhaul/password-reset")` resolves to `spec/intents/auth-overhaul/password-reset/` when `password-reset` is nested inside `auth-overhaul`; both forms return identical-shaped results to callers
- Two features named `password-reset` in different initiatives (`spec/intents/auth-overhaul/password-reset/` and `spec/intents/billing/password-reset/`) coexist without error, and each resolves only via its own qualified identifier
- Listing `spec/intents/` shows a mix of initiative directories and orphan feature directories, and parlay can tell them apart structurally
- Creating a feature or initiative whose slug matches an existing entity in the same parent directory fails with a clear error naming the existing location; creating one whose slug matches something in a different parent succeeds
- A hybrid directory (contains both `intents.md` and subdirectories with `intents.md`) triggers a parlay error at detection time rather than silently picking one interpretation

---

## Create a Feature Inside an Initiative

**Goal**: Scaffold a new feature directly inside an initiative — creating the initiative on the fly if it doesn't yet exist — so the designer can go from "I have an idea" to authoring intents in a single command.
**Persona**: UX Designer
**Priority**: P1
**Context**: The designer is starting work on a new feature they already know belongs to a larger initiative. Forcing them to run three commands — create the initiative, create the feature, move the feature into the initiative — turns one mental action into bureaucracy that discourages using initiatives at all.
**Action**: Run `parlay add-feature <name> --initiative <initiative-name>`. If the named initiative already exists, create the feature inside it. If not, create the initiative directory first, then create the feature inside it. In both cases the end state is `spec/intents/<initiative-slug>/<feature-slug>/` containing the standard `intents.md` and `dialogs.md` scaffolding, and matching empty directories `spec/handoff/<initiative-slug>/<feature-slug>/` and `.parlay/build/<initiative-slug>/<feature-slug>/` are created at the same time so the three trees stay in lockstep.
**Objects**: feature, initiative, command-argument

**Constraints**:
- The existing `parlay add-feature <name>` (without `--initiative`) must continue to work, creating an orphan feature at `spec/intents/<feature-slug>/` with matching orphan paths under the parallel trees
- Auto-creating an initiative must be idempotent — running the command a second time with the same `--initiative` value must reuse the existing initiative directory, never duplicate or error on it
- The command output must distinguish the two side effects when both occur: "initiative created" vs. "feature added to existing initiative", so the designer is never surprised to discover a new top-level directory
- Scope-based collision — the feature slug must be unique **within the target initiative**. If a feature with the same slug already exists inside the named initiative, the command fails with an error naming the existing path. The same slug existing in a different initiative or as an orphan at the top level is NOT a collision.
- Top-level collision — the `--initiative` value must not collide with an existing orphan feature or a different initiative at the top level. If `--initiative auth-overhaul` is passed and `spec/intents/auth-overhaul/` already exists as an orphan feature (has an `intents.md` directly, not subdirectories), the command fails with a top-level-namespace collision error and must not attempt to convert the orphan feature into an initiative.
- Initiative slugs follow the same slugification rules as features (lowercase, hyphenated)
- The `--initiative` argument accepts either a raw human-readable name (`"auth overhaul"`) or an already-slugified form (`auth-overhaul`). Both are slugified on input; slugification is idempotent, so the two invocations are equivalent.
- If auto-creation of the initiative directories succeeds but the feature-directory creation subsequently fails (disk full, permissions change mid-command, etc.), parlay leaves the initiative directories (on all three trees, in deferred-classification state) in place rather than rolling them back. Re-running the command is idempotent and recovers cleanly; the leftover directories are harmless and may already reflect the designer's intent.
- When an initiative is auto-created, no `README.md` is written — the initiative directory starts empty except for the new feature. Narrative is always optional and designer-authored.

**Verify**:
- `parlay add-feature "password reset" --initiative "auth overhaul"` when the initiative does not exist creates both `spec/intents/auth-overhaul/` and `spec/intents/auth-overhaul/password-reset/` with `intents.md` and `dialogs.md`; the output reports "Initiative auth-overhaul created" and "Feature password-reset added"
- `parlay add-feature "sso setup" --initiative "auth overhaul"` run afterward creates only `spec/intents/auth-overhaul/sso-setup/`; the initiative is not re-created; the output reports only "Feature sso-setup added to initiative auth-overhaul"
- `parlay add-feature "password reset" --initiative "auth overhaul"` run a second time fails with an error referencing `spec/intents/auth-overhaul/password-reset/`
- `parlay add-feature "password reset" --initiative "billing"` succeeds even though a `password-reset` feature already exists inside `auth-overhaul`, because the collision rule is scope-based — the target scope is `spec/intents/billing/`, which does not yet contain `password-reset`
- `parlay add-feature "password reset"` (no `--initiative`) creates an orphan at `spec/intents/password-reset/` even when features named `password-reset` exist inside initiatives, because the target scope is the top level, which does not yet contain that slug
- `parlay add-feature "standalone thing"` (no `--initiative` flag) still creates an orphan at `spec/intents/standalone-thing/`
- `parlay add-feature "login" --initiative "password-reset"` fails with a top-level-namespace collision when `password-reset` already exists at the top level as an orphan feature (not an initiative)

---

## Create an Empty Initiative

**Goal**: Create a new initiative directory with no features inside it, so the designer can reserve the initiative's slug and optionally author a `README.md` narrative before any feature work begins.
**Persona**: UX Designer
**Priority**: P2
**Context**: The designer is starting to plan a large project and wants to write the initiative's "why" narrative up front — or simply reserve the name — before inventing any specific features. Forcing them to conjure a first feature name just to call `add-feature --initiative` creates fake content and muddles the thinking.
**Action**: Run `parlay new-initiative <name>`. Creates `spec/intents/<initiative-slug>/`, `spec/handoff/<initiative-slug>/`, and `.parlay/build/<initiative-slug>/` as empty directories in lockstep and reports the created paths.
**Objects**: initiative, command-argument

**Constraints**:
- Top-level collision — the initiative slug must not collide with an existing top-level entity (an initiative, or an orphan feature) under `spec/intents/`. This is the per-scope uniqueness rule from the first intent, applied to the top-level namespace.
- The command must be idempotent — running it a second time with the same name succeeds and reports "initiative already exists", never errors on the existing directory
- No `README.md` is auto-written; narrative is always optional and designer-authored
- An empty initiative created this way must be accepted by subsequent `parlay add-feature --initiative <slug>` and `parlay move-feature @feature --to <slug>` invocations without requiring re-creation
- The resulting directory is in deferred-classification state per the first intent: valid and addressable, but invisible to any future listing command (`parlay list-features`, `parlay list-initiatives`, or equivalent) until it contains at least one feature subdirectory
- The `<name>` argument accepts either a raw human-readable name (`"auth overhaul"`) or an already-slugified form (`auth-overhaul`), slugified on input.

**Verify**:
- `parlay new-initiative "auth overhaul"` creates `spec/intents/auth-overhaul/` as an empty directory
- `parlay new-initiative "auth overhaul"` run a second time succeeds and reports the directory already exists
- `parlay add-feature "login" --initiative "auth overhaul"` after a `new-initiative` call creates `spec/intents/auth-overhaul/login/` inside the pre-existing empty initiative directory, and the matching parallel paths under `spec/handoff/` and `.parlay/build/`; the initiative is not re-created
- `parlay move-feature @password-reset --to auth-overhaul` after a `new-initiative` call succeeds without re-creating the initiative
- `parlay new-initiative "password-reset"` fails with a top-level collision error when `password-reset` already exists at the top level (either as an orphan feature or as another initiative)
- `parlay new-initiative "password-reset"` succeeds even when `password-reset` exists as a nested feature inside some initiative — that nested feature is in a different scope, so there is no collision

---
