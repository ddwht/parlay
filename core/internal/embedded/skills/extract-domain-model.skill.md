# Extract Domain Model

<!-- parlay-feature: studio-support/domain-model-yaml-migration -->
<!-- parlay-component: extract-domain-model-output -->
<!-- parlay-extends: studio-support/domain-model-yaml-migration/extract-domain-model-yaml-emission -->

Analyze all features in the active root and write a single
`domain-model.yaml` at the project level.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

## Steps

1. **Load schemas** — Read `.parlay/schemas/intent.schema.md`,
   `.parlay/schemas/dialog.schema.md`, `.parlay/schemas/surface.schema.md`,
   and (new) `.parlay/schemas/domain-model.schema.md`.

2. **Scan all features** — Read `spec/intents/*/intents.md`, `dialogs.md`,
   and `surface.md`.

3. **Extract entities, enums, relationships, operations** — From intent
   Objects fields and implicit references in dialogs and surfaces:

   - For each entity, derive typed fields from how it is described and
     used. Field types are drawn from the closed set declared in
     `.parlay/schemas/domain-model.schema.md`
     (`{uuid, string, int, float, bool, datetime, ref, <named-enum>}`).
     Inline object literals are not allowed — lift nested shapes into a
     separate entity joined by a `ref`-typed field.
   - Identify relationships (`belongs-to`, `has-many`, references) and
     pick a cardinality from the closed set
     `{one-to-one, one-to-many, many-to-one, many-to-many}`.
   - Identify enums from lists of states declared by intents or
     dialogs. For each enum value, pick a tone from the closed set
     `{neutral, info, warning, danger, success}`. Tones are optional;
     omit when uncertain.
   - Aggregate operations across all features. Each operation declares
     `input:` as a list of `Entity.field` references and `effects:` as
     a list of free-text declarative statements.

4. **Write domain model** — Write the aggregated artifact to
   `<activeRoot>/domain-model.yaml` (resolved via `(*Context).DomainModelPath()`).
   The file uses the shape declared by
   `.parlay/schemas/domain-model.schema.md`:

   ```yaml
   schema_version: 1
   enums: []
   entities: []
   relationships: []
   operations: []
   ```

   - **Greenfield** (no feature intents): produce an empty-but-valid
     YAML — declared `schema_version: 1`, empty
     `enums`/`entities`/`relationships`/`operations` lists.
   - **Per-feature `domain-model.md`** is **never** emitted — not as
     primary output, not as fallback, not as per-feature debug artifact.
     Existing per-feature `.md` files (from the pre-migration world)
     are not touched.
   - **Write failure**: if the YAML write fails (permissions, disk
     full), exit non-zero without writing a fallback `.md`.

5. **Validate** — invoke:

   ```bash
   parlay validate --type domain-model --json <activeRoot>/domain-model.yaml
   ```

   If validation fails, surface the structured errors to the user and
   stop. Do not commit the YAML.

6. **Report** — Print the absolute YAML path on stdout, plus a one-line
   summary on the next line:

   ```
   <activeRoot>/domain-model.yaml
   extracted N entities, M enums, K relationships, J operations
   ```

   No human-readable preamble.
