# Deprecate buildfile `models:` field

> Remove the buildfile's `models:` field outright. The multi-adapter rollout deprecated the field, made build-feature drop per-feature model duplication during normalization, and routed entity resolution through `domain-model.yaml`. This feature is the second step: stop accepting the field at all. After ship, a buildfile with a `models:` block fails parsing.

---

## Remove `models:` from the buildfile schema and parser

**Goal**: Delete the `models:` field from `buildfile.schema.md`, the in-process parser, and every code path that still consulted per-feature model duplicates. Entity resolution at build time runs exclusively through `domain-model.yaml`. After ship, a buildfile that declares `models:` fails parsing with a stable error code that points at the canonical home for entities.

**Persona**: Parlay tool maintainer

**Priority**: P1

**Context**: `models:` predated `domain-model.yaml` as the per-feature home for entity declarations. With the canonical domain model in place, every per-feature duplication is a drift opportunity: two homes for the same fact, with no enforcement that they agree. Multi-adapter normalized the field away during build-feature regeneration and emitted `buildfile-models-deprecated` warnings (errors in build mode) on remaining occurrences. The grace period bought downstream projects time to migrate to `domain-model.yaml`; this feature ends it.

**Action**: Drop the `models:` field from the buildfile schema and the parser. Replace any remaining read sites — entity resolution, fixture scaffolding, plan generation — with reads against `domain-model.yaml`. Surface a stable parse error code `buildfile-legacy-models-field` when the field is encountered, with a fix message naming `domain-model.yaml` as the canonical home and pointing at the relevant migration step (entities authored in the legacy field move to `domain-model.yaml`'s `entities:` block; the migration was the project-wide sweep that ran during the multi-adapter rollout). Remove the build-feature normalization branch that dropped legacy `models:` entries, since the parser will no longer accept the field as input.

**Objects**: legacy-buildfile-models-field, buildfile-schema, domain-model-canonical, parser-removal, entity-resolution

**Constraints**:
- The field is removed from the schema in this feature; there is no parseable-but-warning grace period in this version, since the multi-adapter rollout already provided that grace
- Parse failure code is `buildfile-legacy-models-field`, stable, with a fix message naming `domain-model.yaml`
- Entity resolution at build time runs through `domain-model.yaml` exclusively; no fallback to per-feature model declarations exists after this feature ships
- Every in-repo project's buildfiles must already be migrated before this feature ships; the rollout pass includes a sweep that confirms no `models:` block remains
- `domain-model.yaml` retains its full schema and behavior for entities, relationships, states, enums, and value objects; this feature does not touch the canonical model
- The schema-docs entry for `models:` is replaced with a "removed" note pointing at the multi-adapter legacy-fields intent, so future readers see the audit trail
- The build-feature normalization branch that drops legacy `models:` entries is removed in the same change, since its input is gone

**Verify**:
- A buildfile with a non-empty `models:` block fails parsing with `buildfile-legacy-models-field` and a fix message naming `domain-model.yaml`
- A buildfile that omits `models:` and resolves entities through `domain-model.yaml` builds clean
- Running build-feature on a project whose buildfile still contains `models:` fails with the same parse error rather than silently dropping the entries
- The schema doc no longer lists `models:` as a buildfile field; an entry under "removed fields" cites the multi-adapter legacy-fields intent
- A grep across the in-repo projects' buildfiles for a top-level `models:` block returns zero matches at the moment this feature ships
- Entity-resolution unit tests pass with `domain-model.yaml` as the only entity source

---
