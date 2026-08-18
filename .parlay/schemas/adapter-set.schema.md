<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/multi-adapter
-->

# Adapter-Set Schema

File: `.parlay/adapter-set.yaml`. Pins which adapter occupies each adapter-kind slot in a project, declares per-target source roots, and authorizes cross-kind link relations.

## Structure

```yaml
name: <project-name>
targets:
  presentation: { adapter: <adapter-slug>, root: <project location — e.g. apps/web, or "." > }
  transport:    { adapter: <adapter-slug>, root: <project location — e.g. apps/api> }
  application:  { adapter: <adapter-slug>, root: <project location — e.g. apps/api> }
  persistence:  { adapter: <adapter-slug>, root: <project location — e.g. apps/api> }
links:
  - { from: presentation, relation: calls,      to: transport }
  - { from: transport,    relation: dispatches, to: application }
  - { from: application,  relation: persists,   to: persistence }
```

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Project name; used in deployer output and error messages. |
| `targets` | Yes | Map keyed by adapter kind. Permitted keys: `presentation`, `transport`, `application`, `persistence`. Each entry: `{ adapter, root }`. |
| `targets.<kind>.adapter` | Yes | The adapter slug — must reference a real `.parlay/adapters/<slug>.adapter.yaml`. |
| `targets.<kind>.root` | Yes | The **project location** this slot emits into — `apps/web`, `apps/api`, or `.` for a single-package repo. It substitutes for the adapter's `project-root`, NOT for its `source-root`: the framework's own source directory survives. (For a legacy adapter declaring no `project-root`, it replaces `source-root` outright — see `adapter-root-override-lossy`.) Must not collide with another target's root. |
| `links` | No | List of cross-kind relations. Each entry: `{ from, relation, to }`. Allowed relations: `calls`, `dispatches`, `persists`. |

## Validation rules

The validator enforces:

| Code | When it fires |
|---|---|
| `adapter-kind-unknown` | A `targets:` key is outside the closed set `{presentation, transport, application, persistence}`. |
| `adapter-set-duplicate-kind` | Two entries declare the same kind. |
| `adapter-set-adapter-missing` | `targets.<kind>.adapter` references a slug with no `.parlay/adapters/<slug>.adapter.yaml`. |
| `adapter-set-kind-mismatch` | The adapter referenced from a slot declares a different `kind:` than the slot the project assigns it to. |
| `adapter-root-override-lossy` | A **legacy** adapter (no `project-root:`) is pinned to a `root:` naming a different directory than its `source-root`, which it replaces — so every derived path loses that directory. Adapters declaring `project-root:` cannot hit this. |

### What `root:` replaces

`root:` names a **project location** and substitutes for the adapter's `file-conventions.project-root`. The adapter's `source-root` — the framework's own directory, `src/`, `src/app/`, `cmd/` — is left alone, because where a project sits and how a framework arranges its insides are different facts and the topology only knows the first.

```
emit base = (targets.<kind>.root, else the adapter's project-root) + the adapter's source-root
```

| Slot | adapter `project-root` | adapter `source-root` | `root:` | emits into |
|---|---|---|---|---|
| presentation | `.` | `src/` | `apps/web` | `apps/web/src/` |
| presentation | `.` | `src/app/` | `.` | `src/app/` |
| application | `apps/api` | `src` | `apps/api` | `apps/api/src/` |
| persistence | `apps/api` | `.` | `apps/api` | `apps/api/` |

**Why this is two fields.** It used to be one: `root:` replaced `source-root` outright. That is lossless only when `source-root` holds a project location, which is how the backend adapters used it — `nestjs-application` declared `source-root: apps/api` with `src/` inside its templates. Presentation adapters used it the other way, `source-root: "src/"` with templates starting at `features/…`, so the substitution deleted the framework's directory: `react-antd` pinned to `apps/web` derived `apps/web/features/…` while the app built from `apps/web/src/`. With `tsconfig`'s `include: ["src"]` those files were not merely misplaced — they were outside the TypeScript project, so nothing type-checked them, nothing bundled them, and the build stayed green by not seeing them. Three of the four bundled presets were wrong; the fourth was right only because its `root` happened to equal its `source-root`.

**Legacy adapters.** One declaring no `project-root:` keeps the old replace-`source-root` behaviour exactly, so upgrading parlay never relocates an existing project's output. `adapter-root-override-lossy` reports the shapes where that behaviour discards a directory — a signal to split the field, not a silent change of destination.

## Link enforcement

The link validator walks every cross-kind reference recorded in the buildfile's `targets:` block and rejects edges whose `(from-kind, to-kind)` pair is not present in `links:`.

| Code | When it fires |
|---|---|
| `adapter-set-link-violated` | A buildfile edge crosses kinds in a direction that `links:` does not authorize. |
| `adapter-set-link-missing` | A buildfile contains cross-kind edges but the project's adapter-set has no `links:` block. |
| `adapter-set-link-unfilled-slot` | A `links:` entry references a slot that is not declared in `targets:`. |

## Versioning

No `schema_version:` field (see `schema-versioning.schema.md` for the house rule) — the file's shape is pinned by the project's adapter-set topology (which kinds are filled, what links exist), not by an independent evolution timeline. Adding a new kind or relation to the closed sets this schema enforces is a schema-doc change, not a per-file version bump; there is no "old adapter-set.yaml" to migrate since the file only ever reflects the project's CURRENT topology.

## Backward compatibility

A project with only the `presentation:` slot filled (or no `.parlay/adapter-set.yaml` at all) continues to work — every multi-target validation rule consults `isMultiTarget(adapterSet)` and short-circuits when only presentation is filled. Adding the first non-presentation slot transitions the project into multi-target mode automatically; no explicit migration step is required.
