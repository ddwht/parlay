# Figma-mcp-client — Dialogs

---

### Adopt the official Go MCP SDK as the client library

**Trigger**: A Studio maintainer adds or changes code that touches the MCP client, or opens a PR that introduces MCP traffic.

User: Adds `github.com/modelcontextprotocol/go-sdk` to `studio/go.mod` at an exact version (no `^` or `~` range semantics).
User: Implements the wrapper under `studio/internal/mcpclient/`, importing the SDK and exposing one named method per supported Figma tool (no generic `CallTool(name, args)` passthrough).
System (background): CI runs the import-boundary grep across `studio/` for `modelcontextprotocol/go-sdk` and confirms matches occur only inside `studio/internal/mcpclient/`.
System (background): CI parses `go.mod` and confirms the SDK is pinned at an exact version.
System: PR passes the MCP-client boundary checks. The wrapper's named methods are now the only Studio-side path to Figma MCP.

#### Branch: Maintainer hand-rolls a JSON-RPC client

User: Writes a JSON-RPC-over-stdio client by hand inside `studio/internal/mcpclient/` instead of importing the SDK.
System (background): Review identifies the hand-rolled client.
System: Change rejected with citation to this intent. The official `modelcontextprotocol/go-sdk` is the only acceptable MCP client library in v1. Either delete the hand-rolled code and use the SDK, or open a v2-or-later spec revision against this intent.

#### Branch: Direct SDK import outside the wrapper

User: Imports `github.com/modelcontextprotocol/go-sdk/mcp` from a Studio package other than `studio/internal/mcpclient/`.
System (background): CI's import-boundary grep finds the offending import.
System: CI fails. The SDK may only be imported from `studio/internal/mcpclient/`; route the call through a named wrapper method instead.

#### Branch: SDK version pinned with non-exact semver

User: Updates `studio/go.mod` to `github.com/modelcontextprotocol/go-sdk ^1.==minor==.0` (caret or tilde range).
System (background): CI parses the version spec and finds a range, not an exact pin.
System: CI fails. The SDK version must be pinned exactly so spec revisions don't reach Studio without a deliberate bump.

#### Branch: SDK version bumped alongside unrelated changes

User: Opens a PR that bumps `modelcontextprotocol/go-sdk` from `v1.==old==` to `v1.==new==` alongside unrelated changes.
System (background): Review notes the SDK version bump is in a multi-purpose PR.
System: Change rejected on process. SDK version bumps require a separate PR with an explicit changelog review against the new release; split the version bump into its own PR.

#### Branch: PR proposes switching to an alternative MCP library

User: Opens a PR replacing `github.com/modelcontextprotocol/go-sdk` with `github.com/mark3labs/mcp-go` (or another alternative), citing a perceived issue with the official SDK.
System: Change rejected as an in-v1 escape hatch. If the official SDK has a blocking issue, open a v2-or-later spec revision against this intent first; do not switch libraries inside v1.

---

### Require Figma's remote MCP server, not the desktop variant

**Trigger**: Studio starts up with a configured Figma MCP endpoint.

User: Starts Studio with a remote endpoint URL (e.g. `STUDIO_FIGMA_MCP_URL=https://mcp.figma.com/==path==`) and valid auth for a Dev or Full Figma seat.
System (background): `studio/internal/mcpclient/` invokes the `whoami` tool against the configured endpoint.
System (background): `whoami` returns `{"email": "==maintainer-email==", "plans": [{"key": "org_==id==", "name": "==Org name==", "seat": "Dev", "tier": "==Plan tier=="}]}`. Studio inspects `plans[]` and finds at least one entry with `seat` equal to `Dev` or `Full`.
System: Studio startup succeeded. Connected to remote MCP server at ==endpoint==; authenticated as ==maintainer-email== with seat ==Dev or Full==.

#### Branch: Desktop endpoint configured

User: Starts Studio with `STUDIO_FIGMA_MCP_URL` pointing at Figma's desktop-app MCP server (e.g. `http://localhost:==port==`).
System (background): The `whoami` response, or the endpoint shape itself, matches Figma's desktop-variant signature.
System: Studio startup failed with `figma-mcp-endpoint-unsupported`. The configured MCP endpoint is the Figma desktop variant; Studio requires the remote server because canvas-write capability is only available there. See `studio/docs/figma-mcp-setup.md`.

#### Branch: Remote endpoint unreachable

User: Starts Studio with a remote endpoint URL that does not respond (DNS failure, network drop, transport-level error, or server returns no MCP response).
System (background): The `whoami` invocation fails before receiving an MCP-level response.
System: Studio startup failed with `figma-mcp-endpoint-unreachable`. The configured MCP endpoint did not respond (==transport error detail==). See `studio/docs/figma-mcp-setup.md`.

#### Branch: Remote endpoint reachable but seat lacks canvas-write

User: Starts Studio with a valid remote endpoint; the authenticated Figma account is on a View or Collab seat.
System (background): `whoami` returns `{"email": "==maintainer-email==", "plans": [{"seat": "View", ...}]}` (or similar — no `plans[]` entry has `seat` of `Dev` or `Full`).
System: Studio startup failed with `figma-mcp-seat-insufficient`. Your Figma seat does not include canvas-write capability — `Dev` or `Full` is required. See `studio/docs/figma-mcp-setup.md`.

---

### Pin the bounded Figma MCP tool surface Studio is allowed to call

**Trigger**: A Studio caller invokes a method on `studio/internal/mcpclient/`, or a PR proposes adding a new wrapper method.

User: Calls `mcpclient.UseFigma(ctx, ==typed args==)` to instantiate a design-system component into the active frame.
System (background): Wrapper translates the typed args, invokes the SDK's `tools/call` with name `use_figma`, receives the SDK response, and translates it back into Studio-domain typed-tree types.
System: Returns the typed result to the caller. The SDK boundary stays inside `studio/internal/mcpclient/`.

#### Branch: PR adds a wrapper method for `get_design_context`

User: Opens a PR adding `func (c *Client) GetDesignContext(...)` to `studio/internal/mcpclient/` and routing it to the SDK's `tools/call` for `get_design_context`.
System (background): Review compares the added method against this intent's excluded list.
System: Change rejected. `get_design_context` is excluded in v1 — its React+Tailwind output flattens variants into CSS classes and breaks the component-identity round-trip Studio depends on. Adding it requires a spec change against this intent, not a quiet code change.

#### Branch: Defensive check — wrapper rejects an unknown tool name

User: A unit test invokes the wrapper's internal dispatch with a tool name not on the supported list (e.g. `"get_design_context"`).
System (background): Wrapper compares the requested tool name against the enumerated supported list inside `studio/internal/mcpclient/`.
System: Wrapper returns `figma-mcp-tool-unsupported`; the test asserts this rejection. (The SDK itself would happily call the tool — the wrapper is the enforcement boundary.)

#### Branch: PR adds a wrapper method for an unsupported tool

User: Opens a PR adding `func (c *Client) GetScreenshot(...)` (or any other Figma tool outside the supported list) to `studio/internal/mcpclient/`.
System (background): Review compares the added method against the enumerated supported tool list in this intent.
System: Change rejected. The supported tool subset is enumerated in this intent; adding `get_screenshot` requires a spec change here, not a code-only PR.

#### Branch: PR proposes switching the write entry from `use_figma` to `generate_figma_design`

User: Opens a PR replacing the `UseFigma` wrapper method with a `GenerateFigmaDesign` method, citing a perceived limitation in `use_figma`.
System: Change rejected as an in-v1 escape hatch. `use_figma` is the canonical v1 write entry point; switching to `generate_figma_design` is a v2-or-later spec revision against this intent, not a runtime fallback.

---
