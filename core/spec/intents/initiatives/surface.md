# Initiatives — Surface

---

## Initiative Orientation

**Shows**: message, data-tree, data-list
**Actions**: select-one, invoke
**Flow**: onboarding
**Source**: @initiatives/group-features-under-an-initiative

**Page**: initiatives
**Region**: main
**Order**: 1

**Notes**:
- Educational surface shown when the designer asks about grouping features, or encounters initiatives for the first time.
- The `message` explains the initiative concept (umbrella directory, orphans coexist alongside).
- The `data-tree` shows an example layout — spec/intents/auth-overhaul/password-reset/, spec/intents/auth-overhaul/sso-setup/, and a sibling orphan feature — so the designer can see the shape of the hierarchy concretely.
- The `data-list` states the three load-bearing rules: per-scope slug uniqueness, flat-hierarchy (one level only), top-level namespace shared by initiatives and orphan features.
- `select-one` offers three starting paths: (A) move existing features with /parlay-move-feature, (B) create an empty initiative first with /parlay-new-initiative, (C) create a brand-new feature inside an initiative with /parlay-add-feature --initiative. Each option is `invoke`-style — the designer runs the chosen command next.
- Two supplementary threads extend the fragment when the designer has follow-up questions: qualified-addressing explanation (`@initiative/feature` vs bare `@feature`), and external-tool reconciliation (plain `mv` requires /parlay-repair).
- `Page: initiatives` names a top-level topic rather than an existing cobra subcommand — this orientation is agent-delivered in the Claude Code workflow (when the designer asks about grouping) and would map to a future `parlay help initiatives` command if a CLI equivalent is added.

---

## Feature Creation Result

**Shows**: status, message, data-value
**Actions**: invoke
**Source**: @initiatives/create-a-feature-inside-an-initiative

**Page**: add-feature
**Region**: main
**Order**: 1

**Notes**:
- Output surface for `parlay add-feature <name> --initiative <initiative-name>`.
- `status` carries the outcome: success (initiative created + feature added, or feature added to existing initiative), scope collision (feature already exists inside the target initiative), top-level namespace collision (`--initiative` value matches an orphan feature), or partial failure warning (initiative directories created, feature directory creation failed).
- `message` names the side effects in human-readable form — the intent requires distinguishing "initiative created" from "feature added to existing initiative" so the designer is never surprised by a new top-level directory.
- `data-value` shows the created (or conflicting) qualified paths across the three trees so the designer can verify the result or investigate the collision.
- A follow-up `invoke` action directs the designer to /parlay-scaffold-dialogs for the newly-created feature.
- Collision messages always name the existing path and suggest the next step (pick a different name, move the existing entity first). Partial-failure messages always state that re-running the command is idempotent.

---

## Empty Initiative Creation Result

**Shows**: status, message, data-value
**Actions**: invoke
**Source**: @initiatives/create-an-empty-initiative

**Page**: new-initiative
**Region**: main
**Order**: 1

**Notes**:
- Output surface for `parlay new-initiative <name>`.
- `status` carries the outcome: success (all three parallel directories created), idempotent no-op (initiative already exists at this top-level slug), or top-level namespace collision (slug exists as an orphan feature).
- `message` reports the created paths on all three trees, notes that the directory is in deferred classification until it gains its first feature, and clarifies that a README.md is narrative only (not a classification signal).
- `data-value` lists the created paths so the designer can confirm or navigate to them.
- A "next steps" message directs the designer toward /parlay-add-feature --initiative or /parlay-move-feature --to for populating the initiative — both are `invoke` follow-ups.

---
