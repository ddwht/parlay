<!--
parlay-feature: design-loop/design-loop
parlay-component: cross-cutting/on-disk-artifact-contract
-->

# Design Loop Result Schema

File: `design-loop-result.yaml`, written by the `parlay-design-loop` skill (see `.claude/skills/parlay-design-loop/SKILL.md`) on every successful loop. The file is the **unconditional** output of the round-trip — it is written even when the canonical-vs-Figma diff was empty, because the read-back in step 6 always produces a node list to record.

This schema documents the YAML shape of the result artifact. It is **framework-agnostic prose**: it describes field names, value kinds, and structural relationships, and does NOT name any programming-language type, struct, interface, or import.

## Top-level fields

A `design-loop-result.yaml` file is a YAML mapping with the following required top-level fields:

| Field | Value kind | Description |
|---|---|---|
| `loop-timestamp` | ISO 8601 timestamp string | The moment the loop completed (after step 8's atomic write commit). On a single loop run, this timestamp matches the `loop-timestamp` in the same loop's `design-loop-conflicts.yaml` (when one exists) so a downstream reader can pair the two artifacts. |
| `figma-file-url` | URL string | The Figma file URL the loop targeted, copied from the layout YAML's `figma:` block (`file_url:` field). Recording it in the result lets a reader correlate the result to the canonical layout that drove the loop without re-reading the layout YAML. |
| `nodes` | list of node entries | The read-back node list captured by step 6's `get_metadata` call. May be empty if the Figma file was empty at read-back time, but the key is always present. |

## Node entry shape

Each entry in the `nodes:` list is a YAML mapping with at least the following fields:

| Field | Value kind | Description |
|---|---|---|
| `figma-node-id` | string | The opaque Figma node identifier returned by `get_metadata`. Stable across loops as long as the designer does not delete-and-recreate the node. |
| `component-identity` | string | The design-system vocabulary's component name for this node (e.g. `clarity.button`, `clarity.region`). Resolved through the adapter's `componentVocabulary` at classification time. For designer-authored nodes that did not classify, the value is `unclassified` and the corresponding conflict entry in `design-loop-conflicts.yaml` carries the diagnostic. |
| `canonical-layout-path` | string | A dotted or slash-separated path linking the Figma node back to its position in the canonical layout tree (e.g. `root.region[0].button[2]`). Empty string for designer-authored novelties that have no canonical-layout home. |

Implementations MAY include additional fields on each node entry (e.g. per-node bindings, presentation hints, instance properties) without violating this schema. The three fields above are the minimum required set.

## Empty-diff result

When step 4 produced zero edits and step 6's read-back observed the same node hierarchy as step 3's pre-write capture, the loop is empty-diff. The result file is still written with:

- `loop-timestamp` and `figma-file-url` as usual.
- `nodes:` reflecting the (unchanged) node hierarchy from the read-back.

There is no special "empty" marker. The presence of an empty-diff result is what tells a downstream reader that the loop ran successfully without any material changes — distinct from "the loop did not run" (no file) or "the loop failed" (`design-loop-conflicts.yaml` carries a `kind: tool-call-failure` entry).

## Relationship to the conflicts file

The result file and `design-loop-conflicts.yaml` are paired by their shared `loop-timestamp`. The result file is written on every successful loop; the conflicts file is written ONLY when at least one conflict was staged during steps 2-7. A reader observing a result without a sibling conflicts file at the same timestamp can conclude the loop completed cleanly.

When step 2 (pre-flight) aborts the loop, only `design-loop-conflicts.yaml` is written and the result file is absent — the loop never reached Figma, so there is no read-back to record.

## Atomic write coordination

Step 8 of the skill writes this file via the write-temp + rename pattern (write-then-rename) coordinated with the conflicts file and any updated layout YAML. A reader cannot observe a result file mid-write. If the rename fails, the previously-committed result (if any) remains intact and the failure is itself recorded in the next loop's conflicts file.

## Versioning

No `schema_version:` field (see `schema-versioning.schema.md` for the house rule) — deferred, and for a different reason than the adapter/blueprint deferrals above: this artifact is ephemeral by design. It's written fresh by every loop run, correlated to its sibling conflicts file by `loop-timestamp` rather than persisted and re-read across binary upgrades. There's no cross-version compatibility concern to version against — a reader always consumes the result from the loop that just ran, produced by the binary that's running now.
