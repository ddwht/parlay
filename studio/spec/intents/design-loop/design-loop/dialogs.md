# Design-loop — Dialogs

---

### Design Loop is a parlay-skill executed by the host agent

**Trigger**: A Parlay Studio maintainer (the designer, working at their terminal in Claude Code attached to this repo) needs to perform the Figma round-trip for a feature. They invoke `/parlay-design-loop @<initiative>/<feature>` and the host agent walks the skill's instructions.

User: /parlay-design-loop @design-loop/design-loop
System (background): Loads `.claude/skills/parlay-design-loop/SKILL.md` and reads its instructions.
System: Starting Design Loop for `@design-loop/design-loop`. I'll perform the round-trip via the host's Figma MCP tools — Studio's binary is not involved in this run.
System (background): Reads `spec/intents/design-loop/design-loop/<page>.page.md`, the referenced layout YAML, and the design-system vocabulary resolved through the adapter config.
System: Inputs loaded: 1 page schema, 1 layout YAML (vocabulary `==componentVocabulary==`), 1 figma file URL from the layout's `figma:` block.

#### Branch: Skill name resolves to `parlay-design-loop` (Q1.1 resolution)

User: What's the skill called — `parlay-design-loop` or `parlay-studio-design-loop`?
System: `parlay-design-loop` (unadorned). No Core skill named `design-loop` exists or is planned; the studio prefix would be premature disambiguation. If a Core skill ever needs the name, the rename happens then, not now.

#### Branch: Host agent is not Claude Code

User: I'm running this from Cursor — will the skill work?
System: The skill is invokable from any host whose tool surface includes the supported Figma MCP subset (`use_figma`, `create_new_file`, `add_code_connect_map`, `send_code_connect_mappings`, `get_metadata`, `get_code_connect_map`, `get_code_connect_suggestions`, `whoami`). Cursor exposes Figma MCP tools, so the skill should work; the skill's prompt is host-agnostic by design. Claude Code is the only host validated for v1.

#### Branch: Studio binary is running concurrently

User: My Studio binary is open in another tab serving the Domain Model Editor. Will the skill conflict with it?
System: No. The skill and Studio binary share no memory, sockets, or process state — they communicate only via on-disk artifacts under `spec/intents/<feature>/`. The skill writes outputs atomically (write-temp + rename), so Studio binary cannot observe a partial state. You can run them concurrently.

#### Branch: Operator asks for the skill to authenticate against Figma directly

User: Can the skill log in to Figma for me so I don't need Claude Code's MCP set up?
System: No — by design. The skill defers all authentication to the host agent's existing Figma MCP connection. It does not manage tokens, does not call OAuth endpoints, does not import any MCP client SDK. If Figma MCP is not configured in your host agent, configure it there first; the skill cannot work around that.

---

### On-disk artifact contract between Studio binary and Design Loop skill

**Trigger**: The skill needs to declare exactly which files it reads and writes. This dialog covers the artifact set's shape as the designer sees it on disk after a successful loop, and the conditional layout-merge-back path.

User: /parlay-design-loop @design-loop/design-loop
System (background): Reads three inputs: the page schema, the layout YAML (including its `figma:` block), and the design-system vocabulary resolved via the adapter config per `@design-loop/vocabulary-validation`.
System: Reading 3 input artifacts: page schema, layout YAML, vocabulary. The Figma file URL is `==file_url==` from the layout's `figma:` block.
System (background): Performs the round-trip (see the orchestration dialog) and prepares output artifacts.
System: Writing outputs atomically (write-temp + rename across all three):
  - `design-loop-result.yaml` — read-back state of the Figma file (node IDs, component identities, timestamp)
  - `design-loop-conflicts.yaml` — only if out-of-vocabulary edits were detected
  - layout YAML — only if in-vocabulary designer changes were merged back
System: Loop complete. Artifacts under `spec/intents/design-loop/design-loop/`.

#### Branch: `figma:` block shape resolves to `file_url:` only for v1 (Q2.1 resolution)

User: What fields go in the layout's `figma:` block?
System: For v1, just `file_url:`. The `team_url:` and per-node Figma node IDs are speculative until the first round-trip writes them; pinning them now would force schema-revisions before any real data exists. Once the read-back actually produces node IDs in `design-loop-result.yaml`, a follow-up feature can extend the `figma:` block to carry them. v1's schema makes `figma:` optional so existing layouts without it validate clean (Domain Model Editor read-only use stays unaffected).

#### Branch: Read-back finds zero designer changes

User: I haven't touched the Figma file since the last loop. What does the skill write?
System (condition: no diff and no novelties): Writes only `design-loop-result.yaml` (with the current node IDs and a fresh timestamp). No `design-loop-conflicts.yaml`. No layout YAML update. The absence of conflicts.yaml means "no conflicts on the last loop" — that's the steady-state signal.

#### Branch: Read-back finds in-vocabulary designer changes

User: I added a new `clarity@17/Button` instance in Figma. What happens to it?
System (condition: novelty classifies as in-vocabulary): The skill merges the new node into the canonical layout YAML and writes the updated YAML alongside `design-loop-result.yaml`. The layout's typed tree now reflects your change. No conflicts entry is written for it.

#### Branch: Read-back finds out-of-vocabulary designer changes

User: I added a hand-drawn shape that's not in the clarity vocabulary. What happens?
System (condition: novelty classifies as out-of-vocabulary): The skill refuses to merge it. It writes `design-loop-conflicts.yaml` with an entry naming the offending node, its type, and why the classifier rejected it. The canonical layout YAML is not modified for that node. Your Figma file still has the shape — the skill never deletes from Figma — but on the next loop the canonical layout will re-assert the in-vocabulary state.

#### Branch: Operator asks for the file URL to live in studio-config or env

User: Can I put the Figma file URL in `studio-config.yaml` instead so all my features share it?
System: No — the URL is per-feature. Different features routinely edit different Figma files (a settings page and a checkout page don't share a canvas). studio-config is a global concern; the layout's `figma:` block is the per-feature concern. The skill reads exclusively from there.

#### Branch: Operator asks for outputs outside the feature directory

User: Can the skill write `design-loop-result.yaml` to a top-level `loop-results/` directory so I can collect results across features?
System: No. The skill never writes outside `spec/intents/<feature>/`. If you need cross-feature reporting, build a separate tool that reads the per-feature outputs; the skill's contract is locked to the feature tree.

---

### Tool-call orchestration, read-back, and conflict detection

**Trigger**: The skill walks Claude through the 8-step round-trip every invocation. This dialog captures the canonical happy path and the branches where steps abort or behave conditionally.

User: /parlay-design-loop @design-loop/design-loop
System (background): Step 1 — reads page schema, layout YAML, vocabulary.
System: Step 1/8 complete: inputs loaded.
System (background): Step 2 — runs pre-flight vocabulary validation against the layout YAML.
System (condition: pre-flight passes): Step 2/8 complete: layout is valid against `==componentVocabulary==`.
System (background): Step 3 — calls `get_metadata` against the Figma file from the layout's `figma:` block to learn current state.
System: Step 3/8 complete: Figma current state captured.
System (background): Step 4 — computes the diff between the canonical layout and current Figma state (additions, removals, modifications, designer-authored novelties).
System: Step 4/8 complete: diff has `==n_add==` additions, `==n_remove==` removals, `==n_modify==` modifications, `==n_novelty==` designer novelties.
System (background): Step 5 — applies canonical-layout changes via `use_figma` (component instantiation and layout edits), then `add_code_connect_map`, then `send_code_connect_mappings` for binding establishment. Write order is rigid: use_figma first, then bindings.
System: Step 5/8 complete: canonical changes pushed to Figma.
System (background): Step 6 — calls `get_metadata` again to read the post-write state.
System: Step 6/8 complete: post-write state captured.
System (background): Step 7 — classifies each designer-authored novelty via `@design-loop/vocabulary-validation`. In-vocabulary edits stage for merge-back; out-of-vocabulary edits stage for conflicts.yaml.
System: Step 7/8 complete: `==n_in_vocab==` merge-back, `==n_out_vocab==` conflicts.
System (background): Step 8 — writes all output artifacts atomically (write-temp + rename, coordinated across the result, conflicts, and layout YAML files).
System: Step 8/8 complete. Design Loop finished.

#### Branch: Step 5 write order is rigid (Q3.1 resolution)

User: For a loop where only Code Connect bindings changed and no component layout changed, can the skill skip `use_figma` and go straight to `add_code_connect_map`?
System: No — for v1. Step 5's write order is rigid: `use_figma` always runs first, then `add_code_connect_map`, then `send_code_connect_mappings`. Even when `use_figma` has no edits to apply, the skill calls it with an empty edit set rather than skipping. Determinism over smartness: the same input shape produces the same tool-call sequence every invocation, which is what makes fixture tests viable. A future feature can introduce reversibility once the rigid path is stable.

#### Branch: Pre-flight vocabulary validation fails (step 2 aborts)

User: My layout YAML has a node with type `floating-action-bar` that's not in clarity@17.
System (condition: pre-flight fails): Step 2/8 aborts. The skill makes ZERO Figma calls (steps 3–6 are skipped). It writes `design-loop-conflicts.yaml` documenting the validation failure: the offending node, its declared type, and the resolved vocabulary's available types. The canonical layout YAML is not modified. No `design-loop-result.yaml` is written (the loop didn't reach Figma, so there's no read-back state to record).
System: Pre-flight failed. Fix the layout YAML and re-run, or update the vocabulary if `floating-action-bar` should be added.

#### Branch: Figma MCP is not configured in the host agent

User: /parlay-design-loop @design-loop/design-loop
System (background): Step 3 — attempts `get_metadata`.
System (condition: host agent has no Figma MCP tools registered): The host agent reports no `get_metadata` tool available. The skill stops at step 3, before any state mutation. It writes nothing to disk — no result.yaml, no conflicts.yaml — and reports the failure verbatim.
System: Cannot proceed: Figma MCP is not configured in your host agent. Configure Figma MCP in Claude Code (or your active host) first, then re-run. The skill defers all Figma authentication to the host; it cannot work around a missing host MCP configuration.

#### Branch: Step 3 `get_metadata` fails (Figma API down, file URL invalid)

User: /parlay-design-loop @design-loop/design-loop
System (background): Step 3 — calls `get_metadata`.
System (condition: get_metadata returns an error): Step 3/8 fails. The skill stops; it has not made any writes to Figma yet, so there is no rollback to perform. It writes `design-loop-conflicts.yaml` with a single entry capturing the tool's error and the file URL it tried. No `design-loop-result.yaml`.
System: Loop aborted at step 3. Check the file URL in the layout's `figma:` block, and check whether Figma's MCP server is reachable.

#### Branch: Step 5 write partially succeeds (a `use_figma` call fails mid-sequence)

User: /parlay-design-loop @design-loop/design-loop
System (background): Step 5 — applying canonical changes via `use_figma`.
System (condition: use_figma fails on the Nth edit): Step 5/8 partially failed. The skill does not retry. It proceeds to step 6 to read back whatever state Figma ended up in (so the operator can see what landed), then to step 7 to classify, then writes outputs. The `design-loop-result.yaml` reflects the partial post-write state; `design-loop-conflicts.yaml` includes an entry naming the failed edit and the tool's error.
System: Loop completed with a step 5 partial failure. Review `design-loop-conflicts.yaml` to see which edit failed and re-run after fixing.

#### Branch: Read-back diff in step 6 reveals Figma rejected a write (silent drop)

User: /parlay-design-loop @design-loop/design-loop
System (background): Step 6 — `get_metadata` post-write. Compares to expected post-write state computed from steps 4+5.
System (condition: post-write state lacks an expected node): The skill detects the silent drop and writes an entry to `design-loop-conflicts.yaml` naming the expected node and noting that step 5 reported success but step 6 didn't observe it. The canonical layout is unchanged; the operator decides whether to retry.
System: Detected a write that step 5 reported as successful but step 6 didn't observe. See `design-loop-conflicts.yaml`.

---
