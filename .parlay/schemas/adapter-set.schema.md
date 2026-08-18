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
  presentation: { adapter: <adapter-slug>, root: <source-root> }
  transport:    { adapter: <adapter-slug>, root: <source-root> }
  application:  { adapter: <adapter-slug>, root: <source-root> }
  persistence:  { adapter: <adapter-slug>, root: <source-root> }
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
| `targets.<kind>.root` | Yes | Source root the chosen adapter emits into. Must not collide with another target's root. |
| `links` | No | List of cross-kind relations. Each entry: `{ from, relation, to }`. Allowed relations: `calls`, `dispatches`, `persists`. |

## Validation rules

The validator enforces:

| Code | When it fires |
|---|---|
| `adapter-kind-unknown` | A `targets:` key is outside the closed set `{presentation, transport, application, persistence}`. |
| `adapter-set-duplicate-kind` | Two entries declare the same kind. |
| `adapter-set-adapter-missing` | `targets.<kind>.adapter` references a slug with no `.parlay/adapters/<slug>.adapter.yaml`. |
| `adapter-set-kind-mismatch` | The adapter referenced from a slot declares a different `kind:` than the slot the project assigns it to. |
| `adapter-root-override-lossy` | `targets.<kind>.root` names a different directory than the adapter's `source-root`, which it replaces — so every derived path loses that directory. |

### `root:` replaces `source-root`, and that is only safe one way

`targets.<kind>.root` substitutes for the adapter's `file-conventions.source-root` during plan derivation. Whether that loses information depends on which of two things the adapter put in `source-root`, and nothing forces a choice:

- **A project location** — `nestjs-application` declares `source-root: apps/api` and carries `src/` inside its path templates. Swapping the project location leaves the framework's directory intact. Lossless.
- **A framework convention directory** — `react-antd` declares `source-root: "src/"` and its templates start at `features/…`. Replacing that with `apps/web` deletes `src/` from every derived path, so components land at `apps/web/features/…` while the app builds from `apps/web/src/`. With `tsconfig`'s `include: ["src"]` the files are not merely misplaced, they are outside the TypeScript project — not type-checked, not bundled, and the build stays green by not seeing them.

`adapter-root-override-lossy` fires when the two directories differ, which is exactly the second case. Until the underlying ambiguity is resolved in the adapter model, the fix is one of:

- move the framework's directory into the adapter's `paths:` templates and leave `source-root` naming the project location; or
- set `root:` to the same directory the adapter declares, when the adapter's `source-root` really is where code should land.

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
