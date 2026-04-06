# Feature File Structure

## Project layout

```
.parlay/
  config.yaml                      ← Tool config (agent, sdd framework, prototype framework)
  schemas/                         ← Internal schema definitions (this folder at runtime)

spec/
  intents/                         ← All features
    <feature-name>/
      intents.md                   ← Human-authored
      dialogs.md                   ← Scaffolded → human-authored
      surface.md                   ← Generated, human-reviewed
      devspec/
        buildfile.yaml             ← Generated
        testcases.yaml             ← Generated
      enggspec/
        specification.md           ← Generated, human-reviewed

  pages/                           ← Optional page manifests
    <page-name>.page.md            ← Generated via lock-page, human-reviewed
```

## Feature files

| File | Created by | Editable by human | Appears after |
|---|---|---|---|
| `intents.md` | `/parlay add-feature` | Yes — primary source | Feature creation |
| `dialogs.md` | `/parlay add-feature` (empty) → `/parlay create-dialogs` (scaffolded) | Yes — primary source | Intents authored |
| `surface.md` | `/parlay create-surface` or `/parlay create-surface-by-figma` | Review and adjust only | Dialogs authored |
| `devspec/buildfile.yaml` | `/parlay build-feature` | No | Surface reviewed |
| `devspec/testcases.yaml` | `/parlay build-feature` | No | Surface reviewed |
| `enggspec/specification.md` | `/parlay generate-enggspec` | Review only | Prototype validated |

## Page files

| File | Created by | Editable by human | Appears after |
|---|---|---|---|
| `<page-name>.page.md` | `/parlay lock-page` | Review and adjust | Cross-feature layout needs an owner |

## Feature naming

- Lowercase, hyphen-separated: `upgrade-plan-creation`, `fleet-overview`
- Folder name = canonical identifier for `@feature-name` references
- `/parlay add-feature upgrade plan creation` → folder `upgrade-plan-creation`

## Rules

- `intents.md` and `dialogs.md` are the source of truth — everything else derives from them
- `surface.md`, `devspec/`, `enggspec/` are regeneratable. Preserve designer edits to surface.md during regeneration.
- `spec/pages/` is optional — don't create until `/parlay lock-page` is invoked
- Prototype code lives outside `spec/` (in `src/`, `app/`, etc.)
- Deleting a feature folder removes it from all assembled page views. Page manifests will flag missing fragments.
