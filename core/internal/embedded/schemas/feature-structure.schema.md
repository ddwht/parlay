# Feature File Structure

## Project layout

Parlay projects use three zones with strict ownership:

```
<activeRoot>/                      ← Parlay project root (repo-level root, or a registered child root)
  domain-model.yaml                ← Canonical domain model — ONE per active root, never per-feature

  spec/                            ← Designer authors and reviews; engineering consumes
    intents/                       ← Designer-authored input (per feature)
      <feature-name>/
        intents.md                 ← Human-authored
        dialogs.md                 ← Scaffolded → human-authored
        surface.yaml                ← Generated, human-reviewed (surface.md is the legacy form — see Format rationale)
        capabilities.yaml           ← Generated, human-reviewed (operation-shaped backend behavior)
        infrastructure.md           ← Generated, human-reviewed (architectural prose)
        <page-name>.layout.yaml     ← Generated, human-reviewed (optional — per-page layout tree)
    handoff/                       ← Engineering-consumed output (per feature)
      <feature-name>/
        specification.md           ← Generated, human-reviewed
    pages/                         ← Optional cross-feature page manifests
      <page-name>.page.md          ← Generated via lock-page, human-reviewed (may embed a `## Layout` section)

.parlay/                           ← Tool internals — never user-facing
  config.yaml                      ← Tool config (agent, sdd framework, prototype framework)
  blueprint.yaml                   ← Application blueprint — team-authored, project-level singleton
  adapter-set.yaml                 ← Pins adapter slot topology — multi-target projects only
  schemas/                         ← Internal schema definitions
  adapters/                        ← Framework adapters
  build/                           ← Internal build artifacts (per feature)
    <feature-name>/
      buildfile.yaml                ← Generated, internal
      testcases.yaml                 ← Generated, internal
      coverage-review.yaml           ← Generated, internal (records suite-approval decisions)
      design-spec.yaml               ← Generated from Figma (optional), internal
      .baseline.yaml                 ← Drift detection baseline
```

## Zones

| Zone | Audience | What lives here |
|---|---|---|
| `<activeRoot>/domain-model.yaml` | Designer authors and reviews | The project's one canonical domain model — entities, relationships, shared vocabulary. Never per-feature. |
| `spec/intents/` | Designer authors and reviews | Per-feature design source: intents, dialogs, and whichever subset of the four co-equal spec artifacts (surface, capabilities, infrastructure) the feature needs, plus optional per-page layout trees |
| `spec/handoff/` | Engineering consumes | Per-feature engineering specification |
| `.parlay/` | Tool only — never user-facing | Config, blueprint, adapter-set, schemas, adapters, internal build artifacts |

## The four co-equal spec artifacts

A feature's `spec/intents/<feature-name>/` directory holds intents.md and dialogs.md (designer-authored, primary source) plus any subset of four co-equal generated artifacts. None of the four is a stand-in for another — each covers an orthogonal concern:

- **surface.yaml** (or legacy `surface.md`) — visible output: what the user sees, page assemblies, dialog turns.
- **capabilities.yaml** — operation-shaped backend behavior: closed-vocabulary commands and queries against domain entities.
- **infrastructure.md** — architectural prose for concerns that do not reduce to operations: boundaries, probes, allowlists, dependency pins, and similar shape constraints on the codebase.
- The project's single `domain-model.yaml` — entities, relationships, and shared vocabulary the feature references. This is a project-level singleton at `<activeRoot>/domain-model.yaml`, not a per-feature file (see below).

A feature picks whichever subset it needs, decided by `/parlay-create-artifacts`. Purely user-facing features have only `surface.yaml`; features that expose backend operations have `capabilities.yaml`; features that introduce architectural prose have `infrastructure.md`; many features have several of these in combination.

## Format rationale

File format follows a simple rule: designer-authored prose is markdown; generated closed-vocabulary artifacts are YAML. `intents.md` and `dialogs.md` are markdown because a human writes and edits them directly. `capabilities.yaml` and `domain-model.yaml` are YAML because they hold a closed, machine-validated vocabulary (operation kinds, entity fields) that the tool parses structurally. `surface.yaml` follows the same YAML rule as its target format, with `surface.md` retained as the legacy form during the migration window (see `surface.schema.md`'s resolution-precedence section). `infrastructure.md` stays markdown — it is by definition architectural prose that does not reduce to a closed operation vocabulary, so there is no structural schema to gain by moving it to YAML.

## Domain model: one per active root

`domain-model.yaml` lives at `<activeRoot>/domain-model.yaml` — exactly one canonical model per active root, never per-feature. It is edited by hand, regenerated by `/parlay-create-domain-model`, and consumed by every read path (`create-domain-model`, `load-domain-model`, `build-feature`, `generate-code`, `migrate-domain-model`). A legacy per-feature `domain-model.md` may still exist in older projects, but it is never parsed or merged — see `domain-model.schema.md` and `parlay migrate-domain-model` for the conversion path off of it.

## Feature files

| File | Created by | Editable by human | Appears after |
|---|---|---|---|
| `spec/intents/<feature>/intents.md` | `/parlay add-feature` | Yes — primary source | Feature creation |
| `spec/intents/<feature>/dialogs.md` | `/parlay add-feature` (empty) → `/parlay scaffold-dialogs` (scaffolded) | Yes — primary source | Intents authored |
| `spec/intents/<feature>/surface.yaml` (or legacy `surface.md`) | `/parlay create-artifacts` | Review and adjust only | Dialogs authored, if the feature has surface signals |
| `spec/intents/<feature>/capabilities.yaml` | `/parlay create-artifacts` | Review and adjust only | Dialogs authored, if the feature has operation signals |
| `spec/intents/<feature>/infrastructure.md` | `/parlay create-artifacts` | Review and adjust only | Dialogs authored, if the feature has architectural signals |
| `spec/intents/<feature>/<page-name>.layout.yaml` | `/parlay create-artifacts` (optional) | Review and adjust only | Surface reviewed, if the page needs an explicit layout tree |
| `.parlay/build/<feature>/design-spec.yaml` | `/parlay reference-design-spec` | No — tool internal (optional, from Figma) | Surface reviewed, Figma link available |
| `.parlay/build/<feature>/buildfile.yaml` | `/parlay build-feature` | No — tool internal | At least one spec artifact reviewed |
| `.parlay/build/<feature>/testcases.yaml` | `/parlay build-feature` | No — tool internal | At least one spec artifact reviewed |
| `.parlay/build/<feature>/coverage-review.yaml` | `/parlay review-coverage` | No — tool internal | Testcases generated |
| `.parlay/build/<feature>/.baseline.yaml` | `/parlay build-feature` | No — tool internal | At least one spec artifact reviewed |
| `spec/handoff/<feature>/specification.md` | `/parlay generate-enggspec` | Review only | Prototype validated |

## Page files

| File | Created by | Editable by human | Appears after |
|---|---|---|---|
| `spec/pages/<page-name>.page.md` | `/parlay lock-page` | Review and adjust | Cross-feature layout needs an owner |

A page manifest may embed an optional `## Layout` section (a fenced YAML block conforming to `layout.schema.md`) alongside its fragment-ordering body. Per-feature `<page-name>.layout.yaml` files under `spec/intents/<feature>/` and the `## Layout` section embedded in a page manifest describe the same layout-tree shape; the page schema owns the embedding rule, the layout schema owns the tree shape.

## Feature naming

- Lowercase, hyphen-separated: `upgrade-plan-creation`, `fleet-overview`
- Folder name = canonical identifier for `@feature-name` references
- `/parlay add-feature upgrade plan creation` → folder `upgrade-plan-creation`
- The same `<feature-name>` is reused across all three zones: `spec/intents/<feature>/`, `spec/handoff/<feature>/`, and `.parlay/build/<feature>/`

## Rules

- `intents.md` and `dialogs.md` are the source of truth — everything else derives from them.
- The three zones are strict: never write designer files to `spec/handoff/` or `.parlay/`; never write internal artifacts to `spec/intents/` or `spec/handoff/`; never write engineering output to `spec/intents/` or `.parlay/`.
- `surface.yaml`, `capabilities.yaml`, `infrastructure.md`, everything under `.parlay/build/`, and `spec/handoff/` are regeneratable. Preserve human edits to `surface.yaml`, `capabilities.yaml`, and `infrastructure.md` during regeneration.
- `domain-model.yaml` is regeneratable via `/parlay-create-domain-model`, but hand-editing is a first-class flow — both paths must produce structurally identical files.
- `testcases.yaml` is a tool internal. It drives cross-validation and feeds spec generation, but is **not** handed off to engineering. Engineering writes their own real tests from `specification.md`.
- `specification.md` is currently the only handoff artifact. Future Phase 8 additions (fixtures, API stubs, etc.) will also live under `spec/handoff/<feature>/`.
- `spec/pages/` is optional — don't create until `/parlay lock-page` is invoked.
- **All three per-feature directories are created together, when the feature is created.** `parlay add-feature` makes `spec/intents/<feature>/`, `spec/handoff/<feature>/` and `.parlay/build/<feature>/` in one step, whether or not the feature sits inside an initiative; `parlay new-initiative` does the same for the initiative's own directory. The handoff and build directories start empty — they are *created* eagerly and *filled* later. This is the rule `parlay repair` and `parlay status`'s `trees:` line both enforce: a feature missing any of the three is a mismatch to repair, not a phase not yet reached.
- `.parlay/build/` is created during `parlay init`; its per-feature directory is created by `parlay add-feature` and populated by `/parlay build-feature` (and `/parlay review-coverage` for `coverage-review.yaml`).
- `spec/handoff/` is created during `parlay init`; its per-feature directory is created by `parlay add-feature` and populated by `/parlay generate-enggspec`.
- Prototype code lives outside `spec/` and `.parlay/` (in `src/`, `cmd/`, `app/`, etc.).
- Deleting a feature folder under `spec/intents/<feature>/` should also clean up `spec/handoff/<feature>/` and `.parlay/build/<feature>/`. Page manifests will flag missing fragments. The project's `domain-model.yaml` is unaffected — it is not per-feature.
