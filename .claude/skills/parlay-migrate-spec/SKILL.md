---
name: parlay-migrate-spec
description: "Parlay: Convert each feature's surface.md to surface.yaml"
---

<!--
parlay-feature: parlay-tool/multi-adapter
parlay-component: spec-migration-report
-->

# Migrate Spec

Walk each feature's `spec/intents/<feature>/surface.md`, parse via the legacy parser, and emit an equivalent `surface.yaml` alongside it. Both formats parse to the same in-memory surface model — the build pipeline does not branch on serialization.

## Arguments

None.

<!-- parlay:active-root-aware -->
## Active root

Paths in this skill resolve against the active root. The CLI walks up from cwd, honors `--root <name>` and `PARLAY_ROOT`, and walks `spec/intents/**/surface.md` under whichever root resolves.

## Steps

1. **Run the CLI** — `parlay migrate-spec`. The command:
   - Walks `spec/intents/**/surface.md`.
   - For each feature without an existing `surface.yaml`, parses the legacy file and emits the YAML form.
   - Writes a per-feature migration report listing free-text content with no closed-schema destination.

2. **Review the report** — for each feature, the command prints `wrote <path>` or `surface.yaml already present`. Free-text paragraphs that don't fit the closed schema appear in the report file alongside `surface.md`.

3. **Verify** — run `parlay validate --type surface` on the new YAML to confirm parity with the legacy MD form. The two formats parse to identical in-memory `[]Fragment` shapes.

4. **Cleanup** — `surface.md` is left in place. Delete it manually after reviewing the YAML and the unrouted-content report. The migrator never deletes designer-authored files.

## Behavior

- **Idempotent.** If `surface.yaml` already exists for a feature, the command skips it and reports `already-migrated`.
- **No mutation of `surface.md`.** The legacy file remains in place until the designer manually deletes it.
- **Per-feature scoping.** Features without `surface.md` are silently skipped — the migrator only acts on features that have the legacy form.

## Errors

- `legacy-parse-failed` — a `surface.md` failed to parse. The report records the error; the feature's YAML is not written. Fix the MD and re-run.
