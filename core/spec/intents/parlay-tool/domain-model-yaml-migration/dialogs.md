# Domain-model-yaml-migration — Dialogs

---

### Define a Versioned YAML Schema for the Domain Model

**Trigger**: A designer (or Studio's editor on the designer's behalf) opens or generates `domain-model.yaml` at the active root and expects Core's tooling to parse, validate, and round-trip it.

User: Authors `domain-model.yaml` by hand, or via Studio's Domain Model Editor, with `schema_version: 1` at the top and sections for `enums`, `entities`, `relationships`, `operations`.
System: Loads the file, resolves its `schema_version` against the schema published in the project's schemas directory, and runs deep validation — every relationship endpoint resolves to a declared entity, every operation input names a real field, every enum value referenced by a field is declared.
System: On success, the model is available to downstream tooling (codegen, Studio's editor) without an AI inference pass. Hand-editing remains a first-class flow — the YAML is plain text and the schema is the contract.

#### Branch: schema_version mismatch

User: Loads a `domain-model.yaml` whose `schema_version` is older than what the running Core release supports (e.g., the file says `1`, the binary expects `2`).
System: Fails with a clear, actionable error that names the file's version, the expected version, and the migration path the designer should follow. The error is loud — never a silent parse failure that produces a half-loaded model.

#### Branch: schema_version newer than binary

User: Loads a `domain-model.yaml` whose `schema_version` is newer than the running Core release understands (e.g., the file says `2`, the binary only knows `1`).
System: Refuses to load. Tells the designer to upgrade Core (`parlay upgrade`) before continuing. Does not attempt a forward-migration guess.

#### Branch: deep-validation failure

User: Authors a model where a relationship endpoint references an undeclared entity, or an operation input names a field that does not exist on the involved entity, or an enum field references an enum value that is not declared.
System: Reports each unresolved reference with the YAML path (`entities.Order.fields.status` references undeclared enum `OrderStatus`) and exits non-zero. The model is not partially accepted.

#### Branch: enum tone — closed set vs open strings (Question 1)

This branch surfaces the deferred question on whether enum `tone` is a closed vocabulary or a free-form string. The choice changes how the schema validates a `tone` field and how Studio renders it.

User: Declares an enum value with a `tone` (e.g., `value: paid, label: "Paid", tone: success`).
System: Either (a) validates `tone` against a closed set `{neutral, info, warning, danger, success}` and rejects unknown values with the list of allowed tones, or (b) accepts any string and leaves rendering responsibility to consumers, which means Studio (and any other renderer) needs a deterministic fallback for tones it doesn't recognize.

Trade-off: a closed set keeps Studio's rendering predictable and the schema easy to validate, at the cost of requiring a `schema_version` bump every time a new tone is needed. An open string gives the author flexibility today but forces every consumer to define its own fallback rendering path and weakens the cross-tool contract.

Decision deferred — see Questions section of `intents.md`.

#### Branch: entity shape — flat fields only vs nested types (Question 2)

This branch surfaces the deferred question on whether entities can declare struct-shaped (nested) fields or are restricted to flat primitives plus refs. The choice changes what the schema accepts as a field type and how nested shapes get modeled.

User: Declares an entity with a field whose value is itself a structured shape (e.g., an `address` field with `street`, `city`, `postal_code`).
System: Either (a) accepts only flat field shapes — primitive types, named enums, or a `ref` with `target: <Entity>` — and forces the designer to lift the nested shape into a separate entity joined by a `ref`, or (b) accepts inline nested object literals as a field type and validates them recursively against the same field rules.

Trade-off: flat-only keeps the schema small, makes diffing and codegen straightforward, and gives every shape a stable name; nesting is more expressive for genuine value-object cases (addresses, money amounts) and avoids polluting the entity list with artificial top-level entities that exist only to hold a sub-shape.

Decision deferred — see Questions section of `intents.md`.

#### Branch: hand-editing remains supported

User: Opens `domain-model.yaml` in a text editor and patches a single field's `required` flag from `false` to `true`.
System: Re-validates on next read. No tool round-trip is required; YAML is the source of truth. Studio's editor, when present, also reads and writes the same file — the designer can move freely between text editor and Studio without a sync step.

#### Branch: extending field types or enum values

User: A future Core release wants to add a new field-type primitive (e.g., `decimal`) or a new enum tone.
System: The change lives entirely in the schema file under the project's schemas directory and bumps `schema_version`. No code outside the schema needs editing for the type system itself; the migration path for old files is a per-version migrator (Flow B from the intent).

#### Branch: Flow A vs Flow B separation

User: Asks whether `schema_version` is the same mechanism that ports old `domain-model.md` projects to YAML.
System: Clarifies — Flow A (markdown→YAML port) is a one-shot, project-lifetime event handled by `parlay migrate-domain-model` (see the migration intent's dialog). Flow B (YAML schema evolution) is a recurring concern of every Core release and is what `schema_version` exists for. The two share only the `schema_version` field; they do not share code paths.

---

### Migrate Existing `domain-model.md` Projects to YAML

**Trigger**: A designer upgrades Core in a project that still has `domain-model.md` and wants to switch to the YAML format without manual rework.

User: Runs `parlay migrate-domain-model` (optionally with `--root <name>` in a multi-root project).
System: Detects `domain-model.md` at the active root, parses it via the same AI path that wrote it, emits `domain-model.yaml` beside it, and adds a deprecation header to the `.md` pointing at the YAML. Reports the diff so the designer can review the conversion before committing.
System: Subsequent `parlay` commands at that root read only the YAML; the `.md` is preserved for history but never parsed again.

#### Branch: --dry-run

User: Runs `parlay migrate-domain-model --dry-run` to preview the conversion before writing anything.
System: Prints the planned `domain-model.yaml` and the diff against any existing artifact. Writes nothing. Exit code reflects what a real run would do (zero on a clean migration, non-zero on ambiguity), so `--dry-run` doubles as a CI check.

#### Branch: already migrated (no --force)

User: Runs `parlay migrate-domain-model` in a project that already has `domain-model.yaml`.
System: Exits non-zero with an "already migrated" message. Does not modify the YAML. Does not re-parse the `.md` (the `.yaml` is now authoritative).

#### Branch: --force overwrite

User: Runs `parlay migrate-domain-model --force` to redo the migration after the `.yaml` was hand-edited or corrupted.
System: Re-parses the `.md` and overwrites `domain-model.yaml`. Warns explicitly that any hand edits to the YAML are discarded. The designer is expected to have the YAML under version control.

#### Branch: greenfield project (no .md)

User: Runs `parlay migrate-domain-model` in a project that never had `domain-model.md`.
System: Reports "nothing to migrate" and exits zero. This is a no-op, not an error — the migration is safe to run as part of an upgrade script that doesn't know whether each project has a legacy model.

#### Branch: ambiguous markdown

User: Runs the migration on a markdown model where a field has no declared type, or a relationship's endpoint or cardinality is unclear, or an enum's tone is missing.
System: Emits the YAML with explicit annotation markers on each unresolved field (e.g., `type: <unresolved: original prose said "the date">` ) and exits non-zero. The designer must resolve every annotation before downstream commands (codegen, extract, load) will accept the model.

#### Branch: idempotent re-run

User: Runs `parlay migrate-domain-model` twice in succession on a project that has only `.md` (the second run happens before the designer notices the YAML).
System: First run produces YAML and exits zero. Second run sees the YAML, exits non-zero with "already migrated", and does not re-parse the `.md`. Net result: identical YAML, no surprise mutation.

#### Branch: --root in multi-root project

User: Runs `parlay migrate-domain-model --root child-a` in a project with multiple registered child roots.
System: Targets only `child-a`'s `domain-model.md`. Other children's models are untouched. Without `--root` in an ambiguous multi-root context, the CLI exits with the standard ambiguity envelope and asks the designer to pick a root.

#### Branch: original .md preservation

User: After a successful migration, opens `domain-model.md` to read history.
System: The file is intact, with a deprecation header at the top pointing at `domain-model.yaml`. The header is the only Core-authored change to the file. The designer may delete the `.md` manually at any time; Core never deletes it.

#### Branch: no grace period for coexistence

User: Tries to keep editing `domain-model.md` after migration, expecting Core to merge changes back.
System: Once `domain-model.yaml` exists, Core ignores the `.md` entirely. Edits to the markdown have no effect on tooling. The deprecation header makes this explicit. There is no dual-source-of-truth window.

#### Branch: command ownership — Core CLI vs Studio installer (Question 3)

This branch surfaces the deferred question on whether `migrate-domain-model` lives in Core's CLI or is packaged with Studio's installer. The choice changes who can run the migration and when it naturally surfaces.

User: Wants to convert `domain-model.md` to `domain-model.yaml` after upgrading.
System: Either (a) ships the migration as part of Core's CLI so `parlay migrate-domain-model` is available in every parlay project the moment Core is upgraded, regardless of Studio, or (b) packages it with Studio's installer so the migration is offered at the moment a designer adopts Studio — the natural trigger point — with Studio shelling into a bundled migrator.

Trade-off: Core ownership means every project (including ones that never adopt Studio) has a way off the deprecated format and the canonical implementation has a single home, at the cost of Core CLI surface growth. Studio packaging aligns the migration with the natural adoption moment and keeps Core's CLI lean, at the cost of stranding non-Studio users on the deprecated format until they install Studio.

Decision deferred — see Questions section of `intents.md`.

---

### Update `extract-domain-model` and `load-domain-model` to Round-Trip YAML

**Trigger**: A designer runs the existing extraction or load commands after upgrading to a Core release that uses YAML.

User: Runs `parlay create-domain-model` on a project with feature intents and dialogs.
System: Performs the AI extraction pass over all features and writes a single `domain-model.yaml` at the project (active-root) level — not per-feature. The YAML includes every entity, enum, relationship, and operation the extraction surfaced. Existing per-feature `domain-model.md` files (from the pre-migration world) are not touched and not re-emitted.

User: Runs `parlay load-domain-model <path-or-url>` against a YAML produced by another project.
System: Parses the YAML, validates it against the local schema, and merges into the project's `domain-model.yaml`. Conflicts (an incoming entity shares a name with an existing one but differs in fields) trigger a designer disambiguation prompt — same prompt shape as the markdown era, just over YAML.

#### Branch: extract no longer emits markdown

User: Looks for a fresh `domain-model.md` after running `parlay create-domain-model`.
System: There is none. Extract writes only YAML. Any `.md` present in the project is a pre-migration artifact left behind by `parlay migrate-domain-model` and is never re-emitted by extract — even as a fallback when the YAML write fails (in that case extract exits non-zero without writing anything).

#### Branch: extract is project-level, not per-feature

User: Looks for `domain-model.yaml` under `spec/intents/<feature>/`.
System: Not there. The YAML lives once per project, at the active root. Extract aggregates across all features. This is a deliberate architectural shift — the canonical model is the project's, not any single feature's.

#### Branch: load rejects markdown

User: Runs `parlay load-domain-model some-other-project/domain-model.md` from a project that hasn't migrated yet.
System: Refuses with an actionable error: "load accepts YAML only; run `parlay migrate-domain-model` in the source project first." Markdown is out of scope for cross-project sharing in the post-migration world.

#### Branch: load accepts file path

User: Runs `parlay load-domain-model ./vendor-model.yaml`.
System: Reads the local file, validates `schema_version`, and merges.

#### Branch: load accepts URL

User: Runs `parlay load-domain-model https://example.com/shared-model.yaml`.
System: Fetches the URL, validates `schema_version`, and merges. Network errors and non-YAML payloads exit non-zero with the offending response surfaced in the error.

#### Branch: load with older schema_version

User: Loads a YAML whose `schema_version` is older than the running Core release.
System: Routes the file through the per-version migrator chain (Flow B) before merging. The user sees a "migrating loaded model from v1 to v2" notice; the on-disk file is not modified, only the in-memory model used for the merge.

#### Branch: load with unsupported schema_version

User: Loads a YAML whose `schema_version` is newer than the running Core release understands, or so old that no migrator chain reaches the current version.
System: Fails with an actionable error pointing at the migration path (upgrade Core, or upgrade the source project, depending on direction). Does not silently drop unknown fields.

#### Branch: load conflict — disambiguation prompt

User: The incoming model declares an entity `Order` whose fields differ from the local `Order`.
System: Pauses the merge and presents the designer with a side-by-side: incoming fields, local fields, and choices — keep local, take incoming, merge field-by-field, rename one of them. Same shape as the markdown-era prompt; just sourced from YAML.

#### Branch: round-trip across projects

User: Runs `parlay create-domain-model` in project A, copies the YAML to project B, runs `parlay load-domain-model ./from-a.yaml` in B.
System: B's `domain-model.yaml` ends up structurally equivalent to A's (same entities, enums, relationships, operations), modulo any disambiguation choices the designer made on conflicts. This round-trip is the regression test for the format.

#### Branch: extract on a project still on markdown

User: Runs `parlay create-domain-model` in a project that has `domain-model.md` but no YAML yet (skipped the migration).
System: Treats this as a fresh extraction — writes `domain-model.yaml` from feature intents and dialogs as if the `.md` were not there. The pre-existing `.md` is not consulted; the designer should run `parlay migrate-domain-model` first if the markdown carried hand-authored content not derivable from intents.

---
