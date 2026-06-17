<!--
parlay-feature: design-loop/design-loop
parlay-component: cross-cutting/on-disk-artifact-contract
-->

# Design Loop Conflicts Schema

File: `design-loop-conflicts.yaml`, written by the `parlay-design-loop` skill (see `.claude/skills/parlay-design-loop/SKILL.md`) **only when** at least one conflict is detected during the loop. A clean loop produces no conflicts file at all — the result file alone signals success.

This schema documents the YAML shape of the conflicts artifact. It is **framework-agnostic prose**: it describes field names, value kinds, and enumerations, and does NOT name any programming-language type, struct, interface, or import.

## When this file is written

The conflicts file is written when the skill stages at least one entry from any of the following sources during a loop:

- **Step 2 pre-flight failure** — the layout YAML failed `@design-loop/vocabulary-validation` before any Figma tool call. The loop aborts after writing this file; no `design-loop-result.yaml` is written.
- **Step 3 tool-call failure** — the initial `get_metadata` call returned an error. The loop stops after writing this file; no `design-loop-result.yaml` is written.
- **Step 5 tool-call failure** — a mid-sequence `use_figma`, `add_code_connect_map`, or `send_code_connect_mappings` call returned an error. The loop continues to step 6 to capture whatever state Figma ended up in, and a result file IS written alongside this conflicts file.
- **Step 6 silent drop** — a node that step 5 reported successful was NOT observed by the post-write `get_metadata` read-back. A result file is also written.
- **Step 7 out-of-vocabulary novelty** — a designer-authored Figma node failed vocabulary classification. The canonical layout is NOT updated for that node; a result file is also written.

Implementations MUST NOT write an empty conflicts file (zero entries). The presence of the file means "at least one conflict was staged"; the absence of the file means "the loop produced no conflicts."

## Top-level fields

A `design-loop-conflicts.yaml` file is a YAML mapping with the following required top-level fields:

| Field | Value kind | Description |
|---|---|---|
| `loop-timestamp` | ISO 8601 timestamp string | The moment the loop completed (or aborted). Matches the `loop-timestamp` in the same loop's `design-loop-result.yaml` when a result file was also written. A reader pairs the two artifacts by this shared timestamp. |
| `entries` | list of conflict entries | The conflicts staged during this loop. Always at least one entry — see "When this file is written" above. |

## Conflict entry shape

Each entry in the `entries:` list is a YAML mapping with at least the following fields:

| Field | Value kind | Description |
|---|---|---|
| `kind` | enum string | One of `pre-flight-vocabulary-failure`, `out-of-vocabulary-node`, `silent-drop`, `tool-call-failure`. See the kind enumeration below. |
| `reason` | string | A human-readable diagnostic explaining what happened. For tool-call failures, includes the tool name and the error message returned by the host MCP. For vocabulary failures, includes the offending component identity. |
| `figma-node-id` | string (optional) | The opaque Figma node identifier the conflict applies to, when applicable. Absent on `pre-flight-vocabulary-failure` entries (no Figma node exists yet) and on `tool-call-failure` entries that fired before any node identifier was known. |
| `canonical-layout-path` | string (optional) | A dotted or slash-separated path to the canonical-layout node the conflict applies to, when applicable. Absent on entries whose conflict has no canonical-layout home (designer-authored novelties before classification, transport-level tool failures). |

Implementations MAY include additional kind-specific fields without violating this schema (e.g. a `tool-name:` field on `tool-call-failure` entries, a `validator-rule:` field on vocabulary failures). The four fields above are the minimum required set.

## Kind enumeration

The `kind:` field on each entry takes one of exactly four values. New kinds may be added by future features, but existing entries on disk MUST remain readable under their original kind. Each kind has a fixed semantic:

| Kind | Meaning |
|---|---|
| `pre-flight-vocabulary-failure` | The layout YAML itself failed `@design-loop/vocabulary-validation` in step 2. The loop aborted before any Figma tool call. The `reason:` carries the validator's diagnostic. |
| `out-of-vocabulary-node` | A designer-authored Figma node (detected in step 4 as a novelty, classified in step 7) is not admissible under the active design-system vocabulary. The canonical layout was NOT updated for this node. The `reason:` carries the classifier's diagnostic. |
| `silent-drop` | A node that step 5 reported as successfully written was NOT observed by step 6's post-write `get_metadata` read-back. The Figma API claimed success but the read-back disagreed. The `reason:` names which step 5 tool reported success and what the read-back actually saw. |
| `tool-call-failure` | A Figma MCP tool call (`get_metadata`, `use_figma`, `add_code_connect_map`, or `send_code_connect_mappings`) returned an error. The `reason:` includes the tool name and the error message. Step 3 failures abort the loop; step 5 failures do NOT retry and the skill proceeds to step 6. |

## Atomic write coordination

Step 8 of the skill writes this file via the write-temp + rename pattern (write-then-rename) coordinated with the result file and any updated layout YAML. A reader cannot observe a conflicts file mid-write. If the rename fails, the previously-committed conflicts file (if any) remains intact.

## Reader pairing rule

A reader processing a loop's outputs MUST consult BOTH the result file (for the read-back node list) and the conflicts file (for any staged conflicts) when they exist at the same `loop-timestamp`. Acting on the result alone — without consulting the conflicts file — can lead to treating a silent-drop or tool-call-failure as a clean success. The two files are paired outputs of a single loop; reading them together is the contract.
