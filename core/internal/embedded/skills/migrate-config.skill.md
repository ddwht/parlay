<!--
parlay-feature: parlay-tool/multi-adapter
parlay-component: config-migration-result
-->

# Migrate Config

Convert the legacy `.parlay/config.yaml` `prototype-framework:` field into a single-target presentation `.parlay/adapter-set.yaml`. Idempotent: re-running on already-migrated projects is a no-op.

## Arguments

None.

<!-- parlay:active-root-aware -->
## Active root

Paths in this skill resolve against the active root. The CLI walks up from cwd, honors `--root <name>` and `PARLAY_ROOT`, and reads/writes `.parlay/config.yaml` and `.parlay/adapter-set.yaml` under whichever root resolves.

## Steps

1. **Run the CLI** — `parlay migrate-config`. The command reads `.parlay/config.yaml`, extracts the `prototype-framework:` field, slugs it into an adapter name (e.g., "Go CLI" → `go-cli`), and writes `.parlay/adapter-set.yaml` with a single `presentation:` slot pointing at the matching adapter.

2. **Review the output** — the command prints a one-line conversion summary plus the deprecation note. The legacy `prototype-framework:` field stays parseable in v1; outright removal is owned by a separate deprecation feature.

3. **Next step** — run `/parlay-migrate-spec` to convert each feature's `surface.md` into the new `surface.yaml` form, then `/parlay-migrate-capabilities` to extract operation-shaped fragments from `infrastructure.md` into per-feature `capabilities.yaml`.

## Behavior

- **Idempotent.** If `.parlay/adapter-set.yaml` already exists, the command leaves it alone and prints a one-line skip message.
- **No-op when no legacy field.** If `prototype-framework:` is absent or empty, the command prints `no legacy fields detected; nothing to migrate` and exits cleanly.
- **Adapter resolution.** The slug is computed from the legacy label using a small mapping table (Go CLI → `go-cli`, React + Ant Design → `react-antd`, etc.). Unrecognized labels fall through to a kebab-case transformation.

## Errors

- `read-config-failed` — `.parlay/config.yaml` is missing or unreadable. Run `parlay init` first.
- `parse-config-failed` — `.parlay/config.yaml` is malformed YAML. Fix the YAML and re-run.
