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

None. Flags:

- `--feature <slug>` — explicit target feature (accepts a leading `@`) used whenever an operation has more than one candidate feature. Overrides both the stdin prompt and the headless hard error.
- `--non-interactive` — force headless mode even when a TTY is attached, so ambiguous targeting hard-errors instead of prompting.

<!-- parlay:active-root-aware -->
## Active root

Paths in this skill resolve against the active root. The CLI walks up from cwd, honors `--root <name>` and `PARLAY_ROOT`, and reads/writes `.parlay/domain-model.yaml` and `spec/intents/<feature>/capabilities.yaml` under whichever root resolves.

## Steps

1. **Run the CLI** — `parlay migrate-domain-operations`. The command reads the project `domain-model.yaml`, walks each entry under `operations:`, and for each:
   - Identifies candidate features (every directory under `spec/intents/`).
   - When ambiguous and `--feature` is not set: prompts the designer for the target feature via stdin (interactive) or hard-errors with `ambiguous-target` (headless — see Behavior below).
   - When ambiguous and `--feature <slug>` is set: routes to that feature directly, no prompt.
   - Writes a stub with `kind: unknown` into the chosen feature's `capabilities.yaml` (creating the file if needed).

2. **Set `kind:` explicitly** — every stub lands with `kind: unknown`. Build mode rejects this. For each stub, the designer reviews the prose carried over under `notes:` and sets `kind: command` or `kind: query` explicitly.

3. **Clear `domain-model.operations:`** — after migrating every entry, the designer manually clears the deprecated field. The migrator does NOT delete it automatically — that's the designer's call after reviewing the resulting capabilities.

## Behavior

- **Interactive.** Disambiguation happens via stdin prompts.
- **Headless/non-interactive.** When no TTY is attached (or `--non-interactive` is set), ambiguous feature targeting is never guessed — it is a hard error. The migrator stops at the first ambiguous operation, lists the candidate features, and tells the caller to re-run with `--feature <slug>` naming the target. This mirrors build-feature's headless contract (steps 7.6–7.9): headless mode fails loud on ambiguity instead of silently picking a default. The `--feature` flag works in both modes — set it up front to skip disambiguation entirely.
- **Idempotent at the field level.** A second run on a domain-model.yaml whose `operations:` has been cleared produces a `nothing to migrate` no-op.
- **No automatic deletion.** The migrator preserves the legacy `operations:` field until the designer clears it.

## Errors

- `domain-model-missing` — `.parlay/domain-model.yaml` does not exist. Suggests `parlay create-domain-model`.
- `parse-domain-model-failed` — YAML parse error. Fix and re-run.
- `ambiguous-target` — headless/non-interactive mode hit an operation with more than one candidate feature. Lists the candidate features and tells the caller to re-run with `--feature <slug>` naming the target.
