# Deprecate buildfile top-level `adapter:` field — Dialogs

---

### Remove top-level `adapter:` from the buildfile schema and parser

**Trigger**: The buildfile parser (invoked by `parlay validate`, `parlay build-feature`, `parlay generate-code`, or any in-process consumer) loads a buildfile and walks its top-level keys.

User: Has a project whose buildfiles already migrated to the multi-target shape during the multi-adapter rollout — `adapter-set:` reference plus `targets.<kind>.adapter` declarations. Runs any pipeline command.
System (background): Parses each buildfile against the post-removal schema; the legacy `adapter:` key is no longer in the schema, so its absence causes no warning and its presence would fail parsing. The buildfiles in this project don't have it.
System: Validation passes. The pipeline runs to completion.

#### Branch: A buildfile still carries top-level `adapter:`

User: Has a buildfile that somehow survived the multi-adapter rollout's sweep with `adapter: react-antd` at top level (perhaps a stale branch, an ungenerated buildfile, or a downstream project that never ran the rollout sweep).
System (background): Parser encounters the unexpected top-level key.
System: Fails with `buildfile-legacy-adapter-field` naming the offending key and the buildfile path. Fix message: `the top-level adapter: field was removed in this version — replace with an adapter-set: reference plus targets.<kind>.adapter declarations under targets:; if your project has not run multi-adapter migration, run parlay build-feature to normalize the buildfile`.

#### Branch: Build-feature does not silently rewrite

User: Runs `parlay build-feature` on a project with a legacy `adapter:` field, expecting the prior auto-normalization branch to convert it.
System (background): The build-feature normalization branch that converted legacy `adapter:` into a single-target presentation adapter set was removed alongside the parser branch — it had no parseable input to act on after the schema change.
System: Fails with the same `buildfile-legacy-adapter-field` parse error, propagated through build-feature. Fix message: `manual edit required — replace the legacy field with the multi-target shape; build-feature no longer auto-converts`.

#### Branch: Schema doc and audit table updated

User: Reads `buildfile.schema.md` looking for the `adapter:` field.
System (background): The doc no longer lists `adapter:` as a top-level field; the field appears in a "removed fields" section that cites the multi-adapter intent that introduced the replacement and this feature's intent that removed it.
System: The reader sees the audit trail and the canonical shape (`adapter-set:` + `targets.<kind>.adapter`). The legacy-field audit table inside the multi-adapter intent reflects the same change.

#### Branch: External tool consumer

User: Has an editor integration or downstream tool that still reads the legacy `adapter:` field from a buildfile.
System (background): The tool gets a parse error at the schema layer; the field simply isn't present in the parsed model.
System: The external tool surfaces the error to its caller. Fix path: update the tool to read `adapter-set` + per-target adapter declarations.

#### Branch: In-repo sweep before ship

User: Lands the deprecation removal change.
System (background): The pre-merge sweep step in CI grep-walks every in-repo buildfile for top-level `adapter:` keys (regex anchored at line start, key followed by colon).
System: The sweep returns zero matches at the moment this feature ships. If any match remains, the change is blocked at merge time with a list of offending files.

#### Branch: A project that opted into the legacy schema version is unaffected

User: Has a downstream project that has not upgraded the parlay tool version. Its parser still includes the legacy field branch.
System: The legacy project's parser still accepts the field; this feature affects only the parlay version that ships the schema removal. Upgrading the tool is the trigger for the breakage; the schema-version note in release notes points downstream projects at multi-adapter migration.

---
