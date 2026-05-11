---
name: parlay-migrate-capabilities
description: "Parlay: Move operation-shaped fragments from infrastructure.md into capabilities.yaml; retain architectural prose in place (partial migration is the success case)"
---

<!--
parlay-feature: parlay-tool/multi-adapter
parlay-component: capabilities-migration-operations-extraction
parlay-extends: parlay-tool/multi-adapter/pattern-fragment-classifier
parlay-extends: parlay-tool/architectural-prose-artifact/partial-migration-semantics-in-migrate-capabilities
-->

# Migrate Capabilities

Move operation-shaped fragments from each feature's `infrastructure.md` into `capabilities.yaml`. Architectural-prose fragments — boundaries, probes, allowlists, dependency pins, and other concerns that do not reduce to operations — are retained in `infrastructure.md` by design.

Partial migration is the success case. The four spec artifacts (`surface.md`/`surface.yaml`, `capabilities.yaml`, `infrastructure.md`, `domain-model.yaml`) are co-equal: this command moves operation-shaped content between two of them and never frames `infrastructure.md` as a source to be emptied. A feature whose `infrastructure.md` contains only architectural prose is a successful all-retained run; a feature where every fragment is operation-shaped extracts everything and deletes the now-empty `infrastructure.md`; the common case is a mixture, and both lists print explicitly so the partition shape is always visible.

## Arguments

None. The command accepts a single optional flag: `--dry-run`.

<!-- parlay:active-root-aware -->
## Active root

Paths in this skill resolve against the active root. The CLI walks up from cwd, honors `--root <name>` and `PARLAY_ROOT`, and walks `spec/intents/**/infrastructure.md` plus writes `spec/intents/<feature>/capabilities.yaml` under whichever root resolves.

## Steps

1. **Preview with --dry-run first** — `parlay migrate-capabilities --dry-run`. The command:
   - Walks `spec/intents/**/infrastructure.md`.
   - Splits each file into `## `-delimited fragments.
   - Classifies each fragment as operation-shaped (names a closed-vocabulary step verb plus an entity) or architectural prose.
   - Prints the per-feature partition with a `(dry run — no files written)` header. Nothing on disk changes.

2. **Inspect the partition** — for each feature the output shows two lists:
   - `Extracted to capabilities.yaml:` — feature-local fragment ids paired with the new operation ids the run would create. Empty lists print as `(none)` so the partition shape is always visible.
   - `Retained in infrastructure.md:` — the fragment `## ` headings that stay in place. Empty lists print as `(none)` or, when there are no operation-shaped fragments at all, as `no operation-shaped fragments to migrate; infrastructure.md left in place`.
   - When the run would delete the file (every fragment migrated out), a `Deleted: infrastructure.md (was empty after extraction)` line appears below the lists. The dry-run preview shows this line if applicable but does not delete the file.

3. **Run the migration for real** — `parlay migrate-capabilities`. The command emits `capabilities.yaml` for each feature with at least one operation-shaped fragment, rewrites `infrastructure.md` to contain only the retained fragments, and deletes `infrastructure.md` when every fragment migrated out. Exit code is zero on any successful migration, including the all-retained case where nothing was extracted.

4. **Fill in `kind:`** — every extracted operation lands with `kind: unknown`. Build mode rejects this; the designer must set `kind: command` or `kind: query` explicitly. The `capabilities-stub-unfilled` rule blocks the build until done.

5. **Leave retained fragments in place** — fragments that stayed in `infrastructure.md` are architectural prose by design. They do not need to be paraphrased as operations or moved elsewhere; `infrastructure.md` is their canonical home. The `infrastructure.schema.md` worked examples (SDK import boundary, external-system startup probe, wrapper API allowlist, library version pin) illustrate the categories of content that belong in `infrastructure.md` rather than `capabilities.yaml`.

## Behavior

- **Partial migration is the success case.** A feature with a mix of operation-shaped and architectural fragments produces both a `capabilities.yaml` (with the extracted operations) and a retained `infrastructure.md` (with the architectural prose). Both are valid spec artifacts; neither supersedes the other.
- **Empty-file deletion.** When every fragment in `infrastructure.md` migrates out, the now-empty file is deleted rather than left on disk as a zero-byte stub.
- **Idempotent.** If `capabilities.yaml` already exists for a feature, the command skips that feature and reports it.
- **Conservative detection.** Detection uses a closed-vocabulary verb set (`validate-input`, `create-one`, `update-one`, `delete-one`, `read-one`, `read-many`, `search`); anything outside that set is retained as architectural prose rather than auto-routed.
- **Dry-run is non-destructive.** With `--dry-run`, the feature folder is byte-identical before and after the command runs. The partition output is identical to a real run, plus a `(dry run — no files written)` header.

## Errors

- `read-infrastructure-failed` — `infrastructure.md` is unreadable. Per-feature; the rest of the migration continues.
