# Web-server-harness — Infrastructure

---

## Studio binary boot sequence and graceful-shutdown lifecycle

**Affects**: Studio binary's entry point; the 12-step boot ordering with hard-gate semantics on the MCP probe; the 3 graceful-shutdown trigger sources (signal, idle, explicit) feeding a single shutdown channel; the bounded shutdown deadline; the localhost-only bind that defines the trust boundary for the explicit-close endpoint.

**Behavior**: At Studio startup, the binary's entry point runs a 12-step boot sequence in a fixed order: parse command-line flags, resolve the parlay project root, load and log the merged configuration with secrets redacted, probe the Figma MCP server, construct the authenticated MCP client, build the HTTP router with mounted tool route groups, start the HTTP server on the resolved port, log the bound URL, open the operator's browser (when configured), install signal handlers, start the idle-timeout goroutine (when the timeout is non-zero), and block on a shutdown channel. Each step's failure terminates the process with a non-zero exit code and a structured log line naming the failing step and the upstream stable code. The MCP probe is a hard gate: a probe failure terminates the process before any port is bound, before any browser is opened, and before any signal handler is installed — partial-boot states are never observable from outside the process. The HTTP server binds to the loopback interface only (never an external interface); the loopback bind is the trust boundary that lets the explicit-close endpoint skip authentication. Graceful shutdown is triggered by exactly three events — an interrupt or termination signal, the idle-timeout firing, or a request to the explicit-close endpoint — all feeding a single shutdown channel. The shutdown sequence stops accepting new HTTP connections, drains in-flight requests within a hard-coded 5-second deadline (handlers exceeding the deadline are aborted), closes the MCP client, emits a single INFO log line naming the shutdown reason, and exits zero. The process never exits non-zero on a graceful-shutdown path.

**Invariants**:
- The 12 boot steps execute in a fixed order; each step's failure terminates the process with a non-zero exit code and a structured log line naming the failing step and the upstream stable code
- The MCP probe is a hard gate: a probe failure terminates boot before any port is bound, before any browser is opened, and before any signal handler is installed; an external observer cannot see Studio in a partial-boot state when the probe fails
- The HTTP server binds to the loopback interface only; a non-loopback dial from another host or external interface is refused at the TCP layer
- The explicit-close endpoint does not require authentication; its trust boundary is the loopback bind
- Graceful shutdown has exactly three trigger sources (signal, idle, explicit-close), all feeding a single shutdown channel; the same shutdown sequence runs regardless of which trigger fired
- The graceful-shutdown drain deadline is hard-coded at 5 seconds; handlers exceeding the deadline are aborted, an INFO log line names the aborted-handler count and paths
- The process exits zero on any graceful-shutdown path; non-zero exits are reserved for boot-step failures and panics that escape the harness's recovery middleware
- A single INFO log line names the shutdown reason in one of three formats: `signal: <signal-name>`, `idle: no requests for <duration>`, or `explicit: <endpoint-path>`

**Source**: @web-server-harness/studio-binary-startup-sequence-and-lifecycle

**Backward-Compatible**: yes

**Notes**:
- Foundational change — there is no prior Studio binary to maintain compatibility with
- The "MCP probe is a hard gate" rule is the load-bearing operational property of Studio's boot: Studio without canvas-write capability cannot do its job, so failing fast at startup is the right behavior

---

## HTTP server harness — router, middleware stack, error envelopes, tool registration

**Affects**: Studio's HTTP surface — the router, the fixed middleware-ordering invariant, the request-ID propagation, the panic-recovery boundary, the error-envelope translation from closed-vocabulary error kinds to status codes and JSON shapes, the tool-registration interface that lets each Studio tool plug its handlers in without creating an import cycle, the two universal endpoints, the single-page-app fallback for the embedded UI bundle.

**Behavior**: The HTTP server is built around a chi-style router with a fixed five-stage middleware stack: request-ID assignment, panic recovery, idle-timeout reset (applied only on requests under `/api/*` and excluding `/api/health`), structured request logging, and error-envelope translation at the handler boundary. Two universal endpoints are owned by the harness itself: a liveness endpoint that returns a minimal JSON envelope without advancing the idle timer, and an explicit-close endpoint that triggers the graceful-shutdown path documented in the lifecycle fragment. Each Studio tool subsystem (Domain Model Editor, Design Loop, future Phase 4+ tools) registers itself with the harness via a typed registration interface; the harness invokes each tool's registration callback during router construction. Each tool registers exactly one route group at one path prefix — a tool with multiple internal surfaces attaches them all under its single registered prefix rather than registering twice; a second registration under the same tool name is rejected at construction time with a stable code. The harness package does not import any tool package directly, and a tool package does not register routes anywhere except through the registration interface — both directions are review-enforced to avoid an import cycle. The root path serves the embedded UI bundle with single-page-app fallback semantics: paths not matched by any `/api/*` route serve the UI's `index.html` so client-side routing works; until the UI bundle is built and embedded, the root path returns a 503 envelope with a stable code that names the missing-bundle condition. The error-envelope middleware uses Go's error-matching primitives to translate closed-vocabulary error kinds (validation-failed, not-found, conflict, server-error) into status codes and JSON envelope shapes per the application adapter's convention; unmapped errors translate to server-error with the request ID surfaced to the structured log but not the response body. Every HTTP response carries the request-ID header; the request ID in the header is the same value the log line records for the request.

**Invariants**:
- The middleware stack is installed in a fixed five-stage order: request-ID assignment, panic recovery, idle-timeout reset (scoped to `/api/*` excluding `/api/health`), structured logging, error-envelope translation
- Two universal endpoints are owned by the harness itself: the liveness endpoint (does not advance the idle timer) and the explicit-close endpoint (triggers graceful shutdown)
- Each Studio tool subsystem registers exactly one route group at one path prefix; a second registration under the same tool name is rejected at router-construction time with a stable code
- The harness package does not import any Studio tool package; a Studio tool package does not register HTTP routes anywhere except through the harness's registration interface
- The single-page-app fallback serves the embedded UI bundle's index.html for paths not matched by any `/api/*` route; when the UI bundle is not yet embedded, the fallback returns a 503 envelope with the stable code naming the missing-bundle condition
- The error-envelope middleware maps closed-vocabulary error kinds to status codes and JSON shapes per the adapter convention; unmapped errors translate to server-error with the request ID in the log line but not in the response body
- Every HTTP response carries the request-ID header; the value matches the request-ID field recorded in the structured log line for that request
- Panic detail appears in the structured log line for the panicking request but never in the response body

**Source**: @web-server-harness/http-server-harness-chi-router-middleware-error-envelopes

**Backward-Compatible**: yes

**Notes**:
- The tool-registration interface is the only sanctioned mechanism for attaching new HTTP surface to Studio. Adding a new Studio tool in a later feature is a registration call, not a harness change
- The single-page-app fallback's missing-bundle 503 is the placeholder behavior for Phase 1 before the UI bundle feature ships; after the UI bundle is built and embedded, the same path returns the bundle's index.html

---

## Idle-timeout activity tracking and shutdown trigger

**Affects**: Studio's auto-shutdown lifecycle; the definition of "idle" for the idle-timeout configuration key from studio-config; the synchronization primitive backing the last-activity timestamp; the goroutine that fires the idle shutdown trigger; the test-only override hook for the wake interval.

**Behavior**: Studio defines "idle" as no HTTP request received on a path under `/api/*` (excluding `/api/health`) for the configured idle-timeout duration. The harness's middleware advances a shared last-activity timestamp on every qualifying request; a goroutine launched at boot wakes every 10 seconds (hard-coded interval) and checks whether the elapsed time since last activity exceeds the configured timeout. When the predicate is true, the goroutine pushes onto the shutdown channel with a reason string formatted as `idle: no requests for <duration>` and exits. The last-activity timestamp is protected by a synchronization primitive so the goroutine cannot observe a torn read. A zero idle-timeout value skips the goroutine entirely — no timer is launched, no shutdown trigger originates from this source under any condition. The first idle-check fires at least one full idle-timeout window after boot, so a freshly started harness with zero requests cannot trigger a false shutdown. A package-internal test hook lets tests inject a faster wake interval without exposing the interval as a configuration key; production code never touches the hook.

**Invariants**:
- "Idle" means "no HTTP request received on a path matching `/api/*` and not `/api/health` for the configured idle-timeout duration"; the harness does not track keystrokes, on-disk mutations, or any other signal
- The last-activity timestamp is updated by the middleware on every qualifying request and read by the idle-check goroutine through a synchronization primitive that prevents torn reads
- The idle-check goroutine wakes every 10 seconds; this interval is hard-coded and not configurable via studio-config
- When the configured idle-timeout is zero, the idle-check goroutine is never launched; there is no idle shutdown trigger source for that invocation under any condition
- The first idle-check fires at least one full idle-timeout window after boot; a freshly started harness with zero requests cannot trigger a false shutdown
- The shutdown trigger reason emitted by the goroutine is exactly `idle: no requests for <duration>` where the duration is formatted via Go's standard duration string format
- The test-only override hook swaps the ticker that drives the goroutine; production code does not touch the hook, and the hook is not exposed via any configuration key

**Source**: @web-server-harness/idle-timeout-activity-tracking

**Backward-Compatible**: yes

**Notes**:
- This fragment resolves studio-config's deferred Q3.1 (what counts as "idle") — the answer is last HTTP request on `/api/*` excluding `/api/health`
- The exclusion of `/api/health` is the load-bearing detail: external health monitors poll the liveness endpoint at regular intervals, and including them in the idle calculation would keep Studio alive indefinitely

---

## Figma MCP Phase 0 wiring against the v1.6.0 SDK

**Affects**: The four currently-stubbed pieces of the figma-mcp-client wrapper (Client construction, the dispatch helper, the typed unmarshal helper, the per-tool wrapper input/output structs); the bearer-token authentication mechanism for outbound requests; the persistent MCP session lifetime; the integration-test surface for real Figma MCP traffic.

**Behavior**: The figma-mcp-client wrapper's currently-stubbed pieces are replaced with real implementations against the v1.6.0 MCP Go SDK. Client construction is a three-step sequence: instantiate the SDK's persistent client with a Studio identification value; construct a streamable HTTP client transport pointed at the configured remote endpoint and carrying a custom HTTP client whose round-tripper injects the bearer authentication token into every outbound request; call the SDK client's Connect method with the transport to obtain a persistent session. The session is stored on the wrapper's Client struct for the process lifetime; it is closed during the harness's graceful-shutdown sequence. The dispatch helper invokes the session's tool-call method with the requested tool name and the typed input value as the call's arguments — the SDK accepts an untyped arguments value and handles JSON marshalling internally. The dispatch helper checks the result's error flag to surface MCP-level tool errors with a stable wrapper code; it wraps transport-level failures with the existing endpoint-unreachable stable code from figma-mcp-client; on success it returns the result's structured-content payload. The typed unmarshal helper JSON-round-trips the structured-content value into the typed output struct and wraps unmarshal failures with a malformed-response stable code naming the tool and the offending field. Per-tool input and output structs are hand-maintained for v1 — each of the eight supported tools has explicit Go struct definitions whose JSON field tags match Figma's documented MCP tool schemas; schema drift is detected by integration tests, not by codegen or build-time validation. Integration tests under a dedicated build tag exercise the real Figma remote MCP server with real credentials; they are not part of the default test run. The bearer-token round-tripper is the sole mechanism by which the auth token reaches the network — it is not written into any SDK option, any logged field, or any HTTP response.

**Invariants**:
- Client construction is exactly the three-step sequence: SDK persistent client, streamable HTTP transport pointed at the configured remote endpoint with a custom HTTP client whose round-tripper injects the bearer token, SDK Connect call yielding a persistent session
- The persistent session is stored on the wrapper's Client struct and lives for the entire process lifetime; it is closed during graceful shutdown
- The bearer-token round-tripper is the sole mechanism by which the auth token reaches the network; the token is not stored in any SDK option struct, any logged field, or any HTTP response body
- The dispatch helper calls the session's tool-call method with the typed input value as the call's arguments; the SDK handles JSON marshalling internally
- An MCP-level tool error (result's error flag set) wraps with the wrapper's tool-call-failed stable code carrying the result's content text
- A transport-level failure (the SDK's call returns a non-nil error) wraps with figma-mcp-client's existing endpoint-unreachable stable code
- An unmarshal failure (typed output struct can't accept the structured-content shape) wraps with a malformed-response stable code naming the tool and the offending field path
- Per-tool input and output structs are hand-maintained for v1; their JSON field tags match Figma's documented MCP tool schemas; schema drift is detected by integration tests, not by build-time validation
- Integration tests against a real Figma remote MCP server live under a dedicated build tag and are not part of the default test run

**Source**: @web-server-harness/figma-mcp-phase-0-wiring

**Backward-Compatible**: yes

**Notes**:
- This fragment is the only one in the studio-foundation initiative that explicitly produces network traffic against a third-party service at runtime
- The "v1 hand-maintained schemas" decision is deliberate Phase 1 scope; codegen from Figma's published tool schemas is reserved for a later feature once Figma's schema publication story is clearer
- The persistent session lifetime (one session for the process) matches the SDK's documented usage pattern and avoids reconnection overhead on every tool call

---
