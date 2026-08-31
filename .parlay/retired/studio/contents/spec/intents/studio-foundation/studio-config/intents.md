# Studio-config

> Studio reads runtime configuration — Figma MCP connection, web-server behavior, project root resolution, and other operational knobs — from a defined set of sources with deterministic precedence. This feature pins the config sources (CLI flags, env vars, project-scoped file, user-scoped file, defaults), the canonical file locations, the `STUDIO_*` env-var namespace, and the specific keys that downstream features depend on (figma-mcp-client today; web-server-harness next). The aim is to make Studio's startup deterministic and debuggable: every operational decision Studio makes at boot traces back to a single config source. **Per-feature** values like the Figma file or team URL a designer is editing are deliberately out of scope here — they vary per layout and belong with the layout artifact, not with Studio's global config.

---

## Studio configuration sources, precedence, and file layout

**Goal**: Define a deterministic configuration-loading layer for the Studio binary. Sources are CLI flags, environment variables, a project-scoped config file, a user-scoped config file, and built-in defaults. Precedence (highest to lowest) is CLI flags > environment variables > project-scoped file > user-scoped file > defaults. Both config files use YAML and live at well-known paths: project-scoped at `.parlay-studio/config.yaml` (next to `.parlay/` in the parlay project root, owned per-project, checked into the repo) and user-scoped at `~/.config/parlay-studio/config.yaml` (per-user, never committed). Environment variables use the `STUDIO_*` prefix exclusively.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: Studio's predecessors (Core, the parlay CLI itself) read their configuration from one source — `.parlay/config.yaml`. Studio's configuration is more cross-cutting because it spans (a) per-project values like the Figma file URL the designer is working on, (b) per-user secrets like Figma auth tokens that should never live in a repo, and (c) runtime knobs like idle timeout that are typically the same everywhere but occasionally need a one-off override. A single config-source story doesn't fit those three layers cleanly. The deterministic layered model — CLI > env > project file > user file > defaults — is what every mature CLI tool (kubectl, helm, gh, the AWS CLI) uses for the same reason. Picking this shape now, before figma-mcp-client wiring lands and before web-server-harness adds its own knobs, means downstream features just declare their keys and inherit the loading + precedence behavior. The two file paths are pinned in advance so users authoring `.parlay-studio/config.yaml` in their project repo don't have to wait for the spec to settle. The `STUDIO_*` env prefix is reserved so unrelated environment variables can never collide with Studio's keys.

**Action**: Implement a config-loading layer in `studio/internal/config/` that resolves the merged Studio configuration at startup. The layer accepts CLI flags from cobra, reads env vars matching `STUDIO_*`, loads `.parlay-studio/config.yaml` from the resolved project root (see the project-resolution intent), loads `~/.config/parlay-studio/config.yaml`, applies built-in defaults, and merges them with deterministic precedence. Every key that downstream features depend on is declared once in this package as a typed field on the merged config struct. On startup, the merged config is logged once at INFO level with secrets redacted, so a designer hitting an unexpected behavior can grep the log to see which source contributed each value.

**Objects**: studio-config-loader, config-precedence, project-scoped-config-file, user-scoped-config-file, environment-variable-namespace, config-source-trace, secret-redaction

**Constraints**:
- Configuration sources are exactly five, in the documented precedence order: CLI flags > `STUDIO_*` environment variables > `.parlay-studio/config.yaml` (project-scoped) > `~/.config/parlay-studio/config.yaml` (user-scoped) > built-in defaults
- The project-scoped config file lives at `.parlay-studio/config.yaml` relative to the resolved parlay project root; the user-scoped config file lives at `~/.config/parlay-studio/config.yaml` (XDG-compliant — `$XDG_CONFIG_HOME` overrides `~/.config` when set)
- Environment variables consumed by Studio match the prefix `STUDIO_`; no other prefix is read. Unrelated environment variables (including those from Core or shell users) cannot influence Studio's configuration
- Both config files use YAML format consistent with the rest of the parlay project; unknown keys produce a warning at startup and are otherwise ignored (forward compatibility — future Studio versions may introduce keys this binary doesn't recognize)
- The user-scoped config file is the documented home for per-user secrets (Figma auth token, OAuth client credentials); the project-scoped file MUST NOT contain secrets, and a startup check fails fast with a stable code if a recognized secret key appears in the project-scoped file
- On startup, the merged configuration is logged once at INFO with all secret-typed values redacted as `***`; the log line names the source of each key (e.g. `STUDIO_FIGMA_MCP_URL=https://... (source: env)`)
- The config loader is the single supported import for reading Studio configuration; direct `os.Getenv` calls for `STUDIO_*` variables or direct YAML loads from the two config paths are rejected on review

**Verify**:
- A unit test sets `STUDIO_IDLE_TIMEOUT=10m` in the environment, places `idle_timeout: 30m` in the project-scoped file, and asserts the merged config reports `10m` (env wins)
- A unit test places `figma_mcp_url: https://a` in the user-scoped file and `figma_mcp_url: https://b` in the project-scoped file, and asserts the merged config reports `https://b` (project wins over user)
- A unit test places `figma_token: ==token==` in the project-scoped file and asserts Studio fails startup with a stable code naming the offending key and the user-scoped file as the correct home
- A unit test asserts that on startup, the INFO log line listing the merged configuration redacts `figma_token` to `***` while preserving non-secret keys verbatim
- A grep across `studio/` for `os.Getenv("STUDIO_` and direct YAML loads of either config path returns matches only inside `studio/internal/config/`

---

## Figma MCP connection configuration

**Goal**: Pin the configuration keys that the figma-mcp-client wrapper needs at startup — the MCP endpoint URL and the Figma authentication credential. Define which key lives in which file (project vs user-scoped) and which environment variables override them. Per-feature Figma file or team URLs are explicitly not a Studio-config concern; they live with the layout artifact for the feature, not in Studio's global config.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The figma-mcp-client feature (shipped) names `STUDIO_FIGMA_MCP_URL` in its dialogs and setup doc but does not define who reads it. The startup probe in `studio/internal/mcpclient/probe.go` takes an endpoint string as a function argument — the caller is responsible for supplying it. That caller is Studio's startup code, and the value must come from this configuration layer. The Figma authentication credential is the second load-bearing input: figma-mcp-client's intents defer the auth flow to "whichever Figma's remote server documents," and §7 of the architecture proposal documents the two candidate shapes — OAuth and token. The token-based flow is the simpler v1 target and is the only one this intent commits to; the OAuth flow is reserved for a later spec revision. Pinning these two keys here means the figma-mcp-client wrapper's `Probe()` and per-tool methods can be called with concrete inputs as soon as Phase 0 wiring lands. The Figma file or team URL the designer is editing varies per-feature (each feature's layout may live in a different Figma file) and therefore belongs alongside the per-feature layout artifact, not in Studio's global config — Q13 of the architecture proposal grouped it with global config, but real usage makes it per-layout.

**Action**: Add two Figma-specific fields to the Studio configuration struct in `studio/internal/config/`: `FigmaMCPURL` (the MCP server endpoint) and `FigmaToken` (the per-user auth token). Wire each to its env-var name, its config-file key, and the appropriate file scope. The MCP URL is project-scoped (different parlay projects may target different Figma teams or environments); the auth token is user-scoped and secret. At startup, the harness reads the resolved values and passes the MCP URL to `mcpclient.Probe()` and the token to the authenticated client constructor.

**Objects**: STUDIO_FIGMA_MCP_URL, STUDIO_FIGMA_TOKEN, figma-mcp-url-key, figma-token-key, auth-credential-storage, token-vs-oauth-decision

**Constraints**:
- The MCP endpoint URL is read from `STUDIO_FIGMA_MCP_URL` (env) or `figma_mcp_url:` in the project-scoped config file. There is no built-in default; if neither is set, Studio fails startup with a stable code naming both source options and pointing at `studio/docs/figma-mcp-setup.md`
- The Figma auth token is read from `STUDIO_FIGMA_TOKEN` (env), `figma_token:` in the user-scoped config file (token inline), or `figma_token_file:` in the user-scoped config file (a filesystem path to a file whose contents are the token). The `_file` pointer form supports OS-level secret stores and mounted-secret deployments without putting the token in YAML. The three sources have the same precedence ordering as other keys (env > project file > user file), and within the user file both inline and pointer forms are equivalent. If both `figma_token:` and `figma_token_file:` appear in the same file, startup fails with `studio-config-figma-token-double-source`. There is no built-in default; if no source provides a value, Studio fails startup with `studio-config-figma-token-missing` referencing the setup doc
- Per-feature Figma file or team URLs are not part of Studio's global config. They live alongside the per-feature layout artifact (the page schema's Figma reference field) and are resolved by the Design Loop when it opens a specific feature, not by Studio at startup
- The auth credential is treated as a secret throughout the binary: it is redacted in every log line, never returned in HTTP responses, and never written to disk by Studio (the user provides it once and Studio reads it on every startup)
- v1 supports token-based auth only. OAuth-based auth is deferred to a v2-or-later spec revision and is not a runtime fallback; the token-vs-OAuth choice is recorded here as a deliberate Phase 1 scope decision, not as an open question
- The auth credential is passed to the figma-mcp-client wrapper at construction time. The wrapper's `Client` struct holds the token for the duration of the process; it does not pass through any logging path or HTTP response

**Verify**:
- A unit test runs Studio's config loader with no `STUDIO_FIGMA_MCP_URL` set and no `figma_mcp_url:` key in either file, and asserts startup fails with `studio-config-figma-mcp-url-missing`
- A unit test runs the loader with `STUDIO_FIGMA_TOKEN` unset and no `figma_token:` in the user-scoped file, and asserts startup fails with `studio-config-figma-token-missing`
- A unit test places `figma_token: ==token==` in the project-scoped config file and asserts startup fails with `studio-config-secret-in-project-file` (the constraint from the prior intent fires here)
- A unit test asserts the resolved `figma_token` does not appear in any structured log line emitted during startup
- A grep across `studio/internal/config/` for `FigmaFileURL`, `figma_file_url`, or `STUDIO_FIGMA_FILE_URL` returns zero matches — per-feature Figma URLs are not a Studio-config concern
- A unit test places `figma_token_file: ==/tmp/figma-token==` in the user-scoped file (with the token in the named file) and asserts the resolved `FigmaToken` equals the file's contents
- A unit test places both `figma_token:` and `figma_token_file:` in the user-scoped file and asserts startup fails with `studio-config-figma-token-double-source` naming both keys
- An integration test (build-tagged `integration`) sets both keys via env vars and asserts the figma-mcp-client `Probe()` is invoked once with the resolved endpoint and that the resolved token is passed through to the SDK client constructor

**Questions**:
- Which artifact owns the per-feature Figma file URL field — the page schema's existing structure, a new field on the layout artifact, or a separate spec layer entirely? This question is referenced here so it isn't forgotten, but it is resolved in the page-schema or design-loop feature, not this one.

---

## Web server runtime configuration

**Goal**: Pin the configuration keys that the web-server-harness feature will need — port behavior, idle timeout, browser-open behavior — so the harness intents (still unwritten) can declare keys instead of designing the loading shape from scratch.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: §7 of the architecture proposal describes the ephemeral web-server harness ("starts when needed, shuts down on idle or explicit close"). §13 Q6 fixes the idle-timeout default at 30 minutes "but should be configurable." The port is described as "a free local port" — autoselect by default — but operational realities (corporate firewall rules, port-blocking on developer laptops) mean an explicit-port override is useful. The browser-open behavior is also a knob: most invocations should pop a browser window on startup, but CI / scripted runs need a `--no-browser` mode. Pinning all three keys here unblocks web-server-harness without forcing it to also design its own config loading.

**Action**: Add three web-server fields to the Studio configuration struct: `ServerPort` (0 = autoselect a free port, non-zero = bind to that port and fail if taken), `IdleTimeout` (a Go duration, default `30m`), and `OpenBrowser` (a boolean, default `true`). Wire each to its env var and config-file key. All three are project-scoped — they describe how Studio runs against this project, not user-level preferences.

**Objects**: STUDIO_SERVER_PORT, STUDIO_IDLE_TIMEOUT, STUDIO_OPEN_BROWSER, server-port-key, idle-timeout-key, open-browser-key

**Constraints**:
- The web server port is read from `STUDIO_SERVER_PORT` (env, integer) or `server_port:` in the project-scoped config file. Default is `0`, which means autoselect a free port via the operating system's port-allocation. A non-zero value binds to that specific port and fails startup with a stable code if the port is unavailable
- The idle timeout is read from `STUDIO_IDLE_TIMEOUT` (env, Go duration string like `30m` or `1h`) or `idle_timeout:` in the project-scoped config file. Default is `30m`. A value of `0` disables the timeout entirely (the server runs until explicitly closed); negative durations fail startup
- The browser-open behavior is read from `STUDIO_OPEN_BROWSER` (env, boolean) or `open_browser:` in the project-scoped config file. Default is `true`. A `--no-browser` CLI flag is equivalent to `STUDIO_OPEN_BROWSER=false` at higher precedence
- All three keys are project-scoped; the user-scoped file is rejected as a source for these keys with a startup warning (not error — the value still applies if it is the only source, but the warning recommends moving it to the project file)
- The autoselect-port behavior MUST report the chosen port at startup with a stable log line so a designer or tester can paste the URL into a browser if the autoselect happened without `OpenBrowser`

**Verify**:
- A unit test sets `STUDIO_SERVER_PORT=18080`, asserts the merged config reports `18080`, and asserts startup fails when port 18080 is held by another process
- A unit test asserts the default merged config reports `IdleTimeout: 30m` and `OpenBrowser: true`
- A unit test sets `STUDIO_IDLE_TIMEOUT=-1s` and asserts startup fails with a stable code naming the negative-duration error
- A unit test sets `STUDIO_IDLE_TIMEOUT=0` and asserts the timeout is disabled (no shutdown trigger fires after the simulated idle period)
- An integration test asserts that on startup with `OpenBrowser: true`, the system browser-open hook is invoked once with the resolved server URL

**Questions**:
- Should the idle timeout count from last-request, last-keyboard-activity in the open browser tab, or last-mutation-of-on-disk-state? Last-request is simplest. Resolve during dialog authoring with the web-server-harness feature, not here.
- Should `OpenBrowser: false` still log the URL with a "paste this into a browser" hint? Mostly yes — the designer's manual workflow needs it — but the exact log format is web-server-harness's call.

---

## Studio project root resolution

**Goal**: Define how Studio determines which parlay project to operate on when invoked, whether by Core's CLI hook, by a designer running the binary directly, or by a script. The resolved project root anchors the project-scoped config file path and the file-I/O abstraction that writes to parlay project paths.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: Studio is an extension of Core (§1 of the architecture proposal). Most invocations come from Core's CLI prompting "open editor?" — in those cases Core knows the project root and should pass it to Studio explicitly. Standalone invocations (a designer running `parlay-studio` from a checked-out project, or a CI script driving Studio) need a fallback mechanism. The candidate mechanisms are: explicit `--project /path` flag, `STUDIO_PROJECT_ROOT` env var, and cwd walk-up looking for `.parlay/`. All three have legitimate use cases. Pinning a precedence order — and pinning the cwd walk-up's stop condition — makes Studio's startup deterministic regardless of how it was invoked. The walk-up stop condition is "the first directory containing `.parlay/`" (matching Core's existing convention), with `$HOME` and `/` as terminators.

**Action**: Add a project-root resolution layer in `studio/internal/config/` that runs before the config-file load (because the project-scoped config file path depends on the resolved project root). Precedence (highest to lowest): `--project /path` CLI flag > `STUDIO_PROJECT_ROOT` env var > cwd walk-up. On a successful resolution, the resolved absolute path is logged at INFO. On failure (no `.parlay/` found in the walk-up and no explicit override), startup fails with a stable code naming all three resolution sources and pointing at the setup doc.

**Objects**: project-root-resolver, project-flag, STUDIO_PROJECT_ROOT, cwd-walk-up, resolution-precedence, project-not-found-error

**Constraints**:
- Project root resolution precedence is exactly three sources, in order: `--project <path>` CLI flag > `STUDIO_PROJECT_ROOT` environment variable > cwd walk-up for the nearest ancestor containing `.parlay/`
- The cwd walk-up terminates at `$HOME` and at `/`; it never crosses out of the user's home directory upward unless cwd was already outside `$HOME`
- The explicit `--project` flag and `STUDIO_PROJECT_ROOT` env var both accept absolute or relative paths; relative paths are resolved against the current working directory at startup
- The resolved project root MUST contain a `.parlay/` subdirectory — Studio refuses to operate against a directory that is not a parlay project, even if the path was passed explicitly via `--project`
- Explicit overrides (`--project` flag and `STUDIO_PROJECT_ROOT` env var) MUST point at the actual project root, not a subdirectory of it. The walk-up behavior is reserved for the cwd-based fallback; explicit overrides do not walk. Pointing `--project` at a subdirectory fails startup with `studio-config-project-root-invalid` naming the override source — the rationale is that explicit invocations should be unambiguous
- The resolved project root is logged once at INFO at startup with the source named (e.g. `project root: /home/dev/myapp (source: --project flag)`)
- The project-scoped config file path is derived as `<resolved-project-root>/.parlay-studio/config.yaml`. The config loader from the first intent uses this derived path; it does not perform its own walk-up

**Verify**:
- A unit test sets `--project /tmp/has-parlay/` (where the path contains `.parlay/`) and asserts the resolver reports that path regardless of cwd and env state
- A unit test sets `STUDIO_PROJECT_ROOT=/tmp/has-parlay/` and no `--project` flag, and asserts the resolver reports that path
- A unit test runs from a cwd nested several levels below a `.parlay/`-containing ancestor and asserts the resolver finds the ancestor
- A unit test runs from a cwd with no `.parlay/` ancestor (and no `--project` or env override) and asserts startup fails with `studio-config-project-root-not-found` naming all three resolution sources
- A unit test sets `--project /tmp/not-a-parlay-project/` (a directory without `.parlay/`) and asserts startup fails with `studio-config-project-root-invalid` regardless of whether the path exists
- A unit test sets `--project /tmp/has-parlay/some/subdir/` (a subdirectory of a real parlay project) and asserts startup fails with `studio-config-project-root-invalid` — explicit overrides do not walk up, even when the parent IS a parlay project

**Questions**:
- When Core invokes Studio via a CLI hook, does Core pass the project root via `--project`, `STUDIO_PROJECT_ROOT`, or by setting Studio's cwd to the project root? `--project` is the most explicit; cwd is what shells naturally produce. Resolve in coordination with whatever feature defines Core's hook surface (likely a Core-side feature, not Studio).

---
