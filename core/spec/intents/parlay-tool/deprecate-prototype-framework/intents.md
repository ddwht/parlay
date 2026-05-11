# Deprecate `prototype-framework` legacy config field

> Remove the legacy `prototype-framework` field from project config outright. The multi-adapter rollout already introduced `parlay migrate-config` to convert this field into a single-target presentation adapter set and emit a `prototype-framework-deprecated` warning while the field stayed parseable. This feature ends the grace period: after ship, a config containing `prototype-framework` fails parsing and the migration step itself becomes a no-op (the field it migrated from no longer exists in any supported config).

---

## Remove `prototype-framework` from the config schema and parser

**Goal**: Delete the `prototype-framework` field from `config.yaml`'s schema, the parser, and every code path that still consults it. Replace remaining read sites with reads against `.parlay/adapter-set.yaml`. After ship, a config file containing `prototype-framework` fails parsing with a stable error code that names `parlay migrate-config` as the (one-time) fix, and the migration command itself is sunsetted because it no longer has an input to act on.

**Persona**: Parlay tool maintainer

**Priority**: P1

**Context**: `prototype-framework` was the single-string project-level adapter binding before kinds existed. Multi-adapter introduced `.parlay/adapter-set.yaml` as the topology file and `parlay migrate-config` as the conversion step — that step writes a presentation-only adapter-set whose only filled slot is `presentation`, semantically identical to the legacy field's effect. The grace period (parseable + deprecated warning) bought time for downstream projects to migrate; carrying it forever leaks legacy shape into editor integrations, schema docs, and parser branches.

**Action**: Drop `prototype-framework` from the config schema and the parser. Surface a stable parse error code `config-legacy-prototype-framework` when the field is encountered, with a fix message that points authors at the canonical `.parlay/adapter-set.yaml` shape and notes that `parlay migrate-config` was the one-time conversion path during the multi-adapter rollout. Sunset `parlay migrate-config` itself: keep the command name registered for one minor version that prints "no legacy fields detected; nothing to migrate" so existing scripted callers do not crash, then remove it in the next minor. Update the schema doc, the migration intent inside `multi-adapter`, and any examples that still reference the field.

**Objects**: legacy-prototype-framework-field, config-schema, parser-removal, migrate-config-sunset

**Constraints**:
- The field is removed from the schema in this feature; there is no parseable-but-warning grace period in this version, since the multi-adapter rollout already provided that grace through `prototype-framework-deprecated`
- Parse failure code is `config-legacy-prototype-framework`, stable, with a fix message naming the canonical shape and the (already-run) migration entry point
- `parlay migrate-config` becomes a no-op for one minor version (prints a "nothing to migrate" line), then is removed in the next minor — this avoids breaking scripted callers in the same release that breaks the field
- Every in-repo project's config files must already be migrated before this feature ships; the rollout pass includes a sweep that confirms no `prototype-framework` field remains
- `.parlay/adapter-set.yaml` is the canonical topology file; this feature does not touch it
- The schema-docs entry for `prototype-framework` is replaced with a "removed" note pointing at the multi-adapter migration intent, so future readers see the audit trail

**Verify**:
- A `config.yaml` containing `prototype-framework: react` fails parsing with `config-legacy-prototype-framework` and a fix message naming `.parlay/adapter-set.yaml`
- A project with a valid `.parlay/adapter-set.yaml` and no `prototype-framework` field validates clean
- `parlay migrate-config` on a project with no legacy field prints "nothing to migrate" and exits 0 (one-version no-op behavior)
- The schema doc no longer lists `prototype-framework`; the removal note cites the multi-adapter migration intent
- A grep across the in-repo projects' config files for `prototype-framework` returns zero matches at the moment this feature ships
- A scripted caller that invokes `parlay migrate-config` continues to succeed during the no-op minor version, then receives "unknown command" after the subsequent removal minor

---
