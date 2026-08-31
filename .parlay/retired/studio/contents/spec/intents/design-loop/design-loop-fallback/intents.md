# Design-loop-fallback

> The Design Loop has a fallback path for cases where the canonical-layout-driven round-trip doesn't preserve fidelity — component identity drifts, designer edits keep getting refused, or Figma's tooling can't represent some part of the vocabulary. In fallback mode, designers author layouts directly in Figma using the design system's pre-built Code Connect library, and Studio reads the result back into the canonical layout YAML. The mode is read-only on Studio's side: Studio does not drive Figma; designers do. This feature pins when fallback applies, what the read-only sync path does, and how to exit fallback once round-trip viability is re-established.

---

## Fallback mode — when and how to enter

**Goal**: Pin the criteria for switching a feature into fallback mode and the explicit-action gate that makes the switch a designer decision, not an automatic degradation. Fallback is a deliberate scope reduction (no canonical-layout-driven writes); it should be entered intentionally, not silently.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: The canonical-layout-driven round-trip is the primary Design Loop mode. It's also the most ambitious — it requires Figma MCP's write tools to faithfully instantiate Studio's typed-tree layout, and the read-back to faithfully classify designer edits. Reality may surface cases where this doesn't work: a vocabulary entry that Figma MCP can't render correctly, a component family whose variants don't round-trip through `use_figma`, a Figma file that pre-dates Studio's canonical layouts. For those cases, the designer needs an exit hatch: author the layout directly in Figma using Clarity's pre-built Code Connect library, then have Studio read it back. Auto-switching into fallback on read-back failure would be insidious (silent quality degradation); making the switch a deliberate designer decision keeps the architectural intent clear.

**Action**: Define a per-feature `design-loop-mode:` field on the page schema (or on the layout YAML) with two values: `round-trip` (default) and `fallback`. The Design Loop skill reads this field at startup and dispatches to the appropriate mode. Switching from `round-trip` to `fallback` is a manual spec edit by the designer (or by whoever authors the page); switching back is the same. The skill itself does not switch modes; it executes whichever mode the field declares. A `design-loop-mode-switched.yaml` artifact records every mode-switch event with timestamp and reason (designer-supplied free text) so the project history is auditable.

**Objects**: design-loop-mode-field, round-trip-default-mode, fallback-mode, manual-mode-switch, mode-switch-audit-log

**Constraints**:
- The Design Loop's mode is declared per-feature in a `design-loop-mode:` field; valid values are `round-trip` (default) and `fallback`
- Mode switches are manual spec edits — the Design Loop skill does NOT change the field's value under any condition (including read-back failure, vocabulary errors, Figma MCP errors, or any other runtime state)
- Switching from round-trip to fallback (or vice versa) writes an entry to `design-loop-mode-switched.yaml` with `{timestamp, from, to, reason}`; the reason is a designer-supplied free-text string supplied via the spec edit's commit message or a dedicated arg
- A feature in fallback mode is documented in its page schema as such; downstream tooling (Domain Model Editor, etc.) can read the field and adjust UX accordingly (e.g. show a "fallback mode" indicator in the editor sidebar)
- The skill's behavior in each mode is described in the sibling intents (this intent + the read-only sync intent below + the round-trip orchestration in `@design-loop/design-loop`)
- Re-entering round-trip mode after fallback is a deliberate decision that requires the layout YAML to pass vocabulary validation and reflect the current Figma file's state; the skill asserts this on the next round-trip invocation and fails if the assertion doesn't hold

**Verify**:
- The page schema (or layout YAML, whichever holds the field per Q on the contract intent) is extended to declare a `design-loop-mode:` field; existing pages without the field validate clean (default `round-trip` applies)
- A unit test passes a page with `design-loop-mode: fallback` and asserts the skill dispatches to the read-only sync path
- A unit test passes a page with no field and asserts the skill defaults to `round-trip`
- A unit test asserts the skill does not modify the `design-loop-mode:` field under any input, including a deliberately-broken round-trip scenario
- A grep across the skill source for "mode switch" or "auto-fallback" returns zero matches — the skill explicitly does not auto-switch

**Questions**:
- Should the `design-loop-mode:` field live on the page schema (where layout-driving fields already live) or on the layout YAML's `figma:` block (where Figma-specific configuration already lives)? Page schema is more visible; layout YAML keeps Figma-related state co-located. Resolve during dialog authoring.

---

## Read-only sync path

**Goal**: Pin what the Design Loop skill does in fallback mode. The mode is read-only: the skill never calls Figma MCP write tools, never drives the canvas, never instantiates components. It reads the current Figma file state via `get_metadata` + `get_code_connect_map`, constructs the canonical layout YAML from the read state, validates against the vocabulary, and writes the layout back to disk.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: In fallback mode, the designer is the authoring authority. They open the Figma file directly, use Clarity's pre-built Code Connect-mapped library, and produce the layout. Studio's role becomes: read what the designer made, validate it fits the vocabulary, and persist it as canonical layout YAML so downstream Studio tooling (Domain Model Editor, codegen) can consume it. The read-only sync is structurally simpler than the round-trip — fewer tool calls, no diff computation, no write-then-read sequence. It's also structurally less complete: any designer edit that escapes the vocabulary is reported but not corrected (Studio can't "fix" the designer's Figma file from this mode).

**Action**: Pin the skill's fallback-mode orchestration as a four-step sequence: (1) read the page schema and layout-mode field; (2) call `get_metadata` and `get_code_connect_map` against the configured Figma file; (3) construct a candidate layout YAML from the read state, mapping Code Connect bindings to vocabulary components; (4) run `vocabulary-validation` on the candidate; if it passes, write the layout YAML; if it fails, write `design-loop-conflicts.yaml` with the validation errors and DO NOT overwrite the existing layout YAML (preserve the previous state). The skill never calls `use_figma`, `create_new_file`, `add_code_connect_map`, or `send_code_connect_mappings` in fallback mode.

**Objects**: fallback-orchestration, read-only-tool-subset, candidate-layout-construction, code-connect-to-vocabulary-mapping, preserve-previous-layout-on-validation-failure

**Constraints**:
- The fallback skill orchestration is exactly four ordered steps; each step names its inputs, outputs, and tool calls explicitly in the skill prompt
- The skill calls only the read tools in fallback mode: `get_metadata`, `get_code_connect_map`, `whoami` (for connection verification). Write tools (`use_figma`, `create_new_file`, `add_code_connect_map`, `send_code_connect_mappings`) are NOT called; the skill prompt explicitly lists them as forbidden in fallback mode
- Candidate layout construction maps Figma node identities to vocabulary components via Code Connect; nodes without a Code Connect binding produce a conflict entry but do not block construction of the rest of the layout
- Vocabulary validation runs on the candidate layout before any write; a validation failure preserves the previous layout YAML on disk and writes only the conflicts file
- A successful sync writes the new layout YAML atomically (write-temp + rename); the previous layout YAML's content is replaced in one operation, no partial state observable
- The skill emits a structured log line at the end of each fallback invocation: `{mode: fallback, status: success|conflicts, nodes_read: N, conflicts: M, layout_written: bool}`

**Verify**:
- The skill source contains a `mode: fallback` branch with exactly four steps; each step names its tool calls or artifact paths
- A grep across the skill's `mode: fallback` section for write tools (`use_figma`, `create_new_file`, `add_code_connect_map`, `send_code_connect_mappings`) returns zero matches; a grep for the read tools returns matches
- A unit test invokes the skill in fallback mode with a recorded Figma transcript and asserts only read tools are called
- A unit test invokes the skill in fallback mode with a Figma state that fails vocabulary validation and asserts the previous layout YAML on disk is unchanged
- A unit test invokes the skill in fallback mode with a clean Figma state and asserts the layout YAML is replaced atomically

**Questions**:
- Should fallback mode produce `design-loop-result.yaml` in the same shape as round-trip mode, or a fallback-specific result shape? Same shape is simpler for downstream consumers; different shape captures the mode-distinction explicitly. Resolve during dialog authoring.

---
