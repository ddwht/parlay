# Studio-config — Dialogs

---

### Studio configuration sources, precedence, and file layout

**Trigger**: A Parlay Studio maintainer starts the `parlay-studio` binary and needs to know which configuration source contributed which value.

User: parlay-studio --project /home/dev/myapp
System (background): Resolves the project root (see the project-resolution dialog below).
System (background): Loads `~/.config/parlay-studio/config.yaml` (user-scoped), then `/home/dev/myapp/.parlay-studio/config.yaml` (project-scoped). Reads every `STUDIO_*` env var. Applies CLI flags on top. Built-in defaults fill the remaining keys.
System: studio-config: resolved configuration:
System:   figma_mcp_url      = https://mcp.figma.com/v1   (source: env)
System:   figma_token        = ***                        (source: user-file)
System:   server_port        = 0                          (source: default)
System:   idle_timeout       = 30m                        (source: default)
System:   open_browser       = true                       (source: default)
System: Studio started.

#### Branch: Env beats file (precedence)

User: ==editor==  ~/.config/parlay-studio/config.yaml  # contains figma_mcp_url: https://stage.figma.example
User: STUDIO_FIGMA_MCP_URL=https://prod.figma.com/v1 parlay-studio --project ==/home/dev/myapp==
System: studio-config: resolved configuration:
System:   figma_mcp_url      = https://prod.figma.com/v1  (source: env)
System (background): Env value `https://prod.figma.com/v1` overrode the user-file value `https://stage.figma.example`. Project-scoped file did not declare the key.

#### Branch: Project file beats user file

User: ==editor==  ~/.config/parlay-studio/config.yaml          # contains figma_mcp_url: https://stage.figma.example
User: ==editor==  /home/dev/myapp/.parlay-studio/config.yaml   # contains figma_mcp_url: https://prod.figma.com/v1
User: parlay-studio --project /home/dev/myapp
System: studio-config: resolved configuration:
System:   figma_mcp_url      = https://prod.figma.com/v1  (source: project-file)

#### Branch: Unknown key in config file emits a warning

User: ==editor==  /home/dev/myapp/.parlay-studio/config.yaml  # contains figma_team_url: https://figma.com/team/...  (a typo of figma_mcp_url)
User: parlay-studio --project /home/dev/myapp
System: studio-config: WARN unknown key `figma_team_url` in /home/dev/myapp/.parlay-studio/config.yaml (line ==N==) — ignored. Did you mean `figma_mcp_url`?
System (background): Startup continues; the unknown key has no effect. Forward-compatibility: future Studio versions may define `figma_team_url` and older binaries should not refuse to start.
System: Studio started.

#### Branch: Secret leaked into project-scoped config file

User: ==editor==  /home/dev/myapp/.parlay-studio/config.yaml  # contains figma_token: sk_xxx
User: parlay-studio --project /home/dev/myapp
System: studio-config: ERROR `figma_token` in /home/dev/myapp/.parlay-studio/config.yaml (line ==N==) — secret keys must live in the user-scoped config at ~/.config/parlay-studio/config.yaml or in the STUDIO_FIGMA_TOKEN environment variable. The project-scoped config file is committed to the repository; secrets must never be committed. Studio refuses to start. See studio/docs/figma-mcp-setup.md.
System: (exit non-zero)

#### Branch: STUDIO_CONFIG_PATH escape hatch is rejected

User: STUDIO_CONFIG_PATH=/tmp/test-config.yaml parlay-studio --project ==/home/dev/myapp==
System (background): `STUDIO_CONFIG_PATH` is not a recognized Studio env var (the `STUDIO_*` prefix is for typed configuration keys only, not for redirecting the config-file path).
System: studio-config: WARN unknown env var `STUDIO_CONFIG_PATH` — ignored. The project-scoped config file path is strictly `<project-root>/.parlay-studio/config.yaml`; redirect tests via `--project` or `STUDIO_PROJECT_ROOT` to a fixture project root.
System: Studio started.

---

### Figma MCP connection configuration

**Trigger**: A Parlay Studio maintainer configures Studio's Figma MCP connection for the first time, and Studio attempts to authenticate against the configured endpoint.

User: ==editor==  ~/.config/parlay-studio/config.yaml
User: # adds figma_token: ==token==
User: ==editor==  /home/dev/myapp/.parlay-studio/config.yaml
User: # adds figma_mcp_url: https://mcp.figma.com/v1
User: parlay-studio --project /home/dev/myapp
System (background): Loads merged config. figma_mcp_url from project-scoped file. figma_token from user-scoped file. Passes MCP URL to `mcpclient.Probe()` and token to the SDK client constructor.
System: studio-config: figma_mcp_url=https://mcp.figma.com/v1 (source: project-file); figma_token=*** (source: user-file)
System: mcpclient: probe successful — endpoint=https://mcp.figma.com/v1 email===maintainer@example.com== seat=Dev
System: Studio started.

#### Branch: Missing figma_mcp_url

User: # no figma_mcp_url anywhere — not in env, not in either config file
User: parlay-studio --project /home/dev/myapp
System: studio-config: ERROR `figma_mcp_url` is not set. Resolution sources tried (in precedence order): STUDIO_FIGMA_MCP_URL env var; `figma_mcp_url:` in /home/dev/myapp/.parlay-studio/config.yaml; `figma_mcp_url:` in ~/.config/parlay-studio/config.yaml. There is no default. See studio/docs/figma-mcp-setup.md.
System: (exit non-zero, code: studio-config-figma-mcp-url-missing)

#### Branch: Missing figma_token

User: STUDIO_FIGMA_MCP_URL=https://mcp.figma.com/v1 parlay-studio --project /home/dev/myapp
User: # but no figma_token anywhere
System: studio-config: ERROR `figma_token` is not set. Resolution sources tried: STUDIO_FIGMA_TOKEN env var; `figma_token:` or `figma_token_file:` in ~/.config/parlay-studio/config.yaml. There is no default. See studio/docs/figma-mcp-setup.md for token-acquisition steps.
System: (exit non-zero, code: studio-config-figma-token-missing)

#### Branch: figma_token_file pointer

User: ==editor==  ~/.config/parlay-studio/config.yaml
User: # adds figma_token_file: /run/secrets/figma-token
User: # /run/secrets/figma-token is a mounted secret file containing the token
User: STUDIO_FIGMA_MCP_URL=https://mcp.figma.com/v1 parlay-studio --project /home/dev/myapp
System (background): Loads user-scoped file. Resolves figma_token by reading /run/secrets/figma-token. Same redaction and precedence as the inline form.
System: studio-config: figma_token=*** (source: user-file via figma_token_file → /run/secrets/figma-token)
System: Studio started.

#### Branch: Both figma_token and figma_token_file in the user file

User: ==editor==  ~/.config/parlay-studio/config.yaml
User: # contains both figma_token: sk_xxx AND figma_token_file: /run/secrets/figma-token
User: parlay-studio --project /home/dev/myapp
System: studio-config: ERROR ~/.config/parlay-studio/config.yaml declares both `figma_token` (line ==N==) and `figma_token_file` (line ==M==) — exactly one must be used. The inline-vs-pointer choice is per-deployment.
System: (exit non-zero, code: studio-config-figma-token-double-source)

#### Branch: Token never appears in logs

User: parlay-studio --project /home/dev/myapp 2>&1 | tee studio.log
User: grep -F "$(< ~/.config/parlay-studio/figma-token)" studio.log
System: (no match) — the resolved token never appears in any log line. The startup config-listing redacts it as `***`; the HTTP server's request/response envelopes never echo it; the figma-mcp-client wrapper holds it only in the `Client` struct's auth field, which has no `String()` or `MarshalJSON` exposure.

#### Branch: Per-feature Figma URL is not a Studio-config concern (cross-feature, deferred)

User: ==searches the resolved config for the Figma file URL Studio is editing today==
System (background): Studio-config does not carry a per-feature Figma file URL. Each feature's layout artifact (page schema's Figma reference field, exact location TBD) carries the file URL for that feature.
System: studio-config: no `figma_file_url` key exists in the Studio configuration. The per-feature Figma file URL lives with the feature's layout artifact, not in Studio's global config — see the page-schema or design-loop feature for that field's home.

---

### Web server runtime configuration

**Trigger**: A Parlay Studio maintainer launches Studio and observes the web server's port, idle timeout, and browser-open behavior.

User: parlay-studio --project /home/dev/myapp
System (background): Resolves ServerPort=0 (default) → asks the OS for a free port. IdleTimeout=30m (default). OpenBrowser=true (default).
System: studio-config: server_port=0→==54221== (source: default + os-allocated); idle_timeout=30m (source: default); open_browser=true (source: default)
System: web-server: bound to http://127.0.0.1:==54221==/; opening browser…
System: web-server: idle timeout 30m; will shut down on inactivity

#### Branch: Explicit port conflict

User: STUDIO_SERVER_PORT=18080 parlay-studio --project /home/dev/myapp
User: # but port 18080 is held by another process
System: studio-config: server_port=18080 (source: env)
System: web-server: ERROR cannot bind to 127.0.0.1:18080 — address already in use. Either free the port or unset STUDIO_SERVER_PORT to let Studio pick a free one.
System: (exit non-zero, code: studio-config-server-port-conflict)

#### Branch: Idle timeout disabled (0 = never)

User: STUDIO_IDLE_TIMEOUT=0 parlay-studio --project /home/dev/myapp
System: studio-config: idle_timeout=0 (source: env) — timeout disabled; server runs until explicitly closed
System: web-server: bound to http://127.0.0.1:==54221==/; opening browser…
System: web-server: idle timeout disabled
System (background): Studio runs indefinitely. No auto-shutdown on inactivity. Caller closes via Ctrl+C, SIGTERM, or the explicit `/api/shutdown` endpoint.

#### Branch: Negative idle timeout rejected

User: STUDIO_IDLE_TIMEOUT=-1s parlay-studio --project /home/dev/myapp
System: studio-config: ERROR `idle_timeout=-1s` is negative. The value must be >= 0 (0 disables the timeout; positive values set the inactivity window). See `STUDIO_IDLE_TIMEOUT` documentation.
System: (exit non-zero, code: studio-config-idle-timeout-invalid)

#### Branch: Headless CI run with browser disabled

User: STUDIO_OPEN_BROWSER=false parlay-studio --project /home/dev/myapp
System: studio-config: open_browser=false (source: env)
System: web-server: bound to http://127.0.0.1:==54221==/; browser auto-open disabled
System: web-server: paste this URL into a browser if you want to interact: http://127.0.0.1:==54221==/
System: web-server: idle timeout 30m; will shut down on inactivity
System (background): CI scripts use this branch when driving Studio against a fixture project for automated testing.

#### Branch: Idle-timeout source semantics (cross-feature, deferred)

User: ==asks: idle-timeout counts from what, exactly — last HTTP request, last designer keystroke in the browser, last write to a parlay file?==
System (background): The web-server-harness feature owns this decision (referenced in Q3.1 of studio-config). Studio-config pins the duration knob; the harness decides what "idle" means.
System: studio-config: idle_timeout=30m (source: default) — the "idle" event the timeout fires on is defined by the web-server-harness feature, not here

---

### Studio project root resolution

**Trigger**: A Parlay Studio maintainer or a Core CLI hook starts the `parlay-studio` binary, and Studio must determine which parlay project to operate against.

User: cd /home/dev/myapp/some/deeply/nested/subdir
User: parlay-studio
System (background): No `--project` flag. No `STUDIO_PROJECT_ROOT` env var. Falls back to cwd walk-up: walks `/home/dev/myapp/some/deeply/nested/subdir` → `/home/dev/myapp/some/deeply/nested` → `/home/dev/myapp/some/deeply` → `/home/dev/myapp/some` → `/home/dev/myapp` (found `.parlay/`). Stops.
System: studio-config: project_root=/home/dev/myapp (source: cwd-walkup)
System: Studio started.

#### Branch: Explicit --project flag wins

User: parlay-studio --project /home/dev/myapp
System (background): `--project` is highest precedence. Verifies `/home/dev/myapp/.parlay/` exists. Does not walk up.
System: studio-config: project_root=/home/dev/myapp (source: --project flag)

#### Branch: STUDIO_PROJECT_ROOT env var

User: STUDIO_PROJECT_ROOT=/home/dev/myapp parlay-studio
System (background): No `--project` flag. Env var has next-highest precedence. Verifies `/home/dev/myapp/.parlay/` exists. Does not walk up.
System: studio-config: project_root=/home/dev/myapp (source: STUDIO_PROJECT_ROOT)

#### Branch: Walk-up finds nothing

User: cd /tmp/scratch  # /tmp has no .parlay/ ancestor
User: parlay-studio
System: studio-config: ERROR could not resolve a parlay project root. Resolution sources tried (in precedence order): `--project` flag; `STUDIO_PROJECT_ROOT` env var; cwd walk-up from `/tmp/scratch` (terminated at `/` without finding `.parlay/`). See studio/docs/figma-mcp-setup.md.
System: (exit non-zero, code: studio-config-project-root-not-found)

#### Branch: $HOME terminates the walk-up

User: cd /home/dev/scratch  # no .parlay/ here or in any descendant of $HOME above
User: parlay-studio
System (background): Walks `/home/dev/scratch` → `/home/dev` (== $HOME). Terminates at $HOME without crossing upward into `/home`, `/`, or other users' directories.
System: studio-config: ERROR could not resolve a parlay project root. Walk-up from `/home/dev/scratch` terminated at $HOME=`/home/dev` without finding `.parlay/`. See studio/docs/figma-mcp-setup.md.
System: (exit non-zero, code: studio-config-project-root-not-found)

#### Branch: cwd already outside $HOME walks all the way up

User: cd /var/lib/parlay-projects/team-app/some-subdir  # /var/lib/parlay-projects/team-app has .parlay/
User: parlay-studio
System (background): Walks `/var/lib/parlay-projects/team-app/some-subdir` → `/var/lib/parlay-projects/team-app` (found `.parlay/`). The "stop at $HOME" terminator does not engage because cwd was already outside $HOME at entry.
System: studio-config: project_root=/var/lib/parlay-projects/team-app (source: cwd-walkup)
System: Studio started.

#### Branch: Explicit --project pointed at a subdirectory (rejected — strict-root)

User: parlay-studio --project /home/dev/myapp/some/subdir
System (background): `--project` is explicit. Verifies `/home/dev/myapp/some/subdir/.parlay/` exists — it does not. Does NOT walk up; explicit overrides are strict-root.
System: studio-config: ERROR `--project /home/dev/myapp/some/subdir` is not a parlay project root — no `.parlay/` subdirectory found at that path. Explicit overrides must point at the project root; Studio does not walk up from explicit `--project` or `STUDIO_PROJECT_ROOT` values (the rationale is that explicit invocations should be unambiguous; if you want walk-up behavior, drop the flag and rely on cwd).
System: (exit non-zero, code: studio-config-project-root-invalid)

#### Branch: Explicit override at a non-existent path

User: parlay-studio --project /nonexistent/path
System: studio-config: ERROR `--project /nonexistent/path` does not exist on disk. Explicit overrides must name an existing directory containing `.parlay/`.
System: (exit non-zero, code: studio-config-project-root-invalid)

#### Branch: Core's CLI hook passes the project root (cross-feature, deferred)

User: ==parlay (core) hook invokes parlay-studio with some shape of project-root handoff==
System (background): The exact handoff shape — `--project`, `STUDIO_PROJECT_ROOT`, or setting cwd — is owned by Core's CLI-hook feature (referenced in Q4.1 of studio-config). All three forms are accepted at studio-config's side; the precedence order applies.
System: studio-config: project_root=/home/dev/myapp (source: ==depends on hook handoff==)

---
