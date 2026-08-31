# Web-server-harness — Dialogs

---

### Studio binary startup sequence and lifecycle

**Trigger**: A Parlay Studio maintainer (or Core's CLI hook) invokes `parlay-studio`; the binary's `main.go` runs the 12-step boot sequence.

User: parlay-studio --project /home/dev/myapp
System (background): Step 1 — parse command-line flags. Step 2 — resolve project root (`--project` wins; verifies `/home/dev/myapp/.parlay/` exists).
System: studio-config: project_root=/home/dev/myapp (source: --project flag)
System (background): Step 3 — load merged configuration.
System: studio-config: resolved configuration:
System:   figma_mcp_url      = https://mcp.figma.com/v1   (source: env)
System:   figma_token        = ***                        (source: user-file)
System:   server_port        = 0                          (source: default)
System:   idle_timeout       = 30m                        (source: default)
System:   open_browser       = true                       (source: default)
System (background): Step 4 — probe Figma MCP server (`whoami`). Step 5 — construct authenticated MCP client (session held for process lifetime).
System: mcpclient: probe successful — endpoint=https://mcp.figma.com/v1 email===maintainer@example.com== seat=Dev
System (background): Step 6 — build chi router with mounted tool route groups. Step 7 — start HTTP server on autoselected port. Step 8 — log bound URL.
System: web-server: bound to http://127.0.0.1:==54221==/
System (background): Step 9 — open browser. Step 10 — install signal handlers. Step 11 — start idle-timeout goroutine.
System: web-server: opening browser…
System: web-server: idle timeout 30m; will shut down on inactivity
System (background): Step 12 — block on shutdown channel.
System: Studio started.

#### Branch: Step 2 fails — project root not found

User: cd /tmp/scratch  # no .parlay/ ancestor
User: parlay-studio
System: studio-config: ERROR could not resolve a parlay project root. Resolution sources tried (in precedence order): `--project` flag; `STUDIO_PROJECT_ROOT` env var; cwd walk-up from `/tmp/scratch` (terminated at `/` without finding `.parlay/`). See studio/docs/figma-mcp-setup.md.
System: (exit non-zero, code: studio-config-project-root-not-found)
System (background): Boot terminated at step 2. Steps 3–12 never run. No port bound, no browser opened, no signal handler installed.

#### Branch: Step 3 fails — figma_mcp_url missing

User: parlay-studio --project /home/dev/myapp
System: studio-config: project_root=/home/dev/myapp (source: --project flag)
System: studio-config: ERROR `figma_mcp_url` is not set. Resolution sources tried: STUDIO_FIGMA_MCP_URL env var; `figma_mcp_url:` in /home/dev/myapp/.parlay-studio/config.yaml; `figma_mcp_url:` in ~/.config/parlay-studio/config.yaml. There is no default. See studio/docs/figma-mcp-setup.md.
System: (exit non-zero, code: studio-config-figma-mcp-url-missing)
System (background): Boot terminated at step 3. The MCP probe never runs.

#### Branch: Step 4 fails — MCP probe rejects the endpoint (hard gate)

User: STUDIO_FIGMA_MCP_URL=http://localhost:8765 parlay-studio --project /home/dev/myapp
System: studio-config: project_root=/home/dev/myapp; figma_mcp_url=http://localhost:8765 (source: env); figma_token=*** (source: user-file)
System: mcpclient: probe failed — endpoint matches desktop variant signature
System: studio: ERROR boot step 4 (MCP probe) failed with `figma-mcp-endpoint-unsupported`. The configured MCP endpoint is the Figma desktop variant; Studio requires the remote server. See studio/docs/figma-mcp-setup.md.
System: (exit non-zero, code: figma-mcp-endpoint-unsupported)
System (background): Hard-gate semantics — boot terminated at step 4. Steps 5–12 never run. No chi router was built, no port was bound, no browser was opened. An external observer cannot see Studio in a partial-boot state.

#### Branch: Step 7 fails — port conflict

User: STUDIO_SERVER_PORT=18080 parlay-studio --project /home/dev/myapp
User: # but port 18080 is held by another process
System: studio-config: server_port=18080 (source: env)
System: mcpclient: probe successful — endpoint=https://mcp.figma.com/v1 email===maintainer@example.com== seat=Dev
System: web-server: ERROR cannot bind to 127.0.0.1:18080 — address already in use
System: studio: ERROR boot step 7 (start HTTP server) failed with `studio-config-server-port-conflict`. Either free the port or unset STUDIO_SERVER_PORT.
System: (exit non-zero, code: studio-config-server-port-conflict)
System (background): The MCP probe succeeded but the port bind failed — the MCP session was constructed (step 5) but is closed cleanly during the failed-boot teardown.

#### Branch: Graceful shutdown — SIGTERM

User: parlay-studio --project /home/dev/myapp &
System: Studio started.
User: kill -TERM ==<pid>==
System (background): Signal handler installed at step 10 catches SIGTERM. Pushes onto the shutdown channel.
System: studio: shutdown reason: signal: SIGTERM
System (background): http.Server.Shutdown called with 5-second deadline. Drains in-flight requests. Closes the MCP client. Exits zero.
System: studio: graceful shutdown complete (drained 0 in-flight requests in ==12ms==)
System: (exit 0)

#### Branch: Graceful shutdown — idle timeout fires

User: parlay-studio --project /home/dev/myapp &
System: Studio started.
User: # No requests for 30 minutes (or whatever IdleTimeout was configured)
System (background): The idle-timeout goroutine wakes every 10 seconds. At the first wake where `now - last-activity > 30m`, it pushes onto the shutdown channel.
System: studio: shutdown reason: idle: no requests for 30m0s
System: studio: graceful shutdown complete (drained 0 in-flight requests in ==8ms==)
System: (exit 0)

#### Branch: Graceful shutdown — explicit /api/shutdown

User: parlay-studio --project /home/dev/myapp &
System: Studio started.
User: curl -X POST http://127.0.0.1:==54221==/api/shutdown
System (background): The /api/shutdown handler pushes onto the shutdown channel.
System (response to curl): {"status":"shutting_down"}
System: studio: shutdown reason: explicit: /api/shutdown
System: studio: graceful shutdown complete (drained 1 in-flight request in ==18ms==)
System: (exit 0)

#### Branch: Graceful-shutdown deadline expires — handler exceeds 5 seconds

User: kill -TERM ==<pid>==  # a handler is currently in a 10-second operation
System: studio: shutdown reason: signal: SIGTERM
System (background): http.Server.Shutdown waits up to 5 seconds for in-flight handlers. The 10-second handler is forcibly aborted at the 5-second mark; its connection is closed.
System: studio: WARN graceful shutdown deadline exceeded; 1 handler aborted (path: ==/api/design-loop/long-op==)
System: studio: graceful shutdown complete
System: (exit 0)

---

### HTTP server harness — chi router, middleware, error envelopes

**Trigger**: A Studio tool subsystem registers its route group with the harness; HTTP requests flow through the middleware stack and into the registered handlers.

User: # domain-model-editor feature's package calls harness.RegisterTool("domain-model", "/api/domain-model", domainModelMount)
User: # design-loop feature's package calls harness.RegisterTool("design-loop", "/api/design-loop", designLoopMount)
User: parlay-studio --project /home/dev/myapp
System (background): Boot reaches step 6 — harness.New() builds the chi router, installs the middleware stack (RequestID → Recovery → IdleTimeoutReset → Logging → ErrorEnvelope), mounts the two universal endpoints (/api/health, /api/shutdown), invokes each registered tool's mount callback, mounts the UI bundle at /.
System: web-server: routes mounted: /api/health, /api/shutdown, /api/domain-model/*, /api/design-loop/*, /*
System: Studio started.
User: curl -i http://127.0.0.1:==54221==/api/domain-model/entities
System (response): HTTP/1.1 200 OK
System (response): X-Request-ID: ==abc-123-def==
System (response): Content-Type: application/json
System (response): {"entities": [...]}
System (background): The request ID `abc-123-def` was assigned by middleware.RequestID, propagated through the handler's context, returned in the response header, and recorded in the structured log line for this request.
System (log): {"level":"info","request_id":"abc-123-def","method":"GET","path":"/api/domain-model/entities","status":200,"duration_ms":==4==}

#### Branch: Handler panics — recovery + server-error envelope

User: curl -i http://127.0.0.1:==54221==/api/design-loop/buggy-handler
System (background): The handler panics with a nil-pointer dereference. middleware.Recoverer catches the panic; the panic detail is logged with the request ID; the error-envelope middleware translates the panic into a server-error response.
System (response): HTTP/1.1 500 Internal Server Error
System (response): X-Request-ID: ==xyz-456-pqr==
System (response): {"code":"server-error","request_id":"xyz-456-pqr"}
System (log): {"level":"error","request_id":"xyz-456-pqr","panic":"runtime error: invalid memory address or nil pointer dereference","stack":"==<go stack trace>==","path":"/api/design-loop/buggy-handler"}
System (background): The panic detail appears in the log line, not in the response body — internal error detail never reaches the client.

#### Branch: Handler returns `not-found` sentinel

User: curl -i http://127.0.0.1:==54221==/api/domain-model/entities/==nonexistent==
System (background): The handler returns a `domain.ErrEntityNotFound` wrapped with `%w`. The error-envelope middleware uses `errors.As` to match `ErrEntityNotFound` and translates to the not-found envelope shape.
System (response): HTTP/1.1 404 Not Found
System (response): X-Request-ID: ==abc-123-def==
System (response): {"code":"not-found","target":"entity:==nonexistent=="}

#### Branch: Handler returns `validation-failed` sentinel

User: curl -i -X POST http://127.0.0.1:==54221==/api/domain-model/entities --data '{"name": ""}'
System (background): The handler's input validator returns `validation.ErrValidationFailed` with the `Fields` slice naming the offending fields.
System (response): HTTP/1.1 400 Bad Request
System (response): {"code":"validation-failed","fields":[{"path":"name","reason":"required"}]}

#### Branch: Handler returns `conflict` sentinel

User: curl -i -X PUT http://127.0.0.1:==54221==/api/domain-model/entities/==Task== -H "If-Match: stale-etag" --data ==<update>==
System (background): The repo's optimistic-concurrency check fails; the handler returns `domain.ErrConflict` with current and attempted etags.
System (response): HTTP/1.1 409 Conflict
System (response): {"code":"conflict","current_etag":"v3","attempted_etag":"stale-etag"}

#### Branch: SPA fallback hits the UI bundle

User: curl -i http://127.0.0.1:==54221==/some/client-side/route
System (background): The path doesn't match any `/api/*` route. The harness's catch-all serves the UI bundle's index.html (the client-side router handles the actual route).
System (response): HTTP/1.1 200 OK
System (response): Content-Type: text/html
System (response): <!DOCTYPE html>==<UI bundle's index.html>==

#### Branch: SPA fallback with no UI bundle built yet

User: curl -i http://127.0.0.1:==54221==/some/client-side/route
System (background): The UI bundle is not yet embedded (the studio/internal/ui package's embed.FS is empty because `npm run build` hasn't run). The harness returns a 503 placeholder.
System (response): HTTP/1.1 503 Service Unavailable
System (response): {"code":"studio-ui-bundle-not-built","message":"Studio's UI bundle is not yet packaged. Run `npm run build` in studio/internal/ui/ before invoking parlay-studio."}

#### Branch: Duplicate tool registration rejected

User: # a second package calls harness.RegisterTool("domain-model", "/api/domain-model-v2", anotherMount)
User: parlay-studio --project /home/dev/myapp
System: web-server: ERROR tool "domain-model" is already registered (at /api/domain-model); a tool may register exactly one route group. Reorganize the additional surface under the tool's existing prefix.
System: (exit non-zero, code: studio-server-duplicate-tool-registration)
System (background): Boot fails at step 6. The harness rejects double registration during chi router construction.

#### Branch: 127.0.0.1-only bind (non-loopback dial refused)

User: # from a separate host or network interface on the same machine:
User: curl http://==192.168.1.42==:==54221==/api/health
System (response): curl: (7) Failed to connect to 192.168.1.42 port ==54221==: Connection refused
System (background): The HTTP server binds to 127.0.0.1 only — never 0.0.0.0 or any external interface. Only local processes can reach the server. This is the trust boundary that lets /api/shutdown skip authentication.

---

### Idle-timeout activity tracking

**Trigger**: Studio is running; HTTP requests arrive; the idle-timeout goroutine periodically checks whether to shut down.

User: parlay-studio --project /home/dev/myapp &  # IdleTimeout=30m default
System: Studio started.
System (background): Idle-check goroutine launched (because IdleTimeout > 0). Wakes every 10 seconds.
User: curl http://127.0.0.1:==54221==/api/domain-model/entities
System (background): The IdleTimeoutReset middleware records the timestamp of this request as the new "last activity" value.
System (background): At every 10-second tick, the goroutine checks `now - last-activity > 30m`. As long as designer activity continues, the predicate is false; no shutdown.

#### Branch: Idle expires — shutdown trigger fires

User: parlay-studio --project /home/dev/myapp &  # IdleTimeout=30m
System: Studio started.
User: # No requests on /api/* (excluding /api/health) for 30 minutes
System (background): At the first tick where `now - last-activity > 30m0s`, the goroutine pushes onto the shutdown channel and exits.
System: studio: shutdown reason: idle: no requests for 30m0s
System: (graceful-shutdown sequence runs; exit 0)

#### Branch: /api/health does NOT advance the timestamp

User: parlay-studio --project /home/dev/myapp &  # IdleTimeout=30m
System: Studio started.
User: # an external monitor polls /api/health every 5 minutes
User: curl http://127.0.0.1:==54221==/api/health  # repeated every 5 minutes
System (background): The IdleTimeoutReset middleware skips /api/health — health monitoring is passive observation, not active designer work. The last-activity timestamp is NOT advanced by these requests.
System (background): After 30 minutes of no `/api/*` requests other than `/api/health`, the goroutine fires the idle shutdown.
System: studio: shutdown reason: idle: no requests for 30m0s
System: (exit 0)

#### Branch: IdleTimeout=0 — goroutine never launched

User: STUDIO_IDLE_TIMEOUT=0 parlay-studio --project /home/dev/myapp
System: studio-config: idle_timeout=0 (source: env) — timeout disabled; server runs until explicitly closed
System (background): Boot step 11 checks IdleTimeout > 0; since 0 is falsy, the goroutine is NEVER launched. The shutdown channel has only two trigger sources (signal, /api/shutdown) for this invocation.
System: web-server: idle timeout disabled
System: Studio started.

#### Branch: Idle trigger reason string format

User: parlay-studio --project /home/dev/myapp &  # IdleTimeout=100ms (test fixture)
System (background): Test override injects a 100ms IdleTimeout and a faster 10ms check interval via the package-internal test hook.
User: # no requests
System (after ~110ms): studio: shutdown reason: idle: no requests for 100ms
System (background): The format is exactly `idle: no requests for <duration>` with the duration formatted via Go's standard duration.String() — `100ms`, `30m0s`, `1h0m0s`, etc.

#### Branch: Test hook overrides the wake interval

User: # test code calls server.SetIdleCheckIntervalForTesting(10 * time.Millisecond)
User: # runs the harness under a tight timing window
System (background): The package-internal test hook swaps the ticker that drives the idle-check goroutine; tests don't need to wait 10 seconds for the next check. Production code never touches this hook — it's not configurable via studio-config.

---

### Figma MCP Phase 0 wiring

**Trigger**: Studio's boot step 5 constructs the authenticated MCP client; later steps make real `tools/call` requests through the wrapper.

User: # boot step 5 in main.go calls:
User: # client, err := mcpclient.New(ctx, cfg.FigmaMCPURL, cfg.FigmaToken)
System (background): `New` runs three steps in order: (1) `mcp.NewClient(&mcp.Implementation{Name: "parlay-studio", Version: "==v0==.==1==.==0=="}, nil)` → `*mcp.Client`; (2) constructs `mcp.StreamableClientTransport{Endpoint: cfg.FigmaMCPURL, HTTPClient: &http.Client{Transport: bearerTokenRoundTripper{token: cfg.FigmaToken, base: http.DefaultTransport}}}`; (3) `client.Connect(ctx, transport, nil)` → `*mcp.ClientSession`.
System (background): The session is stored on the wrapper's `Client` struct field. The bearer-token `RoundTripper` is the only place the token reaches the network; the SDK never sees the token as a Go value.
System (background): boot step 4 (Probe) now calls `client.Whoami(ctx)` → real network call to `tools/call` with `Name: "whoami"`. Response unmarshals into `WhoamiOutput{Email, Plans[]}`.
System: mcpclient: probe successful — endpoint=https://mcp.figma.com/v1 email===maintainer@example.com== seat=Dev

#### Branch: Bearer token attached to every outbound request

User: # test fires a tcpdump-style capture against the fake MCP server
User: # repeated CallTool round-trips: Whoami, GetMetadata, UseFigma
System (background): Every outbound HTTP request the SDK makes — including the initial Connect handshake and every subsequent CallTool — carries the header `Authorization: Bearer ==<token>==`. The `RoundTripper` adds the header before delegating to `http.DefaultTransport`. No other auth mechanism is wired (no query-string token, no client-cert, no MCP-spec custom auth).

#### Branch: Tool-level error — IsError true

User: # caller invokes client.UseFigma(ctx, UseFigmaInput{NodeID: "==stale-node-id=="})
System (background): The SDK calls `tools/call` with Name="use_figma" and the marshalled UseFigmaInput. Figma's server responds with `CallToolResult{IsError: true, Content: [{Type: "text", Text: "node not found"}]}`.
System (background): dispatch sees `result.IsError == true`, wraps the result's Content text into a `figma-mcp-tool-call-failed` error.
System (returned to caller): figma-mcp-tool-call-failed: use_figma: node not found

#### Branch: Transport-level error — connection drops mid-call

User: # caller invokes client.GetMetadata(ctx, GetMetadataInput{NodeID: "==node-id=="})
System (background): The SDK's transport-level send fails (network blip, TLS error, server hung up). `session.CallTool` returns `(nil, err)` where `err` is the transport error.
System (background): dispatch wraps with `figma-mcp-endpoint-unreachable` (matching figma-mcp-client's stable code from the startup probe — same code, same meaning, applies after probe success too).
System (returned to caller): figma-mcp-endpoint-unreachable: get_metadata: ==<transport error detail>==

#### Branch: Malformed StructuredContent

User: # caller invokes client.Whoami(ctx)
System (background): Figma's server responds with `CallToolResult{StructuredContent: {"email": 42, "plans": null}}` — `email` is an integer where a string was expected.
System (background): callTyped[WhoamiOutput] JSON-marshals the StructuredContent value then unmarshals into WhoamiOutput; the type mismatch produces a json.UnmarshalTypeError. callTyped wraps with `figma-mcp-response-malformed` naming the tool and the offending field path.
System (returned to caller): figma-mcp-response-malformed: whoami: cannot unmarshal number into field WhoamiOutput.email of type string

#### Branch: Hand-maintained per-tool schemas

User: # designer reads studio/internal/mcpclient/tools.go to understand the wrapper input shape for use_figma
User: less studio/internal/mcpclient/tools.go
System: ==<file contents include>== type UseFigmaInput struct {
System:     NodeID string         `json:"node_id"`
System:     Params map[string]any `json:"params,omitempty"`
System: }
System: ==<file contents include>== type UseFigmaOutput struct {
System:     NodeID string         `json:"node_id"`
System:     Raw    map[string]any `json:"raw,omitempty"`
System: }
System (background): The 16 input/output structs (one input + one output per supported tool) are hand-written in tools.go. Their JSON field tags match Figma's documented MCP tool schemas. Drift between Figma's actual responses and these structs is detected at runtime by integration tests (build-tagged `integration`), not at build time by codegen. Codegen from Figma's published schemas is reserved for a later feature.

#### Branch: Integration test against real Figma (build-tagged)

User: # operator runs the integration suite manually with real Figma credentials
User: STUDIO_FIGMA_MCP_URL=https://mcp.figma.com/v1 STUDIO_FIGMA_TOKEN=*** /home/node/go/bin/go test -tags=integration ./internal/mcpclient/...
System (background): The integration build tag enables tests that hit the real Figma remote MCP server. Tests: (a) Whoami returns a Dev or Full seat; (b) UseFigma round-trips on a fixture Figma file; (c) GetMetadata round-trips on the same fixture; (d) GetDesignContext is rejected by the wrapper allowlist regardless of what Figma's server would return.
System: PASS: TestIntegration_WhoamiReturnsDevOrFullSeat (==2.4s==)
System: PASS: TestIntegration_UseFigmaRoundTrip (==4.1s==)
System: PASS: TestIntegration_GetMetadataRoundTrip (==1.8s==)
System: PASS: TestIntegration_GetDesignContextRejectedByAllowlist (==0.02s==)
System: ok  github.com/parlay-tool/parlay/studio/internal/mcpclient ==8.4s==

#### Branch: Token never appears in logs (end-to-end)

User: parlay-studio --project /home/dev/myapp 2>&1 | tee studio.log
User: grep -F "==<actual token value>==" studio.log
System: (no match) — the resolved token never appears in any log line emitted by studio-config (startup config-listing redacts), figma-mcp-client (Probe and tool calls never log the token), or this feature's RoundTripper (the header injection happens at the http.RoundTripper level, below the SDK's logging surface).

---
