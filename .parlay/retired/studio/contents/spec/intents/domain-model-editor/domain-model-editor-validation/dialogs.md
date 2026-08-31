# Domain-model-editor-validation — Dialogs

---

### Validation parity with Core's deep validation

**Trigger**: A client (the editor, or a maintainer with curl) POSTs a model draft to `/api/domain-model/validate`; the subsystem shells the draft through Core's `parlay validate --type domain-model --json` and returns the finding list.

User: curl -i -X POST http://127.0.0.1:==54221==/api/domain-model/validate --data ==<draft with entities.Order.fields.qty typed "quantity">==
System (background): The handler serializes the submitted draft to YAML and pipes it on stdin to a `parlay validate --type domain-model --json` subprocess — the same binary the build path runs, located by resolving the `parlay` executable once at boot. It reads nothing from disk; the on-disk `domain-model.yaml` is not consulted. It parses the emitted finding list and maps each finding into a `fields[]` entry.
System (response): HTTP/1.1 200 OK
System (response): {"fields":[{"field":"==entities.Order.fields.qty==","code":"==field-type-outside-closed-set==","severity":"error","message":"==`quantity` is not one of the closed field types; use one of uuid, string, int, float, bool, datetime, ref, or a declared enum name=="}]}
System (background): 200, not a 4xx — a finding list is a query result. The code is Core's, verbatim; the element path resolves to the offending field; the message is the schema's actionable phrasing, not a bare code.

#### Branch: Clean draft returns 200 with an empty finding list

User: curl -i -X POST http://127.0.0.1:==54221==/api/domain-model/validate --data ==<a schema-clean draft>==
System (response): HTTP/1.1 200 OK
System (response): {"fields":[]}
System (background): Clean is an empty list at 200, never a 204 or a bare body — the editor reads "zero findings" the same way it reads "some findings", off the same envelope shape.

#### Branch: Deprecated operations block is a warning, not an error

User: curl -X POST http://127.0.0.1:==54221==/api/domain-model/validate --data ==<draft carrying a populated deprecated operations: block>==
System (response): {"fields":[{"field":"==operations==","code":"domain-operations-deprecated","severity":"warning","message":"==operations on the domain model are deprecated; migrate them to per-feature capabilities.yaml with `parlay migrate-domain-operations`=="}]}
System (background): Severity is taken from Core's authoring-mode table as emitted through the CLI — `domain-operations-deprecated` is the one warning; every other code is an error. Studio does not recompute or reclassify severity.

#### Branch: Whole-model violation uses the distinguished top-level path

User: curl -X POST http://127.0.0.1:==54221==/api/domain-model/validate --data ==<draft with no schema_version>==
System (response): {"fields":[{"field":"==$==","code":"missing-schema-version","severity":"error","message":"==the model is missing schema_version=="}]}
System (background): A finding that applies to the whole model carries the distinguished top-level path (not a blank or fabricated element path), so the UI can render it in the panel without trying to anchor it to an element.

#### Branch: Malformed request is a validation-failed HTTP error, distinct from a finding list

User: curl -i -X POST http://127.0.0.1:==54221==/api/domain-model/validate --data ==<unparseable bytes / bad request envelope>==
System (response): HTTP/1.1 400 Bad Request
System (response): {"code":"validation-failed","fields":[{"field":"==body==","message":"==request body is not a parseable model draft=="}]}
System (background): `validation-failed` at the HTTP-error level means the request itself was malformed — the subprocess was never run. This is a different thing from a well-formed but schema-invalid draft, which is a 200 finding list. The two are never conflated.

#### Branch: Side-effect-free — validates the submitted bytes alone

User: # the on-disk domain-model.yaml is clean, but the POSTed draft carries an error
User: curl -X POST http://127.0.0.1:==54221==/api/domain-model/validate --data ==<draft with an error>==
System (response): {"fields":[{"field":"==entities.Order.fields.status==","code":"undeclared-entity-reference","severity":"error","message":"==…=="}]}
System (background): The findings are computed from the submitted draft bytes, never the file on disk. The call writes nothing and mutates nothing — validating a draft is a pure function over its argument.

#### Branch: Parity is guarded by a shared fixture corpus, not a second validator

User: # CI runs the parity suite: one fixture per closed code, plus clean fixtures
System (background): Each fixture is run twice — once through `parlay validate --type domain-model --json` directly, once through `POST /api/domain-model/validate` — and the two finding sets are asserted identical. Because both paths run the same binary, the suite guards the Studio wrapper (request shaping, stdin transport, path mapping), not Core's rules. A guard test also asserts Studio imports no Core package: the editor reaches the validator only out of process.
System: ==parity suite green; no-Core-import guard green==

---

### Live in-editor validation surfacing

**Trigger**: The designer is in the editor at `/domain-model`; the editor revalidates the in-memory draft against `/api/domain-model/validate` on load and after every committed mutation, surfacing findings inline and in a validation panel.

User: # a teammate hand-edited domain-model.yaml, deleting the OrderStatus enum but leaving Order.status: {type: OrderStatus, enum: OrderStatus}
User: # designer opens the editor on that file
System (background): On load the editor validates the freshly-loaded draft before any edit. Core flags the orphaned reference; the finding carries `entities.Order.fields.status` as its element path.
System: ==validation panel (1 finding):==
System:   ⛔ ==undeclared-entity-reference== · entities.Order.fields.status
System:      ==`OrderStatus` is not a declared enum or entity; declare it or retype this field==
System: ==inline: the Order form's `status` field row shows an error marker==
System: ==header count indicator: "1 error"==
System (background): The panel is the complete finding list; the inline marker is the same finding anchored at the element it names. Both carry error styling.

#### Branch: Debounced revalidation collapses a burst of edits

User: # designer retypes the status field, then quickly edits five field names in a row
System (background): Each committed mutation schedules a revalidation, but the calls are debounced — a rapid burst produces one trailing call against the final draft, not five. The findings from that trailing call are what render.
System: ==one validate call fires after the burst settles; the panel updates once==

#### Branch: Clicking a finding navigates to and highlights its element

User: # designer clicks the panel finding whose path is relationships.customer-orders.to
System (background): The click resolves the element path to its owning editor surface and navigates there, highlighting the element. A relationship path opens the relationships panel with that relationship selected.
System: ==relationships panel opens; the customer-orders relationship row is highlighted==

#### Branch: Whole-model finding highlights nothing but explains itself

User: # designer clicks a panel finding whose path is the distinguished top-level path (e.g. missing-schema-version)
System (background): A whole-model finding has no element to anchor to; clicking it highlights nothing and the panel row carries the full actionable message in place.
System: ==panel row stays expanded with the fix text; no element is highlighted==

#### Branch: Warnings are visually distinct from errors

User: # the loaded model carries a populated deprecated operations: block and one field-type error
System (background): The validate response marks `domain-operations-deprecated` as warning severity and the field-type finding as error. The panel, the inline markers, and the count indicator all distinguish the two.
System: ==panel: one ⚠ warning row (domain-operations-deprecated) and one ⛔ error row, styled distinctly==
System: ==count indicator: "1 error, 1 warning"==

#### Branch: Fixing the last error flips the indicator to an explicit clean state

User: # designer retypes Order.status to a declared enum, clearing the last error
System (background): The commit triggers a trailing revalidation; the response is an empty finding list. The count indicator shows an explicit clean state, not merely the absence of markers.
System: ==inline error marker on the status row clears==
System: ==count indicator: "✓ No problems"==

#### Branch: A stale validate response is discarded, never rendered over fresher findings

User: # designer makes edit A, then edit B before A's validation returns; A's response arrives after B's
System (background): Each validate call is tagged to the draft it was issued for. When the older (edit A) response arrives after the newer (edit B) response, it is recognized as superseded and dropped — the panel keeps edit B's findings. Validation state is recomputed per response, never merged from a cache.
System: ==the panel shows edit B's findings; edit A's stale set is discarded==

---

### Save gating on validation findings

**Trigger**: The designer clicks Save (or a client POSTs to the save endpoint); the save path validates the draft server-side before writing — errors block, warnings don't.

User: # the draft carries a field-type-outside-closed-set error (from a hand-edited load the designer hasn't fixed)
System (background): The save bar already reflects the gate: while error findings exist it shows a blocked state, not a bare disabled button.
System: ==save bar: "Can't save — 1 error. [View problems]"== (links to the validation panel)
User: # designer clicks Save anyway (or the state is stale on a second tab)
System (background): The save path validates server-side before touching disk. The error-severity finding fails the save; nothing is written.
System (response): HTTP/1.1 400 Bad Request
System (response): {"code":"validation-failed","fields":[{"field":"==entities.Order.fields.qty==","code":"field-type-outside-closed-set","message":"==…=="}]}
System (background): The gate is server-side and authoritative — the on-disk file is byte-for-byte untouched. The envelope's fields[] render at their controls exactly as the MVP's validation-failed handling does.

#### Branch: A warning-only draft saves normally

User: # the draft's only finding is domain-operations-deprecated (a warning); the designer edits an entity and saves
System (background): Warnings never gate. The save proceeds through the MVP's normal compare-and-swap write path; the deprecated operations block round-trips untouched.
System (response): HTTP/1.1 200 OK
System (response): {"etag":"==sha256:41ab…=="}
System: ==save bar: "Saved just now"==

#### Branch: Load-time findings populate the blocked state before any edit

User: # designer opens a file that already carries two errors
System (background): The editor validates on load, so the findings — and the blocked save bar — are present before the designer touches anything. The repair scope is visible up front, not discovered at first save.
System: ==save bar: "Can't save — 2 errors. [View problems]"==
System: ==validation panel already lists both, with inline markers at their elements==

#### Branch: Fixing the errors transitions the save bar to the normal dirty/save state

User: # designer fixes both errors; a trailing revalidation returns an empty list
System (background): With no error findings the gate opens; the save bar drops the blocked state and shows the normal dirty affordance.
System: ==save bar: "Unsaved changes — [Save]"==
User: # designer clicks Save
System (response): HTTP/1.1 200 OK
System: ==save bar: "Saved just now"==

#### Branch: The validation gate orders before the etag compare-and-swap

User: # the draft carries an error AND the etag is stale (the file changed on disk since load)
System (background): The gate runs first. Validity is designer-actionable and comes before conflict resolution, so an invalid draft fails with validation-failed even though the etag is also stale.
System (response): HTTP/1.1 400 Bad Request
System (response): {"code":"validation-failed","fields":[{"field":"==entities.Order.fields.qty==","code":"field-type-outside-closed-set","message":"==…=="}]}
User: # designer fixes the error and re-saves with the same still-stale etag
System (background): Now that the draft is valid, the gate passes and the compare-and-swap runs — and catches the stale etag.
System (response): HTTP/1.1 409 Conflict
System (response): {"code":"conflict","current_etag":"==sha256:77e0…==","attempted_etag":"==sha256:9f2c…=="}
System (background): The MVP's reload-and-reapply prompt takes over. The two failures are distinct: validation-failed (the gate) precedes conflict (the etag).

#### Branch: A direct API save bypassing the UI is gated identically

User: curl -i -X PUT http://127.0.0.1:==54221==/api/domain-model/model -H "If-Match: ==sha256:9f2c…==" --data ==<draft carrying an error>==
System (background): The gate lives on the server, so a client that skips or staled-out the UI's blocked state cannot write an invalid file. There is no force-save or override path — the escape hatch for deliberately writing an invalid file is a text editor, which the editor's contract is not to be.
System (response): HTTP/1.1 400 Bad Request
System (response): {"code":"validation-failed","fields":[{"field":"==…==","code":"==…==","message":"==…=="}]}
System (background): Identical rejection, on-disk file untouched.

---
