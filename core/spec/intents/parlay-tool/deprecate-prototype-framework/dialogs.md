# Deprecate `prototype-framework` legacy config field — Dialogs

---

### Remove `prototype-framework` from the config schema and parser

**Trigger**: The config parser (invoked at every parlay command's startup) loads `config.yaml` and walks its top-level keys.

User: Has a project whose `config.yaml` migrated to the multi-target shape during the multi-adapter rollout — no `prototype-framework` field, `.parlay/adapter-set.yaml` carries the topology. Runs any pipeline command.
System (background): Parses `config.yaml` against the post-removal schema; the legacy key is no longer in the schema. Reads the adapter-set from `.parlay/adapter-set.yaml` for topology resolution.
System: Validation passes. The pipeline runs to completion.

#### Branch: Project still carries `prototype-framework`

User: Has a `config.yaml` with `prototype-framework: react` (perhaps a stale branch or a downstream project that did not run multi-adapter migration).
System: Fails with `config-legacy-prototype-framework` naming the offending key and citing the canonical `.parlay/adapter-set.yaml` shape. Fix message: `prototype-framework was deprecated in the multi-adapter rollout and removed in this version — run parlay migrate-config (sunsetting in a later minor) to convert, or author .parlay/adapter-set.yaml directly`.

#### Branch: `parlay migrate-config` runs as a one-version no-op

User: Runs `parlay migrate-config` on a project whose `config.yaml` already lacks `prototype-framework` (the field is gone or the project never had it).
System (background): The command's removal branch (legacy field detection) is gone with the schema change. The no-op branch reports nothing to migrate.
System: Prints `no legacy prototype-framework field detected; nothing to migrate` and exits 0. This keeps existing scripted callers from crashing during the deprecation minor; the command itself is removed in the next minor.

#### Branch: After the no-op minor, command is removed

User: Runs `parlay migrate-config` after the next minor version has shipped.
System: Returns the CLI's standard "unknown command" error. Fix message at the CLI layer points the user at `parlay --help`. The migration was meant for one-time use; scripted callers have had a release to update.

#### Branch: Schema doc shows the removal

User: Reads the config schema doc looking for `prototype-framework`.
System (background): The doc no longer lists the field; it appears under "removed fields" with a reference to the multi-adapter migration intent and this deprecation feature's intent.
System: The reader sees the audit trail and the canonical adapter-set shape.

#### Branch: External tool that read `prototype-framework`

User: Has a downstream tool or script that still reads `config.prototype-framework`.
System (background): The parsed config has no such field; the read returns nil/undefined depending on the consumer language.
System: The external tool either crashes (if it required the field) or proceeds with a wrong default. Fix path: read `.parlay/adapter-set.yaml` for topology instead.

#### Branch: In-repo sweep before ship

User: Lands the deprecation removal change.
System (background): The pre-merge sweep grep-walks every in-repo `config.yaml` for the legacy field.
System: Sweep returns zero matches at ship time. If any match remains, the change is blocked at merge with a list of offending files.

#### Branch: Downstream project upgrades parlay versions

User: Updates the parlay binary in a downstream project that still has `prototype-framework: react` in its config.
System (background): The first parlay command after the upgrade hits the parse error at startup.
System: Fails with `config-legacy-prototype-framework`. The fix path is the (still-available, no-op-bound-for-removal) `parlay migrate-config` if the project is still on the deprecation minor, or a manual edit if it is on the post-removal minor.

---
