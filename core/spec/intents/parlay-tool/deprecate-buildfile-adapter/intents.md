# Deprecate buildfile top-level `adapter:` field

> Remove the buildfile's legacy top-level `adapter:` field outright. The multi-adapter rollout already replaced its role with an `adapter-set` reference plus per-target `adapter:` declarations under `targets:`, and made build-feature normalize legacy buildfiles into the new shape on first regeneration. This feature is the second step: stop accepting the field at all. After this feature ships, a buildfile with a top-level `adapter:` fails parsing in both authoring and build mode.

---

## Remove top-level `adapter:` from the buildfile schema and parser

**Goal**: Delete the top-level `adapter:` field from the buildfile schema, the parser, and every code path that still consults it. The field was kept parseable through the multi-adapter rollout so existing projects could keep validating during migration; that grace period ends with this feature. After ship, a buildfile with a top-level `adapter:` produces a parse error naming the field and pointing at the `adapter-set` + `targets.<kind>.adapter` shape.

**Persona**: Parlay tool maintainer

**Priority**: P1

**Context**: The legacy top-level `adapter:` field bound a whole feature to a single adapter, which the multi-adapter model superseded by registering kinds in `.parlay/adapter-set.yaml` and naming the adapter inside each `targets.<kind>` entry. Build-feature already normalizes legacy buildfiles into the new shape on first regeneration, and CI builds across the in-repo projects no longer rely on the old field. Carrying the parser branch indefinitely keeps a stale shape alive in tooling, schema docs, and editor integrations; removing it tightens the contract.

**Action**: Drop the `adapter:` field from `buildfile.schema.md`, the in-process parser, and every consumer that read it. Replace any remaining read sites with reads against `adapter-set` plus `targets.<kind>.adapter`. Surface a stable parse error code `buildfile-legacy-adapter-field` when the field is encountered, with a fix message naming the new shape and pointing at the migration step (`parlay build-feature` will normalize, OR the author edits manually). Update the schema doc, the legacy-field audit table inside `multi-adapter`, and any lingering examples that still reference the field.

**Objects**: legacy-buildfile-adapter-field, buildfile-schema, parser-removal, adapter-set-reference

**Constraints**:
- The field is removed from the schema in this feature — there is no parseable-but-warning grace period in this version, since the multi-adapter rollout already provided that grace
- Parse failure code is `buildfile-legacy-adapter-field`, stable, with a fix message that names the migration entry point and the new field shape
- Build-feature's normalization path that converted legacy `adapter:` into a single-target presentation adapter set is removed alongside, since there is no longer a parseable input to convert
- Every in-repo project's buildfiles must already be migrated before this feature ships; the rollout pass for this feature includes a sweep that re-runs build-feature on every project to confirm no legacy field survives
- The `adapter-set` reference and `targets.<kind>.adapter` declarations remain the canonical shape; this feature does not touch them
- The schema-docs entry for `adapter:` is replaced with a "removed" note pointing at the multi-adapter intent that introduced the replacement, so future readers see the audit trail

**Verify**:
- A buildfile with `adapter: react-antd` at top level fails parsing with `buildfile-legacy-adapter-field` and a fix message naming `adapter-set` plus `targets.<kind>.adapter`
- A buildfile that uses only the multi-target shape (`adapter-set:` reference and `targets.presentation.adapter:` field) parses and validates clean
- Running build-feature on a project whose buildfile still contains the legacy field fails with the same error rather than silently rewriting the file
- The schema doc no longer lists `adapter:` as a top-level field; an entry under "removed fields" cites the multi-adapter intent
- An editor integration (or any external tool) that consumed the legacy field now receives the parse error and surfaces it to the author
- A grep across the in-repo projects' buildfiles for top-level `adapter:` returns zero matches at the moment this feature ships

---
