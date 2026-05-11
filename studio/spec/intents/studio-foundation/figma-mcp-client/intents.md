# Figma-mcp-client

> Studio talks to Figma exclusively through Figma's official MCP server. This feature pins the Go MCP client library Studio depends on, the Figma server variant Studio requires, and the bounded subset of Figma's MCP tool surface Studio is allowed to call. The point is to lock these choices before Phase 0 begins so the spike measures the right thing — round-trip fidelity through the chosen tools — instead of relitigating which tools to use.

---

## Adopt the official Go MCP SDK as the client library

**Goal**: Use `github.com/modelcontextprotocol/go-sdk` (the official MCP project + Google Go SDK) as the only MCP client implementation in Studio. No hand-rolled JSON-RPC, no fork, no parallel client wrapper. Studio imports the SDK's `mcp.Client` directly and wraps it only with a thin Studio-side adapter that translates SDK responses into Studio's typed-tree types.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: Earlier framing of Studio assumed the Go MCP ecosystem was thin and that Studio would either hand-roll a JSON-RPC-over-stdio client or wrap an unofficial library. That assumption no longer holds: the official `modelcontextprotocol/go-sdk` is jointly maintained by the MCP project and Google, supports the client side of MCP including stdio transport, and has reached a stable release suitable for adoption. Pinning to it removes a Phase 0 risk (custom client correctness) and removes a long-tail maintenance burden (tracking spec revisions ourselves).

**Action**: Add `github.com/modelcontextprotocol/go-sdk` to `studio/go.mod` at the latest stable release. Build a single Studio-internal package (`studio/internal/mcpclient/`) that owns the SDK import, exposes only the verbs Studio needs (see the tool-surface intent below), and shields callers from SDK-version churn. Disallow direct SDK imports from anywhere else in Studio. Pin the SDK version in `go.mod` exactly (no caret/tilde semantics) so spec revisions don't reach Studio without a deliberate bump.

**Objects**: mcp-client-sdk, mcpclient-package, sdk-version-pin, studio-side-adapter

**Constraints**:
- The official `modelcontextprotocol/go-sdk` is the only acceptable MCP client library in v1. Alternatives (e.g. `mark3labs/mcp-go`) were considered and rejected for v1; switching to an alternative is a v2-or-later spec revision, not a runtime or v1 escape hatch.
- All Studio MCP traffic flows through the `studio/internal/mcpclient/` package; direct SDK imports elsewhere fail review
- The SDK version is pinned exactly in `go.mod`; bumping requires a separate PR with an explicit changelog review
- The wrapper exposes only the bounded Figma tool surface (next intent); arbitrary `tools/call` passthrough is disallowed

**Verify**:
- `studio/go.mod` declares `github.com/modelcontextprotocol/go-sdk` at an exact version
- A grep across `studio/` for `modelcontextprotocol/go-sdk` returns matches only inside `studio/internal/mcpclient/`
- A unit test confirms the wrapper rejects calls to unsupported Figma tool names

---

## Require Figma's remote MCP server, not the desktop variant

**Goal**: Studio targets Figma's **remote** MCP server. The desktop-app server variant is not supported because canvas-write capability — required by Studio's Design Loop — is only available on the remote server. Configuration enforces this: Studio fails fast at startup if the configured MCP endpoint resolves to the desktop server.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: Figma ships two MCP server variants. The local/desktop server is read-only and intended for IDE-side context extraction. The remote server is the one that exposes canvas-write tools (`use_figma`, `generate_figma_design`, `add_code_connect_map`). Studio's Design Loop is fundamentally a write operation — Studio must instantiate design-system components into a fresh frame — so the desktop variant is not a viable target. Pinning this in the intent prevents a Phase-0-time discovery that the wrong server was wired up.

**Action**: In `studio/internal/mcpclient/`, accept only the remote server endpoint shape (HTTPS URL, OAuth or token auth flow as Figma's docs specify). At Studio startup, probe the endpoint with the `whoami` tool and reject the configuration with a stable error code if the response indicates a desktop/local variant or if no entry in the `whoami` response's `plans[]` array has a `seat` of `Dev` or `Full` (the seats Figma grants canvas-write capability). The error message links to the Studio-side setup doc at `studio/docs/figma-mcp-setup.md`, which in turn links out to Figma's docs for Figma-side prereqs.

**Objects**: figma-mcp-endpoint, remote-server-required, desktop-server-rejected, startup-probe

**Constraints**:
- The remote server is the only supported endpoint variant in v1
- Stable error codes: `figma-mcp-endpoint-unsupported` covers "endpoint is the desktop variant" (configuration error); `figma-mcp-endpoint-unreachable` covers "endpoint is the remote variant but does not respond" (network or availability error); `figma-mcp-seat-insufficient` covers "endpoint reachable but no `plans[].seat` is `Dev` or `Full`"
- Authentication uses whichever flow Figma's remote server documents; Studio does not invent its own auth
- The probe runs once at Studio startup, not on every tool call
- The canonical setup doc lives at `studio/docs/figma-mcp-setup.md` (Studio-side; itself links out to Figma's docs for Figma-side prereqs); every stable error code from this intent's probe references this doc

**Verify**:
- Configuring Studio with a desktop-server endpoint fails startup with `figma-mcp-endpoint-unsupported`
- Configuring Studio with a remote endpoint that does not respond (e.g. unreachable host, transport error) fails startup with `figma-mcp-endpoint-unreachable`
- Configuring Studio with a valid remote endpoint but a View/Collab seat fails with `figma-mcp-seat-insufficient`
- A successful startup logs the resolved endpoint, the user's email, and the seat tier (`Dev` or `Full`) from the `whoami` response's `plans[].seat` field (without leaking auth secrets)

---

## Pin the bounded Figma MCP tool surface Studio is allowed to call

**Goal**: Studio uses a small, named subset of Figma's 16-tool MCP surface. The write path goes through `use_figma`. The read-back path goes through `get_metadata` and `get_code_connect_map`, **not** through `get_design_context`. The tool subset is enumerated in the wrapper and any attempt to extend it is a deliberate spec change, not a casual addition.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: Figma's MCP exposes both a structural tool surface and a "render to React + Tailwind" surface (`get_design_context`). The React+Tailwind path loses component identity — variants become CSS classes, Code Connect bindings flatten into class names, and round-trip fidelity collapses. Studio's typed-tree round-trip depends on stable component identity through the loop, so Studio reads structure via `get_metadata` (sparse XML of properties) and identity via `get_code_connect_map` (node-to-Code-Connect-component mapping). On the write side, `use_figma` is Figma's general-purpose create/edit tool and is the entry point for instantiating design-system components into a fresh frame. Enumerating this subset in the wrapper turns Phase 0's question into a concrete one — *does this exact toolchain round-trip a Tasks screen?* — instead of an open-ended exploration of Figma's tool catalog.

**Action**: Define the supported tool list inside `studio/internal/mcpclient/`:
- **Write**: `use_figma` (component instantiation, layout edits, frame creation; the canonical v1 write entry point), `add_code_connect_map` and `send_code_connect_mappings` (Studio establishes Code Connect bindings itself), `create_new_file` (ephemeral frame container creation)
- **Read**: `get_metadata` (structural read-back), `get_code_connect_map` (identity resolution), `get_code_connect_suggestions` (identity inference for new nodes)
- **Probe**: `whoami` (startup verification)
- **Explicitly excluded in v1**: `get_design_context` (lossy React+Tailwind output), `get_screenshot` (visual-only, not load-bearing for the loop), `generate_diagram` / `get_figjam` (FigJam, out of scope), `create_design_system_rules` / `search_design_system` (authoring helpers, out of scope), `generate_figma_design` (overlaps with `use_figma`; `use_figma` is the chosen v1 write entry — switching to `generate_figma_design` is a v2-or-later spec revision if `use_figma` proves inadequate, not a runtime fallback)

Each tool gets a named wrapper method with typed inputs and outputs. The wrapper rejects calls to any tool name outside the supported list with `figma-mcp-tool-unsupported`.

**Objects**: figma-tool-surface, write-tools, read-tools, excluded-tools, get-design-context-rejection

**Constraints**:
- The supported tool subset is enumerated in code; adding a tool is a spec change in this intent, not a quiet code change
- `get_design_context` is rejected in v1 with a comment in the wrapper citing this intent and the React+Tailwind identity-loss reason
- `use_figma` is the canonical v1 component-instantiation entry point. `generate_figma_design` is excluded in v1; switching to it is a v2-or-later spec revision, not a runtime escape hatch.
- Studio establishes Code Connect bindings via `add_code_connect_map` / `send_code_connect_mappings` and uses `get_code_connect_suggestions` for identity inference on new nodes; these tools are supported in v1, not deferred
- The wrapper's typed input/output structs live in `studio/internal/mcpclient/` and are translated into Studio-domain types (typed tree nodes) by callers, not by the wrapper itself — keeps the SDK boundary clean

**Verify**:
- The wrapper exposes exactly the methods listed above (one per supported tool)
- A unit test asserts the wrapper rejects `get_design_context` with `figma-mcp-tool-unsupported` even though the SDK would happily call it
- A grep for `get_design_context` across `studio/` returns matches only in this intent and in the wrapper's rejection list (with comment)

---
