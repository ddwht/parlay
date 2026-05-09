# Domain-model-yaml-migration — Infrastructure

---

## domain-model.yaml schema declaration and registry

**Affects**: domain-model schema (new top-level schema artifact published alongside the existing parlay schemas), schema registry, schema loader
**Behavior**: A new schema describes the shape of `domain-model.yaml` — top-level `schema_version` (integer), `enums`, `entities`, `relationships`, `operations` — with closed-set field types (`uuid`, `string`, `int`, `float`, `bool`, `datetime`, `ref`, plus named enums) and closed-set relationship cardinalities (`one-to-one`, `one-to-many`, `many-to-one`, `many-to-many`). The schema lives in the same place as parlay's other schemas so it ships in the binary, round-trips through `parlay upgrade`, and is loadable by any consumer (validator, codegen, Studio editor) without an AI inference pass. Authoring the schema by hand against the published file produces a model that downstream tooling parses, validates, and round-trips deterministically.
**Invariants**:
- The schema artifact is published at the repo-level root's schemas path (the same path as adapter, buildfile, surface, and other parlay schemas) so `parlay upgrade` redeploys it alongside the others
- The schema declares `schema_version` as a required top-level integer; a YAML missing `schema_version` fails parse with an error naming the missing field
- The schema's first released `schema_version` value is the integer `1`. Future releases bump to `2`, `3`, etc. — never semver, never a string tag
- Field types are a closed set; a YAML declaring a field with a type outside the set fails validation naming the offending entity, field, and disallowed type
- Relationship cardinalities are a closed set; a YAML declaring an unknown cardinality fails validation naming the offending relationship and disallowed cardinality
- Operation `effects` are declarative free-text statements, not executable code; the schema accepts arbitrary strings here and does no syntactic check on them
- Adding a new field-type primitive or relationship cardinality requires a `schema_version` bump and a per-version migrator entry; no code outside the schema changes for the type system itself
**Source**: @studio-support/domain-model-yaml-migration/define-a-versioned-yaml-schema-for-the-domain-model
**Caching**: per-process — the parsed schema is cached after first load within a CLI invocation
**Backward-Compatible**: no — projects with `domain-model.md` must run `parlay migrate-domain-model` to produce a YAML before any post-migration command will read a domain model. The deprecated `.md` continues to exist on disk but is never parsed by post-migration code paths.

**Notes**:
- The schema is the contract between Core, Studio's Domain Model Editor, and any third-party consumer. Hand-editing the YAML against this published schema is a first-class flow, not a degraded mode.
- Q1 from dialogs (enum tone closed set vs open string) and Q2 (flat fields vs nested types) directly shape this schema. The artifact must be authored once those questions resolve. Until then, the safe-default is closed-tone-set (matches `adapter-vocabulary-extension`'s color-token tone vocabulary) and flat-only fields with refs.

---

## domain-model deep-validation pipeline

**Affects**: domain-model validation pipeline, reference resolution, error reporting
**Behavior**: Every read of `domain-model.yaml` runs deep validation in addition to schema-shape validation. Deep validation walks every relationship endpoint and verifies it names a declared entity, every operation input and verifies it names a field that exists on the involved entity, and every enum-typed field reference and verifies the enum value is declared. Failures are reported per-reference with the YAML path of the offending value. The model is not partially accepted — any unresolved reference fails the load. The same pipeline runs for hand-authored YAML, generated YAML (from `create-domain-model`), and merged YAML (from `load-domain-model`).
**Invariants**:
- A relationship endpoint that references an undeclared entity fails validation naming the relationship, the offending endpoint, and the missing entity name
- An operation input that names a field absent from the involved entity fails validation naming the operation, the offending input, and the entity scope where the field was expected
- An enum-typed field that references an enum value not declared on its enum fails validation naming the entity, field, enum name, and offending value
- Validation reports every unresolved reference, not just the first — the designer sees the full set of issues in one pass
- A model with all references resolved produces a deep-validation pass; the resulting model is then available to consumers without further checks
- Deep validation is intrinsic to every read path — there is no separate "validate" entry point a caller can skip
**Source**: @studio-support/domain-model-yaml-migration/define-a-versioned-yaml-schema-for-the-domain-model
**Caching**: per-process — once a YAML passes validation in a single CLI invocation, subsequent reads in the same invocation reuse the validated in-memory model
**Backward-Compatible**: yes — for projects that have a valid `domain-model.yaml`, this is purely additive (validation was already implicit in markdown parsing; YAML makes it explicit and deterministic).

**Notes**:
- The validator is one code path; every consumer (extract, load, build-feature, generate-code) calls into it. There is no per-command validation duplication.
- The error shapes are the contract surfaced by the "domain-model.yaml validation error reporting" surface fragment.

---

## schema_version routing and per-version migrator chain

**Affects**: domain-model schema-version dispatch, in-memory migrator chain, version comparison
**Behavior**: Every read of `domain-model.yaml` first inspects `schema_version` and dispatches to the matching reader. When the file's version equals the binary's expected version, the reader proceeds directly. When the file's version is older, the reader routes the in-memory model through a per-version migrator chain (e.g., v1→v2, v2→v3) before handing it to the rest of the pipeline. The on-disk file is not rewritten — only the in-memory model is migrated. When the file's version is newer than the binary, or so old that no migrator chain reaches the current version, the read fails fast with an actionable error. This is Flow B (standing forward-compatibility) — independent of Flow A (one-shot markdown→YAML port).
**Invariants**:
- A YAML with the same `schema_version` as the binary's expected version skips the migrator chain entirely
- A YAML with an older `schema_version` is routed through every intermediate migrator; skipping a step is rejected (`v1` cannot leap directly to `v3` if `v2` is the current binary version — it routes v1→v2→current)
- A YAML with a newer `schema_version` than the binary fails with an actionable error instructing the designer to upgrade Core; no forward-migration guess is attempted
- A YAML so old that no migrator chain reaches the current version fails with the same error shape, naming both the source version and the oldest version the binary supports
- Migrators are pure functions over the in-memory model — they do not write to disk, do not consult the network, and do not require AI inference
- Adding a new `schema_version` requires adding exactly one migrator (from the previous version to the new one); chaining is automatic
**Source**: @studio-support/domain-model-yaml-migration/define-a-versioned-yaml-schema-for-the-domain-model, @studio-support/domain-model-yaml-migration/update-extract-domain-model-and-load-domain-model-to-round-trip-yaml
**Caching**: none — version dispatch is constant-time and runs on every read; the chain itself is in-memory and stateless
**Backward-Compatible**: yes — the chain is the mechanism by which future format changes stay backward-compatible at the file level.

**Notes**:
- Flow A (markdown→YAML port) is a one-shot lifecycle event handled by `parlay migrate-domain-model`. Flow B (this fragment) is a recurring concern of every Core release. The two share only the `schema_version` field and never share code paths.
- `load-domain-model` invokes this same chain before merging — a loaded YAML at v1 against a v2 binary is migrated in-memory before disambiguation, never on-disk.

---

## migrate-domain-model command and idempotency guard

**Affects**: CLI command registration (Core CLI), migration parser, idempotency guard, deployer-shared command list, generic deployer hardcoded command list
**Behavior**: A new `parlay migrate-domain-model` command is registered in Core's CLI. It detects an existing `domain-model.md` at the active root, parses it via the same AI path that wrote it, emits `domain-model.yaml` beside it, and prepends a deprecation header to the `.md`. The migration is idempotent: a second run (without `--force`) when the YAML already exists exits non-zero without re-parsing the `.md` or modifying the YAML. `--dry-run` previews the conversion without writing. `--force` overwrites the existing YAML after re-parsing. `--root <name>` scopes to a single child root in multi-root projects. The command is registered in the deployer-shared command list (so it ships in every deployer's skill catalog) and in the generic deployer's hardcoded command list (so AGENT_INSTRUCTIONS.md surfaces it).
**Invariants**:
- The command exits zero on a successful migration and on the greenfield no-op case (no `.md` to convert)
- The command exits non-zero with an `already migrated` message when `domain-model.yaml` exists and `--force` was not passed; the YAML is not modified and the `.md` is not re-parsed
- The command exits non-zero with annotation markers in the YAML when the source `.md` is ambiguous (missing types, unclear cardinalities, missing tones); the YAML is still written so the designer can resolve in place
- `--dry-run` writes nothing to disk under any path (success, ambiguous, already-migrated) — the filesystem is untouched, including the deprecation header on the `.md`
- The original `.md` is preserved post-migration with exactly one deprecation header prepended; a second `--force` run does not stack a second header
- Adding the command requires updates in three places: the CLI command registration (Core), the deployer-shared command list (so all deployers package the skill), and the generic deployer's hardcoded command list. Skipping any of the three drifts the deployers
- Multi-root invocations without `--root` exit with code 11 and the standard ambiguity JSON envelope on stderr
**Source**: @studio-support/domain-model-yaml-migration/migrate-existing-domain-model-md-projects-to-yaml
**Caching**: none — the command is invoked once per migration; nothing to cache
**Backward-Compatible**: no — the command is net-new. Pre-migration projects continue to work until the user runs the command, at which point Core stops reading the `.md` and only reads the YAML.

**Notes**:
- Q3 from dialogs (Core CLI vs Studio installer ownership) directly shapes this fragment. If Studio owns the command, the CLI registration moves to Studio's bundled installer and Core's deployer-shared command list does NOT include it; the AI path that parses markdown and the idempotency guard still live in Core (so any Studio installer can shell into Core), but the user-visible command is delivered through Studio. Until Q3 resolves, this fragment assumes Core ownership (the conservative choice that gives every project a path off the deprecated format regardless of Studio adoption).
- The idempotency guard is the single piece of logic that makes upgrade scripts safe to re-run unconditionally. It is the contract behind the greenfield no-op and the already-migrated branches.
- Per project memory ("new commands need both artifacts"): adding this command means updating the deployer registry, the generic CLI hardcoded list, AND adding a skill file. This fragment names all three; build-feature will resolve them to concrete file paths.

---

## domain-model.yaml as exclusive live state post-migration

**Affects**: domain-model read paths (every parlay command that reads a domain model), `.md` vs `.yaml` precedence rule
**Behavior**: After a project has a `domain-model.yaml`, every parlay command's domain-model read path consults only the YAML. The `.md` (if present, as a pre-migration artifact) is ignored — never parsed, never merged, never consulted as a fallback. There is no grace period during which both files coexist as live state. The deprecation header on the `.md` documents this for the designer; the read path enforces it for the tool.
**Invariants**:
- A project with both a `domain-model.md` and a `domain-model.yaml` has the YAML as the sole live state; the `.md` is treated as historical-only
- A project with only a `domain-model.md` (skipped the migration) and no YAML is treated as having no domain model — `create-domain-model` writes a fresh YAML from feature intents and dialogs without consulting the `.md`; `load-domain-model` and other consumers fail with "no domain-model.yaml; run parlay migrate-domain-model or parlay create-domain-model first"
- Edits to the `.md` after migration have no effect on tooling output — the YAML is the only source
- The precedence rule has a single enforcement point (the read path); no caller bypasses it
**Source**: @studio-support/domain-model-yaml-migration/update-extract-domain-model-and-load-domain-model-to-round-trip-yaml, @studio-support/domain-model-yaml-migration/migrate-existing-domain-model-md-projects-to-yaml
**Caching**: per-process — the read-path cache stores the resolved YAML model; the `.md` is never loaded into the cache
**Backward-Compatible**: no — pre-migration callers that depended on `.md` parsing must migrate. The migration intent commits to this breakage as the cost of the format swap.

**Notes**:
- This fragment is the enforcement counterpart to the migration command. Together they ensure there is no dual-source-of-truth window: the migration writes the YAML and prepends the header; the read path stops consulting the `.md`.
- This fragment also scopes the "extract on a project still on markdown" branch from dialogs: extract treats the `.md` as absent for its own purposes, even when the file is on disk.

---

## extract-domain-model project-level YAML emission

**Affects**: create-domain-model command (existing), per-feature vs project-level output, markdown emission removal
**Behavior**: The existing `parlay create-domain-model` command is modified to emit a single `domain-model.yaml` at the active root (project level), aggregating across all features' intents and dialogs. Per-feature `domain-model.md` emission is removed entirely — the command no longer writes to `spec/intents/<feature>/domain-model.md` under any path, and the project-level emission is the only output. If the YAML write fails (permissions, disk full), the command exits non-zero without writing a fallback `.md`.
**Invariants**:
- Extract writes exactly one YAML file per invocation, at the active root, named `domain-model.yaml`
- Extract never emits markdown — not as primary output, not as fallback, not as per-feature debug artifact
- Extract aggregates across every feature in the active root; per-feature scoping is no longer supported (deliberate architectural shift)
- A project with no feature intents (greenfield) produces an empty-but-valid YAML (declared `schema_version`, empty `enums`/`entities`/`relationships`/`operations` lists)
- Existing per-feature `.md` files (from the pre-migration world) are not touched by extract — they remain on disk untouched, and extract does not read them as input
**Source**: @studio-support/domain-model-yaml-migration/update-extract-domain-model-and-load-domain-model-to-round-trip-yaml
**Caching**: none — extract runs full AI inference per invocation; reuse is at the read-path cache layer for downstream consumers
**Backward-Compatible**: no — callers that previously parsed per-feature `.md` files break. The migration intent commits to this breakage as part of the format swap.

**Notes**:
- This fragment changes both the artifact location (per-feature → project-level) and the format (md → yaml) in one step. The two changes are coupled because the project-level model is the right scope for a single canonical artifact.
- The skill file for `parlay-create-domain-model` and any related dialogs/intents in the parlay-tool tree need updating in the same release; that's a different feature's concern, but the read-path contract is established here.

---

## load-domain-model YAML-only acceptance and URL fetcher

**Affects**: load-domain-model command (existing), input format gate, URL fetcher, conflict resolver
**Behavior**: The existing `parlay load-domain-model` command is modified to accept only YAML input — markdown sources are refused with an actionable error pointing at `parlay migrate-domain-model`. The argument may be a local file path or an HTTP(S) URL; URL inputs are fetched, validated as YAML, and routed through `schema_version` migration before merging. Conflicts (incoming entity name collides with existing) trigger a designer disambiguation prompt with side-by-side fields and four choices (keep local, take incoming, merge field-by-field, rename one). Non-conflict entities merge silently.
**Invariants**:
- A markdown input (URL or path with a `.md` body) is refused with a single error message; the local YAML is not modified
- A URL fetch failure (network error, non-200 response, non-YAML body) exits non-zero with the offending response surfaced in the error
- The URL fetcher honors HTTPS certificate verification by default; bypass flags are out of scope for this feature
- Loaded YAML is always routed through the `schema_version` migrator chain before merging — the on-disk source is not modified, only the in-memory model
- Conflict prompts pause the merge until the designer answers; there is no silent default choice
- A merge that would leave the local YAML in an invalid state (broken references introduced by partial merge) is rejected as a whole — partial writes are not committed
**Source**: @studio-support/domain-model-yaml-migration/update-extract-domain-model-and-load-domain-model-to-round-trip-yaml
**Caching**: none — the URL fetcher does not cache responses across invocations; the in-memory model uses the read-path cache once merged
**Backward-Compatible**: no — callers that loaded markdown break. The migration intent commits to this breakage; users with a markdown source must run `parlay migrate-domain-model` in the source project first.

**Notes**:
- This fragment carries through the existing markdown-era disambiguation behavior; the prompt shape and the four choices are unchanged in spirit, just sourced from YAML.
- The URL path is new (load was file-only previously). Network errors get the same exit-non-zero, surface-the-response treatment as any other I/O failure in parlay commands.

---

## domain-model artifact path resolution

**Affects**: domain-model artifact path resolution (active-root vs repo-level-root), multi-root awareness
**Behavior**: The `domain-model.yaml` artifact lives at the active root's domain-model path — exactly one per parlay project, not per-feature. In a multi-root project, each child root has its own `domain-model.yaml` (or none); the parent root may also have one. The path resolver consults the same active-root resolution the rest of parlay uses (cwd-walk, `--root` flag, `PARLAY_ROOT`). Read paths consult the resolver; they do not hard-code the path.
**Invariants**:
- The artifact path is computed by the active-root resolver; no caller hard-codes `spec/intents/<feature>/domain-model.yaml` (a pre-migration shape) or any other location
- A multi-root invocation without an explicit root in an ambiguous context exits with code 11 and the standard ambiguity envelope; this is shared with every other active-root-aware command
- Each child root in a multi-root project has an independent `domain-model.yaml`; one child's model never bleeds into another's
- The repo-level root holds the schema (under `.parlay/schemas/`), shared across every active root; this is the existing schema-load discipline, not new behavior
**Source**: @studio-support/domain-model-yaml-migration/define-a-versioned-yaml-schema-for-the-domain-model, @studio-support/domain-model-yaml-migration/update-extract-domain-model-and-load-domain-model-to-round-trip-yaml
**Caching**: per-process — resolved paths are cached after first resolution within a CLI invocation
**Backward-Compatible**: no — the path moves from `spec/intents/<feature>/domain-model.md` (pre-migration) to `<active-root>/domain-model.yaml` (post-migration). Pre-migration callers that walked `spec/intents/*/domain-model.md` find nothing post-migration.

**Notes**:
- This fragment formalizes the architectural shift the intents commit to: one canonical model per project (active root), not per feature.
- The exact filename within the active root (`domain-model.yaml`) is fixed by the schema; the directory is whatever the active-root resolver returns.
