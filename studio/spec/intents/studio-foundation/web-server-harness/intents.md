# Web-server-harness

> Studio's binary entry point: an ephemeral, on-demand HTTP server that boots the Studio tools (Domain Model Editor, Design Loop), authenticates against Figma's MCP server, and shuts down when work is complete. This feature pins the binary's startup sequence (config → project root → MCP probe → server → browser → block), the chi-based HTTP server harness that mounts each tool's route group, the idle-timeout lifecycle (no HTTP requests for the duration → graceful shutdown), and the Phase 0 MCP wiring that fills in the figma-mcp-client wrapper's currently-stubbed transport so Studio can actually talk to Figma. After this feature ships, `parlay-studio` is a runnable binary that boots cleanly against a real Figma remote MCP server.

---

## Studio binary startup sequence and lifecycle

**Goal**: Define the boot ordering, the failure-mode handling at every boot step, and the graceful-shutdown triggers for the Studio binary. Pin the startup sequence so an operator (or Core's CLI hook) can reason about exactly which step a failure happened at and what to do about it.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: Studio is a single-binary, ephemeral process — it starts when invoked, serves its tools, and exits on idle / signal / explicit close. The boot path has dependency ordering: project-root resolution must run before config loading (the project-scoped config path depends on the resolved root); config loading must run before the MCP probe (the probe needs the endpoint URL and the auth token); the MCP probe must run before the HTTP server starts (Studio without canvas-write capability cannot do its job, so probe failures fail fast before any port is bound or browser is opened); the HTTP server must start before the browser is opened (the browser needs a live URL to point at); signal handlers and the idle-timeout timer must be installed last (so they only trigger after a clean boot). Each step has a known failure mode with a stable code, already pinned by the upstream feature (studio-config, figma-mcp-client) — this intent composes them into a single boot orchestration. Graceful shutdown collapses three triggers (signal, idle-timeout fired, `/api/shutdown` received) into one shutdown path: stop accepting new HTTP connections, drain in-flight requests within a short window, close the MCP client, log the shutdown reason, exit zero.

**Action**: Implement `studio/cmd/parlay-studio/main.go` as the entry point. The boot sequence runs the steps in the listed order; each step's failure mode terminates the process with a non-zero exit code naming the upstream stable code in the log. After the chi router is built and the HTTP server is started, the browser-open hook fires (if `OpenBrowser=true`), signal handlers are installed for SIGINT and SIGTERM, and the idle-timeout goroutine (if enabled) is launched. The main goroutine then blocks on a shutdown channel that any of the three shutdown triggers can close. The graceful-shutdown sequence is wrapped in a short deadline (e.g. 5 seconds) so a misbehaving handler cannot prevent process exit.

**Objects**: studio-binary-entry-point, boot-sequence, boot-step-ordering, hard-gate-mcp-probe, shutdown-triggers, graceful-shutdown-sequence, shutdown-reason-log

**Constraints**:
- Boot steps execute in a fixed order: (1) parse command-line flags, (2) resolve project root, (3) load and log redacted merged configuration, (4) probe Figma MCP server, (5) construct authenticated MCP client, (6) build chi router with mounted tool route groups, (7) start HTTP server on the resolved port, (8) log the bound URL, (9) open the operator's browser if `OpenBrowser=true`, (10) install signal handlers, (11) start idle-timeout goroutine if `IdleTimeout > 0`, (12) block on the shutdown channel
- Any boot step's failure terminates the process with a non-zero exit code and a structured log line naming the failing step and the upstream stable code (e.g. `studio-config-figma-mcp-url-missing`, `figma-mcp-endpoint-unsupported`, `studio-config-server-port-conflict`)
- The MCP probe is a hard gate: a probe failure terminates the process before any port is bound, before any browser is opened, and before any signal handler is installed; partial-boot states are never observable from outside the process
- Graceful shutdown is triggered by exactly three events: SIGINT or SIGTERM received, the idle-timeout firing (when enabled), or an authenticated `/api/shutdown` request. The three triggers feed a single shutdown channel; the same shutdown sequence runs regardless of which trigger fired
- Graceful shutdown stops accepting new HTTP connections, drains in-flight requests with a bounded 5-second deadline (hard-coded; handlers exceeding this deadline are aborted by the http.Server.Shutdown call), closes the MCP client, emits a single INFO log line naming the shutdown reason (signal / idle / explicit), and exits zero
- The HTTP server binds to `127.0.0.1` only (never `0.0.0.0` or any external interface); the localhost bind is the trust boundary that lets `/api/shutdown` skip authentication — only local processes can reach it, and on a single-user developer machine that is sufficient
- The process never exits non-zero on a graceful-shutdown path; non-zero exits are reserved for boot-step failures and panics that escape the harness's recovery middleware

**Verify**:
- A unit test runs the boot sequence with a fixture that injects a `studio-config-figma-mcp-url-missing` failure at step 3 and asserts the process exits non-zero with that code in the log, no port is bound, no browser is opened
- A unit test injects a `figma-mcp-endpoint-unsupported` failure at step 4 (MCP probe) and asserts the process exits non-zero with that code, no port is bound, and the chi router was never built
- A unit test runs a successful boot, sends SIGTERM, and asserts the shutdown reason "signal: SIGTERM" appears in the log; the process exits zero; the HTTP server is no longer listening
- A unit test runs a successful boot, fires the idle-timeout trigger via the test hook, and asserts the shutdown reason "idle: no requests for 30m0s" appears in the log and the process exits zero
- A unit test runs a successful boot, posts to `/api/shutdown`, and asserts the shutdown reason "explicit: /api/shutdown" appears in the log and the process exits zero
- A unit test confirms the HTTP server binds only to `127.0.0.1` by attempting to dial the server from a non-loopback interface in the test environment and asserting connection refusal

---

## HTTP server harness — chi router, middleware, error envelopes

**Goal**: Pin the HTTP server's structure that every Studio tool's handlers plug into. Define the chi router setup, the middleware stack (request IDs, panic recovery, error-envelope translation, logging), the route-group mount points reserved for each Studio tool subsystem, the universal endpoints (`/api/health`, `/api/shutdown`), and the SPA fallback for the UI bundle.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: Studio's HTTP surface is intentionally small but consistent. The architecture proposal (§7) pins chi as the router with one route group per tool subsystem; the go-studio-app adapter pins the request-ID convention, the JSON-only content type, and the error-envelope shape per closed-vocabulary error kind. Each Studio tool (Domain Model Editor, Design Loop, plus future tools in Phase 4+) owns its handlers; the harness owns the wiring. Two universal endpoints exist outside any tool's group: `/api/health` for liveness probes (used by Core's hook, by health-monitoring scripts, and by integration tests) and `/api/shutdown` for the explicit-close shutdown trigger. The root path `/` serves the embedded UI bundle with SPA fallback semantics (any path not matched by a more specific route serves the UI's `index.html` so client-side routing works) — when the UI bundle is not yet built (a future feature owns it), the root path returns a 503 placeholder so operators see a clear message rather than a generic 404. The middleware stack runs in a fixed order: request-ID assignment first (so every subsequent middleware sees the ID), then panic recovery (so a panic in a handler doesn't kill the process), then idle-timeout reset (so request activity keeps Studio alive), then logging (so the log line includes the request ID, status, and duration), then error-envelope translation at the handler boundary.

**Action**: Implement `studio/internal/server/` as the harness package. Expose `func New(cfg config.Config, mcp *mcpclient.Client, register ToolRegistration) *Server` that builds the chi router with the middleware stack installed, mounts the two universal endpoints, mounts each registered tool's route group at its declared path, and serves the UI bundle at `/`. Define `ToolRegistration` as a typed interface or function value that each tool's feature implements; the harness invokes the registration callback once per tool during construction so handlers can be attached without the harness package importing the tool packages directly (avoids an import cycle between server and tool packages). The error-envelope translation middleware uses `errors.Is` and `errors.As` against the closed-vocabulary error kinds (validation-failed → 400 with {code, fields}; not-found → 404 with {code, target}; conflict → 409 with {code, current_etag, attempted_etag}; server-error → 500 with {code, request_id}); unmapped errors fall through to server-error with the request ID surfaced to the operator's log but not the response.

**Objects**: chi-router, middleware-stack, request-id-middleware, panic-recovery-middleware, idle-timeout-reset-middleware, error-envelope-middleware, tool-route-group-mount-points, universal-health-endpoint, universal-shutdown-endpoint, spa-fallback, tool-registration-interface

**Constraints**:
- The chi router's middleware stack is installed in a fixed order: request-ID assignment, then panic recovery, then idle-timeout-reset on every request matching a `/api/*` path, then logging, then error-envelope translation at the handler boundary
- The middleware that resets the idle timer applies only to requests under `/api/*` AND excluding `/api/health` (health checks are passive observation, not active designer work; including them would keep Studio alive indefinitely when external monitoring is in place)
- Two universal route mounts are owned by the harness itself: `/api/health` returns 200 with a minimal JSON liveness envelope, and `/api/shutdown` triggers the explicit-close shutdown path documented in the lifecycle intent
- Each Studio tool subsystem registers itself via the harness's tool-registration interface; the harness invokes each registration callback during router construction. Each tool registers exactly ONE route group at one path prefix — a tool with a split surface (e.g. a public surface and a side-channel surface) attaches both routes under its single registered prefix rather than registering twice. The harness package does NOT import any tool package directly, and a tool package does NOT register routes anywhere except through the registration interface (review-enforced boundary)
- The root path `/` serves the embedded UI bundle with SPA fallback semantics — paths not matched by a `/api/*` route serve the UI's `index.html` — when the UI bundle is built and embedded. Until the UI bundle is built, the root path returns a 503 envelope with code `studio-ui-bundle-not-built` and a body that explains the UI is not yet packaged
- The error-envelope middleware maps the closed-vocabulary error kinds to status codes and envelope shapes per the application adapter's convention; unmapped errors translate to server-error with the request ID surfaced to the structured log (`request_id` field) but the request body returns only `{code: server-error, request_id: <id>}` — internal error detail never reaches the response
- Every HTTP response carries the request-ID header (`X-Request-ID`) and the request ID is the same value the log line records for the request

**Verify**:
- A handler test mounts a tool route group via the registration interface, fires a request matching the tool's path, and asserts the handler receives a request whose context carries a request ID
- A handler test injects a panic in a registered tool handler and asserts the panic-recovery middleware catches it, the response is 500 with `{code: server-error, request_id: <id>}`, and the panic detail appears in the log but NOT in the response
- A handler test fires a handler returning a `not-found` sentinel error and asserts the response is 404 with `{code: not-found, target: <named>}` and the request-ID header matches the logged request ID
- A unit test fires three requests under `/api/domain-model/*` and asserts the idle-timeout's "last activity" timestamp advances on each; fires three requests to `/api/health` and asserts the timestamp does NOT advance
- A unit test fires a request to `/some/unknown/path` (no matching `/api/*` route) with the UI bundle embedded and asserts the response is the UI's `index.html`; the same test with the UI bundle absent asserts the response is 503 with code `studio-ui-bundle-not-built`
- An integration test fires `POST /api/shutdown` against a running harness and asserts the harness initiates graceful shutdown (the lifecycle intent's trigger fires)
- A unit test invokes the tool-registration interface twice with the same tool name and asserts the second registration is rejected (one route group per tool is the enforced rule)

---

## Idle-timeout activity tracking

**Goal**: Define what "idle" means for Studio's auto-shutdown timer. Pin the semantics (last HTTP request received under `/api/*` excluding `/api/health`), the disabled sentinel (zero duration), and the timer's interaction with graceful shutdown. Resolves studio-config's deferred Q3.1 (idle-timeout source) and Q3.2 (OpenBrowser-false URL log).

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: studio-config's `IdleTimeout` configuration key is a duration with a 30-minute default and a zero-disables sentinel, but it does NOT define what counts as "idle" — that decision was deliberately deferred to this feature because the answer depends on which events the harness can observe. Three candidate definitions exist: last HTTP request received (simple, observable in middleware), last designer keystroke in the browser tab (requires the UI to ping a heartbeat endpoint, which couples the timeout to the UI bundle), and last mutation of an on-disk parlay artifact (couples the timeout to the file-I/O layer). The simplest definition that captures actual designer engagement is the HTTP request one: every request under `/api/*` except `/api/health` advances a "last activity" timestamp, and a goroutine periodically checks whether the elapsed time since last activity exceeds the configured timeout. The browser-keystroke definition is a Phase 4+ refinement when the UI bundle is in scope; for Phase 1 the request-based definition is sufficient and is what most local-server tools (jupyter, vite, parcel) use.

**Action**: Implement the idle-timeout lifecycle inside `studio/internal/server/`. A goroutine launched by the lifecycle intent's boot step 11 reads the current "last activity" timestamp at a fixed interval (e.g. every 10 seconds) and compares against the configured `IdleTimeout`; when `now - last-activity > IdleTimeout`, the goroutine pushes onto the shutdown channel with reason `idle: no requests for <duration>`. The "last activity" timestamp is updated by the idle-timeout-reset middleware from the HTTP harness intent. A zero `IdleTimeout` skips the goroutine entirely — no timer, no shutdown trigger from this source.

**Objects**: idle-activity-timestamp, idle-timeout-goroutine, idle-check-interval, last-activity-update-rule, idle-shutdown-trigger

**Constraints**:
- "Idle" means "no HTTP request received on a path under `/api/*` (excluding `/api/health`) for the configured `IdleTimeout` duration"; the harness does NOT track keystrokes, on-disk activity, or any other signal
- The "last activity" timestamp is updated by the idle-timeout-reset middleware from the HTTP harness intent; both updates and reads use a synchronization primitive (mutex or atomic) so the goroutine cannot observe a torn read
- The idle-check goroutine wakes every 10 seconds (hard-coded; not configurable via studio-config). Tests override the interval via a package-internal test hook that swaps the ticker, not via a configuration key. The 10-second granularity is the maximum lag between idle threshold being crossed and shutdown trigger firing; for a 30-minute default timeout this is acceptable
- A zero `IdleTimeout` skips the goroutine entirely — the harness MUST NOT launch the goroutine in that case, and there is no idle-shutdown trigger source under any condition
- The shutdown trigger reason is exactly `idle: no requests for <duration>` where `<duration>` is the resolved `IdleTimeout` formatted via Go's duration string format
- The goroutine's first check fires after the harness has been running for at least one `IdleTimeout` window (no false trigger on a freshly started harness with zero requests so far)

**Verify**:
- A unit test runs the harness with `IdleTimeout=100ms`, fires no requests, and asserts the idle shutdown trigger fires within 200ms with reason `idle: no requests for 100ms`
- A unit test runs the harness with `IdleTimeout=100ms`, fires one request to `/api/domain-model/test` at the 80ms mark, and asserts no shutdown trigger fires for at least another 100ms after the request
- A unit test runs the harness with `IdleTimeout=100ms`, fires repeated requests to `/api/health` every 50ms, and asserts the idle shutdown trigger fires (health checks do NOT reset the timer)
- A unit test runs the harness with `IdleTimeout=0` and asserts no idle-check goroutine is launched (the test inspects a launched-goroutines counter or a side channel the goroutine writes to on start)
- A unit test asserts the shutdown trigger reason string format exactly matches `idle: no requests for <duration>` with the duration as Go's standard format

---

## Figma MCP Phase 0 wiring

**Goal**: Fill in the four stubbed pieces of the figma-mcp-client wrapper (`New()`, `dispatch()`, `callTyped()`, plus per-tool input/output marshalling) so Studio can actually talk to Figma's remote MCP server. After this intent ships, `mcpclient.Probe()` is a real network call that returns real `WhoamiOutput` data from Figma; the 8 supported wrapper methods round-trip real Figma MCP traffic.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: figma-mcp-client (shipped) defined the wrapper's API surface and the import boundary but deliberately left the transport stubbed — its Context paragraph reads "The prototype's connection construction is intentionally minimal; Phase 0 wiring fills in the concrete transport against the live Studio harness." That Phase 0 work is THIS intent. The four stubs are: `New(ctx, endpoint)` currently returns `&Client{}` with the SDK client field never initialized; `dispatch(ctx, toolName, in)` returns an empty `map[string]any`; `callTyped[Out]` returns a zero-value `Out`; the per-tool wrapper methods (UseFigma, CreateNewFile, etc.) route through these stubs so they currently return zero-value outputs. Real Phase 0 wiring constructs a `mcp.Client` from the SDK against the remote HTTPS endpoint, attaches the bearer-token auth header on every call, marshals typed inputs into the SDK's `CallToolParams.Arguments`, calls the SDK's `tools/call` for the given tool name, and unmarshals the response into the typed output struct. The auth mechanism is token-based per studio-config's deliberate v1 scope. Per-tool marshalling depends on Figma's actual MCP tool schemas — each of the 8 supported tools has its own input/output JSON shape that the wrapper's typed structs must match. This intent is the only one in the studio-foundation initiative that explicitly produces network traffic against a real third-party service.

**Action**: Replace the four stubs in `studio/internal/mcpclient/` with real implementations against the v1.6.0 SDK API. `New(ctx, endpoint, token)` constructs `mcp.NewClient(impl, nil)` for the persistent client; constructs a `mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: <custom>}` where the custom `http.Client` wraps its `Transport` in a `RoundTripper` that injects `Authorization: Bearer <token>` on every outbound request; calls `client.Connect(ctx, transport, nil)` to obtain a `*mcp.ClientSession`; stores the session on the wrapper's `Client` struct for the process lifetime. `dispatch(ctx, toolName, in)` calls `session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: in})` (the SDK accepts `Arguments any` and handles JSON marshalling internally), checks `result.IsError` to surface MCP-level tool errors, and returns `result.StructuredContent` (the typed payload Figma's tools return). `callTyped[Out]` JSON-round-trips the `StructuredContent` value into the typed `Out` struct. Per-tool input/output structs are hand-maintained for v1 — each of the 8 supported tools gets explicit Go struct definitions whose JSON field tags match Figma's documented MCP tool schemas. Integration tests under `//go:build integration` exercise the real Figma remote server.

**Objects**: mcp-sdk-client-construction, streamable-client-transport, bearer-token-roundtripper, client-session-lifetime, dispatch-via-call-tool, calltyped-structured-content, per-tool-hand-maintained-schemas, integration-test-tag

**Constraints**:
- `New(ctx, endpoint, token)` instantiates the SDK in three steps that mirror the SDK's documented API: `mcp.NewClient(impl, nil)` for the persistent `*Client`; a `mcp.StreamableClientTransport{Endpoint, HTTPClient}` whose `HTTPClient.Transport` is a custom `RoundTripper` adding `Authorization: Bearer <token>` to every outbound request; `client.Connect(ctx, transport, nil)` to obtain a `*ClientSession`. The session is stored on the wrapper's `Client` struct and lives for the process lifetime
- The bearer-token `RoundTripper` is the sole mechanism by which the auth token reaches the network; the token is not written into the `Implementation` struct, the `ClientOptions`, or any logged field
- `dispatch(ctx, toolName, in)` invokes `session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: in})`; when the call returns `(result, nil)` with `result.IsError == true`, dispatch wraps the result's `Content` text into `figma-mcp-tool-call-failed` and returns; when the call returns `(_, err)` with a transport-level error, dispatch wraps with `figma-mcp-endpoint-unreachable` (matching figma-mcp-client's stable code); on success dispatch returns `result.StructuredContent`
- `callTyped[Out]` reads the `StructuredContent any` from dispatch, JSON-marshals it, then unmarshals into the typed `Out`; unmarshal failures wrap with `figma-mcp-response-malformed` naming the tool and the offending field
- Per-tool wrapper input and output structs are hand-maintained for v1 — each of the 8 supported tools has explicit Go struct definitions in `studio/internal/mcpclient/tools.go` whose JSON tags match Figma's documented MCP tool schemas. Schema drift between Figma's MCP server and the wrapper structs is detected by integration tests, not by codegen or build-time validation. Codegen from Figma's published schemas is reserved for a later feature once Figma's schema publication story is clearer
- Auth credential handling is constrained by figma-mcp-client's existing secret invariants — the token never appears in logs, HTTP responses, or persisted disk writes
- A `//go:build integration` test suite under `studio/internal/mcpclient/` covers, against a real Figma remote MCP server: a successful Whoami probe returning a Dev or Full seat, a successful UseFigma round-trip on a fixture file, a successful GetMetadata round-trip on the same fixture, and a rejected GetDesignContext call (the wrapper's allowlist prevents it, regardless of the SDK)

**Verify**:
- A unit test using a fake MCP server (httptest.Server returning canned MCP responses) asserts `New()` returns a working client, `Whoami()` returns a parsed `WhoamiOutput` with the expected `Email` and `Plans[].Seat`, and the bearer token from the wrapper's `Client` appears as an `Authorization: Bearer <token>` header on every outbound request
- A unit test against the fake server asserts `dispatch("use_figma", input)` marshals `input` into the request body, the response's MCP error envelope translates to `figma-mcp-tool-call-failed` when the server returns one, and a transport-level failure translates to `figma-mcp-endpoint-unreachable`
- A unit test against the fake server asserts `callTyped[Out]` correctly unmarshals into the typed `Out` struct; a deliberately-malformed response (e.g. wrong field type) fails with `figma-mcp-response-malformed`
- A unit test runs Probe() against the fake server returning a Dev-seat Whoami response and asserts Probe returns a `ProbeResult` with the resolved endpoint, the email, and the seat value "Dev"
- An integration test (build-tagged `integration`) runs Probe() against a real Figma remote MCP server using a real token and asserts the result is a valid `ProbeResult` — manual invocation with environment-supplied credentials
- A regression test asserts that the bearer token never appears in the captured outbound-request log; the redaction from studio-config and figma-mcp-client's existing constraints holds end-to-end
- A unit test against the fake server asserts the bearer-token `RoundTripper` injects `Authorization: Bearer <token>` on every request the SDK makes via the transport, including the initialization handshake and every subsequent `CallTool`

---
