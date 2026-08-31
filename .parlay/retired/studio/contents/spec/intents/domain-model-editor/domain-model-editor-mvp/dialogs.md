# Domain-model-editor-mvp — Dialogs

---

### Domain-model tool subsystem and persistence API

**Trigger**: Studio boots with the domain-model tool subsystem registered; the editor UI (or any local client) loads and saves the model through `/api/domain-model`.

User: parlay-studio --project /home/dev/myapp
System (background): Boot step 6 — the domain-model subsystem's registration callback mounts its route group at /api/domain-model.
System: web-server: routes mounted: /api/health, /api/shutdown, /api/domain-model/*, /*
System: Studio started.
User: curl -i http://127.0.0.1:==54221==/api/domain-model/model
System (background): The handler reads ==/home/dev/myapp==/domain-model.yaml, runs the standard domain-model loader (parse, schema_version check, in-memory migration if older), and computes the etag from the raw file bytes.
System (response): HTTP/1.1 200 OK
System (response): X-Request-ID: ==abc-123-def==
System (response): {"etag":"==sha256:9f2c…==","model":{"schema_version":1,"enums":[…],"entities":[…],"relationships":[…]}}
User: curl -i -X PUT http://127.0.0.1:==54221==/api/domain-model/model -H "If-Match: ==sha256:9f2c…==" --data ==<edited model JSON>==
System (background): The etag matches the current file bytes. The handler serializes the model deterministically (stable key order, declaration order preserved as edited) and writes ==/home/dev/myapp==/domain-model.yaml.
System (response): HTTP/1.1 200 OK
System (response): {"etag":"==sha256:41ab…=="}
System (background): The response carries the new etag so the UI's next save uses it without re-loading.

#### Branch: Stale etag — file changed on disk since load

User: # designer loaded the model in the browser, then hand-edited domain-model.yaml in a text editor
User: # the browser now saves with the etag from its earlier load
System (background): The handler recomputes the etag from the current file bytes; it no longer matches the presented one. Nothing is written — the save never merges.
System (response): HTTP/1.1 409 Conflict
System (response): {"code":"conflict","current_etag":"==sha256:77e0…==","attempted_etag":"==sha256:9f2c…=="}
System (background): The UI's reload-and-reapply prompt takes over (see @domain-model-editor-mvp/editor-ui-shell-as-the-first-embedded-ui-bundle-consumer).

#### Branch: No domain-model.yaml yet — empty-model bootstrap

User: curl -i http://127.0.0.1:==54221==/api/domain-model/model  # fresh project, no domain-model.yaml
System (background): The file does not exist. The handler does NOT return not-found — the empty state is the editor's entry point for a fresh project.
System (response): HTTP/1.1 200 OK
System (response): {"etag":"==empty==","model":{"schema_version":1,"enums":[],"entities":[],"relationships":[]}}
User: # designer creates the first entity in the UI and saves
System (background): The save presents the distinguished empty-state etag; the handler creates domain-model.yaml on disk.
System (response): HTTP/1.1 200 OK
System (response): {"etag":"==sha256:c210…=="}

#### Branch: Older schema_version — in-memory migration, save persists current

User: curl http://127.0.0.1:==54221==/api/domain-model/model  # file on disk has schema_version: 1, binary is at 2
System (background): The loader routes the model through the per-version migrator chain (v1 → v2) in memory; the on-disk file is untouched by the load.
System (response): {"etag":"==sha256:9f2c…==","model":{"schema_version":2,…}}
User: # designer edits and saves
System (background): The save is a deliberate write path: the file is written in the binary's current shape with schema_version: 2. The first save persists the migrated form.

#### Branch: schema_version newer than the binary

User: curl -i http://127.0.0.1:==54221==/api/domain-model/model  # file has schema_version: 3, binary understands 2
System (response): HTTP/1.1 400 Bad Request
System (response): {"code":"validation-failed","fields":[{"field":"schema_version","message":"schema-version-newer-than-binary: this file requires a newer parlay binary — run `parlay upgrade`"}]}
System (background): The load fails with the schema's stable code and its actionable message, not a generic server-error. The editor UI shows the message and offers no editing surface for a model it cannot faithfully parse.

#### Branch: Deprecated operations block round-trips untouched

User: # domain-model.yaml on disk carries a populated deprecated `operations:` block
User: # designer edits an entity in the UI and saves
System (background): The load carries the operations entries through to the in-memory model; no edit path in the editor can touch them; the save serializes them structurally unchanged.
User: git diff domain-model.yaml
System: ==diff shows only the edited entity — the operations: block is byte-for-byte identical==

#### Branch: Deterministic serialization

User: # save the same in-memory model twice (no edits in between)
System (background): Serialization is deterministic — stable key order per the schema's documented shape, declaration order of enums/entities/fields/relationships preserved as edited.
System: ==the two written files are byte-identical; the second save produces the same etag==

#### Branch: Legacy domain-model.md is never consulted

User: curl http://127.0.0.1:==54221==/api/domain-model/model  # project has a stale pre-migration domain-model.md AND a domain-model.yaml
System (background): Only the resolved project root's domain-model.yaml is read. The .md is never parsed, never merged, never used as a fallback — matching the schema's read-path precedence rule. In a multi-root project, v1 offers no root selector: the resolved root's file is the one and only target.

---

### Blocking domain-edit invocation for agent handoff

**Trigger**: An agent walking the parlay design loop triggers the domain-edit hook (or the designer runs the command directly); the process blocks until the editing session ends.

User: # during /parlay-loop, the loop determines the domain model needs designer editing and invokes the hook:
User: parlay-studio domain-edit --project /home/dev/myapp
System (background): The identical boot sequence as the bare invocation runs — same harness, same registered tool route groups, same lifecycle. The only difference: the browser-open URL is the bound URL suffixed with /domain-model.
System: web-server: bound to http://127.0.0.1:==54221==/
System: web-server: opening browser at /domain-model…
System: Studio started.
System (background): The process blocks on the shutdown channel. The agent that invoked the command is now waiting on process exit.
User: # designer edits entities and enums in the browser, clicks Save
System (background): Edits flow through the normal save path; the draft is now clean.
User: # designer clicks "Done" in the editor shell
System (background): The draft is clean — the Done control fires the explicit-shutdown request. The browser shows the session-ended screen.
System: studio: shutdown reason: explicit: /api/shutdown
System: studio: graceful shutdown complete
System: (exit 0)
System (background): The blocked agent unblocks on process exit, compares domain-model.yaml against its pre-invocation copy, sees the change, and proceeds with the loop task.

#### Branch: Done with a dirty draft — save-or-discard guard

User: # designer edits a field but does NOT save, then clicks "Done"
System: You have unsaved changes.
  A: Save and finish
  B: Discard changes and finish
  C: Keep editing
User: Selects A
System (background): The draft is saved through the normal etag flow first; only after a successful save does the explicit-shutdown request fire. Edits are never silently dropped and never saved without the designer's confirmation.
System: (exit 0)

#### Branch: Designer walks away — idle timeout unblocks the agent

User: parlay-studio domain-edit --project /home/dev/myapp  # invoked by the loop's hook
User: # designer gets pulled into a meeting; the browser tab sits untouched for 30 minutes
System (background): The idle timeout remains armed during a domain-edit session precisely so an abandoned tab cannot block a waiting agent forever.
System: studio: shutdown reason: idle: no requests for 30m0s
System: (exit 0)
System (background): The agent unblocks, compares the file, finds no change (or a partial earlier save), and proceeds accordingly.

#### Branch: Boot failure — the hook can branch on exit

User: parlay-studio domain-edit --project /home/dev/myapp  # figma_mcp_url not configured
System: studio-config: ERROR `figma_mcp_url` is not set. ==resolution sources listed==
System: (exit non-zero, code: studio-config-figma-mcp-url-missing)
System (background): Non-zero exits are reserved for boot-step failures — no browser was opened, no session ran. The hook distinguishes "session ran" (exit 0) from "Studio failed to start" (non-zero) and surfaces the boot error to the user instead of proceeding as if editing happened.

#### Branch: Standalone invocation outside any loop

User: # no loop, no agent — designer just wants to edit the model
User: parlay-studio domain-edit
System (background): Same command, same behavior: boots, opens the browser on /domain-model, blocks in the designer's terminal until Done / idle / Ctrl-C.
User: # designer finishes, clicks Done
System: (exit 0)

#### Branch: Bare invocation still reaches the editor

User: parlay-studio --project /home/dev/myapp
System (background): The bare invocation is unchanged: it boots the same server, and while the domain-model editor is the only tool in the bundle, the shell's initial redirect lands on /domain-model anyway. domain-edit adds the landing-URL convenience and the blocking-hook contract — not a separate server mode.

#### Branch: Exit status does not encode "did the model change"

User: # agent-side hook logic after the process exits 0
System (background): Process exit is the sole completion signal; v1 defines no exit-code or side-channel semantics for how the session ended. A caller that needs to know whether the model changed compares the file (or its etag) captured before the invocation against the file after exit.

---

### Entity and field editing

**Trigger**: The designer is in the editor at `/domain-model` and creates or reshapes entities through the entity form panel.

User: # designer clicks "New entity", names it Order
System (background): The name is unique in the draft — the entity is created with an empty field list. Nothing touches disk; this is an in-memory draft edit and the save bar goes dirty.
User: # adds field: name "id", type dropdown → uuid, required toggle on
User: # adds field: name "status", type dropdown → OrderStatus
System (background): OrderStatus is a declared enum, so choosing it auto-sets the `enum: OrderStatus` companion key — the schema's two-key invariant is never the designer's job.
User: # adds field: name "customer_id", type dropdown → ref
System: Select the target entity for this reference:
  A: Customer
  B: ==other declared entities…==
User: Selects Customer
System (background): The field commits as `type: ref, target: Customer, required: ==true==`. The dropdown offered exactly the seven closed scalar types plus `ref` plus the declared enums — free-text type entry does not exist, so `field-type-outside-closed-set` is unrepresentable from the editor.
User: # designer clicks Save
System: Saved ==just now==

#### Branch: Duplicate entity name rejected at entry

User: # designer creates a second entity and types the name "Order"
System: An entity named "Order" already exists. Entity names must be unique.
System (background): The rejection is a field-level message at the name control, before any save round-trip.

#### Branch: Rename propagates atomically

User: # designer renames Customer to Client while Order.customer_id targets it and relationship ==customer-orders== names it
System (background): The rename rewrites every ref field's `target:` and every relationship endpoint naming Customer, atomically in the same in-memory edit. No intermediate state with a dangling reference is ever observable in the draft.
System: Renamed Customer → Client (updated 1 referencing field, 1 relationship)

#### Branch: Delete blocked while referenced

User: # designer clicks delete on Customer
System: Customer can't be deleted — it is referenced by:
System:   • Order.customer_id (ref field)
System:   • customer-orders (relationship endpoint)
System: Remove or retype these references first.
User: # designer retypes Order.customer_id to string and deletes the relationship, then deletes Customer again
System: Deleted Customer.
System (background): With no remaining referents the delete succeeds immediately.

#### Branch: Field reordering is preserved to the file

User: # designer drags the "status" field above "customer_id" and saves
System (background): Declaration order is designer-controlled and meaningful; the serialized YAML lists the fields in the new order and the next load returns them in that order.

---

### Enum editing

**Trigger**: The designer is in the editor at `/domain-model` and defines or reshapes enums through the enum form panel.

User: # designer clicks "New enum", names it OrderStatus
User: # adds value row: value "pending", label "Pending", tone → picks from the tone selector
System: Tone: ==(each option rendered as its actual badge treatment)==
  A: neutral
  B: info
  C: warning
  D: danger
  E: success
User: Selects C (warning)
User: # adds value row: value "paid", tone success — no label
User: # clicks Save
System: Saved ==just now==
System (background): The serialized YAML matches the schema shape: `label` and `tone` are omitted where unset, never written as empty strings. The tone selector offered exactly the five closed tones plus "none" — free-text tone entry does not exist, so `enum-tone-outside-closed-set` is unrepresentable from the editor.

#### Branch: Enum rename propagates to referencing fields

User: # designer renames OrderStatus to Status while Order.status references it
System (background): The rename rewrites both companion keys (`type:` and `enum:`) of every referencing field atomically.
System: Renamed OrderStatus → Status (updated 1 referencing field)

#### Branch: Delete blocked while referenced

User: # designer clicks delete on Status
System: Status can't be deleted — it is referenced by:
System:   • Order.status
System: Retype these fields first.

#### Branch: Enum name colliding with a scalar type rejected

User: # designer tries to name an enum "string"
System: "string" is a built-in field type name and can't be used as an enum name.
System (background): Enum names must not collide with the closed scalar type vocabulary (`uuid`, `string`, `int`, `float`, `bool`, `datetime`, `ref`) — a collision would make field types ambiguous. Rejected at entry, field-level.

#### Branch: Duplicate value rejected

User: # designer adds a second value row with value "pending"
System: "pending" is already a value in this enum.

#### Branch: Value reordering is preserved to the file

User: # designer drags "paid" above "pending" and saves
System (background): Value order within the enum is preserved to the serialized file as edited.

---

### Editor UI shell as the first embedded UI bundle consumer

**Trigger**: `parlay-studio` boots with the UI bundle embedded; the browser loads the shell and the designer works inside it.

User: # maintainer builds the bundle before building the binary
User: npm run build   # in studio/internal/ui/
User: go build ./studio/cmd/parlay-studio && parlay-studio --project /home/dev/myapp
System (background): The bundle is embedded via go:embed and satisfies the harness's UIBundle interface. The 503 studio-ui-bundle-not-built placeholder no longer occurs.
User: # browser opens at /
System (background): The client-side router's initial redirect lands on /domain-model. The shell renders: header, save indicator, the editor.
User: # designer edits a field
System: ==save bar (sticky bottom):== Unsaved changes — [Save]
User: # designer clicks Save
System: ==save bar:== Saving…
System: ==save bar:== Saved just now
System (background): The save bar tracks dirty state across all editor panels; a clean state shows the last-saved time.

#### Branch: 409 conflict — reload-and-reapply prompt

User: # designer saves, but the file changed on disk since their load (hand-edit, second tab, regeneration)
System (background): The save returns the harness conflict envelope with both etags.
System: The file changed on disk since you loaded it. Your unsaved edits can't be applied to the old version.
  A: Reload the current file (you'll reapply your edits)
  B: Keep looking at my draft (no save)
User: Selects A
System (background): The UI refetches the model with a fresh etag. No second save fires without designer action — a silent overwrite is never offered.

#### Branch: validation-failed renders at the field, not only as a toast

User: # a save is rejected with {"code":"validation-failed","fields":[{"field":"==entities.Order.fields.status==","message":"==…=="}]}
System (background): Each fields[] entry renders at its form control; a summary toast points at the first offending panel.

#### Branch: server-error toast carries the request id

User: # a save fails with {"code":"server-error","request_id":"xyz-456-pqr"}
System: ==toast:== Something went wrong saving the model. Request ID xyz-456-pqr — check the studio log.

#### Branch: Session ended under the tab

User: # the ephemeral server idles out (or was shut down) while the tab is open; the next API call gets a connection error
System: ==full-screen:== Session ended. Run `parlay-studio` to resume.
System (background): The shell does not spin on retries; it names the restart command and stops.

#### Branch: beforeunload warning while dirty

User: # designer closes the tab with unsaved edits
System: ==browser prompt:== Changes you made may not be saved.
System (background): The beforeunload listener is armed only while the draft is dirty.

#### Branch: Client routes vs API routes

User: # designer pastes ==http://127.0.0.1:54221/some/unknown/route== into the browser
System (background): The harness's SPA fallback serves index.html; the client router renders within the shell (no browser-level 404).
User: curl -i http://127.0.0.1:==54221==/api/unknown-path
System (response): HTTP/1.1 404 Not Found
System (response): {"code":"not-found","target":"/api/unknown-path"}
System (background): /api/* requests are never intercepted by the client router or the SPA fallback.

---
