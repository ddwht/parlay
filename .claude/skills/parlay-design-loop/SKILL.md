---
name: parlay-design-loop
description: "Parlay: Run the Design Loop round-trip for a feature — push canonical layout to Figma via the host agent's MCP connection, read back, classify designer edits, write results to disk"
---

<!--
parlay-feature: design-loop/design-loop
parlay-component: cross-cutting/design-loop-is-a-parlay-skill
parlay-extends: design-loop/design-loop/cross-cutting/tool-call-orchestration
-->

# Parlay Design Loop

This skill performs the Parlay Studio Design Loop round-trip for a single feature. The host agent (Claude Code) runs the skill against its existing Figma MCP connection: read the canonical layout YAML for the feature, push the canonical changes into Figma, read the post-write state back, classify any designer-authored novelties through the vocabulary validator, and write the loop result and any conflicts to disk as YAML artifacts.

The skill is a markdown instruction set for the host agent's LLM. It contains no executable code. All Figma authentication is handled by the host agent's pre-existing MCP connection — the skill never names a Figma file URL flag, a token flag, an authentication credential, or any environment variable; the only configuration the skill consumes is the per-feature Figma file URL that lives in the layout YAML's `figma:` block.

The eight steps below are an ordered sequence with rigid write order in step 5 and a mandatory post-write read-back in step 6. The skill does NOT group tool calls, does NOT bypass steps, and does NOT re-sequence them. Determinism over smartness — the same inputs against the same Figma state should produce the same tool-call sequence on every invocation so fixture tests over recorded MCP transcripts remain viable.

## Arguments

The skill takes exactly ONE argument:

- A **feature reference** in the form `@<initiative>/<feature>` (e.g. `@design-loop/design-loop`). This resolves to a directory under `spec/intents/<initiative>/<feature>/` containing the feature's page schema, layout YAML, and intent files.

No other arguments are supported. There is no Figma URL flag, no Figma token flag, no endpoint flag, no environment variable. The per-feature Figma file URL is carried inside the layout YAML's `figma:` block — the skill reads it from there.

## Inputs

The skill consumes three input artifacts, all resolved from the feature reference:

1. The feature's **page schema** — `spec/intents/<feature>/<page>.page.md`, which names the layout YAML for the page.
2. The feature's **layout YAML** — the file the page schema points at, including its top-level `figma:` block (whose `file_url:` field is the per-feature Figma file URL the loop targets).
3. The active adapter's **design-system vocabulary** — resolved through the adapter config per `@design-loop/vocabulary-validation`. The vocabulary is the source of truth for which component identities are admissible in the canonical layout and for the read-back classifier in step 7.

## Outputs

The skill produces three output artifacts:

1. `design-loop-result.yaml` — written on every successful loop. Carries the loop timestamp, the Figma file URL the loop targeted, and the read-back node list. Shape documented in `core/internal/embedded/schemas/design-loop-result.schema.md`.
2. `design-loop-conflicts.yaml` — written ONLY when at least one conflict is detected (pre-flight vocabulary failure, out-of-vocabulary designer node, silent-drop after the read-back, or tool-call failure). Shape documented in `core/internal/embedded/schemas/design-loop-conflicts.schema.md`.
3. The **updated layout YAML** — written ONLY when at least one in-vocabulary designer-authored novelty was classified for merge-back into the canonical layout.

All three outputs are written **atomically** using the write-temp + rename pattern (write-then-rename), coordinated across the artifacts so a reader cannot observe a partial state. The skill writes every staged output to a sibling temporary file first, then renames each one into place. If any rename fails, the skill leaves the previously-committed state intact and records the failure as a conflict entry.

## Steps

The skill performs the round-trip as the following ordered eight-step sequence. Each step names its inputs, outputs, and the MCP tools it invokes.

1. **Read inputs.**
   - Inputs: the single feature reference argument.
   - The skill reads the three input artifacts: the page schema (`<page>.page.md`), the layout YAML it references (including its `figma:` block), and the design-system vocabulary resolved through the adapter config per `@design-loop/vocabulary-validation`.
   - MCP tools used: none.
   - Outputs: in-memory representations of the three inputs; in particular, the layout YAML's `figma:` block contributes the per-feature `file_url:` that drives steps 3 and 5.

2. **Pre-flight vocabulary validation.**
   - Run `@design-loop/vocabulary-validation` against the layout YAML.
   - If the layout itself violates the vocabulary, **abort** early: write only `design-loop-conflicts.yaml` documenting the validation failure (`kind: pre-flight-vocabulary-failure`), make zero Figma tool calls, and exit. Steps 3-7 are skipped on pre-flight failure. No `design-loop-result.yaml` is written because the loop never reached Figma.
   - MCP tools used: none.

3. **Read current Figma state.**
   - Call `get_metadata` against the Figma file URL from the layout's `figma:` block. Capture the current node hierarchy, component identities, and bindings.
   - If `get_metadata` returns an error (Figma API down, file URL invalid, host MCP not configured), the skill stops; no writes to Figma have happened, so no rollback is needed. Write a `design-loop-conflicts.yaml` entry of `kind: tool-call-failure` naming the tool and the error. No `design-loop-result.yaml` is written.
   - MCP tools used: `get_metadata`.

4. **Compute the canonical-vs-Figma diff.**
   - Compare the canonical layout YAML against the current Figma state captured in step 3. Classify into four buckets: additions (layout has, Figma doesn't), removals (Figma has, layout doesn't, AND the node is one the loop previously created), modifications (both have, properties differ), and designer-authored novelties (Figma has, layout doesn't, AND the node is NOT one the loop previously created).
   - MCP tools used: none.

5. **Apply canonical-layout changes to Figma. RIGID ORDER.**
   - The write order is rigid and the skill MUST NOT branch on the diff to omit or re-sequence these calls:
     1. `use_figma` — apply the component instantiation and layout edits derived from step 4's additions and modifications. **Even when step 4 produced zero edits, the skill calls `use_figma` with an empty edit set rather than skipping.** Determinism over smartness.
     2. `add_code_connect_map` — establish Code Connect bindings for any new components introduced in this loop.
     3. `send_code_connect_mappings` — push the bindings to Figma.
   - A `use_figma` mid-sequence failure does NOT retry. The skill proceeds to step 6 to capture whatever state Figma ended up in; the failed edit appears in `design-loop-conflicts.yaml` as a `kind: tool-call-failure` entry.
   - MCP tools used: `use_figma`, `add_code_connect_map`, `send_code_connect_mappings`.

6. **Read post-write state. MANDATORY ON EVERY LOOP.**
   - Call `get_metadata` again. The skill does NOT omit this step when step 4's diff was empty — the read-back is what makes drift detectable, and it runs even if step 5 had nothing material to apply. The mandatory read-back fires on every loop that reaches it, even when the diff was empty.
   - Compare the post-write state to the expected state computed from steps 4+5. Any node that step 5 reported as successful but step 6 didn't observe is recorded as a `kind: silent-drop` (silent drop) entry in `design-loop-conflicts.yaml`.
   - MCP tools used: `get_metadata`.

7. **Classify designer-authored novelties.**
   - For each novelty captured in step 4, invoke `@design-loop/vocabulary-validation` exactly ONCE as the classifier. In-vocabulary edits stage for merge-back into the canonical layout YAML; out-of-vocabulary edits stage for `design-loop-conflicts.yaml` as `kind: out-of-vocabulary-node` entries. The classifier's output drives the merge-back vs refuse-and-warn decision deterministically.
   - MCP tools used: none.

8. **Write outputs atomically.**
   - Write the staged outputs **atomically** using the write-temp + rename pattern, coordinated across `design-loop-result.yaml`, `design-loop-conflicts.yaml` (only if any conflicts were staged), and the updated layout YAML (only if any in-vocabulary merge-backs were staged in step 7). A reader cannot observe a partial state.
   - On a successful loop, `design-loop-result.yaml` always carries the read-back node list captured in step 6, the loop timestamp, and the Figma file URL the loop targeted.
   - MCP tools used: none.

## Supported MCP tools (v1)

The skill calls the following Figma MCP tools, all of which the host agent exposes through its pre-existing MCP connection. The skill names each tool by its literal MCP name:

- `use_figma` — apply component instantiation and layout edits to the Figma file (step 5).
- `create_new_file` — create a new Figma file when the loop targets a file URL that does not yet exist. v1 expects the file to exist; this tool is named here for completeness and is reserved for first-time bootstrap.
- `add_code_connect_map` — establish Code Connect bindings for new components (step 5).
- `send_code_connect_mappings` — push the established bindings to Figma (step 5).
- `get_metadata` — read the Figma file's current node hierarchy and component identities. Called in step 3 (pre-write capture) and again in step 6 (post-write read-back). The duplicate invocation is intentional — the read-back is what makes drift detection possible.
- `get_code_connect_map` — read the current Code Connect bindings on the Figma file (consulted opportunistically during step 4's diff computation).
- `get_code_connect_suggestions` — retrieve suggested bindings the host environment can propose for new components (consulted opportunistically during step 5b).
- `whoami` — read the host agent's identity on the Figma MCP. Used as a connectivity smoke-test the skill MAY call at the start of step 3 to surface a configuration error before any other tool call.

## Excluded in v1

The following Figma MCP tools are explicitly **excluded** in v1. The skill does NOT call them. The exclusion rationale carried forward from the figma-mcp-client scoping work is: not required for the v1 round-trip; v1 ships the smallest sufficient subset, and each excluded tool can be admitted by a separate future feature with its own scoping rationale.

- `get_design_context`
- `get_screenshot`
- `generate_diagram`
- `get_figjam`
- `create_design_system_rules`
- `search_design_system`
- `generate_figma_design`

These tool names appear in the skill source ONLY inside this Excluded-in-v1 block. Any reference to them outside this block would violate the v1 scoping contract.

## Failure handling

The skill records every tool failure as a structured entry in `design-loop-conflicts.yaml`. There is no retry logic — the determinism contract is what makes fixture tests viable, and silent retries would invalidate recorded MCP transcripts.

- **Step 3 failure**: `get_metadata` errors before any write. The skill stops immediately, writes a `kind: tool-call-failure` conflict entry naming the tool and the error, and writes no `design-loop-result.yaml`. No Figma state changed.
- **Step 5 partial failure**: a mid-sequence `use_figma` (or `add_code_connect_map`, or `send_code_connect_mappings`) error does NOT retry. The skill proceeds to step 6 to capture whatever state Figma ended up in; the failed edit appears as a `kind: tool-call-failure` conflict entry. The partial failure is intentionally surfaced rather than retried.
- **Step 6 silent drop**: a node that step 5 reported successful but step 6 didn't observe is recorded as a `kind: silent-drop` (silent drop) conflict entry. The read-back is the source of truth, not the tool's local success signal.
- **Step 7 out-of-vocabulary novelty**: a designer-authored node that fails vocabulary classification is recorded as a `kind: out-of-vocabulary-node` conflict entry. The canonical layout is NOT updated for that node — the skill refuses-and-warns rather than absorbing arbitrary designer additions.

## Determinism contract

Same inputs (page schema + layout YAML + adapter vocabulary) against the same Figma state should produce the same tool-call sequence on every invocation. This is the contract that makes fixture tests over recorded MCP transcripts viable. The rigid step order, the empty-edit-set call in step 5a, and the mandatory read-back in step 6 are all in service of this contract — the skill does not optimize calls away on the basis of locally-observed state.

## Per-feature Figma URL convention

The per-feature Figma file URL lives in the layout YAML's `figma:` block (under the `file_url:` field). It is NOT in `studio-config.yaml`, NOT in any environment variable, NOT in the page schema's root. Different features routinely operate on different Figma files; the URL is a per-feature concern, not a global one. The `figma:` block is optional on the layout schema so existing layouts without it continue to validate clean — but a layout without `figma:` cannot be the target of a design-loop run, since step 3 would have nothing to call `get_metadata` against.

## Host-agent neutrality

The skill prompt is host-agnostic in wording. Claude Code is the only host validated for v1, but the prompt speaks to "the host agent's Figma MCP tool surface" generically and avoids naming any specific host's mechanisms. Any host that exposes the supported MCP tools listed above can run this skill against its own MCP connection.
