---
name: parlay-migrate-domain-operations
description: "Parlay: Migrate deprecated domain-model.operations entries into per-feature capabilities.yaml stubs"
---

<!--
parlay-feature: parlay-tool/multi-adapter
parlay-component: domain-operations-migration-prompt
-->

# Migrate Domain Operations

Walk each entry under `domain-model.operations:` and write a stub with `kind: unknown` into the appropriate feature's `capabilities.yaml`. The `operations:` field on `domain-model.yaml` is **deprecated** in favor of per-feature capabilities.

## Arguments

None.

<!-- parlay:active-root-aware -->
## Active root

Paths in this skill resolve against the active root. The CLI walks up from cwd, honors `--root <name>` and `PARLAY_ROOT`, and reads/writes `.parlay/domain-model.yaml` and `spec/intents/<feature>/capabilities.yaml` under whichever root resolves.

## Steps

1. **Run the CLI** — `parlay migrate-domain-operations`. The command reads the project `domain-model.yaml`, walks each entry under `operations:`, and for each:
   - Identifies candidate features (every directory under `spec/intents/`).
   - When ambiguous, prompts the designer for the target feature via stdin.
   - Writes a stub with `kind: unknown` into the chosen feature's `capabilities.yaml` (creating the file if needed).

2. **Set `kind:` explicitly** — every stub lands with `kind: unknown`. Build mode rejects this. For each stub, the designer reviews the prose carried over under `notes:` and sets `kind: command` or `kind: query` explicitly.

3. **Clear `domain-model.operations:`** — after migrating every entry, the designer manually clears the deprecated field. The migrator does NOT delete it automatically — that's the designer's call after reviewing the resulting capabilities.

## Behavior

- **Interactive.** Disambiguation happens via stdin prompts. CI-mode runs without a TTY pick the first candidate feature; check the output to confirm the routing landed correctly.
- **Idempotent at the field level.** A second run on a domain-model.yaml whose `operations:` has been cleared produces a `nothing to migrate` no-op.
- **No automatic deletion.** The migrator preserves the legacy `operations:` field until the designer clears it.

## Errors

- `domain-model-missing` — `.parlay/domain-model.yaml` does not exist. Suggests `parlay create-domain-model`.
- `parse-domain-model-failed` — YAML parse error. Fix and re-run.
