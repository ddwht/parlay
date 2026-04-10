# Feature File Structure

## Project layout

Parlay projects use three zones with strict ownership:

```
spec/                              ← Designer authors and reviews; engineering consumes
  intents/                         ← Designer-authored input (per feature)
    <feature-name>/
      intents.md                   ← Human-authored
      dialogs.md                   ← Scaffolded → human-authored
      surface.md                   ← Generated, human-reviewed
      domain-model.md              ← Generated, human-reviewed
  handoff/                         ← Engineering-consumed output (per feature)
    <feature-name>/
      specification.md             ← Generated, human-reviewed
  pages/                           ← Optional cross-feature page manifests
    <page-name>.page.md            ← Generated via lock-page, human-reviewed

.parlay/                           ← Tool internals — never user-facing
  config.yaml                      ← Tool config (agent, sdd framework, prototype framework)
  blueprint.yaml                   ← Application blueprint — team-authored, project-level singleton
  schemas/                         ← Internal schema definitions
  adapters/                        ← Framework adapters
  build/                           ← Internal build artifacts (per feature)
    <feature-name>/
      buildfile.yaml               ← Generated, internal
      testcases.yaml               ← Generated, internal
      .baseline.yaml               ← Drift detection baseline
```

## Zones

| Zone | Audience | What lives here |
|---|---|---|
| `spec/intents/` | Designer authors and reviews | Per-feature design source: intents, dialogs, surface, domain model |
| `spec/handoff/` | Engineering consumes | Per-feature engineering specification |
| `.parlay/` | Tool only — never user-facing | Config, blueprint, schemas, adapters, internal build artifacts |

## Feature files

| File | Created by | Editable by human | Appears after |
|---|---|---|---|
| `spec/intents/<feature>/intents.md` | `/parlay add-feature` | Yes — primary source | Feature creation |
| `spec/intents/<feature>/dialogs.md` | `/parlay add-feature` (empty) → `/parlay create-dialogs` (scaffolded) | Yes — primary source | Intents authored |
| `spec/intents/<feature>/surface.md` | `/parlay create-surface` or `/parlay create-surface-by-figma` | Review and adjust only | Dialogs authored |
| `spec/intents/<feature>/domain-model.md` | `/parlay extract-domain-model` | Review and adjust | Intents and dialogs authored |
| `.parlay/build/<feature>/buildfile.yaml` | `/parlay build-feature` | No — tool internal | Surface reviewed |
| `.parlay/build/<feature>/testcases.yaml` | `/parlay build-feature` | No — tool internal | Surface reviewed |
| `.parlay/build/<feature>/.baseline.yaml` | `/parlay build-feature` | No — tool internal | Surface reviewed |
| `spec/handoff/<feature>/specification.md` | `/parlay generate-enggspec` | Review only | Prototype validated |

## Page files

| File | Created by | Editable by human | Appears after |
|---|---|---|---|
| `spec/pages/<page-name>.page.md` | `/parlay lock-page` | Review and adjust | Cross-feature layout needs an owner |

## Feature naming

- Lowercase, hyphen-separated: `upgrade-plan-creation`, `fleet-overview`
- Folder name = canonical identifier for `@feature-name` references
- `/parlay add-feature upgrade plan creation` → folder `upgrade-plan-creation`
- The same `<feature-name>` is reused across all three zones: `spec/intents/<feature>/`, `spec/handoff/<feature>/`, and `.parlay/build/<feature>/`

## Rules

- `intents.md` and `dialogs.md` are the source of truth — everything else derives from them.
- The three zones are strict: never write designer files to `spec/handoff/` or `.parlay/`; never write internal artifacts to `spec/intents/` or `spec/handoff/`; never write engineering output to `spec/intents/` or `.parlay/`.
- `surface.md`, `domain-model.md`, everything under `.parlay/build/`, and `spec/handoff/` are regeneratable. Preserve human edits to `surface.md` and `domain-model.md` during regeneration.
- `testcases.yaml` is a tool internal. It drives cross-validation and feeds spec generation, but is **not** handed off to engineering. Engineering writes their own real tests from `specification.md`.
- `specification.md` is currently the only handoff artifact. Future Phase 8 additions (fixtures, API stubs, etc.) will also live under `spec/handoff/<feature>/`.
- `spec/pages/` is optional — don't create until `/parlay lock-page` is invoked.
- `.parlay/build/` is created during `parlay init` and populated per-feature by `/parlay build-feature`.
- `spec/handoff/` is created during `parlay init` and populated per-feature by `/parlay generate-enggspec`.
- Prototype code lives outside `spec/` and `.parlay/` (in `src/`, `cmd/`, `app/`, etc.).
- Deleting a feature folder under `spec/intents/<feature>/` should also clean up `spec/handoff/<feature>/` and `.parlay/build/<feature>/`. Page manifests will flag missing fragments.
