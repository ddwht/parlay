# Design-loop — Infrastructure

---

## Design Loop runtime is a parlay-skill, not Studio binary code

**Affects**: feature runtime location and execution boundary
**Behavior**: The Design Loop's runtime implementation is exactly one parlay-skill — a markdown source that the operator's host agent (Claude Code today; Cursor and other catalog-listed hosts when their skill surfaces mature) executes via its LLM. The Studio binary contains no code path that performs the round-trip; it has no Figma MCP client, manages no Figma tokens, and exposes no IPC surface to the skill. The skill and Studio binary are sibling tools that collaborate exclusively through named on-disk artifacts under the feature's spec tree. The skill is invokable from any host whose tool surface includes the supported Figma MCP subset (`use_figma`, `create_new_file`, `add_code_connect_map`, `send_code_connect_mappings`, `get_metadata`, `get_code_connect_map`, `get_code_connect_suggestions`, `whoami`); excluded tools are explicitly enumerated in the skill source with their v1-exclusion rationale. The skill ships project-local for v1 at the project's deployed-skill surface; the deployment mechanism that publishes Studio-owned skills into other operator projects is reserved for a separate future feature.
**Invariants**:
- The skill source contains zero references to any Studio Go package, zero Go function calls, and zero language keywords — it is pure markdown instructions for the host agent's LLM.
- The skill source contains zero references to `mcp.figma.com`, OAuth keywords, bearer-token language, or `STUDIO_FIGMA_*` environment variables — all Figma authentication is deferred to the host agent's existing MCP connection.
- The skill's arguments section names exactly one argument: a feature reference. No Figma URL flag, no token flag, no endpoint flag.
- A grep across the skill source returns at least one match for every tool name in the supported subset and zero matches for the v1-excluded tool names outside an explicit "excluded in v1" comment block.
- The skill and Studio binary share no memory, sockets, or process-level state; their only communication channel is on-disk artifacts under the feature's spec directory.
**Source**: @design-loop/design-loop/design-loop-is-a-parlay-skill-executed-by-the-host-agent
**Backward-Compatible**: yes

**Notes**:
- The skill name resolves to `parlay-design-loop` (unadorned). A Studio-specific prefix was considered for disambiguation against a future Core skill of the same name and rejected as premature; the rename happens when (if) a Core skill ever claims the slot.
- The skill's behavior is deterministic up to the LLM's instruction-following: the same input artifacts plus the same Figma state should produce the same tool-call sequence on every invocation. The skill prompt is precise enough to constrain the LLM to one canonical path.
- The bounded tool subset is the same one figma-mcp-client originally pinned before that feature was retracted; that scoping work carries forward unchanged into the skill source.

---

## On-disk artifact contract between Studio binary and Design Loop skill

**Affects**: feature-scoped file contract and write boundary
**Behavior**: The skill reads exactly three input artifacts and writes exactly three output artifacts, all within the feature's spec directory or its referenced adapter config. The inputs are the page schema for the feature, the layout YAML referenced by the page schema (which carries a new top-level `figma:` metadata block holding the per-feature Figma file URL), and the design-system vocabulary resolved through the adapter config per `@design-loop/vocabulary-validation`. The outputs are `design-loop-result.yaml` (the read-back state of the Figma file after the round-trip, including node IDs, component identities, and a timestamp), `design-loop-conflicts.yaml` (out-of-vocabulary or out-of-spec edits detected during read-back, written only when conflicts exist), and an updated layout YAML when the read-back merges in-vocabulary designer changes back into the canonical layout. The per-feature Figma file URL lives in the layout YAML's `figma:` block because different features routinely operate on different Figma files; the URL cannot live in a global config or environment variable. All writes are atomic (write-temp + rename), coordinated across the output set so a reader cannot observe a partial state.
**Invariants**:
- The skill's input set is exactly three files; the skill makes no read against any path outside the feature's spec directory or its referenced adapter config.
- The skill's output set is exactly three artifacts under the feature's spec directory; the skill never writes outside those paths.
- `design-loop-result.yaml` is written on every successful invocation; the absence of `design-loop-conflicts.yaml` after a loop means "no conflicts on the last loop".
- The skill never modifies the page schema or the vocabulary; it only writes to its declared output set.
- File writes are atomic: a reader observing any of the output files sees a fully-written file, never a partial one.
- The layout schema declares the `figma:` block as an optional top-level key with at least a `file_url:` field; layouts without the block validate clean so read-only Domain Model Editor use stays unaffected.
- Two schema files are documented at the repo-level embedded schema surface: one for the result YAML shape and one for the conflicts YAML shape. The skill source references both by name.
**Source**: @design-loop/design-loop/on-disk-artifact-contract-between-studio-binary-and-design-loop-skill
**Backward-Compatible**: yes

**Notes**:
- The `figma:` block carries only `file_url:` for v1. A `team_url:` field and per-node Figma node IDs were considered for the initial schema and deferred: pinning speculative fields before the first round-trip produces real data forces premature schema revisions. Once `design-loop-result.yaml` actually carries node IDs from a real round-trip, a follow-up feature can extend the `figma:` block to persist them.
- The conditional layout-YAML write is the third output, distinct from the unconditional result write and the conditional conflicts write. The skill's atomic-write step coordinates whichever subset of the three the loop produced.
- The schemas at the embedded surface are framework-agnostic: they document YAML shape, not Go types or any other language's representation.

---

## Tool-call orchestration is an ordered eight-step sequence

**Affects**: skill orchestration sequence and conflict-detection contract
**Behavior**: The skill instructs the host agent's LLM through exactly eight ordered steps per invocation: (1) read the three input artifacts, (2) run pre-flight vocabulary validation against the layout — abort early with a conflicts entry and zero Figma calls if the layout itself violates the vocabulary, (3) call `get_metadata` against the configured Figma file to learn current state, (4) compute the diff between the canonical layout and the current Figma state (additions, removals, modifications, designer-authored novelties), (5) apply the canonical-layout-driven changes via `use_figma` first, then `add_code_connect_map`, then `send_code_connect_mappings` in that rigid order, (6) call `get_metadata` again to read the post-write state — mandatory on every loop, even when step 4's diff was empty, (7) classify each designer-authored novelty by invoking `@design-loop/vocabulary-validation` once per novelty (in-vocabulary edits stage for merge-back into the layout YAML; out-of-vocabulary edits stage for the conflicts YAML), (8) write the output artifacts atomically across the result, conflicts, and layout YAML files. The skill prompt presents the steps in this order; each step names its inputs, outputs, and tool calls explicitly.
**Invariants**:
- The skill source contains a numbered list with at least eight steps; each step names at least one MCP tool or one artifact path.
- A layout that fails step 2's pre-flight vocabulary validation produces zero Figma tool calls; the orchestration aborts at step 2 and writes only `design-loop-conflicts.yaml` documenting the failure.
- Step 5's write order is rigid: `use_figma` is called first (with an empty edit set when no component-layout edits exist), then `add_code_connect_map`, then `send_code_connect_mappings`. The skill does not branch on the diff to skip or reorder; reversibility is reserved for a future feature.
- Step 6's read-back is mandatory on every loop that reaches it; the skill does not skip step 6 when step 4's diff was empty.
- Step 7 calls `@design-loop/vocabulary-validation` exactly once per designer-authored novelty; the classifier's output drives the merge-back vs refuse-and-warn decision deterministically.
- The skill source contains zero references to the v1-excluded tools (`get_design_context`, `get_screenshot`, `generate_diagram`, `get_figjam`, `create_design_system_rules`, `search_design_system`, `generate_figma_design`) except inside the explicit "excluded in v1" comment block.
- The skill prompt does not instruct the LLM to batch tool calls or skip steps; the eight-step order is structural to the round-trip's correctness.
**Source**: @design-loop/design-loop/tool-call-orchestration-read-back-and-conflict-detection
**Backward-Compatible**: yes

**Notes**:
- Determinism is the design priority over smartness. A "smart" variant that skips `use_figma` when only bindings changed was considered and rejected for v1: rigid ordering makes fixture tests viable (same inputs produce the same recorded transcript), and the cost of an empty `use_figma` call is acceptable.
- The read-back-then-classify step pair (6 + 7) is the load-bearing piece of the round-trip — without it, designer edits in Figma would either silently overwrite the canonical layout (data loss) or silently fail to merge back (drift). The mandatory read-back exists precisely to make drift detectable.
- Failure handling outside pre-flight: a step 3 failure (Figma API down, file URL invalid) aborts with a conflicts entry and no result write, since the loop never reached Figma state. A step 5 partial failure (one `use_figma` edit fails mid-sequence) does not retry — the loop proceeds to step 6 to capture whatever state Figma ended up in, classifies normally, and the failed edit appears in the conflicts YAML. A step 6 silent-drop (a write that step 5 reported as successful but step 6 didn't observe) appears in the conflicts YAML as a separate diagnostic.

---
