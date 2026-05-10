<!--
parlay-feature: parlay-tool/multi-adapter
parlay-component: capabilities-migration-operations-extraction
parlay-extends: parlay-tool/multi-adapter/pattern-fragment-classifier
-->

# Migrate Capabilities

Extract operation-shaped fragments from each feature's `infrastructure.md` into `capabilities.yaml`. Residual paragraphs are classified by detected shape (pipeline, registry, dispatcher, traversal, resolver, validator, aspect, cache, migrator, hook, helper) and recorded in a per-feature migration report. The migrator only writes the report; `capabilities.yaml`, `domain-model.yaml`, and `blueprint.yaml` are unchanged for non-operation-shaped fragments.

## Arguments

None.

<!-- parlay:active-root-aware -->
## Active root

Paths in this skill resolve against the active root. The CLI walks up from cwd, honors `--root <name>` and `PARLAY_ROOT`, and walks `spec/intents/**/infrastructure.md` plus writes `spec/intents/<feature>/capabilities.yaml` under whichever root resolves.

## Steps

1. **Run the CLI** — `parlay migrate-capabilities`. The command:
   - Walks `spec/intents/**/infrastructure.md`.
   - Splits each file into paragraph-sized chunks.
   - Detects operation-shaped paragraphs (those naming a closed-vocabulary step verb plus an entity) and emits stubs into the feature's `capabilities.yaml` with `kind: unknown`.
   - Classifies residual paragraphs via `pattern_fragment_classifier.go` and writes a per-feature `migration-report.md`.

2. **Review per-feature reports** — each `migration-report.md` lists extracted operations + classified unrouted fragments with suggested destinations from the closed list:

   - `pipeline` → command operation in capabilities
   - `registry` → domain entity plus register/list/lookup operations
   - `dispatcher` → application-layer dispatcher
   - `validator` → validate-input step in command operation
   - `aspect` → blueprint aspect (errors, auth, state)
   - `cache` → blueprint data.caching
   - `helper` → adapter-level pattern (not spec-layer)
   - `unrouted` → designer review required
   - `v2-deferred` → preserved verbatim

3. **Fill in `kind:`** — every extracted operation lands with `kind: unknown`. Build mode rejects this; the designer must set `kind: command` or `kind: query` explicitly. The `capabilities-stub-unfilled` rule blocks the build until done.

4. **Address unrouted fragments** — for each unrouted paragraph, route it to the suggested destination (or pick a different one). The migrator never auto-applies suggestions.

## Behavior

- **Idempotent.** If `capabilities.yaml` already exists, the command skips that feature and reports it.
- **Report-only for non-operation-shaped fragments.** The migrator never writes to `domain-model.yaml`, `blueprint.yaml`, or other artifacts.
- **Conservative detection.** Anything ambiguous lands in `unrouted` rather than being auto-routed.

## Errors

- `read-infrastructure-failed` — `infrastructure.md` is unreadable. Per-feature; the rest of the migration continues.
