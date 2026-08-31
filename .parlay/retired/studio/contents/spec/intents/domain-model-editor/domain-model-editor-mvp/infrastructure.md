# Domain-model-editor-mvp — Infrastructure

---

## Domain-model subsystem registration boundary

**Affects**: tool-subsystem registration boundary; the single route-group prefix reserved for the domain-model editor; the two persistence endpoints the group exposes; the import direction between the web-server harness and the editor subsystem

**Behavior**: The domain-model editor registers itself as one tool subsystem on the web-server harness through the harness's tool-registration mechanism, mounting exactly one route group at the `/api/domain-model` prefix. That group exposes exactly two persistence endpoints: a model-read endpoint that loads the current domain model and returns it together with the content identity token, and a model-write endpoint that saves an edited model under compare-and-swap. No editor route is mounted outside the `/api/domain-model` prefix. The editor depends on the harness only for the registration mechanism and the closed error-envelope kinds (`validation-failed`, `not-found`, `conflict`, `server-error`); it consumes nothing else from the harness. The harness in turn never depends on the editor subsystem — the dependency runs one way, from the editor to the registration mechanism, so the two remain decoupled and no dependency cycle forms.

**Invariants**:
- The editor registers exactly one route group, at the `/api/domain-model` prefix; a second registration under the editor's tool name is rejected at router-construction time with a stable code.
- The route group exposes exactly two persistence endpoints — a model-read (load) endpoint and a model-write (compare-and-swap save) endpoint; the read/write semantics of each are pinned in the loader-reuse and compare-and-swap fragments.
- The model-read endpoint returns the loaded model together with its content identity token; its error surface is closed to `validation-failed` (schema-version or parse failure on load) and `server-error`. A load against a project with no model file returns the empty-model bootstrap, never `not-found`.
- The model-write endpoint accepts an edited model plus the token from its originating load; its error surface is closed to `validation-failed` (a rejected model body), `conflict` (a stale content identity token), and `server-error`.
- No editor HTTP route is reachable outside the `/api/domain-model` prefix.
- The harness does not depend on the editor subsystem, and the editor attaches HTTP routes only through the harness's registration mechanism — both directions are review-enforced to keep the dependency acyclic.
- The editor references the harness only for the registration mechanism and the closed error-envelope kinds.

**Source**: @domain-model-editor-mvp/domain-model-tool-subsystem-and-persistence-api

**Backward-Compatible**: yes

**Notes**:
- This is the first tool subsystem to consume the harness registration mechanism after the harness's own universal endpoints; adding it is a registration call, not a harness change.

---

## Domain-model read-path resolution

**Affects**: read-path precedence for the domain model artifact; the single-canonical-file rule per active root; the multi-root resolution policy for v1

**Behavior**: The editor reads and writes exactly one canonical `domain-model.yaml`, the one at the resolved project root. In a multi-root project, v1 offers no root selector and no configuration override — the resolved root's file is the sole target. The legacy `domain-model.md` is never parsed, never merged, and never consulted as a fallback, matching the domain-model artifact's documented read-path precedence rule.

**Invariants**:
- All reads and writes target the resolved project root's `domain-model.yaml` and no other file.
- A legacy `domain-model.md` present in the project is never read by the editor under any code path.
- v1 exposes no root selector and no configuration override for choosing which root's model is edited.

**Source**: @domain-model-editor-mvp/domain-model-tool-subsystem-and-persistence-api

**Backward-Compatible**: yes

---

## Domain-model loader reuse and schema-version gating

**Affects**: the load pipeline shared with the command-line read paths; schema-version gating policy; in-memory migration on load

**Behavior**: The editor's load path routes through the same loader the command-line read paths use — parse, schema-version check, and per-version in-memory migration. A model whose `schema_version` is missing or unreachable fails the load; a model newer than the running binary fails the load with the actionable "run parlay upgrade" message rather than a generic server error; a model older than the running binary is migrated in memory to the binary's current shape and served to the editor in that shape. Unlike command-line reads, which leave the on-disk file untouched, the editor's first successful save is a deliberate write path that persists the migrated form at the binary's current `schema_version`. A project with no `domain-model.yaml` yet loads as an empty model at the current `schema_version` with a distinguished sentinel identity — never as a not-found — because the empty state is the editor's entry point for a fresh project; the first save creates the file.

**Invariants**:
- Missing or unreachable `schema_version` fails the load with the loader's stable code, surfaced through the error envelope.
- A `schema_version` newer than the running binary fails the load with the "run parlay upgrade" message surfaced through the error envelope, not as a generic server error.
- A model older than the running binary is served to the editor in the binary's current shape via in-memory migration; the on-disk file is untouched until a save.
- The first successful save of a migrated model persists it at the binary's current `schema_version`.
- A load against a project with no `domain-model.yaml` returns an empty model at the current `schema_version` with a sentinel identity, never a not-found; the first save creates the file.

**Source**: @domain-model-editor-mvp/domain-model-tool-subsystem-and-persistence-api

**Backward-Compatible**: yes

**Notes**:
- Reusing the command-line loader is the load-bearing decision: the editor and the command-line tools agree on schema-version semantics because they share one loader, not two parallel ones.

---

## Compare-and-swap save on a content identity token

**Affects**: the write-path concurrency guard for the domain model; the content identity token derived at load time; the conflict-detection contract shared with the harness error envelope

**Behavior**: The load response carries a content identity token derived from the on-disk file's raw bytes at load time. A save must present the token from its originating load. When the presented token matches the current on-disk bytes, the save writes the file; when it does not match — because a second browser tab, a hand-edit, or a command-line regeneration changed the file since load — the save writes nothing and fails with the harness conflict envelope carrying both the current and the attempted token, so the editor can prompt reload-and-reapply. A save never merges: on conflict the editor reloads and the designer reapplies.

**Invariants**:
- Every load response carries a content identity token derived from the on-disk file bytes at load time.
- A save presenting a token that no longer matches the on-disk bytes writes nothing and returns the conflict envelope with both the current and the attempted token.
- A save never merges an editor draft into concurrently-changed on-disk content.

**Source**: @domain-model-editor-mvp/domain-model-tool-subsystem-and-persistence-api

**Backward-Compatible**: yes

**Notes**:
- The editor is one writer among several (hand-editing, command-line regeneration); compare-and-swap is what keeps it from silently clobbering a change the designer never saw.

---

## Deterministic serialization and deprecated-operations passthrough

**Affects**: the write-path serialization contract; declaration-order preservation; structural passthrough of the deprecated operations field

**Behavior**: Serialization is deterministic — saving the same in-memory model twice produces byte-identical files, with a stable key order and the declaration order of enums, entities, fields, and values preserved exactly as the designer arranged them. The deprecated operations field, when present in the loaded file, is carried through load and save structurally unchanged: the editor exposes no path that can mutate, reorder, or drop its entries, and a save that touched only an unrelated entity leaves the operations block byte-for-byte identical.

**Invariants**:
- Two saves of the same in-memory model produce byte-identical files, including a stable key order and preserved declaration order of enums, entities, fields, and values.
- A deprecated operations field present at load survives the next save structurally unchanged; no editor code path mutates, reorders, or drops it.

**Source**: @domain-model-editor-mvp/domain-model-tool-subsystem-and-persistence-api

**Backward-Compatible**: yes

**Notes**:
- The deprecated operations field is preserved on disk until the designer migrates it via parlay migrate-domain-operations; the editor's job is to not be the thing that loses it.

---

## Blocking domain-edit invocation contract

**Affects**: the domain-edit subcommand dispatch; the blocking-until-shutdown invocation contract consumed by agent hooks; the graceful-exit and boot-failure exit-code partition

**Behavior**: The binary gains a domain-edit subcommand that runs the identical boot sequence as the bare invocation — same harness, same registered tool route groups, same lifecycle — differing only in the browser-open target, which lands on the editor route `/domain-model` instead of the root. The invocation blocks until graceful shutdown and exits zero on every graceful path (the editor's Done control firing explicit shutdown, the idle timeout firing, or a termination signal). Non-zero exits remain reserved for boot-step failures, so a hook can branch on "the session ran" versus "Studio failed to start". Process exit is the sole completion signal — v1 defines no side-channel file, socket, or exit-code semantics for how the session ended; a caller that needs to know whether the model changed compares the file before and after the invocation. The idle timeout stays armed during a domain-edit session so an abandoned browser tab cannot block a waiting agent forever. The bare invocation is unchanged and remains a valid way to reach the editor; domain-edit is an entry-point and blocking-contract convenience, not a separate server mode.

**Invariants**:
- The domain-edit subcommand runs the same boot sequence and mounts the same registered tools as the bare invocation; only the browser-open target differs (`/domain-model`).
- The invocation blocks until graceful shutdown and exits zero on every graceful path; non-zero exits occur only on boot-step failures, before any browser is opened.
- Process exit is the sole completion signal; no side-channel encodes how the session ended or whether the model changed.
- The idle timeout remains armed for the duration of a domain-edit session.
- The bare invocation continues to boot the same server and reach the editor unchanged.

**Source**: @domain-model-editor-mvp/blocking-domain-edit-invocation-for-agent-handoff

**Backward-Compatible**: yes

**Notes**:
- The completion signal is deliberately the lowest common denominator every process runner observes — the command exiting — so the agent-side handoff is portable across hook runners.

---

## Embedded UI bundle and stack pin

**Affects**: the build-time embedding of the Studio UI bundle into the binary; the harness UI-bundle contract the bundle satisfies; the pinned presentation stack the editor is built on

**Behavior**: The Studio UI is built by its own build step and embedded into the binary at build time, so a released binary serves the UI with no runtime network fetches and no on-disk asset directory. Once the bundle is embedded, it satisfies the harness's UI-bundle contract and the harness serves it at the root path with single-page-app fallback; the placeholder 503 that names the missing-bundle condition no longer occurs. Client-side routes render within the shell via the harness's fallback, while requests under the `/api/` prefix are never intercepted by the client router. The presentation stack is the one the registered Studio adapter pins — React, Vite, Radix, and Tailwind — and the editor's forms and save flow follow that adapter's entity-form-panel, save-bar, and toast-async-feedback compositions rather than inventing parallel patterns.

**Invariants**:
- A released binary serves the UI with no runtime network fetches and no on-disk asset directory.
- Once the bundle is embedded, the root path serves it and the missing-bundle 503 no longer occurs.
- Client-side routes render within the shell; requests under `/api/` are never served the single-page-app fallback.
- The editor is built on the stack the registered Studio adapter pins (React, Vite, Radix, Tailwind).

**Source**: @domain-model-editor-mvp/editor-ui-shell-as-the-first-embedded-ui-bundle-consumer

**Backward-Compatible**: yes

**Notes**:
- This is the feature the harness's missing-bundle placeholder was waiting on — it turns the root path from a 503 into the working editor shell.
