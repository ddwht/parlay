# Domain Model YAML Format

> Migrate Core's domain model artifact from prose markdown to a machine-friendly YAML format with a versioned schema. The YAML form is the format Studio's Domain Model Editor reads and writes; Core's existing `create-domain-model` and `load-domain-model` commands switch over to consume and emit it. Hand-editing remains supported but is no longer the primary workflow.

---

## Define a Versioned YAML Schema for the Domain Model

**Goal**: Replace the prose `domain-model.md` artifact with a structured `domain-model.yaml` that downstream tooling — Core's codegen, Studio's editor — can parse, validate, and round-trip without ambiguity.
**Persona**: UX Designer
**Priority**: P0
**Context**: The current `domain-model.md` is human-readable prose. Studio's Domain Model Editor and Core's layout-aware codegen both need a machine-parseable form so they can present and consume the model without an AI inference pass.

This intent governs two distinct, separable flows that share the `schema_version` field:

- **Flow A — One-shot markdown→YAML port**: a single, project-lifetime migration from the legacy `domain-model.md` to the new `domain-model.yaml` shape. This is covered by the "Migrate Existing `domain-model.md` Projects to YAML" intent below.
- **Flow B — Standing forward-compatibility hook (`schema_version` evolution)**: the mechanism by which a future `v2` (or `v3`) of the YAML schema can be introduced without breaking existing projects. Every YAML file carries a `schema_version`; tooling reads it and either consumes the file directly (matching version) or routes it through a per-version migrator (older version). This is the standing hook this intent establishes and is independent of Flow A.

The two flows are explicit so neither is silently absorbed by the other: the markdown→YAML port is a one-time event per project, while `schema_version` evolution is a recurring, mechanical concern of every Core release.
**Action**: Define `domain-model.yaml` with top-level sections — `schema_version`, `enums`, `entities`, `relationships`, `operations` — and a JSON-Schema-like description in the project's schemas directory. Each entity declares typed fields with required/optional flags; each enum declares values with presentation metadata (`label`, `tone`); each relationship names its endpoints and cardinality; each operation declares inputs and effects.
**Objects**: domain-model, entity, enum, field, relationship, operation, schema_version

**Constraints**:
- `schema_version` is present from day one so future format changes can be migrated mechanically. The first released value is the integer `1` — an integer (not semver) keeps version comparison trivial and matches parlay's existing JSON-envelope convention (e.g., `parlay status --json` emits `schema_version: 1`). Future releases bump to `2`, `3`, etc.
- Enum presentation metadata (`label`, `tone`) is part of the domain model artifact, not redeclared per page — this is a deliberate exception to the "domain is logic-only" rule, accepted because every consumer of an enum needs the same presentation data
- Field types are a closed set of primitives plus references: `uuid`, `string`, `int`, `float`, `bool`, `datetime`, `ref` (with `target: <Entity>`), and named enums
- Relationships reference entities by name and are typed (`one-to-one`, `one-to-many`, `many-to-one`, `many-to-many`)
- Operations declare `input` (list of field names from involved entities) and `effects` (declarative side-effect statements; not executable code)
- The artifact lives at the active root's domain-model path — exactly one per parlay project, not per-feature
- Schema is published in the same place as Core's other schemas so it ships in the binary and round-trips through `parlay upgrade`
- Hand-editing the YAML must remain supported — the format is YAML precisely so a developer can open it in a text editor and patch a field

**Verify**:
- `domain-model.yaml` parses cleanly when authored by hand against the schema published in the project's schemas directory — top-level `schema_version`, `enums`, `entities`, `relationships`, `operations`, with the field types, relationship cardinalities, and operation shape described in this intent's Action
- `schema_version` mismatch produces a clear error pointing at the migration path, not a silent parse failure
- A domain model with enums, entities, relationships, and operations all defined produces a deep-validation pass — every relationship endpoint resolves to an entity, every operation input names a field that exists, every enum value referenced by a field is declared
- Adding a new field type or enum value does not require editing any code outside the schema

**Questions**:
- Are enum tones a closed set (`neutral`, `info`, `warning`, `danger`, `success`) or open strings?
- Can entities have nested types (struct-shaped fields), or only flat fields with refs? Phase 1 can be flat; revisit if real domain models need nesting.

---

## Migrate Existing `domain-model.md` Projects to YAML

**Goal**: Convert any pre-existing `domain-model.md` in the user's project to `domain-model.yaml` in a single, reviewable step, so projects authored before this migration continue to work.
**Persona**: UX Designer (existing parlay project owner)
**Priority**: P0
**Context**: Projects in the wild have a markdown domain model. After this Core release, those projects must continue to load, codegen, and run without manual rework. This is Flow A from the schema-definition intent above — a one-shot, per-project port — and is distinct from the standing `schema_version` evolution mechanism (Flow B).
**Action**: Add a one-shot migration that detects an existing `domain-model.md`, parses it via AI (the same path that wrote it), emits `domain-model.yaml`, and leaves the original `.md` in place with a deprecation note. The migration is invoked explicitly via a new `parlay migrate-domain-model` command, not implicitly during other operations.

The CLI surface this intent commits to (the artifacts phase will pin the full shape; this is an intents-level sketch):
- Command name: `parlay migrate-domain-model`.
- Flags: `--dry-run` (print the planned YAML and the diff against any existing artifact, write nothing), `--force` (overwrite an existing `domain-model.yaml` instead of failing the "already migrated" check), and the standard `--root <name>` for multi-root projects so the migration targets a specific child root.
- Exit codes: `0` on a successful migration or a no-op (no `domain-model.md` to convert in a greenfield project); non-zero when `domain-model.md` is present but unreadable; non-zero when `domain-model.yaml` already exists and `--force` was not passed; non-zero when the markdown is ambiguous and the emitted YAML carries unresolved-field annotations.
**Objects**: domain-model.md, domain-model.yaml, migration

**Constraints**:
- The migration is explicit — never silent. The user opts in by running the command, sees the diff, and can revert if needed
- The original `.md` is not deleted — it stays beside the new `.yaml` with a deprecation header pointing at the YAML
- The migration is idempotent — running it twice produces the same YAML; running it when the YAML already exists fails with a clear "already migrated" message rather than overwriting
- For projects that never had `domain-model.md`, the migration is a no-op — there is nothing to convert
- Conversion is best-effort: if the markdown is ambiguous (missing types, unstructured prose), the migration emits the YAML with annotations marking unresolved fields and exits non-zero so the designer fixes them before continuing
- A grace period is not maintained: once a project has `domain-model.yaml`, Core stops reading `domain-model.md`. The two never coexist as live state

**Verify**:
- A project with a hand-written `domain-model.md` produces a corresponding `domain-model.yaml` after running the migration; subsequent `parlay` commands read the YAML
- Re-running the migration on a project that already has `domain-model.yaml` exits non-zero with an "already migrated" message and does not modify the YAML
- A project with no domain model at all (greenfield) reports "nothing to migrate" and exits zero
- Ambiguous markdown (e.g., a field with no declared type) produces a YAML with annotation markers and a non-zero exit, so the designer is forced to resolve before codegen

**Questions**:
- Should the migration command live in Core or be packaged with Studio's installer? Installing Studio is the most likely trigger; Core ownership is more conservative. Decide during dialog authoring.

---

## Update `extract-domain-model` and `load-domain-model` to Round-Trip YAML

**Goal**: Switch the existing `/parlay-create-domain-model` and `/parlay-load-domain-model` commands to emit and consume the YAML format, so the existing extraction-and-sharing workflow keeps working with the new artifact shape.
**Persona**: UX Designer
**Priority**: P1
**Context**: The existing domain-model commands write per-feature `domain-model.md` files (extraction) or merge an external markdown model into the current project (load). After the format change, these commands must produce and consume YAML.
**Action**: Update `create-domain-model` to write `domain-model.yaml` at the project root (not per-feature), populating it from the AI's extraction pass over all features' intents and dialogs. Update `load-domain-model` to accept either a YAML file path or a URL, and merge into the project's `domain-model.yaml` with the same disambiguation prompts the markdown version had.
**Objects**: create-domain-model, load-domain-model, domain-model.yaml

**Constraints**:
- After this migration, `create-domain-model` writes only `domain-model.yaml`. It must not emit `domain-model.md`, even as a fallback or alongside the YAML — the markdown form is gone from the post-migration world. The deprecated `.md` only exists in projects that pre-date this change and is left in place by the one-shot migration; nothing in Core ever produces a fresh `.md` again
- Extraction emits the project-level `domain-model.yaml`, not per-feature files — there is exactly one canonical domain model per parlay project
- Load accepts only YAML — markdown loads from external projects are out of scope; if a user has a markdown model they want to load, they migrate it first
- Disambiguation behavior on load is unchanged in spirit: when an incoming entity conflicts with an existing one, the agent asks the designer how to resolve
- Both commands respect the schema version. Loading a model with an older `schema_version` triggers the migration path before merging

**Verify**:
- `parlay create-domain-model` on a project with feature intents produces a `domain-model.yaml` with the entities, enums, relationships, and operations described in those intents
- `parlay load-domain-model <path-to-yaml>` merges into the existing `domain-model.yaml`; conflicting entities trigger a designer disambiguation prompt
- Loading a YAML with an unsupported `schema_version` fails with an actionable error pointing at the migration path
- The extracted YAML round-trips through `parlay extract` → `parlay load` (extract from project A, load into a fresh project B) and produces a structurally equivalent model in B

---
