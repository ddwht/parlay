# Architectural-prose-artifact — Infrastructure

---

## Co-equal spec-artifact framing

**Affects**: Spec-layer documentation in CLAUDE.md; deployed skill docs the AI agent reads (parlay-create-artifacts, parlay-build-feature); command-line help text for spec-layer commands; framing of the artifact model for new feature authors and AI classifiers.

**Behavior**: The spec layer is described as four co-equal artifacts — `surface.md`, `capabilities.yaml`, `infrastructure.md`, `domain-model.yaml` — across CLAUDE.md, the deployed skill docs the AI agent reads (parlay-create-artifacts, parlay-build-feature), and the relevant command-line help text. Each artifact has a distinct job: `surface.md` for UI surface fragments, `capabilities.yaml` for closed-vocabulary backend operations on domain entities, `infrastructure.md` for architectural prose about concerns that do not reduce to operations, `domain-model.yaml` for project-level entity definitions. The word "legacy" no longer attaches to `infrastructure.md` in any tool-surface text the user or agent encounters. New features whose intents are architectural — package import boundaries, dependency pinning rules, build-time constraints, startup invariants and probes against external systems, internal API surface restrictions, feature-stable error codes outside the closed errors vocabulary, code-level allowlists and denylists — author them in `infrastructure.md`, not in `capabilities.yaml`. Existing on-disk content is not modified by this change.

**Invariants**:
- CLAUDE.md's spec-layer description names exactly four co-equal artifacts: surface, capabilities, infrastructure, domain-model
- A repository-wide search of CLAUDE.md, the deployed skill prompts the AI agent reads, and the deployed schema docs finds no line that frames `infrastructure.md` as legacy, deprecated, or to-be-replaced
- The help text for the spec-layer commands describes both `infrastructure.md` and `capabilities.yaml` as valid authoring targets, with one sentence each on what content belongs where
- The deployed `parlay-create-artifacts` skill prompt classifies a feature with architectural-only intents into `infrastructure.md` and does not author `capabilities.yaml` for that feature
- The build-feature readiness rule continues to admit any non-empty subset of {surface, infrastructure, capabilities} — behavior is documented as already-correct; no validator or readiness-checker code change is introduced
- Every existing `infrastructure.md` file in the repository is byte-identical before and after this feature ships

**Source**: @architectural-prose-artifact/restore-infrastructuremd-to-co-equal-spec-artifact-status

**Backward-Compatible**: yes

**Notes**:
- This is a documentation-and-framing-only change. No schema validator additions, no rename of any file or directory, no migration of existing content
- The published multi-adapter design document (the long-running proposal whose §6 frames `capabilities.yaml` as a rename of `infrastructure.md`) is treated as historical reference material and is intentionally not touched by this feature; the spec-layer source-of-truth becomes the schema docs and CLAUDE.md

---

## Partial-migration semantics in migrate-capabilities

**Affects**: `parlay migrate-capabilities` command description and help text; command stdout on successful migration; the deployed parlay-migrate-capabilities skill documentation; developer expectations about whether running the migration empties `infrastructure.md`.

**Behavior**: The `parlay migrate-capabilities` command documents and emits its partial-migration semantics explicitly. The command's description and help text state that operation-shaped fragments are extracted into `capabilities.yaml` while architectural fragments are retained in `infrastructure.md`; partial migration is the success case, not a degraded case. On a successful run, the command's stdout prints two lists — extracted operations (named by their feature-local id and their new operation id in `capabilities.yaml`) and retained fragments (named by their fragment headings in `infrastructure.md`) — with empty lists reported explicitly rather than suppressed. When every fragment is extracted and `infrastructure.md` becomes empty, the migrator deletes the now-empty file; the command does not leave zero-byte placeholders. A `--dry-run` flag previews the same partition output a real run would emit but writes nothing, modifies nothing, and deletes nothing. The exit code is zero on any successful migration regardless of whether fragments were retained.

**Invariants**:
- The command's `--help` text states explicitly that operation-shaped fragments are extracted and architectural fragments are retained; a developer reading the help text alone understands the partial-migration semantics
- On a feature with mixed `infrastructure.md` content, the command emits a `capabilities.yaml` with the operations, leaves an `infrastructure.md` with the architectural fragments, and prints both lists to stdout
- On a feature with only architectural fragments, the command emits no `capabilities.yaml` and prints "no operation-shaped fragments to migrate; infrastructure.md left in place"; the exit code is zero
- On a feature where every fragment in `infrastructure.md` is operation-shaped, the command emits `capabilities.yaml`, deletes the now-empty `infrastructure.md`, and prints "Deleted: infrastructure.md (was empty after extraction)"; no zero-byte file is left on disk
- The command exits zero on any successful migration regardless of how the fragments partitioned; retained fragments are never reported as warnings or errors
- The `--dry-run` flag produces the same partition output as a real run with a "(dry run — no files written)" header; after the dry run, the on-disk state of the feature folder is byte-identical to before the dry run
- The deployed parlay-migrate-capabilities skill documentation references the four-artifact spec layer and characterizes the migration as moving operation-shaped content between two co-equal artifacts; it does not describe `infrastructure.md` as legacy or as a target for total replacement

**Source**: @architectural-prose-artifact/make-migrate-capabilities-partial-migration-semantics-explicit

**Backward-Compatible**: yes

**Notes**:
- The command's detection logic for operation-shaped fragments, the YAML emission shape for `capabilities.yaml`, and the leave-alone of architectural fragments are preserved exactly as they exist today; this feature only changes the surrounding documentation and the stdout output
- Idempotent re-runs are preserved: running the command twice in succession produces the same result as running it once

---

## Architectural scope in infrastructure.schema.md

**Affects**: `infrastructure.schema.md` schema document; complementary cross-reference in `capabilities.schema.md`; the AI agent's ability to classify a new feature's intents into the correct artifact using schema documentation alone.

**Behavior**: The infrastructure schema document opens by describing the artifact's purpose as "architectural prose for concerns that do not reduce to operations" and cross-references the capabilities schema for operation-shaped content. A "When to use infrastructure.md vs capabilities.yaml" section provides prose guidance (not a closed decision table) that explains which artifact takes which kind of content. The schema's worked examples are drawn from four documented architectural categories — boundary (package import constraint), probe (external-system startup check), allowlist (closed API surface), and dependency pin (library/version constraint) — and each example populates the existing fragment field set (Name, Source intent, Affects, Behavior, Invariants) without changing the field semantics. The capabilities schema receives a symmetric cross-reference at its introduction. Historical operation-shaped fragments from Parlay's own `infrastructure.md` files are deliberately excluded from the schema's example set, because using them would mislead authors into thinking operation-shaped content is the intended scope. Schema guidance is advisory: no new validator rule rejects operation-shaped fragments authored directly in `infrastructure.md`; enforcement remains via the opt-in `parlay migrate-capabilities` path.

**Invariants**:
- The introduction of `infrastructure.schema.md` describes the artifact's purpose as architectural prose, not operational behavior
- The schema includes at least one worked example for each of the four documented architectural categories: boundary, probe, allowlist, and dependency pin
- The schema includes a "When to use infrastructure.md vs capabilities.yaml" section with prose guidance and concrete examples; this section is not a closed decision table and the schema does not auto-classify
- `capabilities.schema.md` carries a complementary cross-reference at its introduction pointing back to `infrastructure.schema.md` for architectural prose
- The existing fragment field set (Name, Source intent, Affects, Behavior, Invariants) is unchanged; the existing portability lint is unchanged
- The schema remains advisory — no validator rule fails when an operation-shaped fragment is authored in `infrastructure.md`; the migrator remains the only enforcement path and remains opt-in
- An AI agent that ingests only the two schema docs (no other project context) classifies the figma-mcp-client intents correctly: SDK import boundary → `infrastructure.md`, startup probe → `infrastructure.md`, bounded tool allowlist → `infrastructure.md`, and zero `capabilities.yaml` authored

**Source**: @architectural-prose-artifact/clarify-architectural-scope-in-infrastructureschema-md

**Backward-Compatible**: yes

**Notes**:
- The four architectural categories (boundary, probe, allowlist, dependency pin) are representative, not exhaustive; the prose guidance signals that other architectural concerns (e.g. build-time constraints, lint rules, dependency policies) may also live in `infrastructure.md`. A category list closure would conflict with the artifact's prose nature
- The schema rewrite is documentation only — no change to how `infrastructure.md` is parsed, validated, or transformed at build-feature time

---
