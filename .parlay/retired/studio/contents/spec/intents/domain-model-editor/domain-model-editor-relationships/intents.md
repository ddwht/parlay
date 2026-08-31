# Domain-model-editor-relationships

> Depth features for the domain-model editor: relationship editing and the visual ER diagram. Builds directly on domain-model-editor-mvp's subsystem, persistence, and UI shell — this feature adds the `relationships:` collection to the editable surface and gives the model a graph view (entities as nodes, relationships as edges) with draw-to-connect creation. There is deliberately no operation editor: `domain-model.operations` is deprecated in favor of per-feature `capabilities.yaml`, and this feature pins the editor's read-only handling of that deprecated field instead.

---

## Relationship editing

**Goal**: Let a designer declare, edit, and delete relationships between entities — the schema's construct for entity-to-entity references beyond a single `ref` field, and the only way to express one-to-many and many-to-many links — through the same closed-vocabulary form discipline the MVP established for fields and enums.

**Persona**: Product designer

**Priority**: P1

**Context**: A `DomainRelationship` is a `name`, `from`, `to`, and a `cardinality` from the closed set `{one-to-one, one-to-many, many-to-one, many-to-many}`. Both endpoints must resolve to declared entities (`undeclared-entity-reference` on deep validation) and unknown cardinalities fail with `relationship-cardinality-unknown` — both unrepresentable from a form with entity pickers and a closed dropdown. The MVP already made entity rename propagate into relationship endpoints and made entity delete block on relationship references, so this intent's job is the relationship collection itself plus its own consistency rules. Note the schema's v2-deferral context: a list of entity references (e.g. `Task.watchers` → several `User`s) is expressible today only as a relationship, so the relationship form is also the editor's answer when a designer reaches for a "list of X" field — the field editor can point them here.

**Action**: The editor UI gains a relationships panel: a list of declared relationships and a form with a `from` entity picker, a `to` entity picker, a cardinality dropdown over the closed set, and a name field pre-filled from the endpoints (e.g. `customer-orders` from `Customer` → `Order`) but freely editable. Relationships have no dependents in the model, so deletion is immediate. Self-referential relationships (`from` = `to`, e.g. an org hierarchy) are legal and supported by the form.

**Objects**: relationships-panel, relationship-form, from-entity-picker, to-entity-picker, cardinality-dropdown, relationship-name-prefill, self-referential-relationship

**Constraints**:
- The `from` and `to` pickers offer exactly the declared entities; free-text endpoint entry does not exist, so `undeclared-entity-reference` is unrepresentable from the editor
- The cardinality dropdown offers exactly `{one-to-one, one-to-many, many-to-one, many-to-many}`, so `relationship-cardinality-unknown` is unrepresentable from the editor
- Relationship names are unique within the model; a duplicate is rejected at entry with a field-level message
- The name pre-fill is a convenience, not a constraint — the designer can rename freely, and editing endpoints after a manual rename does not regenerate the name
- `from` = `to` is accepted (self-referential relationships are legal); the form does not special-case or block it
- Deleting a relationship succeeds immediately — nothing in the model references a relationship by name
- Relationship edits operate on the in-memory draft and flow through the MVP's compare-and-swap save; declaration order is preserved to the file as edited

**Verify**:
- A test creates `Customer` → `Order` with cardinality `one-to-many`, asserts the serialized YAML matches the schema's documented relationship shape, and asserts the pre-filled name `customer-orders` was offered
- A test asserts the endpoint pickers for a three-entity model offer exactly those three entities, and the cardinality dropdown exactly the four closed values
- A test creates a self-referential relationship `Employee` → `Employee` (`many-to-one`) and asserts it round-trips through save without warning or mangling
- A test creates a second relationship with an already-used name and asserts a field-level duplicate rejection
- A test renames entity `Order` to `Invoice` (MVP rename propagation) and asserts existing relationship endpoints follow while manually-set relationship names stay untouched
- A test deletes a relationship and asserts immediate success and its absence from the serialized file

---

## ER diagram view with draw-to-connect

**Goal**: Give the designer a visual graph of the whole model — entities as nodes showing their fields, relationships as labeled edges — where drawing a connection between two nodes is the gesture that creates a relationship.

**Persona**: Product designer

**Priority**: P1

**Context**: The registered studio adapter already pins the composition for exactly this view: `domain-model-er-diagram`, backed by React Flow, with nodes/edges/selectedNodeId state and a side panel rendering the selected node — so the stack question is settled and this intent pins behavior, not technology. The design tension is layout state: node positions are presentation, not domain, and `domain-model.yaml` must stay pure domain vocabulary (the schema's only sanctioned presentation metadata is enum `label`/`tone`). The Studio server is also ephemeral — it boots on invocation and shuts down on idle — so any persisted layout would need an on-disk home that survives restarts. The v1 decision: deterministic auto-layout on every load, no persisted node positions — the diagram is a projection of the model, not a second artifact to keep in sync. Manual drag repositioning works within a session for readability but is not saved.

**Action**: The editor UI gains a diagram route/tab rendering the model as a React Flow graph per the adapter's `domain-model-er-diagram` composition: one node per entity (entity name, field list with type badges), one edge per relationship (labeled with name and cardinality rendered as endpoint markers). Auto-layout runs deterministically from the model on load. Clicking a node opens the MVP's entity form in the side panel; clicking an edge opens the relationship form. Dragging a connection handle from one node to another opens the relationship form pre-filled with `from` and `to`; committing creates the relationship, cancelling creates nothing. Diagram edits and form edits mutate the same in-memory draft, so the MVP's save bar and etag flow apply unchanged.

**Objects**: er-diagram-view, entity-node, field-badge, relationship-edge, cardinality-markers, deterministic-auto-layout, node-side-panel, edge-side-panel, draw-to-connect-gesture, in-session-drag-positioning

**Constraints**:
- Node positions are never written to `domain-model.yaml` and no sidecar layout file exists in v1; auto-layout is deterministic — the same model always renders the same initial arrangement, so a reload after save is not a surprise
- Drawing a connection is a proposal, not a mutation: the pre-filled relationship form must be committed before anything enters the in-memory draft, and cancelling leaves the model untouched
- The diagram and the form panels are views over one in-memory draft; an edit in either is immediately reflected in the other, and the save bar's dirty state covers both
- Edges render the relationship name and encode cardinality visually at the endpoints (one/many markers); `ref`-typed fields render as part of the node's field list, not as edges — the diagram distinguishes the schema's two reference constructs rather than conflating them
- Manual node dragging within a session only affects the live view; it does not mark the draft dirty and is discarded on reload
- The diagram remains usable at small scale without configuration (pan and zoom); no minimum-entity-count gate — a one-entity model renders one node

**Verify**:
- A test renders a fixture model and asserts one node per entity with its fields listed and one edge per relationship labeled with name and cardinality markers
- A test renders the same fixture twice and asserts identical initial node positions (deterministic auto-layout)
- A test drags a connection from `Customer` to `Order`, cancels the pre-filled form, and asserts the in-memory draft is unchanged and the save bar stays clean
- A test drags a connection, commits the form with `one-to-many`, and asserts the new relationship appears as an edge, in the relationships panel, and in the serialized YAML after save
- A test edits an entity name in the node's side panel and asserts the node label, the entity list, and dependent relationship endpoints all update from the single draft mutation
- A test drags a node to a new position and asserts the draft stays clean and the position resets on reload
- A test asserts a `ref`-typed field renders inside its entity's node field list and produces no edge

**Questions**:
- Is discarded layout acceptable for large models, or will designers want persisted positions? If persistence is later wanted, the layout would live under `.parlay/` (tool internals), never in `domain-model.yaml` — deferred until real models prove auto-layout insufficient

---

## Deprecated operations handling

**Goal**: Pin the editor's posture toward the deprecated `domain-model.operations` field: surface it read-only with a migration pointer, never edit it, never lose it. The editor offers no operation editing at all — operation-shaped behavior lives in per-feature `capabilities.yaml` now, outside the domain model.

**Persona**: Product designer

**Priority**: P2

**Context**: The domain-model schema deprecates the top-level `operations:` field: authoring-mode validation warns with `domain-operations-deprecated`, build mode errors, and `parlay migrate-domain-operations` walks the entries into per-feature `capabilities.yaml` stubs with designer input on ownership. The migration command deliberately does not delete the field — the designer clears it manually after reviewing the migrated capabilities. So the editor will encounter real files with populated `operations:` blocks for some time, and its obligations are exactly three: don't hide the situation, don't offer to edit a deprecated construct, don't destroy the data before the designer has migrated it. The MVP's persistence intent already guarantees structural passthrough on save; this intent adds the designer-facing surface.

**Action**: When the loaded model has a non-empty `operations:` field, the editor shows a deprecation notice panel: the operation entries rendered read-only, an explanation that operations have moved to per-feature `capabilities.yaml`, and a pointer to run `parlay migrate-domain-operations`. When the field is empty or absent, no operations UI exists at all — a designer on a clean model never learns the construct existed.

**Objects**: deprecated-operations-notice, read-only-operation-entries, migration-pointer, operations-passthrough

**Constraints**:
- The editor never provides create, edit, or delete affordances for `operations:` entries; the only offered action is the pointer to the migration command
- The notice names the actual command (`parlay migrate-domain-operations`) and the destination artifact (per-feature `capabilities.yaml`); it does not merely say "deprecated"
- With an empty or absent `operations:` field, no operations panel, tab, menu entry, or empty-state renders anywhere in the editor
- Saving a model with a populated `operations:` block preserves it structurally unchanged (restating the MVP persistence constraint from the designer-facing side: no edit path exists that could touch it)
- The notice is informational, not blocking: it does not gate saves or any other editing in this feature (whether validation surfaces `domain-operations-deprecated` as a warning belongs to domain-model-editor-validation)

**Verify**:
- A test loads a fixture with two `operations:` entries and asserts the notice panel renders both entries read-only with the migration command named
- A test loads a fixture with no `operations:` field and asserts no operations-related UI exists in the rendered editor
- A test loads a fixture with a populated `operations:` block, performs unrelated entity edits, saves, and asserts the block survives structurally unchanged
- A test asserts the notice contains no interactive controls other than dismissal/navigation — no add, edit, or delete affordances for the entries

---
