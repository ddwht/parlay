# Deprecate `domain-model.yaml` `operations:` field — Dialogs

---

### Remove `operations:` from the domain-model schema and parser

**Trigger**: The domain-model parser (invoked by `parlay validate`, `parlay build-feature`, `parlay generate-code`, or any in-process consumer) loads `domain-model.yaml` and walks its top-level keys.

User: Has a project whose `domain-model.yaml` migrated during the multi-adapter rollout — `operations:` is empty or absent; backend operations live in per-feature `capabilities.yaml`. Runs any pipeline command.
System (background): Parses `domain-model.yaml` against the post-removal schema; `operations:` is no longer in the schema. Validation walks entities, relationships, states, enums, and value objects; backend operations are read from per-feature `capabilities.yaml`.
System: Validation passes. The pipeline runs to completion.

#### Branch: `domain-model.yaml` still carries `operations:`

User: Has a `domain-model.yaml` with a non-empty `operations:` block (perhaps a stale branch or a downstream project that did not run `parlay migrate-domain-operations`).
System: Fails with `domain-model-legacy-operations-field` naming the offending key. Fix message: `move these entries to per-feature capabilities.yaml; the operations: field was deprecated in the multi-adapter rollout and removed in this version — run parlay migrate-domain-operations (sunsetting in a later minor) to scaffold stubs, or edit by hand`.

#### Branch: `parlay migrate-domain-operations` runs as a one-version no-op

User: Runs `parlay migrate-domain-operations` on a project whose `domain-model.yaml` already lacks an `operations:` block.
System (background): The command's removal branch (legacy field detection) is gone with the schema change. The no-op branch reports nothing to migrate.
System: Prints `no legacy domain-model operations detected; nothing to migrate` and exits 0. Scripted callers continue to succeed during the deprecation minor; the command is removed in the next minor.

#### Branch: After the no-op minor, command is removed

User: Runs `parlay migrate-domain-operations` after the next minor version has shipped.
System: Returns the CLI's standard "unknown command" error. The migration was meant for one-time use during the multi-adapter rollout.

#### Branch: Build-feature ignores any prior fallback

User: Has an external tool that previously fed `domain-model.operations` into a routing or codegen path.
System (background): Build-feature has no fallback to `domain-model.operations`; it reads only `capabilities.yaml`. The prior path was removed during the multi-adapter rollout's deprecation phase.
System: External tools that relied on the field receive empty operation sets. Fix path: read `spec/intents/<feature>/capabilities.yaml`.

#### Branch: Schema doc shows the removal

User: Reads the domain-model schema doc looking for `operations:`.
System (background): The doc no longer lists `operations:`; it appears under "removed fields" citing the multi-adapter intent that deprecated it and this feature's intent that removed it.
System: The reader sees the audit trail and `capabilities.yaml` as the canonical home for backend operations.

#### Branch: Domain-model retains its true scope

User: Edits `domain-model.yaml` to add a new entity and a state machine.
System (background): Validation walks the post-removal schema (entities, relationships, states, enums, value objects). The change is unaffected by this feature.
System: Validation passes. Domain-model continues to describe what the data is; backend behavior continues to live in `capabilities.yaml`.

#### Branch: In-repo sweep before ship

User: Lands the deprecation removal change.
System (background): The pre-merge sweep grep-walks every in-repo `domain-model.yaml` for `operations:` keys at the top level.
System: Sweep returns zero matches at ship time. If any match remains, the change is blocked at merge with a list of offending files.

#### Branch: Downstream project upgrades parlay versions

User: Updates the parlay binary in a downstream project whose `domain-model.yaml` still carries an `operations:` block.
System (background): The first parlay command after the upgrade hits the parse error.
System: Fails with `domain-model-legacy-operations-field`. The fix path is `parlay migrate-domain-operations` if the project is still on the deprecation minor, or a manual edit if it is on the post-removal minor.

---
