# Deprecate `domain-model.yaml` `operations:` field

> Remove the `operations:` field from `domain-model.yaml` outright. The multi-adapter rollout deprecated the field, surfaced `domain-operations-deprecated` warnings (errors in build mode), and shipped `parlay migrate-domain-operations` to lift each entry into a stub inside the relevant feature's `capabilities.yaml`. This feature is the second step: stop accepting the field at all. After ship, a `domain-model.yaml` containing `operations:` fails parsing.

---

## Remove `operations:` from the domain-model schema and parser

**Goal**: Delete the `operations:` field from `domain-model.schema.md`, the in-process parser, and every consumer that still read it. After ship, a `domain-model.yaml` declaring `operations:` fails parsing with a stable error code that names `capabilities.yaml` as the canonical home and points at the migration step (`parlay migrate-domain-operations`) that ran during the multi-adapter rollout.

**Persona**: Parlay tool maintainer

**Priority**: P1

**Context**: `domain-model.operations` conflated the data model with system behavior, partly because there was no other home for operation-shaped fragments. With `capabilities.yaml` as the dedicated backend artifact, the field became redundant and an invitation to drift. Multi-adapter kept the field parseable while emitting deprecation warnings in authoring mode and errors in build mode; that grace period gave downstream projects time to run the migrator and route entries into per-feature `capabilities.yaml` stubs. This feature ends the grace.

**Action**: Drop `operations:` from the domain-model schema and the parser. Replace any remaining read sites with reads against `capabilities.yaml`. Surface a stable parse error code `domain-model-legacy-operations-field` when the field is encountered, with a fix message naming `capabilities.yaml` and the (already-run) `parlay migrate-domain-operations` entry point. Sunset `parlay migrate-domain-operations` itself: keep the command name registered for one minor version that prints "no legacy operations detected; nothing to migrate" so scripted callers do not crash, then remove it in the next minor. Update the schema doc, the multi-adapter intent that introduced the deprecation, and any examples that still reference the field.

**Objects**: legacy-domain-model-operations-field, domain-model-schema, capabilities-canonical, parser-removal, migrate-domain-operations-sunset

**Constraints**:
- The field is removed from the schema in this feature; there is no parseable-but-warning grace period in this version, since the multi-adapter rollout already provided that grace
- Parse failure code is `domain-model-legacy-operations-field`, stable, with a fix message naming `capabilities.yaml`
- `parlay migrate-domain-operations` becomes a no-op for one minor version (prints a "nothing to migrate" line), then is removed in the next minor — this avoids breaking scripted callers in the same release that breaks the field
- Every in-repo project's `domain-model.yaml` files must already be migrated before this feature ships; the rollout pass includes a sweep that confirms no `operations:` block remains
- `domain-model.yaml` retains its full schema and behavior for entities, relationships, states, enums, and value objects; this feature does not touch the rest of the model
- `capabilities.yaml` remains the canonical home for closed-vocabulary backend operations; this feature does not touch it
- The schema-docs entry for `operations:` is replaced with a "removed" note pointing at the multi-adapter intent that deprecated it, so future readers see the audit trail

**Verify**:
- A `domain-model.yaml` declaring an `operations:` block fails parsing with `domain-model-legacy-operations-field` and a fix message naming `capabilities.yaml`
- A `domain-model.yaml` with only entities, relationships, states, enums, and value objects parses clean
- Running build-feature on a project whose `domain-model.yaml` still contains `operations:` fails with the parse error rather than silently dropping the entries
- `parlay migrate-domain-operations` on a project with no legacy field prints "nothing to migrate" and exits 0 (one-version no-op behavior)
- The schema doc no longer lists `operations:` as a domain-model field; the removal note cites the multi-adapter deprecation intent
- A grep across the in-repo projects' `domain-model.yaml` files for `operations:` returns zero matches at the moment this feature ships
- A scripted caller that invokes `parlay migrate-domain-operations` continues to succeed during the no-op minor version, then receives "unknown command" after the subsequent removal minor

---
