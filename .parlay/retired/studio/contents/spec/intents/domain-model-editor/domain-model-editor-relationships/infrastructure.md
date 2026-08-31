# Domain-model-editor-relationships — Infrastructure

---

## Diagram layout state stays out of the domain model

**Affects**: presentation-state boundary for the domain model artifact; the no-sidecar-layout constraint for the diagram view; the deterministic projection of the model to a graph

**Behavior**: The ER diagram is a deterministic projection of the domain model, not a second persisted artifact kept in sync with it. Node positions and any other diagram layout state are presentation, not domain vocabulary, and are never written to the domain model file — its only sanctioned presentation metadata remains the enum label/tone pair the schema already permits. No sidecar layout file is introduced in v1: there is deliberately no on-disk home for positions. This matters because the Studio server is ephemeral — it boots on invocation and shuts down on idle — so any persisted layout would need a restart-surviving store that v1 does not add. Instead, layout is recomputed by a deterministic auto-layout on every load: the same model always yields the same initial arrangement, so a reload after save reproduces the prior arrangement rather than surprising the designer. Manual node dragging repositions nodes within the live session only; it never marks the draft dirty and is discarded on the next load. The graph is rendered through the registered Studio adapter's `domain-model-er-diagram` composition, so the rendering substrate is an adapter-pinned decision, not a fresh dependency introduced by this feature.

**Invariants**:
- Node positions and diagram layout state are never written to the domain model file (`domain-model.yaml`) under any code path; the file stays pure domain vocabulary apart from the enum label/tone presentation pair.
- No sidecar layout file is created, read, or required in v1 — the diagram has no persisted position store.
- Auto-layout is deterministic: rendering the same model twice produces identical initial node positions.
- Manual in-session node repositioning does not mark the draft dirty and does not survive a reload.
- The diagram is a read-projection of the model for layout purposes; the only mutation path from the diagram is the draw-to-connect gesture, which enters nothing into the draft until the pre-filled relationship form is committed.

**Source**: @domain-model-editor-relationships/er-diagram-view-with-draw-to-connect

**Caching**: none

**Backward-Compatible**: yes

**Notes**:
- If persisted positions are ever warranted — large models where deterministic auto-layout proves insufficient — the layout store would live under the tool-internals area, never in the domain model file. Deferred until real models prove the need; this is the one open question carried from the intent.
- The persistence, serialization, declaration-order, and deprecated-operations-passthrough guarantees this feature relies on are already pinned by the domain-model-editor-mvp infrastructure (compare-and-swap save, deterministic serialization, operations passthrough); relationship editing and the diagram reuse them and introduce no new persistence path.

---
