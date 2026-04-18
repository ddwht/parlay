# Initiatives — Dialogs

---

### Group Features under an Initiative

**Trigger**: The designer has accumulated several related features at the top level of `spec/intents/` and wants to group them, or is planning a multi-feature project and wants to start organized.

User: I've got a bunch of features at spec/intents/ that belong together — password-reset, sso-setup, login-flow, all part of an auth overhaul. Can I group them?
System: Yes — use an **initiative**. An initiative is a directory under spec/intents/ that holds member features; orphan features (not in any initiative) sit at the top level alongside.
System: With grouping, your layout becomes:
System:   spec/intents/auth-overhaul/                    ← initiative
System:   spec/intents/auth-overhaul/password-reset/     ← feature
System:   spec/intents/auth-overhaul/sso-setup/          ← feature
System:   spec/intents/auth-overhaul/login-flow/         ← feature
System:   spec/intents/some-unrelated-feature/           ← orphan (still works)
System: Member features are addressed as @auth-overhaul/password-reset. Orphans stay bare: @some-unrelated-feature.
System: Under the hood, spec/handoff/ and .parlay/build/ mirror the same hierarchy in lockstep — parlay keeps all three trees in sync whenever you use its commands.
System: A few rules to know:
System: - Feature slugs are unique **per parent**, not globally. Two initiatives can each have a `password-reset` feature without conflict.
System: - Initiatives are flat — one level only. You can't nest an initiative inside another initiative.
System: - At the top level, initiative names and orphan feature names share one namespace. An initiative and an orphan feature can't share a slug at spec/intents/.
System: How would you like to start?
  A: Move my existing features into a new initiative
  B: Create an empty initiative first, then populate it
  C: Create a brand-new feature directly inside an initiative
User: Selects A
System: Run /parlay-move-feature @password-reset --to auth-overhaul (and the same for @sso-setup and @login-flow). The initiative auto-creates on the first move, so you don't need to create it separately.
System: After the moves, the three features will live at spec/intents/auth-overhaul/<feature>/ and be addressed as @auth-overhaul/<feature>. Their slug-keyed build and handoff artifacts get relocated in lockstep; nothing is lost.

#### Branch: Asking about qualified addressing

User: So if two initiatives both have a `password-reset` feature, how do I tell them apart?
System: Each is addressed by its qualified identifier — @auth-overhaul/password-reset and @billing/password-reset. The / mirrors the filesystem. A bare @password-reset always means the orphan at the top level; it never reaches into an initiative.
System: If the orphan and a nested feature both happen to be named `password-reset`, that's fine — they live in different scopes and each resolves uniquely via its qualified identifier.

#### Branch: Hybrid directory detected

User: I added an intents.md to my initiative directory and also have features inside it. Now parlay is erroring.
System: That's a **hybrid directory** — it contains both an `intents.md` at its own level (making it look like a feature) and subdirectories with their own `intents.md` files (making it look like an initiative). A directory must be one or the other, never both.
System: [ERR] Hybrid directory at `spec/intents/auth-overhaul/`: contains `intents.md` (feature) and subdirectories with `intents.md` (initiative). A directory cannot be both — pick one role.
System: To fix, choose:
System: - **Keep it as an initiative**: remove the top-level `intents.md` — move its content into a new feature subdirectory instead (e.g., `spec/intents/auth-overhaul/overview/intents.md`).
System: - **Keep it as a feature**: move the subdirectories out with /parlay-move-feature.

#### Branch: Sub-initiative detected

User: I tried to organize features into sub-groups inside an initiative — like spec/intents/auth-overhaul/passwords/login-flow/ where passwords/ itself has child features. Parlay rejects it.
System: Initiatives are **flat — one level only**. A directory at depth 2 or deeper that would classify as an initiative (has subdirectories containing `intents.md`) is a sub-initiative, and that's invalid.
System: [ERR] Sub-initiative detected at `spec/intents/auth-overhaul/passwords/`: contains subdirectories with `intents.md` at depth 2, violating the flat-hierarchy rule. Initiatives can only be direct children of `spec/intents/`.
System: To fix:
System: - **Promote** the sub-group to its own top-level initiative: move `passwords/` up to `spec/intents/passwords/`.
System: - **Flatten**: move the child features directly under `auth-overhaul/` and drop the intermediate `passwords/` directory.

#### Branch: Duplicate-slug corruption detected

User: I duplicated a feature directory by hand and now parlay won't resolve the slug.
System: Parlay found two directories under the same parent whose names slugify to the same identifier. Different directory names can produce identical slugs — for example, `password-reset/` and `password_reset/` both become `password-reset`.
System: [ERR] Duplicate slug `password-reset` under `spec/intents/auth-overhaul/`: both `password-reset/` and `password_reset/` resolve to the same identifier. Remove or rename one, then run /parlay-repair to reconcile the three trees.
System: This can't happen through parlay commands — they enforce per-scope uniqueness. It results from external filesystem operations (copy, manual mkdir).

#### Branch: Empty initiative not appearing in listings

User: I created an initiative and added a README.md, but it doesn't show up when I list initiatives or features.
System: An initiative with no feature subdirectories is in **deferred classification** — valid, accepted by parlay commands, but invisible to listing commands until it contains at least one feature.
System: A `README.md` is narrative, not a classification signal. The directory won't appear in listings until you add a feature:
System: - /parlay-add-feature <name> --initiative auth-overhaul
System: - /parlay-move-feature @<feature> --to auth-overhaul

#### Branch: Asking about external tools

User: What if I just rename a folder with mv or drag it in my IDE?
System: Parlay can't see external filesystem operations, so plain mv or IDE renames will leave spec/handoff/ and .parlay/build/ stale. Commands that resolve qualified identifiers under the new name will fail until you reconcile.
System: To put the project back in lockstep after external operations, run /parlay-repair. It validates the three trees and asks interactive questions when the correct reconciliation is ambiguous.

---

### Create a Feature Inside an Initiative

**Trigger**: /parlay-add-feature ==feature-name== --initiative ==initiative-name==

User: /parlay-add-feature password reset --initiative auth overhaul
System (background): Runs `parlay add-feature "password reset" --initiative "auth overhaul"`.
System (background): Slugifies inputs — feature → `password-reset`, initiative → `auth-overhaul`. Checks the top-level namespace for initiative collisions and the `auth-overhaul/` scope for feature collisions.
System (condition: initiative does not yet exist): Creates spec/intents/auth-overhaul/, spec/handoff/auth-overhaul/, and .parlay/build/auth-overhaul/ as empty directories in lockstep.
System (background): Creates spec/intents/auth-overhaul/password-reset/intents.md and dialogs.md from templates. Creates matching empty directories spec/handoff/auth-overhaul/password-reset/ and .parlay/build/auth-overhaul/password-reset/.
System (condition: initiative was auto-created just now): Initiative auth-overhaul created.
System: Feature password-reset added to initiative auth-overhaul at spec/intents/auth-overhaul/password-reset/.
System: Start with intents.md. When ready, run: /parlay-scaffold-dialogs @auth-overhaul/password-reset

#### Branch: Initiative already exists

User: /parlay-add-feature sso setup --initiative auth overhaul
System (background): Finds spec/intents/auth-overhaul/ already exists as an initiative. Creates only the feature subdirectories across the three trees.
System: Feature sso-setup added to initiative auth-overhaul at spec/intents/auth-overhaul/sso-setup/.

#### Branch: Same feature slug allowed in a different initiative

User: /parlay-add-feature password reset --initiative billing
System (background): `password-reset` exists under auth-overhaul/, but the target scope is billing/ — a different parent. No collision under the per-scope uniqueness rule.
System: Feature password-reset added to initiative billing at spec/intents/billing/password-reset/.

#### Branch: Scope-based collision — feature already in this initiative

User: /parlay-add-feature password reset --initiative auth overhaul
System (background): Finds existing spec/intents/auth-overhaul/password-reset/.
System: [ERR] Feature `password-reset` already exists inside initiative `auth-overhaul` at spec/intents/auth-overhaul/password-reset/. Pick a different feature name, or move the existing feature somewhere else first.

#### Branch: Top-level collision — initiative name taken by an orphan feature

User: /parlay-add-feature login --initiative password-reset
System (background): Finds spec/intents/password-reset/ already exists as an orphan feature (it has an intents.md directly, not subdirectories).
System: [ERR] `password-reset` exists at the top level as a feature, not an initiative. A feature and an initiative can't share a top-level slug. Either pick a different initiative name, or first move the existing `password-reset` feature into an initiative with /parlay-move-feature.

#### Branch: Partial failure — initiative created, feature creation fails

User: /parlay-add-feature password reset --initiative auth overhaul
System (background): Creates spec/intents/auth-overhaul/ and matching parallel directories successfully. Feature directory creation fails (disk full).
System: [WARN] Created initiative auth-overhaul (in deferred classification — no features yet), but couldn't create feature password-reset inside it: disk full. Free some space and re-run the same command — it's idempotent and will reuse the existing initiative directory to finish the job.

---

### Create an Empty Initiative

**Trigger**: /parlay-new-initiative ==initiative-name==

User: /parlay-new-initiative auth overhaul
System (background): Runs `parlay new-initiative "auth overhaul"`. Slugifies to `auth-overhaul`. Checks the top-level namespace.
System (background): Creates spec/intents/auth-overhaul/, spec/handoff/auth-overhaul/, and .parlay/build/auth-overhaul/ as empty directories in lockstep.
System: Initiative auth-overhaul created at spec/intents/auth-overhaul/ (with matching empty parallel paths under spec/handoff/ and .parlay/build/).
System: The directory is empty and in deferred classification — it becomes a proper initiative once it contains at least one feature subdirectory. Adding a README.md on its own doesn't change that — a README is narrative, not a classification signal.
System: Next steps:
System: - Optionally write spec/intents/auth-overhaul/README.md with the initiative's "why" and scope notes.
System: - Add features with /parlay-add-feature <name> --initiative auth-overhaul.
System: - Or move existing features in with /parlay-move-feature @<feature> --to auth-overhaul.

#### Branch: Idempotent second run

User: /parlay-new-initiative auth overhaul
System (background): Finds spec/intents/auth-overhaul/ already exists at the top level. No changes to any tree.
System: Initiative auth-overhaul already exists at spec/intents/auth-overhaul/ — no changes made.

#### Branch: Top-level collision with an existing orphan feature

User: /parlay-new-initiative password-reset
System (background): Finds spec/intents/password-reset/ already exists as an orphan feature (has intents.md directly).
System: [ERR] `password-reset` exists at the top level as a feature. An initiative can't share a top-level slug with a feature. Pick a different name, or first move the existing feature into an initiative with /parlay-move-feature.

#### Branch: Same slug allowed as a nested feature

User: /parlay-new-initiative password-reset
System (background): `password-reset` exists inside auth-overhaul/ as a nested feature — that's a different scope from the top level. No top-level collision.
System: Initiative password-reset created at spec/intents/password-reset/.

---
