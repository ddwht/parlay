# Architectural-prose-artifact — Dialogs

---

### Restore infrastructure.md to co-equal spec-artifact status

**Trigger**: A Parlay Developer is authoring or scaffolding a feature whose intents are architectural in shape (boundaries, probes, allowlists, dependency pins), and needs to know which spec artifact to use.

User: Opens CLAUDE.md to review the spec-layer description.
System: CLAUDE.md describes four co-equal spec artifacts — `surface.md` (UI surface, feature-local prose), `capabilities.yaml` (closed-vocabulary operations, feature-local), `infrastructure.md` (architectural prose, feature-local), `domain-model.yaml` (entities, project-level). No mention of "legacy" alongside any of them.
User: Invokes `/parlay-create-artifacts @==initiative==/==architectural-feature==`.
System (background): Skill inspects intents.md and detects architectural patterns — subjects are libraries/configurations/internal APIs (not domain entities), errors are feature-stable codes (not the closed `errors.schema.md` set), Goal/Action describe enforcement boundaries (not CRUD on entities).
System: Authored `infrastructure.md` (==N== fragments). Did not author `capabilities.yaml` — intents are architectural, not operation-shaped.

#### Branch: Mixed intents — both artifacts authored

User: Invokes `/parlay-create-artifacts` on a feature whose intents include one architectural intent (a package boundary) and one operation intent (CRUD on a domain entity declared in `domain-model.yaml`).
System (background): Skill partitions intents — boundary → architectural; CRUD → operation-shaped.
System: Authored `infrastructure.md` for the boundary fragment AND `capabilities.yaml` for the CRUD operation. Same feature, both artifacts, no warning — co-equal authoring is the expected shape.

#### Branch: Surface + architectural intents

User: Invokes `/parlay-create-artifacts` on a feature that has a UI surface AND an architectural startup-probe intent.
System: Authored `surface.md` (UI fragments) AND `infrastructure.md` (probe fragment). No `capabilities.yaml` — there are no domain-entity operations.

#### Branch: Grep for "legacy" returns no infrastructure-related matches

User: After the docs change, runs `grep -rn "legacy" CLAUDE.md .claude/skills/ core/internal/embedded/skills/ core/internal/embedded/schemas/`.
System: No match line associates "legacy" with `infrastructure.md`, `infrastructure.schema.md`, or the infrastructure artifact concept. (The word may still appear unrelated — e.g., "legacy surface.md format" — but never as a framing for the infrastructure artifact.)

#### Branch: Existing infrastructure.md files untouched

User: Compares `core/spec/intents/parlay-tool/==feature==/infrastructure.md` byte-for-byte against the version on the previous commit.
System: Files are identical. This feature's scope is documentation and framing only; existing `infrastructure.md` content is not migrated or rewritten. Content-level migration of operation-shaped fragments is opt-in via `parlay migrate-capabilities` (see the next dialog).

#### Branch: check-readiness already permits any non-empty subset

User: Runs `parlay check-readiness --stage build-feature @==initiative==/==infrastructure-only-feature==` on a feature with only `infrastructure.md` (no `surface.md`, no `capabilities.yaml`).
System: `{"ready": true, "issues": null}`. No code change to `check_readiness.go` was required — the rule already admitted any non-empty subset of the three artifacts. This dialog confirms the documented behavior matches the runtime behavior.

---

### Make migrate-capabilities partial-migration semantics explicit

**Trigger**: A Parlay Developer wants to migrate operation-shaped fragments out of `infrastructure.md` into `capabilities.yaml` for a feature with mixed content, or to verify what would migrate without writing.

User: parlay migrate-capabilities --help
System: ==Updated help text==:
System:   Extract operation-shaped fragments from infrastructure.md into capabilities.yaml.
System:   Partial by design: architectural fragments (boundaries, probes, allowlists,
System:   dependency pins) are retained in infrastructure.md. The command prints both
System:   the extracted operations and the retained fragments. Use --dry-run to preview
System:   without writing.
User: parlay migrate-capabilities @==initiative==/==mixed-feature==
System (background): Reads infrastructure.md, partitions fragments by shape — operation-shaped (entity subject, CRUD-shaped step sequence, errors drawn from closed set) vs architectural (everything else).
System: Migrated ==mixed-feature==:
System:   Extracted to capabilities.yaml:
System:     - ==task-create-fragment== (==task.create==)
System:     - ==task-update-fragment== (==task.update==)
System:   Retained in infrastructure.md:
System:     - ==Task storage boundary==
System:     - ==Validation pipeline==
System: (exit 0)

#### Branch: Architecture-only feature

User: parlay migrate-capabilities @==initiative==/==architecture-only-feature==
System (background): No operation-shaped fragments detected in infrastructure.md.
System: no operation-shaped fragments to migrate; infrastructure.md left in place
System: (exit 0)

#### Branch: Operation-only feature — empty file deleted

User: parlay migrate-capabilities @==initiative==/==operation-only-feature==
System (background): Every fragment in infrastructure.md is operation-shaped; all migrate.
System: Migrated ==operation-only-feature==:
System:   Extracted to capabilities.yaml:
System:     - ==fragment-a== (==entity.create==)
System:     - ==fragment-b== (==entity.read==)
System:     - ==fragment-c== (==entity.update==)
System:   Retained in infrastructure.md:
System:     (none)
System:   Deleted: infrastructure.md (was empty after extraction)
System: (exit 0)

#### Branch: --dry-run preview

User: parlay migrate-capabilities @==initiative==/==mixed-feature== --dry-run
System (background): Reads infrastructure.md, computes the same partition as a real run.
System: ==Same partition output as a real run, with a leading "(dry run — no files written)" header==
System (background): Writes nothing. Does not modify or delete infrastructure.md. Does not create capabilities.yaml.
System: (exit 0)
User: ls -la spec/intents/==initiative==/==mixed-feature==/
System: ==infrastructure.md and capabilities.yaml directory contents are byte-identical to before the dry-run invocation==

#### Branch: CI script exit-code check

User: ==CI script== invokes parlay migrate-capabilities for every feature and asserts exit code == 0.
System (background): Whether migration extracted operations, retained architectural fragments, or did nothing, exit code is 0 — partial migration is the success case, not a degraded case.
System: CI passes. Retained fragments are not treated as warnings or errors.

#### Branch: AI agent reading the skill doc

User: AI agent loads parlay-migrate-capabilities skill documentation.
System (background): Skill doc references the four-artifact spec layer (surface.md, capabilities.yaml, infrastructure.md, domain-model.yaml) and describes the migration as moving operation-shaped content between two co-equal artifacts.
System: AI agent does not see the word "legacy" applied to infrastructure.md and does not characterize the command as "deprecating" infrastructure.md to users.

---

### Clarify architectural scope in infrastructure.schema.md

**Trigger**: A Parlay Developer is authoring a new architectural feature, or an AI agent is classifying a feature's intents into the right artifact, and consults the schema docs for guidance.

User: Opens `.parlay/schemas/infrastructure.schema.md`.
System: Schema doc opens with "Architectural prose for concerns that do not reduce to operations." Cross-references `capabilities.schema.md` for operation-shaped content.
User: Reads the "When to use infrastructure.md vs capabilities.yaml" section.
System: Prose guidance: `capabilities.yaml` is for backend operations on domain entities — commands or queries (subscriptions/jobs deferred to v2), with `subject.entity` resolving in `domain-model.yaml`, `steps[]` from the closed step vocabulary, `errors[]` from the closed error vocabulary, `policies[]` from the closed policy vocabulary. `infrastructure.md` is for architectural prose — package import boundaries, dependency pinning rules, build-time constraints, startup invariants and probes against external systems, internal API surface restrictions, feature-stable error codes outside `capabilities.yaml`'s closed error vocabulary, code-level allowlists and denylists. Mixed features author both.
User: Scrolls to the worked examples.
System: Four worked examples populate the fragment field structure (Name, Source intent, Affects, Behavior, Invariants):
System:   - ==SDK import boundary== (boundary)
System:   - ==External-system startup probe== (probe)
System:   - ==Wrapper API allowlist== (allowlist)
System:   - ==Library version pin== (dependency pin)
User: Opens `.parlay/schemas/capabilities.schema.md`.
System: Capabilities schema opens with a symmetric cross-reference: "Operation-shaped content lives here. Architectural prose for boundaries, probes, allowlists, and dependency pins lives in `infrastructure.md` — see `infrastructure.schema.md`."

#### Branch: AI agent classifies via schemas alone

User: AI agent ingests only `infrastructure.schema.md` and `capabilities.schema.md` (no external context, no project history).
System (background): For each intent in a hypothetical feature, the agent checks: does the subject resolve to a domain entity? do the constraints describe CRUD-shaped steps? are errors in the closed set? If yes → `capabilities.yaml`. If no → `infrastructure.md`.
System: Agent correctly classifies architectural intents (boundaries, probes, allowlists, version pins) → `infrastructure.md`; operation-shaped intents → `capabilities.yaml`; mixed → both. Classification accuracy on the figma-mcp-client test case is 3/3 architectural.

#### Branch: Schema stays advisory — operation-shaped fragment in infrastructure.md is not rejected

User: Authors an operation-shaped fragment (subject is a domain entity, steps are CRUD-shaped) directly in `infrastructure.md` and skips `capabilities.yaml`.
System: parlay validate --type infrastructure ==feature==/infrastructure.md
System: OK
System (background): Schema is advisory — no validator rule rejects operation-shaped content in `infrastructure.md`. The migration path (`parlay migrate-capabilities`) is the only enforcement, and it is opt-in.
User: parlay migrate-capabilities @==initiative==/==feature==
System: Extracted to capabilities.yaml: ==operation-shaped-fragment== (==entity.create==). Retained in infrastructure.md: (none). Deleted: infrastructure.md (was empty after extraction).

#### Branch: Existing Parlay infrastructure.md examples deliberately excluded from schema

User: Compares the schema's worked examples to the actual `infrastructure.md` files in `core/spec/intents/parlay-tool/`.
System (background): The schema's examples are architectural (SDK import boundary, startup probe, allowlist, version pin). Parlay's historical `infrastructure.md` content — skill deployment, registry traversal, validation pipeline — is operation-shaped and migrates to `capabilities.yaml` after `parlay migrate-capabilities` runs.
System: The schema excludes those historical fragments from its examples because using them as architectural exemplars would mislead authors. The exclusion is by design and is noted in the schema's "When to use" section.

#### Branch: New architectural category emerges

User: Authors a feature with an architectural concern that is not in the four documented categories (e.g., a graph traversal invariant).
System (background): Schema's prose guidance is advisory, not closed. The four categories (boundary, probe, allowlist, dependency pin) are representative, not exhaustive.
System: Feature is authored in `infrastructure.md`. parlay validate --type infrastructure passes. The schema's category list may grow in a later schema revision; this is not blocking.

---
