# Domain-model-editor-mvp

> A web-based visual editor for `domain-model.yaml`, mounted as a tool subsystem on Studio's web-server harness. Scope: entity, field, and enum editing with load/save against the on-disk YAML — relationships and the ER diagram are the depth feature (domain-model-editor-relationships), live deep-validation is its own feature (domain-model-editor-validation). The editor's contract is schema correctness only: no domain reasoning, no inference from code, no mock generation. There is deliberately no operation editor: `domain-model.operations` is deprecated in favor of per-feature `capabilities.yaml`, so the editor preserves but never edits that field. After this feature ships, a designer can run `parlay-studio` (or `parlay-studio domain-edit`), open the editor in a browser, and round-trip real entity and enum edits to the project root's `domain-model.yaml` — and an agent walking the parlay design loop can invoke `domain-edit` as a blocking hook, treating process exit as the signal that the designer finished editing.

---

## Domain-model tool subsystem and persistence API

**Goal**: Register the domain-model editor as a tool subsystem on the web-server harness and pin its persistence contract: how the model is loaded from disk, how edits are written back, and how concurrent modification (a second browser tab, a hand-edit in a text editor, a `parlay create-domain-model` regeneration) is detected instead of silently clobbered.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The web-server harness (studio-foundation) exposes a `ToolRegistration` interface and reserves one route group per tool; its error-envelope middleware already defines the closed error kinds the editor needs (`validation-failed`, `not-found`, `conflict` with an etag pair, `server-error`). The domain model artifact is pinned by `domain-model.schema.md`: exactly one canonical `domain-model.yaml` per active root, `schema_version` required as an integer, hand-editing is a first-class flow, and older-than-binary files are migrated in memory through the per-version migrator chain without touching disk. The editor is one more writer alongside hand-editing and CLI regeneration, so its save path must be a compare-and-swap: the load response carries an etag derived from the on-disk file content, and a save presenting a stale etag fails with the harness's `conflict` envelope rather than overwriting a change the designer never saw. The deprecated `operations:` field is the sharp edge on the write path — the schema preserves it on disk until the designer migrates it via `parlay migrate-domain-operations`, so the editor must round-trip it untouched even though it never offers to edit it.

**Action**: Implement `studio/internal/domainmodel/` as the tool subsystem. It registers one route group at `/api/domain-model` via the harness's registration interface. `GET /api/domain-model/model` reads `<activeRoot>/domain-model.yaml`, runs it through the standard domain-model loader (parse, schema_version check, in-memory migration), and returns the parsed model plus an etag computed from the raw file bytes. `PUT /api/domain-model/model` requires the etag from the last load; on match it serializes the model deterministically and writes the file, on mismatch it returns the harness `ConflictError` with both etags so the UI can prompt reload-and-reapply. A project with no `domain-model.yaml` yet loads as an empty model (schema_version at the binary's current value, empty collections) with a sentinel etag; the first save creates the file.

**Objects**: domain-model-tool-subsystem, domain-model-route-group, model-load-endpoint, model-save-endpoint, content-etag, compare-and-swap-save, empty-model-bootstrap, deterministic-yaml-serialization, deprecated-operations-passthrough

**Constraints**:
- The subsystem registers exactly one route group at `/api/domain-model` through the harness's tool-registration interface; no routes are mounted outside it and the harness package is never imported for anything but the registration and error-envelope types
- All reads and writes target the resolved project root's `domain-model.yaml` only — in a multi-root project, v1 offers no root selector and no config override, and the legacy `domain-model.md` is never parsed, never merged, never consulted as a fallback (matching the schema's read-path precedence rule)
- The load path routes through the same loader semantics as Core's CLI read paths: `missing-schema-version` and `schema-version-unreachable` fail the load, `schema-version-newer-than-binary` fails with the actionable "run `parlay upgrade`" message, and older-than-binary models are migrated in memory
- A model loaded through the migrator chain is served to the UI in the binary's current schema shape; the first successful save persists that migrated form with the binary's current `schema_version` — the editor's save is a deliberate write path, unlike CLI reads which leave the on-disk file unchanged
- The save is a compare-and-swap on the content etag: the etag is derived from the on-disk file bytes at load time, a stale etag fails with the harness `conflict` envelope carrying `current_etag` and `attempted_etag`, and a save never merges — the UI reloads and the designer reapplies
- Serialization is deterministic: two saves of the same in-memory model produce byte-identical files (stable key order per the schema's documented shape, declaration order of enums/entities/fields/relationships preserved as edited)
- The deprecated `operations:` field, when present in the loaded file, is carried through load and save structurally unchanged; the editor never mutates, reorders, or drops its entries
- A `GET` on a project with no `domain-model.yaml` returns an empty model (current `schema_version`, empty collections) and a distinguished etag; it does NOT return `not-found` — the empty state is the editor's entry point for a fresh project, and the first save creates the file

**Verify**:
- A handler test loads a fixture `domain-model.yaml`, asserts the response carries the parsed model and an etag, saves with that etag and a modified model, and asserts the on-disk file reflects the edit
- A handler test loads the model, rewrites the file on disk out-of-band (simulating a hand-edit), then saves with the now-stale etag and asserts a 409 `conflict` envelope with `current_etag` ≠ `attempted_etag` and an unchanged on-disk file
- A handler test loads a fixture at an older `schema_version` with a registered migrator, asserts the served model is in the current shape, saves it, and asserts the on-disk file now carries the binary's current `schema_version`
- A handler test loads a fixture with `schema_version` greater than the binary's and asserts the load fails with `schema-version-newer-than-binary` surfaced in the error envelope, not a generic `server-error`
- A round-trip test loads a fixture containing a populated deprecated `operations:` block, saves without edits, and asserts the operations entries survive structurally unchanged
- A determinism test saves the same in-memory model twice and asserts byte-identical output
- A handler test against a project root with no `domain-model.yaml` asserts the load returns an empty model with the current `schema_version`, and a first save creates the file on disk

---

## Blocking domain-edit invocation for agent handoff

**Goal**: Pin how the editor is launched as a step inside an agent-driven workflow: a `parlay-studio domain-edit` invocation boots the harness, opens the browser on the editor, and blocks until the editing session ends — so the parlay design loop can trigger it as a hook, wait, and treat process exit as "the designer is done editing," and a designer can run the exact same command standalone outside any loop.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The decided workflow is: while an agent walks a feature through the parlay design loop and domain-model editing is needed, the loop triggers a hook that opens the browser with the domain editor; the designer edits and saves; the agent then proceeds with the task. For the agent's side of that handoff to be portable, the completion signal must be the one thing every process runner can observe — the hook command exiting. The harness already has everything the blocking semantics need: the boot sequence blocks the main goroutine on a shutdown channel, and graceful shutdown fires on exactly three triggers (signal, idle timeout, explicit `/api/shutdown`), always exiting zero. What is missing is a designer-facing way to say "I'm finished" (an editor control that fires the explicit-shutdown trigger, guarded against losing a dirty draft) and an invocation that lands the browser directly on the editor route. The idle timeout doubles as the walk-away guard: an agent blocked on the process is never blocked forever, because an abandoned session ends itself. The hook side — how the loop decides editing is needed and what it does after the process exits — belongs to the loop/hook integration, not this feature; this intent pins only the invocation contract that side consumes.

**Action**: Add `domain-edit` to the binary's subcommand dispatch. It runs the identical boot sequence as the bare invocation — same harness, same registered tool route groups, same lifecycle — differing only in the browser-open URL, which points at `/domain-model` instead of `/`. The process blocks until graceful shutdown and exits zero. The editor shell gains a "Done" control: with a clean draft it fires the explicit-shutdown request and shows the session-ended screen; with a dirty draft it first prompts save-or-discard and never silently drops edits. Callers detect whether the model actually changed by comparing the file (or its etag) before and after the invocation — the exit status only means "session over."

**Objects**: domain-edit-subcommand, editor-landing-url, blocking-invocation, process-exit-completion-signal, done-control, dirty-draft-done-guard, idle-timeout-walk-away-guard, standalone-invocation

**Constraints**:
- `parlay-studio domain-edit` runs the same boot sequence as the bare invocation with all registered tools mounted; the only behavioral difference is the browser-open URL (`/domain-model`) — no editor-only server mode exists
- The invocation blocks until graceful shutdown and exits zero on every graceful path (done control, idle timeout, signal); non-zero exits remain reserved for boot-step failures, so a hook can branch on "session ran" vs "Studio failed to start"
- Process exit is the sole completion signal; no side-channel files, sockets, or exit-code semantics distinguish how the session ended in v1 — a caller that needs to know whether the model changed compares the file before and after
- The Done control with a clean draft fires the explicit-shutdown request; with a dirty draft it prompts to save or discard first — it never silently discards and never saves without the designer's confirmation
- The idle timeout remains armed during a `domain-edit` session so an abandoned browser tab cannot block a waiting agent indefinitely
- The bare `parlay-studio` invocation is unchanged: it boots the same server and remains a valid way to reach the editor; `domain-edit` is an entry-point convenience plus the blocking contract, not a separate mode

**Verify**:
- A subcommand test boots `domain-edit` with injected fakes and asserts the browser-open hook receives the bound URL suffixed with `/domain-model`, while the bare invocation's hook receives the root URL
- A test drives a `domain-edit` session, fires the Done control with a clean draft, and asserts the explicit-shutdown trigger fires, the process exits zero, and the session-ended screen was shown
- A UI test fires Done with a dirty draft and asserts a save-or-discard prompt appears, no shutdown fires until a choice is made, and choosing save persists the draft before the shutdown request
- A test with a short idle timeout starts a `domain-edit` session, sends no requests, and asserts the process exits zero after the timeout (the walk-away guard unblocks a waiting caller)
- A test injects a boot-step failure into a `domain-edit` invocation and asserts a non-zero exit before any browser open, matching the harness's boot failure rules
- A test performs an edit-and-save session and asserts the caller can detect the change by comparing the file content before invocation and after exit

---

## Entity and field editing

**Goal**: Let a designer create, rename, and delete entities and manage their typed fields through forms — without knowing the YAML schema, and without being able to express anything the schema's closed vocabulary rejects.

**Persona**: Product designer

**Priority**: P0

**Context**: `DomainEntity` is a name plus a list of `DomainField`s; each field has a name, a type from the closed set (`uuid`, `string`, `int`, `float`, `bool`, `datetime`, `ref`, or a declared enum name), a `required` flag, and — depending on type — a `target:` (ref) or `enum:` (enum-typed) companion key. The closed vocabulary is the editor's opportunity: a form with a type dropdown populated from the closed set plus the declared enums makes `field-type-outside-closed-set` unrepresentable, and pickers for ref targets and enum references make dangling references unrepresentable at authoring time. Cross-entity consistency on rename and delete is the part hand-editing gets wrong most often — renaming an entity in YAML silently orphans every `ref` field targeting it — so the editor owns that consistency mechanically. The adapter registered for Studio's own prototyping (react-vite-radix-tailwind) already pins the `entity-form-panel` composition (react-hook-form state, dirty tracking, Zod-validated save) this intent's UI builds on.

**Action**: The editor UI presents the entity list and a per-entity form panel. Creating an entity requires a unique name; fields are added inline with a name, a type dropdown (closed scalar set + declared enums + ref), a required toggle, and a conditional second control: a target-entity picker when type is `ref`, auto-filled `enum:` when the type names an enum. Renaming an entity rewrites every `ref` field `target:` and every relationship endpoint naming it, atomically in the same in-memory edit. Deleting an entity is blocked while other entities' `ref` fields or any relationship endpoints reference it; the block message lists the referents so the designer knows what to unwind first.

**Objects**: entity-list, entity-form-panel, field-row, field-type-dropdown, ref-target-picker, enum-reference-picker, required-toggle, rename-propagation, referenced-entity-delete-block

**Constraints**:
- The field-type control offers exactly the closed scalar set plus the currently declared enum names plus `ref`; free-text type entry does not exist, so `field-type-outside-closed-set` is unrepresentable from the editor
- Selecting `ref` requires choosing a target from the declared entities before the field can be committed; selecting an enum name sets the `enum:` companion key automatically — the two-key invariant of the schema is never the designer's job
- Entity names are unique within the model; the form rejects a duplicate name at entry with a field-level message rather than on save
- Renaming an entity atomically updates every `ref`-typed field's `target:` and every relationship's `from`/`to` that named it; no intermediate state with a dangling reference is ever observable in the in-memory model
- Deleting an entity that is referenced by any `ref` field or relationship endpoint is blocked with a message enumerating the referents; deleting an unreferenced entity succeeds immediately
- Field edits operate on the in-memory draft only; nothing touches disk until the designer saves through the persistence intent's compare-and-swap
- Declaration order is meaningful and designer-controlled: new entities append, new fields append, and the UI provides reordering; the serialized file reflects the edited order

**Verify**:
- A UI-level test creates an entity `Order` with fields `id: uuid required` and `status: <enum>` and asserts the resulting in-memory model matches the schema's documented shape, including the auto-set `enum:` key
- A test renames `Customer` to `Client` in a model where `Order.customer_id` is `ref → Customer` and a relationship names `Customer`, and asserts both the field target and the relationship endpoint now read `Client` with no intermediate dangling state
- A test attempts to delete `Customer` while `Order.customer_id` targets it and asserts the delete is blocked with a message naming `Order.customer_id`; after retyping the field to `string`, the delete succeeds
- A test asserts the type dropdown for a model with enums `OrderStatus` and `Priority` offers exactly the seven closed scalar types plus `ref` plus those two enum names
- A test creates a second entity named `Order` and asserts a field-level duplicate-name rejection before any save round-trip
- A test reorders fields within an entity, saves, and asserts the serialized YAML lists the fields in the new order

---

## Enum editing

**Goal**: Let a designer define enums and their values — including the presentation metadata (`label`, `tone`) that the schema deliberately carries on enums — with the tone vocabulary constrained to the closed set and previewed visually rather than guessed.

**Persona**: Product designer

**Priority**: P0

**Context**: `DomainEnum` is the one place the domain model carries presentation metadata: each value has an optional human `label` and an optional `tone` from the closed set `{neutral, info, warning, danger, success}`, matching the adapter color-token tones, so designers can preview rendering without an adapter round-trip. That preview is exactly what a visual editor can do better than YAML: show each tone as its rendered badge color while editing. Enums are referenced from entity fields by name (the `enum:` companion key), so enum rename and delete carry the same cross-reference obligations as entity rename and delete.

**Action**: The editor UI presents the enum list and a per-enum form: value rows with raw `value`, optional `label`, and a tone selector rendering each closed-set tone as its badge treatment. Renaming an enum rewrites the `type:` and `enum:` keys of every field referencing it atomically. Deleting an enum is blocked while any field references it, with the referents listed. Value rows can be reordered; order is preserved to the file.

**Objects**: enum-list, enum-form-panel, enum-value-row, tone-selector, tone-preview, enum-rename-propagation, referenced-enum-delete-block

**Constraints**:
- The tone selector offers exactly the closed set `{neutral, info, warning, danger, success}` plus "none"; free-text tone entry does not exist, so `enum-tone-outside-closed-set` is unrepresentable from the editor
- Each tone option renders with its actual visual treatment (badge color) in the selector, so the designer picks by appearance, not by token name alone
- `label` and `tone` are optional per value and omitted from the serialized YAML when unset, not written as empty strings
- Renaming an enum atomically updates the `type:` and `enum:` keys of every field that references it; deleting an enum with referencing fields is blocked with the referents enumerated
- Enum names are unique within the model and must not collide with the closed scalar type names (`uuid`, `string`, `int`, `float`, `bool`, `datetime`, `ref`), which would make field types ambiguous; the form rejects such names at entry
- Value rows within an enum are unique on `value`; duplicates are rejected at entry with a field-level message

**Verify**:
- A test creates enum `OrderStatus` with values `pending` (label "Pending", tone `warning`) and `paid` (tone `success`), and asserts the serialized YAML matches the schema's documented shape with no empty-string keys for unset options
- A test renames `OrderStatus` to `Status` while `Order.status` references it and asserts the field's `type:` and `enum:` keys both read `Status` afterward
- A test attempts to delete a referenced enum and asserts the block message names the referencing fields; deleting an unreferenced enum succeeds
- A test attempts to name an enum `string` and asserts a field-level rejection naming the collision with the scalar type vocabulary
- A test adds a duplicate value `pending` to an enum and asserts a field-level rejection before save
- A test asserts the tone selector offers exactly the five closed tones plus none, each with a visual preview

---

## Editor UI shell as the first embedded UI bundle consumer

**Goal**: Stand up the Studio UI bundle — the browser app the harness's SPA fallback has been waiting on — with the domain-model editor as its first route, so `parlay-studio` boots to a working editor instead of the `studio-ui-bundle-not-built` 503 placeholder.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The web-server harness already serves an embedded UI bundle at `/` with SPA fallback semantics, and returns a 503 with code `studio-ui-bundle-not-built` until a bundle exists — the harness intent explicitly assigned building the bundle to "a future feature," and this is that feature. The UI stack question is settled by the adapter Studio registered for its own prototyping: react-vite-radix-tailwind, whose compositions already specify the editor's building blocks — `entity-form-panel` (react-hook-form + Zod), the sticky `save-bar` with dirty-state tracking and a beforeunload warning, and `toast-async-feedback` for mutation outcomes. The shell also carries the common UI obligations every Studio tool shares: a header, a save indicator, error display, and the "session ended" screen shown when the ephemeral server has shut down under the tab.

**Action**: Implement the UI under `studio/internal/ui/` as a Vite + React + Radix + Tailwind app, built by `npm run build` and embedded into the binary via `go:embed`, satisfying the harness's `UIBundle` interface. The app shell provides the header, client-side routing with `/domain-model` as the editor route (and the initial redirect target), the save bar wired to the persistence API's etag flow, toast feedback mapping the harness error envelopes (`validation-failed` field messages inline, `conflict` as a reload-and-reapply prompt, `server-error` with the request id), and a "session ended — run `parlay-studio` to resume" screen when API calls start failing with connection errors after a period of activity.

**Objects**: studio-ui-bundle, ui-embed, app-shell, client-side-router, domain-model-route, save-bar, dirty-state-tracking, error-envelope-toasts, conflict-reload-prompt, session-ended-screen

**Constraints**:
- The bundle is embedded into the binary at build time; a released `parlay-studio` binary serves the UI with no network fetches at runtime and no on-disk asset directory
- The stack is the one the registered studio adapter pins (React + Vite + Radix + Tailwind); the editor's forms and save flow follow the adapter's `entity-form-panel`, `save-bar`, and `toast-async-feedback` compositions rather than inventing parallel patterns
- The save bar tracks dirty state across all editor panels: any uncommitted in-memory edit shows the Save affordance, saving shows progress, a clean state shows the last-saved time; a beforeunload warning fires while dirty
- A 409 `conflict` on save presents a reload-and-reapply prompt that names what happened ("the file changed on disk since you loaded it"); it never offers a silent overwrite
- A failed API connection after previous success presents the session-ended screen naming the restart command; it does not spin on retries
- Unmatched client-side routes render within the shell (the harness's SPA fallback already serves `index.html`); `/api/*` requests are never intercepted by the client router

**Verify**:
- A build-integration test builds the bundle, boots the harness with it embedded, fetches `/` and asserts the editor shell HTML is served (the 503 `studio-ui-bundle-not-built` envelope no longer occurs)
- A UI test edits a field, asserts the save bar shows dirty state, saves, and asserts the bar returns to clean with a last-saved timestamp
- A UI test receives a 409 on save and asserts the reload-and-reapply prompt appears, reload refetches the model with a fresh etag, and no second save fires without designer action
- A UI test receives a `validation-failed` envelope and asserts each `fields[]` entry renders at its form control, not only as a toast
- A UI test simulates connection refusal after a successful session and asserts the session-ended screen appears with the restart command
- A UI test navigates to an unknown route and asserts the shell renders (no browser-level 404), while a fetch to an unknown `/api/*` path still surfaces the harness's `not-found` envelope

---
