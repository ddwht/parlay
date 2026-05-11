# Figma-mcp-client — Infrastructure

---

## Official MCP SDK adoption and import boundary

**Affects**: MCP client integration in Studio; dependency manifest; import-boundary enforcement
**Behavior**: All MCP traffic from Studio routes through a single internal client package that owns the import of the official MCP SDK. The package exposes one named method per supported Figma tool — there is no generic tool-name passthrough. The dependency manifest pins the SDK at an exact version, with no range or wildcard semantics. The build pipeline enforces that the SDK import appears only inside the dedicated client package and that the version pin is exact. Hand-rolled MCP client implementations and substitutions of alternative MCP libraries are rejected on review.
**Invariants**:
- Every MCP-related import in Studio resolves to the official MCP SDK; alternative MCP libraries are rejected on review
- The official MCP SDK is imported only from the dedicated client package; any other import location fails the boundary check
- The SDK version pin uses exact semantics; range or wildcard version specs fail the boundary check
- The client wrapper exposes only named methods, one per supported Figma tool; a generic name+args passthrough is rejected on review
- Hand-rolled MCP client implementations inside the dedicated client package are rejected on review with a citation to the official-SDK rule
- An SDK version bump combined with unrelated changes is rejected on review; bumps require a dedicated change with explicit changelog review
**Source**: @figma-mcp-client/adopt-the-official-go-mcp-sdk-as-the-client-library
**Backward-Compatible**: yes

**Notes**:
- Foundational change — there is no prior MCP client in Studio to maintain compatibility with
- The escape hatch for an alternative MCP client library is a v2-or-later spec revision, not a runtime fallback or same-version hedge

---

## Figma MCP server startup probe with stable failure modes

**Affects**: Studio startup configuration; MCP endpoint validation; stable error-code surface for setup failures; canonical setup-doc reference
**Behavior**: At Studio startup, the MCP client invokes Figma's identity tool (`whoami`) against the configured endpoint and inspects the response. Only a remote-server endpoint shape is accepted; configurations that resolve to the desktop/local-server variant are rejected. The response's plan list is inspected for a seat tier that grants canvas-write capability; configurations whose authenticated user does not have such a seat are rejected. Endpoints that fail at transport level (no response, network error) are distinguished from configuration-shape errors. Three stable error codes name the three failure modes. All error messages link to a Studio-owned setup doc at a canonical Studio-side path; that doc in turn links out to Figma's documentation for upstream prerequisites. The probe runs once at Studio startup, not on every subsequent tool call.
**Invariants**:
- A configured MCP endpoint matching the desktop-server signature fails startup with `figma-mcp-endpoint-unsupported`
- A configured MCP endpoint that fails to respond at the transport layer fails startup with `figma-mcp-endpoint-unreachable`
- A `whoami` response whose plan list contains no seat tier granting canvas-write capability fails startup with `figma-mcp-seat-insufficient`
- A successful probe records, in order, the resolved endpoint, the authenticated user's email, and the canvas-write seat tier; authentication secrets do not appear in the recorded output
- The probe invokes `whoami` exactly once per Studio startup, regardless of how many subsequent tool calls Studio makes
- Every stable error code emitted by the probe links to the canonical Studio-side setup doc
**Source**: @figma-mcp-client/require-figmas-remote-mcp-server-not-the-desktop-variant
**Backward-Compatible**: yes

**Notes**:
- The seat-tier check inspects Figma's identity-tool response, which surfaces plan entries each carrying a seat field; the canvas-write-capable seats are the named subset (`Dev` and `Full`)
- The authentication mechanism is delegated to Figma's remote-server documentation; Studio does not invent its own auth flow
- The Studio-side setup doc itself is a separate deliverable; this fragment pins only that error messages must link to it

---

## Bounded Figma MCP tool surface with enumerated wrapper methods

**Affects**: MCP client wrapper API; tool-call enforcement boundary; spec-change gating for tool-list extensions
**Behavior**: The client wrapper enumerates a bounded set of Figma MCP tools and exposes a named method for each. Supported tools include the canonical write tool (`use_figma`) for component instantiation and layout edits, the file-creation tool (`create_new_file`), the two Code Connect binding tools (`add_code_connect_map`, `send_code_connect_mappings`), the structural read tool (`get_metadata`), the Code Connect identity tools (`get_code_connect_map`, `get_code_connect_suggestions`), and the startup probe tool (`whoami`). The wrapper explicitly excludes Figma's lossy React+Tailwind context tool (`get_design_context`), screenshot and FigJam tools, design-system authoring helpers, and the alternative write tool (`generate_figma_design`). Calls to any tool not on the supported list are rejected with a stable error code; the underlying SDK would accept them, but the wrapper is the enforcement boundary. Adding a tool to the supported list requires editing the spec, not a code-only change. Wrapper input and output types live alongside the wrapper and are translated into Studio-domain types by callers, not by the wrapper itself.
**Invariants**:
- The wrapper exposes exactly one named method per tool in the supported set
- A request to invoke a tool name outside the supported set is rejected with `figma-mcp-tool-unsupported`, even though the underlying SDK would accept the call
- Figma's lossy React+Tailwind context tool (`get_design_context`) has no wrapper method; any code change that adds one fails review with a citation to this fragment
- A code change that adds a wrapper method for any excluded tool fails review with a citation to the supported-set list
- A code change that proposes replacing the canonical write entry (`use_figma`) with the alternative (`generate_figma_design`) is rejected on review; the switch is reserved for a v2-or-later spec revision
- Wrapper input and output types do not reference Studio-domain entity types; translation between wrapper types and domain types happens at the caller boundary
**Source**: @figma-mcp-client/pin-the-bounded-figma-mcp-tool-surface-studio-is-allowed-to-call
**Backward-Compatible**: yes

**Notes**:
- The supported set comprises one canonical write entry, one file-creation tool, two Code Connect binding tools, two structural-read tools, one Code Connect identity-inference tool, and one identity-probe tool
- The exclusion of the React+Tailwind context tool is load-bearing: it preserves component identity through Studio's typed-tree round-trip
- Code Connect bindings are established by Studio itself (not assumed to pre-exist in the Figma file), which is why all three Code Connect tools are part of the supported set

---
