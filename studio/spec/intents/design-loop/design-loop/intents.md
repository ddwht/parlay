# Design-loop

> The Design Loop is the round-trip mechanism between Studio's canonical layout (a typed tree of design-system components, on disk as YAML) and Figma's canvas. After the `figma-mcp-via-host-agent` retraction, Studio's binary no longer talks to Figma MCP at all — the Design Loop is implemented as a parlay-skill that the operator runs in their host agent (Claude Code today; Cursor / future catalog clients later), piggybacking on the host's existing Figma MCP catalog connection. The skill reads the per-feature layout artifacts on disk, performs the round-trip via the host's Figma MCP tools, and writes results back to disk. Studio binary and skill execution communicate exclusively through parlay project artifacts; there is no IPC, no shared process state. This feature pins three things: the skill-as-implementation framing, the on-disk artifact contract that defines what the skill reads and writes, and the tool-call orchestration that drives the round-trip including read-back sync and conflict detection.

---

## Design Loop is a parlay-skill executed by the host agent

**Goal**: Pin that the Design Loop's runtime implementation is a parlay-skill the operator invokes in their host agent's terminal, not code that runs in Studio's binary. The host agent's existing Figma MCP catalog connection is the transport; Studio binary and skill are sibling tools that collaborate via on-disk artifacts.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The `figma-mcp-via-host-agent` retraction (shipped 2026-05-13) removed all Studio-binary code that called Figma MCP directly. Figma's MCP server is catalog-gated and only admits pre-approved clients (VS Code, Cursor, Claude Code); Studio is not on the catalog and the registration endpoint returns 403. Rather than block on Figma's waitlist or pivot Studio entirely into a Claude Code skill, this loop chose a surgical hybrid: Studio binary serves the Domain Model Editor's web UI; the Design Loop is a separate runtime path that lives as a parlay-skill executed by whichever host agent the operator is already running. The skill's prompt instructs the host's LLM (Claude in Claude Code; whatever Cursor uses in Cursor) to perform the round-trip via the host's Figma MCP tool surface. Studio binary and skill never share memory, sockets, or process state — they communicate only through named YAML files in the parlay project's spec tree.

**Action**: Define a parlay-skill named `parlay-design-loop` whose markdown source instructs the host agent how to perform the round-trip for a given feature. The skill takes a feature reference as its argument (e.g. `@<initiative>/<feature>`), reads the on-disk inputs declared by the artifact-contract intent below, walks Claude through the tool-orchestration sequence from the third intent, and writes results to the on-disk outputs. For v1, the skill ships project-local at `.claude/skills/parlay-design-loop/SKILL.md` — it lives in this repo and is invoked from the operator's Claude Code session attached to this repo. The Studio-side equivalent of `parlay upgrade` that deploys Studio-owned skills to operator projects is a separate future feature; for now, project-local is sufficient to validate the architecture.

**Objects**: parlay-design-loop-skill, host-agent-mediated-execution, on-disk-artifact-collaboration, project-local-skill-deployment, studio-skill-deployment-mechanism-deferred

**Constraints**:
- The Design Loop's runtime is exactly one parlay-skill (`parlay-design-loop`) executed by the host agent; there is no Studio-binary code path that performs the round-trip
- The skill uses only the host agent's existing Figma MCP connection; it does not authenticate against Figma directly, does not manage tokens, and does not import any MCP client SDK
- The skill's markdown source declares the bounded tool subset it instructs Claude to use; the supported set is the same subset figma-mcp-client originally pinned (`use_figma`, `create_new_file`, `add_code_connect_map`, `send_code_connect_mappings`, `get_metadata`, `get_code_connect_map`, `get_code_connect_suggestions`, `whoami`); excluded tools (`get_design_context`, `get_screenshot`, FigJam tools, etc.) are explicitly listed in the skill prompt with the same v1-exclusion rationale
- Studio binary and skill collaborate exclusively through on-disk artifacts under `spec/intents/<feature>/`; there is no IPC, no shared sockets, no environment-variable handoff at runtime, no shared in-memory state
- The skill ships at `.claude/skills/parlay-design-loop/SKILL.md` (project-local) for v1; the deployment mechanism that puts the skill into other operator projects is reserved for a separate future feature
- The skill is invokable from any host agent whose tool surface includes the supported Figma MCP tool subset (Claude Code today; Cursor and other catalog-listed hosts when their skill / agent surfaces mature)
- The skill's behavior is deterministic up to the LLM's instruction-following: the same input artifacts + the same Figma state should produce the same tool-call sequence on every invocation; the skill's prompt is precise enough to constrain Claude to one canonical path

**Verify**:
- `.claude/skills/parlay-design-loop/SKILL.md` exists, is readable, and contains a description, an arguments section, and a numbered steps list
- The skill source references the supported Figma MCP tool names by their literal MCP names (`use_figma`, `get_metadata`, etc.) — a grep across the skill source returns matches for every name in the supported subset
- The skill source contains zero references to any Studio Go package (no `studio/internal/`), no Go function calls, no language keywords — it is pure markdown instructions for Claude
- A grep across the skill source for `mcp.figma.com`, OAuth keywords, bearer-token language, or `STUDIO_FIGMA_*` env vars returns zero matches — the skill defers all authentication to the host agent
- The skill's "Arguments" section names exactly one argument: a feature reference; no Figma URL flag, no token flag, no endpoint flag

**Questions**:
- Should the skill name match the parlay convention exactly (`parlay-design-loop`), or take a Studio-specific prefix like `parlay-studio-design-loop`? The studio prefix disambiguates from any future Core skill named "design-loop"; the unadorned name reads more naturally. Resolve during dialog authoring.

---

## On-disk artifact contract between Studio binary and Design Loop skill

**Goal**: Pin the exact set of files the Design Loop skill reads and writes. These files are the only communication channel between the skill and the rest of the parlay project; they're also what the Studio binary's Domain Model Editor and any future tooling consume.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The skill ↔ Studio binary contract has to be pinned in the spec because it's the only mechanism for state to flow between the two. A future change that adds a field to the layout YAML must update the skill prompt; a change that adds a result-shape field must update both writers and readers. The artifact set has to be small enough to spec precisely and large enough to carry the round-trip's full state. The layout artifact is a typed tree of design-system components, embedded in the page schema and declared by a top-level `componentVocabulary:` field (e.g. `clarity@17`) that names the design system the layout is bound to; nodes carry typed properties, variant selections, and layout parameters drawn from that vocabulary. The Figma file or team URL is per-feature — different features routinely operate on different Figma files — which is why it cannot live in studio-config (a global concern); this feature decides exactly where in the artifact set that URL lives.

**Action**: Pin the on-disk artifact set for the Design Loop. **Inputs** (the skill reads): (1) the page schema for the feature, which declares the layout via a `layout:` field — the layout is the typed tree; (2) the layout YAML referenced by the page schema, which contains the typed-tree node hierarchy, component-vocabulary declaration, and a new `figma:` metadata block carrying the Figma file or team URL the designer is editing for this feature; (3) the design-system vocabulary referenced by the layout's `componentVocabulary:` field — resolved against the adapter config per `vocabulary-validation`. **Outputs** (the skill writes): (1) `design-loop-result.yaml` — the read-back state of the Figma file after the round-trip, including node IDs, component identities, and timestamp; (2) `design-loop-conflicts.yaml` — out-of-vocabulary or out-of-spec edits detected during read-back, written only when conflicts exist; (3) an updated layout YAML when the read-back merges in-vocabulary designer changes back into the canonical layout. All paths are relative to `spec/intents/<feature>/`.

**Objects**: layout-yaml, page-schema-layout-field, figma-metadata-block, design-loop-result-yaml, design-loop-conflicts-yaml, layout-read-back-merge, per-feature-figma-url

**Constraints**:
- The per-feature Figma file or team URL lives in the layout YAML's `figma:` block (a new top-level key in the layout schema); not in `studio-config`, not in any environment variable, not in the page schema's root
- The skill's inputs are exactly three files (page schema, layout YAML, vocabulary), all under `spec/intents/<feature>/` or its referenced adapter config; the skill does not read any file outside the parlay project tree
- The skill's outputs are exactly three artifacts: `design-loop-result.yaml`, `design-loop-conflicts.yaml`, and (conditionally) an updated layout YAML; all under `spec/intents/<feature>/`
- `design-loop-result.yaml` is written on every successful invocation; `design-loop-conflicts.yaml` is written only when conflicts are detected (its absence means "no conflicts on the last loop")
- The skill never writes outside the named output paths; it does not modify the page schema, does not modify the vocabulary, does not write to any path outside `spec/intents/<feature>/`
- File writes are described as atomic (write-temp + rename) in the skill prompt so a partially-written result cannot be observed by the Studio binary or by a subsequent skill invocation
- The result and conflicts artifacts use YAML for consistency with the rest of the parlay project; their schemas are documented in `core/internal/embedded/schemas/` as `design-loop-result.schema.md` and `design-loop-conflicts.schema.md` (added by this feature's code phase)

**Verify**:
- The layout schema's documented shape (added to `core/internal/embedded/schemas/layout.schema.md`) declares a `figma:` block with at least a `file_url:` field; existing layout YAMLs without the block validate clean (the field is optional for read-only Domain Model Editor use)
- The skill source enumerates the three input artifacts and the three output artifacts by name; a grep across the skill source for `design-loop-result.yaml`, `design-loop-conflicts.yaml`, and `figma:` returns matches
- The skill instructs Claude to write atomically (write-temp + rename); a grep for "atomic" or "write-temp" in the skill source returns matches
- `core/internal/embedded/schemas/design-loop-result.schema.md` and `design-loop-conflicts.schema.md` are added by this feature and document the result/conflict YAML shapes
- A unit test (in the skill's accompanying fixture-test) confirms that a sample layout YAML with a `figma:` block parses correctly and the file URL is accessible to the skill

**Questions**:
- Should the layout YAML's `figma:` block carry just a `file_url:` field, or also include a `team_url:` and per-node Figma node IDs once the first round-trip is complete? Adding fields later requires schema-revision; pinning all expected fields now is forward-looking but speculative. Resolve during dialog authoring.

---

## Tool-call orchestration, read-back, and conflict detection

**Goal**: Pin the order and shape of Figma MCP tool calls the skill instructs Claude to perform during a Design Loop invocation, including the pre-flight vocabulary check, the write path, the read-back, and the conflict-classification logic that separates in-vocabulary changes (merge back) from out-of-vocabulary ones (refuse and warn).

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The round-trip is not a single tool call; it's a sequence of read+write+read+classify steps that have to happen in a specific order for the result state to be correct. This intent makes the exact sequence executable; the high-level loop concept (write canonical layout to Figma, read what the designer changed, classify those changes, merge them back) becomes an 8-step orchestration with named tool calls and named intermediate artifacts. The conflict detection is the load-bearing piece of the round-trip — without it, designer edits in Figma either silently overwrite the canonical layout (data loss) or silently fail to merge back (drift). The `vocabulary-validation` feature provides the classifier the conflict detector uses to decide whether a Figma edit is in-vocabulary; this intent describes how the classifier is invoked from the orchestration, not how it's implemented.

**Action**: Pin the skill's orchestration as an ordered sequence of steps, each step describing exactly what Claude is instructed to do. The steps are: (1) read the three input artifacts (page schema, layout YAML, vocabulary); (2) run pre-flight vocabulary validation against the layout — abort early if the layout itself violates the vocabulary, before any Figma call; (3) call `get_metadata` against the configured Figma file to learn current state; (4) compute the diff between the canonical layout and the current Figma state (additions, removals, modifications, designer-authored novelties); (5) apply the canonical-layout-driven changes via `use_figma`, `add_code_connect_map`, and `send_code_connect_mappings` in the order documented in the skill prompt; (6) call `get_metadata` again to read the post-write state; (7) classify each designer-authored novelty using `vocabulary-validation` — in-vocabulary edits merge back into the layout YAML; out-of-vocabulary edits become entries in `design-loop-conflicts.yaml`; (8) write the three output artifacts atomically. Each step's prompt names the exact MCP tool(s) it uses and the exact input/output shape.

**Objects**: round-trip-orchestration, pre-flight-vocabulary-check, get-metadata-current-state, diff-computation, use-figma-write-path, code-connect-bindings, get-metadata-post-write, conflict-classification, merge-back, atomic-output-write

**Constraints**:
- The skill's orchestration is exactly eight ordered steps (1–8 above); the prompt presents them in that order, and each step names its inputs, outputs, and tool calls explicitly
- Pre-flight vocabulary validation is step 2 — before any Figma tool call. A layout that fails pre-flight aborts the loop and writes a `design-loop-conflicts.yaml` documenting the validation failure; no Figma calls are made
- The write path uses only the tools in figma-mcp-client's supported subset: `use_figma` for component instantiation and layout edits, `create_new_file` only when the target Figma file doesn't yet exist, `add_code_connect_map` and `send_code_connect_mappings` for binding establishment
- The read-back (`get_metadata` in step 6) is mandatory — every loop reads back the post-write state, even if the diff in step 4 was empty
- Conflict classification (step 7) calls `vocabulary-validation` once per designer-authored novelty; the classifier's output drives the merge-back vs refuse-and-warn decision
- Output writes (step 8) are atomic (write-temp + rename) and all three outputs land in one coordinated write so a reader cannot observe a partial state
- The skill prompt does NOT instruct Claude to batch tool calls or to skip steps; the eight-step order is structural to the round-trip's correctness

**Verify**:
- The skill source contains a numbered list with at least 8 steps; each step names at least one MCP tool or one artifact path
- A grep across the skill source for the eight supported tool names (`use_figma`, `get_metadata`, `get_code_connect_map`, etc.) returns at least one match per tool
- The skill source contains zero references to the v1-excluded tools (`get_design_context`, `get_screenshot`, `generate_diagram`, `get_figjam`, `create_design_system_rules`, `search_design_system`, `generate_figma_design`) except in the explicit "excluded in v1" comment block
- An end-to-end fixture test runs the skill against a recorded Figma MCP transcript (canned tool responses) and asserts the skill calls tools in the documented order
- A unit test asserts that a layout failing pre-flight vocabulary validation results in zero Figma calls being made; the test inspects the recorded transcript for the absence of any `tools/call` MCP request

**Questions**:
- Should step 5's write order (use_figma first, then Code Connect bindings) be reversible based on what changed (e.g., binding-only changes skip use_figma entirely)? Reversibility makes the skill smarter; rigid order makes it more deterministic. Resolve during dialog authoring.

---
