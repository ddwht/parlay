# Domain-model-editor-relationships — Dialogs

---

### Relationship editing

**Trigger**: The designer is in the editor at `/domain-model` and declares, edits, or deletes relationships through the relationships panel and its form.

User: # designer opens the relationships panel — a list of declared relationships plus a "New relationship" control
User: # clicks "New relationship"; the form shows a `from` picker, a `to` picker, a cardinality dropdown, and a name field
User: # sets `from` → Customer
System: Select the `to` entity:
  A: Customer
  B: Order
  C: ==other declared entities…==
User: Selects B (Order)
System (background): Both endpoints are chosen from the closed set of declared entities — free-text endpoint entry does not exist, so `undeclared-entity-reference` is unrepresentable from the editor. With `from` and `to` set, the name field pre-fills.
System: Name: ==customer-orders== (pre-filled from Customer → Order; editable)
User: # sets cardinality
System: Cardinality:
  A: one-to-one
  B: one-to-many
  C: many-to-one
  D: many-to-many
User: Selects B (one-to-many)
System (background): The dropdown offers exactly the four closed cardinalities, so `relationship-cardinality-unknown` is unrepresentable from the editor. The relationship commits to the in-memory draft as `{name: customer-orders, from: Customer, to: Order, cardinality: one-to-many}` and the save bar goes dirty.
User: # designer clicks Save
System (background): The edit flows through the MVP's compare-and-swap save; declaration order is preserved to the file as edited. The serialized YAML matches the schema's documented relationship shape.
System: Saved ==just now==

#### Branch: Endpoint pickers and cardinality dropdown offer exactly the closed sets

User: # a three-entity model (Customer, Order, Product); designer opens the relationship form
System (background): Both the `from` and `to` pickers offer exactly those three entities and nothing else; the cardinality dropdown offers exactly `{one-to-one, one-to-many, many-to-one, many-to-many}`. There is no free-text path to an undeclared entity or an unknown cardinality.

#### Branch: Duplicate relationship name rejected at entry

User: # a relationship named `customer-orders` already exists; designer sets up a second relationship and types the name `customer-orders`
System: A relationship named "customer-orders" already exists. Relationship names must be unique.
System (background): The rejection is a field-level message at the name control, before any save round-trip — relationship names are unique within the model.

#### Branch: Name prefill is a convenience, not a constraint

User: # designer sets `from` → Customer, `to` → Order; the name pre-fills to `customer-orders`
User: # designer renames it to `places` and then changes `to` → Invoice
System (background): The name pre-fill fires only as a convenience while the field tracks the endpoints; once the designer has manually set the name, editing endpoints does NOT regenerate it. The name stays `places`.
System: Name: ==places== (unchanged by the endpoint edit)

#### Branch: Self-referential relationship accepted

User: # designer creates a relationship with `from` → Employee and `to` → Employee, cardinality many-to-one (an org hierarchy)
System (background): `from` = `to` is legal; the form does not special-case or block it. The relationship commits like any other.
User: # designer saves
System (background): The self-referential relationship round-trips through save without warning or mangling.
System: Saved ==just now==

#### Branch: Delete is immediate

User: # designer clicks delete on the `customer-orders` relationship
System (background): Nothing in the model references a relationship by name, so there are no dependents to check. The relationship is removed from the in-memory draft immediately.
System: Deleted relationship customer-orders.
User: # designer saves
System (background): The relationship is absent from the serialized file.

#### Branch: Entity rename propagates into relationship endpoints; manual names stay put

User: # relationship `places` has `from: Customer, to: Order`; the designer renames entity Order to Invoice
System (background): The MVP's rename propagation rewrites every relationship endpoint naming Order to Invoice, atomically in the same in-memory edit. The relationship's manually-set name `places` is untouched — only endpoints follow the rename, never the designer's chosen name.
System: Renamed Order → Invoice (updated 1 relationship endpoint)

---

### ER diagram view with draw-to-connect

**Trigger**: The designer opens the diagram route/tab; the model renders as a React Flow graph and the designer edits it by clicking nodes/edges and drawing connections.

User: # designer navigates to the diagram tab
System (background): The adapter's `domain-model-er-diagram` composition renders the model: one node per entity (entity name, field list with type badges), one edge per relationship (labeled with name and cardinality shown as endpoint markers). Auto-layout runs deterministically from the model on load.
System: ==diagram: Customer and Order nodes, one edge labeled `customer-orders` with a one-marker at Customer and a many-marker at Order==
User: # designer clicks the Order node
System (background): Clicking a node opens the MVP's entity form in the side panel — the same form used elsewhere in the editor.
System: ==side panel: Order entity form (fields, types, required toggles)==
User: # designer clicks the `customer-orders` edge
System (background): Clicking an edge opens the relationship form in the side panel.
System: ==side panel: relationship form for customer-orders (from Customer, to Order, one-to-many)==
User: # designer drags a connection handle from the Customer node to the Product node
System (background): Dragging a connection is a proposal, not a mutation. The relationship form opens pre-filled with `from: Customer, to: Product`; nothing enters the draft yet.
System: ==side panel: relationship form pre-filled from Customer → Product; name pre-fills `customer-products`; cardinality unset==
User: # designer picks cardinality one-to-many and commits
System (background): Committing creates the relationship in the in-memory draft. The diagram and the form panels are views over the one draft, so the new edge appears immediately and the save bar goes dirty.
System: ==diagram: new edge `customer-products` appears; save bar: Unsaved changes==
User: # designer saves
System (background): The new relationship appears in the relationships panel and in the serialized YAML after save.
System: Saved ==just now==

#### Branch: Deterministic auto-layout

User: # designer renders the diagram, then reloads the page
System (background): Auto-layout is deterministic — the same model always renders the same initial arrangement. Node positions are never written to `domain-model.yaml` and no sidecar layout file exists in v1, yet the reload-after-save arrangement is identical, so it is not a surprise.
System: ==both renders place the nodes in the same positions==

#### Branch: Draw-to-connect cancelled leaves the model untouched

User: # designer drags a connection from Customer to Order, then cancels the pre-filled form
System (background): Drawing a connection is a proposal; cancelling before commit enters nothing into the draft. The in-memory draft is unchanged and the save bar stays clean.
System: ==no new edge; save bar: (clean, last saved …)==

#### Branch: Draw-to-connect self-loop (from = to on one node)

User: # designer drags a connection handle from the Employee node back onto the Employee node itself
System (background): A self-loop gesture is a legal self-referential relationship. The form opens pre-filled with `from: Employee, to: Employee` — the same accepted `from` = `to` case the relationship form supports — as a proposal, not yet in the draft.
System: ==side panel: relationship form pre-filled from Employee → Employee; name pre-fills `employee-employee` (editable); cardinality unset==
User: # designer renames it `reports-to`, picks many-to-one, and commits
System (background): The self-referential relationship enters the draft and renders as a self-edge on the Employee node.
System: ==diagram: self-loop edge `reports-to` on the Employee node; save bar: Unsaved changes==

#### Branch: Draw-to-connect between an already-related pair degrades to duplicate-name rejection

User: # a relationship `customer-orders` (Customer → Order) already exists; designer drags a second connection from Customer to Order
System (background): The gesture is still just a proposal. The form opens pre-filled `from: Customer, to: Order`; the name pre-fills `customer-orders` — which already exists. The uniqueness rule applies to the diagram-gesture path exactly as it does in the panel form.
System: ==side panel: relationship form pre-filled from Customer → Order; name field: `customer-orders`==
System: A relationship named "customer-orders" already exists. Relationship names must be unique.
User: # designer renames it `customer-returns`, picks many-to-many, and commits
System (background): With a unique name the second relationship commits — a distinct edge between the same pair. Had the designer cancelled instead, nothing would enter the draft.
System: ==diagram: a second edge `customer-returns` between Customer and Order; save bar: Unsaved changes==

#### Branch: Edit in one view reflects in the other from a single draft mutation

User: # designer edits the Order entity's name to Invoice in the node's side panel
System (background): The diagram and the form panels are views over one in-memory draft. The single mutation updates the node label, the entity list, and dependent relationship endpoints together; the save bar's dirty state covers both views.
System: ==node relabels to Invoice; entity list shows Invoice; the `customer-orders` edge's endpoint follows==

#### Branch: In-session node drag does not dirty the draft

User: # designer drags the Order node to a new position for readability, makes no other change
System (background): Manual node dragging within a session only affects the live view; node positions are presentation, not domain. It does not mark the draft dirty.
System: ==save bar stays clean==
User: # designer reloads the page
System (background): No position was persisted; deterministic auto-layout runs again and the node returns to its computed position.
System: ==the node resets to its auto-layout position==

#### Branch: ref-typed field renders in the node, not as an edge

User: # Order has a `customer_id` field of type `ref` targeting Customer, and no `customer-orders` relationship exists
System (background): The diagram distinguishes the schema's two reference constructs. A `ref`-typed field renders inside its entity's node field list (with a type badge), not as an edge — only declared relationships become edges.
System: ==Order node lists `customer_id: ref → Customer` in its field list; no edge is drawn for it==

#### Branch: Usable at small scale — pan, zoom, single-entity model

User: # a one-entity model
System (background): The diagram remains usable without configuration: pan and zoom are always available and there is no minimum-entity-count gate.
System: ==one node renders, pannable and zoomable; no empty-state gate blocks the view==

---

### Deprecated operations handling

**Trigger**: The editor loads a model; its behavior toward the deprecated top-level `operations:` field depends entirely on whether that field is populated.

User: # designer opens a model whose `domain-model.yaml` has a non-empty `operations:` block (two entries)
System (background): The editor detects the populated deprecated field and renders a deprecation notice panel — read-only, informational, non-blocking.
System: ==deprecation notice panel:==
System: Operations have moved to per-feature `capabilities.yaml`. The `operations:` field on the domain model is deprecated and read-only here.
System:   • ==operation entry 1== (read-only)
System:   • ==operation entry 2== (read-only)
System: To migrate these, run `parlay migrate-domain-operations`.
System (background): The notice names the actual command and the destination artifact — not merely "deprecated". The only offered action is the migration pointer; there is no create, edit, or delete affordance for the entries.

#### Branch: Empty or absent operations field — no operations UI at all

User: # designer opens a clean model with no `operations:` field (or an empty one)
System (background): With an empty or absent `operations:` field, no operations panel, tab, menu entry, or empty-state renders anywhere in the editor. A designer on a clean model never learns the construct existed.
System: ==the editor shows entities, enums, relationships, and the diagram — nothing operations-related==

#### Branch: Populated operations block survives an unrelated save unchanged

User: # a model with a populated `operations:` block; designer edits an entity and saves
System (background): No edit path in the editor can touch the operations entries. The save serializes them structurally unchanged, restating the MVP persistence guarantee from the designer-facing side.
User: git diff domain-model.yaml
System: ==diff shows only the edited entity — the operations: block is byte-for-byte identical==

#### Branch: Notice is informational, not interactive

User: # designer inspects the deprecation notice panel
System (background): The notice contains no interactive controls other than dismissal/navigation — no add, edit, or delete affordances for the entries. It does not gate saves or any other editing in this feature. (Whether validation surfaces `domain-operations-deprecated` as a warning belongs to @domain-model-editor/domain-model-editor-validation.)

---
