# Deprecate buildfile `models:` field — Dialogs

---

### Remove `models:` from the buildfile schema and parser

**Trigger**: The buildfile parser (invoked by `parlay validate`, `parlay build-feature`, `parlay generate-code`, or any in-process consumer) loads a buildfile and walks its top-level keys.

User: Has a project whose buildfiles already dropped per-feature `models:` blocks during the multi-adapter rollout — entity resolution runs through `domain-model.yaml`. Runs any pipeline command.
System (background): Parses each buildfile against the post-removal schema; `models:` is no longer in the schema. Entity resolution paths read from `domain-model.yaml` exclusively.
System: Validation passes. The pipeline runs to completion.

#### Branch: A buildfile still carries `models:`

User: Has a buildfile with a non-empty `models: {Task: {...}, User: {...}}` block (perhaps a stale branch or a downstream project that did not run the rollout sweep).
System: Fails with `buildfile-legacy-models-field` naming the offending key and the buildfile path. Fix message: `move entity declarations to domain-model.yaml; per-feature model duplication was deprecated in the multi-adapter rollout and removed in this version — manual edit required`.

#### Branch: Build-feature does not silently drop the field

User: Runs `parlay build-feature` on a project with a legacy `models:` block, expecting the prior auto-drop branch to clean it up.
System (background): The build-feature normalization branch that dropped legacy `models:` entries was removed alongside the parser branch — it had no parseable input to act on after the schema change.
System: Fails with the same parse error, propagated through build-feature. Fix message: `move the entries into domain-model.yaml's entities: block by hand; build-feature no longer auto-drops`.

#### Branch: Entity resolution runs from domain-model.yaml only

User: Has a buildfile that references a `Task` entity in its `targets.persistence.repositories.TaskRepository.entity`.
System (background): Entity resolution walks `domain-model.yaml`'s `entities:` block to find `Task`. There is no fallback to a per-feature `models:` declaration.
System: If `Task` is in `domain-model.yaml`, resolution succeeds. If `Task` is missing from `domain-model.yaml`, resolution fails with `domain-model-entity-not-found` regardless of whether the legacy `models:` block previously declared it.

#### Branch: Schema doc shows the removal

User: Reads `buildfile.schema.md` looking for `models:`.
System (background): The doc no longer lists `models:` as a top-level field; it appears under "removed fields" citing the multi-adapter legacy-fields intent and this feature's intent.
System: The reader sees the audit trail and `domain-model.yaml` as the canonical home for entities.

#### Branch: Fixture scaffolding reads from domain-model

User: Has a buildfile whose `fixtures:` declarations reference entity shapes.
System (background): Fixture scaffolding now resolves entity shapes from `domain-model.yaml` exclusively. The legacy fallback to `models:` is gone.
System: Fixtures continue to work as long as the referenced entities exist in `domain-model.yaml`. A reference to an entity that lived only in the legacy `models:` block (and never got migrated) fails fixture scaffolding with the same `domain-model-entity-not-found` code.

#### Branch: In-repo sweep before ship

User: Lands the deprecation removal change.
System (background): The pre-merge sweep grep-walks every in-repo buildfile for top-level `models:` keys.
System: Sweep returns zero matches at ship time. If any match remains, the change is blocked at merge with a list of offending files.

#### Branch: Entity-resolution unit tests

User: Runs the entity-resolution test suite.
System (background): The tests now exercise `domain-model.yaml` as the only source. The fixtures previously testing `models:` fallback are removed.
System: All tests pass. The test surface is smaller; the resolution path is single-source.

---
