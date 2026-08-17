# v0.3 Deprecation Removal — inventory and execution record

2026-08-17. Removes every deprecated/stale artifact and field. Ground rule: **migrators keep
their read paths** (they are the upgrade path into the post-removal world); what goes is the
RUNTIME tolerance of legacy forms. Buildfile v1 shape (frozen, single-target) is explicitly
out of scope — only the deprecated top-level `models:` goes.

## Items and dispositions

1. **surface.md as runtime artifact.** `ResolveSurfacePath` drops the `.md` fallback (an
   `.md`-only feature resolves no surface and gets a hard error pointing at `migrate-spec`);
   the two warning codes (`surface-md-superseded`, `surface-md-legacy-format`) are replaced by
   one error `surface-md-unsupported`. `ParseSurfaceFile`'s markdown branch STAYS —
   migrate-spec/--retire-md read it to convert. Docs: strip "(or surface.md)" everywhere
   (deployer ownership, build-feature, create-artifacts, enggspec, buildfile schema,
   feature-structure, create-domain-model); rewrite surface.schema.md's coexistence/precedence
   prose; **retarget reference-design-spec.skill.md to surface.yaml** (live consumer).
2. **`prototype-framework:` config key.** Field removed from ProjectConfig; topology.go's
   child-first resolution entry removed; check_readiness fallback condition simplifies to
   adapter-set-only. migrate-config is SAFE (reads raw yaml via its own inline struct —
   verified migrate_config.go:41-45). `prototype-framework-deprecated` code + severity entry
   removed. Dogfood: core/.parlay/config.yaml drops the key (core has an adapter-set).
3. **Buildfile top-level `models:`.** deepBuildfile field + emit site (validate.go:612) +
   `buildfile-models-deprecated` severity entry + schema section/example/rows removed. Section
   hashers keep iterating ["models","routes","fixtures"] over raw yaml — absent key hashes
   nothing (v1 files unaffected). **generate-code.skill.md:98 retargeted**: the model layer
   comes from domain-model.yaml, never from buildfile models: (live consumer). Dogfood: strip
   `models:` from all 26 buildfiles → baselines change → re-bless.
4. **domain-model.md legacy form.** `LegacyDomainModelMarkdownPath` removed if no callers
   outside the migrator (verify); docs strip "(or domain-model.md)" (deployer, build-feature,
   enggspec); read-path-precedence prose rewritten to "unsupported; migrate". Dogfood: the one
   remaining domain-model.md migrated or removed.
5. **Intent Verify bullets as testcase fallback.** build-feature derives from `verify:` ONLY;
   missing `verify:` on a rebuilt entry = run migrate-verify (error path, not fallback).
   `intent` dropped from criterion.ref kind set (testcases.schema.md:114 — vestigial;
   walker already enumerates operation/fragment only). Intent hashing for freeze detection
   is NOT touched.
6. **testcases v1 shape.** "Legacy v1 ingestion" schema section removed;
   `testcases-source-refs-missing-legacy` code + severity entry removed; validator v1
   branches removed; coverage-review's "legacy v1 reviewable" framing rewritten.
7. **`no_studio` / `--no-studio` spellings.** NoStudio field, the OR in NoEditorEnabled, the
   hidden flag registration (no_editor_flag.go:56,61), and loop.skill.md's alias sentence
   removed. `no_editor` stays.
8. **domain-model.yaml `operations:` block.** Tolerance ends: `domain-operations-deprecated`
   becomes error in both modes (migrator remains the path out); create-domain-model's two
   templates stop emitting `operations: []`; schema "Deprecated" section rewritten to
   "removed in v0.3". Dogfood already migrated.

## Cross-item cautions (from the docs inventory)

- Two LIVE consumers retargeted, not deleted: generate-code's models: merge (item 3) and
  reference-design-spec's surface.md path (item 1).
- build-feature.skill.md:194 vs buildfile.schema.md:196 contradiction ("fails validation" vs
  "warning") resolves itself: with the field gone, unknown-key handling applies.
- Meta-test lockstep: severity_doc rows for removed codes, DIGEST regen (codes count drops),
  conformance corpus references, audit pins — moved in the same commits.
- docs/plans/* are HISTORY — not edited.

## Execution stages (each commit-gated on full suite + vet + verify-skills)

A. Config keys (items 2, 7) — engine + tests + dogfood key removal.
B. Buildfile models: (item 3) — engine + schema + skill retarget + 26 dogfood files.
C. surface.md runtime (item 1) — resolution, codes, docs sweep, Figma-skill retarget.
D. domain-model.md + operations: (items 4, 8) — tolerance ends, templates cleaned.
E. Testcases v1 + Verify fallback (items 5, 6).
F. DIGEST regen, upgrade deploy, dogfood re-bless, final suite.
