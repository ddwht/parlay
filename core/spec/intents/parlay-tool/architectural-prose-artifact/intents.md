# Architectural-prose-artifact

> Restore `infrastructure.md` to a co-equal role alongside `capabilities.yaml` in the spec layer, after real implementation revealed that `capabilities.yaml`'s closed vocabulary cannot express architectural intents (package boundaries, dependency pins, startup invariants, internal API surfaces, allowlists). The two artifacts have different jobs: `capabilities.yaml` carries closed-vocabulary backend operations on domain entities; `infrastructure.md` carries architectural prose for concerns that do not reduce to operations. This feature reverses the "legacy" framing of `infrastructure.md` introduced when `capabilities.yaml` was added, makes `migrate-capabilities` partial-migration semantics explicit, and clarifies the scope of each artifact in its schema documentation. The rename of operation-shaped infrastructure content into `capabilities.yaml` is preserved; what changes is the implication that the rename absorbs every infrastructure fragment.

---

## Restore infrastructure.md to co-equal spec-artifact status

**Goal**: Drop the "legacy" framing of `infrastructure.md` across CLAUDE.md, tool help, and skill documentation, so features with architectural intents have a documented home in the spec layer without being steered toward a closed vocabulary that does not fit them.

**Persona**: Parlay Developer

**Priority**: P0

**Context**: When `capabilities.yaml` was introduced, the architecture framed it as the replacement for `infrastructure.md` — the rename was justified as "infrastructure was originally meant to capture events that happen behind the scenes of the presentation level." That framing was correct for operational behind-the-scenes work (CRUD on domain entities) and wrong for architectural behind-the-scenes work (invariants, boundaries, probes). The figma-mcp-client implementation made the gap concrete: its three intents — SDK pin and import boundary, Figma `whoami` startup probe, and bounded MCP tool allowlist at the wrapper — each fail the closed-vocabulary fields of `capabilities.yaml`. `subject.entity` requires a name that resolves in `domain-model.yaml`, but the subjects are libraries, configurations, and APIs. `errors[]` is closed to a CRUD-shaped set that excludes feature-stable codes like `figma-mcp-endpoint-unsupported`. `steps[]` enumerates CRUD verbs, but the intents describe boundary checks and allowlist enforcement. Forcing the content into `capabilities.yaml` would either invent fake domain entities, break the closed error set, or replace closed steps with feature-defined assertions — each of which erodes the closure discipline that makes `capabilities.yaml` work. The honest answer is that `infrastructure.md` remains the home for architectural prose, distinct from `capabilities.yaml`'s home for closed-vocabulary operations.

**Action**: Update CLAUDE.md, tool help text, and skill documentation to describe `infrastructure.md` and `capabilities.yaml` as co-equal spec artifacts with distinct responsibilities. The user-visible surface of this change is in command help (`parlay create-artifacts --help`, `parlay build-feature --help`, `parlay check-readiness --help`, `parlay migrate-capabilities --help`), in the `parlay-create-artifacts` and `parlay-build-feature` skill docs that the AI agent reads, and in CLAUDE.md's description of the spec layer. The change is documentation and framing only — no schema changes, no validator changes, no migration of existing content. Existing features that already have `infrastructure.md` files keep them; features being newly authored may use either or both artifacts depending on whether their intents are operation-shaped, architectural, or mixed.

**Objects**: infrastructure.md, capabilities.yaml, spec layer, CLAUDE.md, tool help, parlay-create-artifacts skill, parlay-build-feature skill

**Constraints**:
- The "spec layer is three artifacts" framing currently in CLAUDE.md and the architecture proposal must change to four: `surface.md`, `capabilities.yaml`, `infrastructure.md`, `domain-model.yaml`. `surface.md` and `infrastructure.md` are feature-local prose; `capabilities.yaml` is feature-local closed vocabulary; `domain-model.yaml` is project-level.
- Any new feature whose intents are architectural — package import boundaries, dependency pinning rules, build-time constraints, startup invariants and probes against external systems, internal API surface restrictions, feature-stable error codes outside `capabilities.yaml`'s closed error vocabulary, code-level allowlists and denylists — must author them in `infrastructure.md`, not in `capabilities.yaml`.
- `capabilities.yaml` remains the documented home for backend operations triggered by surface actions or other events: commands, queries (subscriptions and jobs deferred to v2), with `subject.entity` resolving to a domain entity, `steps[]` drawn from the closed step vocabulary, `errors[]` from the closed error vocabulary, `policies[]` from the closed policy vocabulary.
- A feature may have any combination of `surface.md`, `capabilities.yaml`, and `infrastructure.md`. The current `check-readiness` rule already admits any non-empty subset of the three (via `hasInfra := fileExistsAt(infraPath) || fileExistsAt(capabilitiesPath)` paired with `hasSurface`); no code change is required to preserve this behavior.
- The word "legacy" does not appear in any reference to `infrastructure.md` in CLAUDE.md, tool help, or skill documentation after this change.
- No existing `infrastructure.md` content is touched by this intent. Migration of operation-shaped fragments via `parlay migrate-capabilities` remains opt-in and partial (clarified by a separate intent in this feature).

**Verify**:
- CLAUDE.md describes the spec layer as four co-equal artifacts; the description of `infrastructure.md` does not include the words "legacy," "deprecated," or "to be replaced."
- `parlay create-artifacts --help` describes both `infrastructure.md` and `capabilities.yaml` as valid authoring targets, with one sentence each on what kind of content goes where.
- The `parlay-create-artifacts` skill prompt does not steer a feature whose intents are architectural toward `capabilities.yaml` first; the skill recognizes when intents describe boundaries, probes, or invariants and authors `infrastructure.md` for them.
- The `check-readiness` rule passes a feature that has only `infrastructure.md` (no `surface.md`, no `capabilities.yaml`), preserving the behavior introduced by the infrastructure-layer feature.
- A feature with `surface.md` + `infrastructure.md` + `capabilities.yaml` passes `check-readiness` with all three artifacts loaded into the build context.

---

## Make migrate-capabilities partial-migration semantics explicit

**Goal**: Update the `parlay migrate-capabilities` command description, help text, and output to make explicit that the migration extracts only operation-shaped fragments from `infrastructure.md` and leaves architectural fragments in place — eliminating the implication that running the command empties `infrastructure.md`.

**Persona**: Parlay Developer

**Priority**: P0

**Context**: The `parlay migrate-capabilities` command's description currently reads "Extract operation-shaped fragments from `infrastructure.md` into `capabilities.yaml`" — which is accurate but surrounded by language that implies a total replacement of `infrastructure.md` by `capabilities.yaml`. In practice, the command does the right thing (it extracts operation-shaped fragments and leaves the rest alone), but a developer reading the docs reasonably expects that after running it the `infrastructure.md` should be empty or removed. That expectation is wrong, and the gap between documented intent and actual behavior is the source of confusion. This intent does not change the command's behavior; it makes the behavior explicit in the docs and adds explicit output that names what was migrated, what was left in place, and why.

**Action**: Update the command description, the `--help` text, the command's stdout output on a successful migration, and the `parlay-migrate-capabilities` skill documentation to state clearly that the migration is partial by design. The command's output, on a successful migration of a feature with mixed content, lists the operation-shaped fragments that were extracted (with their new operation ids in `capabilities.yaml`) and the architectural fragments that were retained in `infrastructure.md` (with their fragment names). The exit code remains zero on partial migration — partial migration is the success case, not a degraded case.

**Objects**: parlay migrate-capabilities, infrastructure.md, capabilities.yaml, migration output

**Constraints**:
- Command behavior is not changed by this intent. The detection of operation-shaped fragments, the YAML emission for `capabilities.yaml`, and the leave-alone of architectural fragments are all preserved as they exist today.
- The command's `--help` text states explicitly that operation-shaped fragments are extracted and architectural fragments are retained. A developer reading the help text alone, without consulting the architecture proposal or schema docs, understands the partial-migration semantics.
- The command's stdout on a successful migration prints two lists: extracted operations (named by their feature-local ids and their new operation ids in `capabilities.yaml`) and retained fragments (named by their `## Fragment Title` headings in `infrastructure.md`). Empty lists are reported as "no operation-shaped fragments detected" or "no architectural fragments retained" rather than being suppressed.
- The command exits zero on a successful migration regardless of whether any fragments were retained. Retained architectural fragments are not warnings or errors; they are the expected outcome for features with mixed content.
- The `parlay-migrate-capabilities` skill documentation references the four-artifact spec layer and describes the migration as moving operation-shaped content between two co-equal artifacts, not as deprecating one.
- If a feature's `infrastructure.md` contains only architectural fragments, the command exits zero with output "no operation-shaped fragments to migrate; infrastructure.md left in place." The command does not refuse to run on features that have no operation-shaped content.
- When every fragment in `infrastructure.md` is extracted to `capabilities.yaml`, the now-empty `infrastructure.md` is deleted by the migrator — feature presence on disk follows content state, with no zero-byte placeholders left behind.
- The command supports `--dry-run`, which prints the same partition output (extracted vs retained vs deleted) a real run would emit but does not write `capabilities.yaml` and does not modify or delete `infrastructure.md`.

**Verify**:
- Running `parlay migrate-capabilities --help` produces help text that names both extracted and retained fragments as the expected outputs of the command.
- Running `parlay migrate-capabilities` on a feature with a mixed `infrastructure.md` (operation-shaped fragments and architectural fragments) emits a `capabilities.yaml` with the operations and leaves an `infrastructure.md` with the architectural fragments; the command stdout names both sets.
- Running `parlay migrate-capabilities` on a feature with only architectural fragments emits no `capabilities.yaml` and prints "no operation-shaped fragments to migrate."
- Running `parlay migrate-capabilities` on a feature with only operation-shaped fragments emits `capabilities.yaml` and removes the now-empty `infrastructure.md` (no zero-byte file left behind).
- Running `parlay migrate-capabilities --dry-run` on any feature emits the same partition output (extracted / retained / deleted) as a real run but does not write `capabilities.yaml`, does not modify `infrastructure.md`, and does not delete `infrastructure.md`.
- The `parlay-migrate-capabilities` skill documentation does not refer to `infrastructure.md` as legacy or as a target for total replacement.

---

## Clarify architectural scope in infrastructure.schema.md

**Goal**: Update `infrastructure.schema.md` to explicitly describe what content `infrastructure.md` is for after the introduction of `capabilities.yaml`, with examples drawn from real architectural intents (boundaries, probes, allowlists, dependency pins) instead of the operation-shaped examples that motivated the rename.

**Persona**: Parlay Developer

**Priority**: P1

**Context**: The current `infrastructure.schema.md` describes the artifact's purpose as "behind-the-scenes capabilities that produce no user-facing surface" — a framing that fits both architectural concerns (good) and operational ones (now redirected to `capabilities.yaml`). The schema's existing fragment examples lean toward operational shapes (validation pipelines, traversal logic) which now belong in `capabilities.yaml`. A developer reading the schema today does not see examples of pure architectural content (boundary checks, allowlists, startup probes) and reasonably assumes the artifact is for operational content that should migrate. This intent updates the schema's prose introduction and examples to make the architectural scope explicit, without changing the fragment field set (Name, Source intent, Affects, Behavior, Invariants).

**Action**: Rewrite the introductory section of `infrastructure.schema.md` to describe the artifact's purpose as "architectural prose for concerns that do not reduce to operations." Add a "When to use infrastructure.md vs capabilities.yaml" decision section with concrete examples of architectural intents (the three figma-mcp-client intents are good real-world cases). Replace operation-shaped examples in the schema with architectural ones, preserving the field structure (Name, Source intent, Affects, Behavior, Invariants) but populating it with content from the four documented categories: "SDK import boundary" (boundary), "External-system startup probe" (probe), "Wrapper API allowlist" (allowlist), and "Library version pin" (dependency pin). Add a short cross-reference to `capabilities.schema.md` for operation-shaped content.

**Objects**: infrastructure.schema.md, capabilities.schema.md, schema documentation, architectural prose

**Constraints**:
- Fragment field set is not changed by this intent. Name, Source intent, Affects, Behavior, Invariants remain the documented fields; their semantics are unchanged.
- The schema's portability lint (flagging framework-specific keywords in `Behavior:` fields) is unchanged.
- Examples in the schema are drawn from real architectural patterns, not invented ones. The figma-mcp-client feedback document is the primary source for example shapes; existing Parlay `infrastructure.md` files (skill deployment, registry traversal, validation pipeline) are excluded from the schema examples because their operation-shaped content now belongs in `capabilities.yaml` after migration.
- The decision section "When to use infrastructure.md vs capabilities.yaml" is prose, not a closed decision table. A developer reads the prose and chooses; the schema does not auto-classify.
- `capabilities.schema.md` gets a complementary cross-reference: an introductory note that operation-shaped content lives here, while architectural prose lives in `infrastructure.md`. Symmetric framing across the two schemas.

**Verify**:
- The introduction of `infrastructure.schema.md` describes the artifact's purpose as architectural prose, not operational behavior.
- The schema includes at least one worked example of each major architectural category: a boundary (package import constraint), a probe (external-system startup check), an allowlist (closed API surface), and a dependency pin (library/version constraint).
- The schema includes a "When to use infrastructure.md vs capabilities.yaml" section with prose guidance and concrete examples.
- `capabilities.schema.md` includes a symmetric cross-reference pointing back to `infrastructure.schema.md` for architectural prose.
- A new feature being authored after this change can be classified by the AI agent into the right artifact (operation-shaped → `capabilities.yaml`; architectural → `infrastructure.md`) based on the schema docs alone, without consulting external context.

---
