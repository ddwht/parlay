# Studio-config — Infrastructure

---

## Layered Studio configuration loader

**Affects**: Studio binary's startup configuration; the project-scoped and user-scoped configuration files; the environment-variable namespace; the secret-vs-non-secret invariant that gates which file a value may live in; the startup log line that traces every resolved key to its source.

**Behavior**: At Studio startup, a single configuration-loading component resolves a typed Studio configuration from five sources, merged in deterministic precedence — command-line flags, environment variables matching a reserved prefix, a project-scoped YAML file, a user-scoped YAML file, and built-in defaults. Command-line flags win over the environment; the environment wins over both files; the project file wins over the user file; defaults are last. The project-scoped file lives at a fixed path inside the resolved parlay project root and is intended to be committed to the project repository. The user-scoped file lives at a fixed XDG-compliant path under the operator's home directory and is never committed. Environment variables consumed by Studio match the reserved prefix exclusively; other environment variables cannot influence Studio's configuration even when their name happens to look related. Unknown keys in either configuration file emit a warning and are otherwise ignored, so future Studio versions can introduce keys without breaking older binaries that read the same file. Keys flagged as secret in the configuration schema must appear only in the user-scoped file or in the environment; their presence in the project-scoped file fails startup with a stable code. On a successful merge, the configuration is logged once with every secret-typed value redacted and every non-secret value labeled by the source that supplied it. The component is the single supported import for reading Studio configuration; direct environment-variable reads matching the reserved prefix or direct YAML loads of either configuration path from any other location are rejected on review.

**Invariants**:
- Configuration sources are exactly five, in the precedence order: command-line flags, then environment variables, then project-scoped file, then user-scoped file, then built-in defaults
- Environment variables consumed by Studio match a reserved prefix exclusively; environment variables outside the prefix do not influence Studio configuration regardless of how they are named
- The project-scoped configuration file lives at a fixed path relative to the resolved parlay project root; that path is not redirectable by an environment variable or flag (testing redirects via the project-root resolution layer, not via a config-path escape hatch)
- The user-scoped configuration file location honors the XDG base-directory convention: an environment variable override for the XDG config home takes precedence over the default home-relative path
- Both configuration files use YAML; an unknown key emits a warning naming the file, line, and key, and the startup continues
- Keys flagged as secret in the configuration schema fail startup with a stable code when they appear in the project-scoped file; the same keys are allowed in the user-scoped file and in the environment
- The startup log line listing the merged configuration redacts every secret-typed value and labels every entry with the source that contributed it
- A repository-wide search for direct environment-variable reads matching Studio's reserved prefix, or direct YAML loads of either configuration path, finds matches only inside the dedicated configuration component

**Source**: @studio-config/studio-configuration-sources-precedence-and-file-layout

**Backward-Compatible**: yes

**Notes**:
- Foundational change — there is no prior Studio configuration loader to maintain compatibility with
- The unknown-key warning policy is forward-compat-friendly; switching to hard-error semantics later is a v2-or-later spec revision
- The "no STUDIO_CONFIG_PATH escape hatch" rule means tests that need a custom configuration path redirect by pointing the project-root resolver at a fixture directory containing its own project-scoped configuration file

---

## Figma MCP connection configuration keys

**Affects**: figma-mcp-client wrapper's startup inputs (the endpoint URL and the authentication credential); secret-leak prevention for the authentication credential; the inline-vs-pointer choice for the credential's storage; the separation between Studio-global configuration and per-feature layout configuration.

**Behavior**: Studio's configuration carries two keys consumed by the figma-mcp-client wrapper at startup: the MCP endpoint URL and the Figma authentication credential. The endpoint URL is project-scoped — it lives in the project-scoped configuration file or in an environment variable, because different parlay projects may target different Figma teams or environments. The authentication credential is secret and user-scoped — it lives in the user-scoped configuration file or in an environment variable, never in the project-scoped file. The credential's user-scoped storage supports both an inline form (the credential value directly in the YAML) and a pointer form (a filesystem path to a file whose contents are the credential value); both forms share the same precedence relative to other sources, and exactly one of the two may appear in the user-scoped file (declaring both is a stable startup error). The pointer form supports operator workflows where the credential is mounted from an OS-level secret store rather than written into a YAML file. The credential is redacted in every log line, never returned in any HTTP response, and never persisted by Studio beyond the lifetime of the in-memory wrapper client. v1 supports token-based authentication only; the OAuth flow is reserved for a v2-or-later spec revision, not a runtime fallback. The per-feature Figma file or team URL — the specific Figma file a designer is editing for a given feature — is deliberately not a Studio-config concern; it varies per layout and belongs alongside the feature's layout artifact, not in Studio's global configuration.

**Invariants**:
- The MCP endpoint URL is read from either an environment variable with the reserved prefix or the project-scoped configuration file; missing-from-both fails startup with a stable code
- The Figma authentication credential is read from either an environment variable with the reserved prefix, an inline key in the user-scoped configuration file, or a pointer key in the user-scoped configuration file; missing-from-all-three fails startup with a stable code
- The authentication credential MUST NOT appear in the project-scoped configuration file; its presence there fails startup with the secret-in-project-file stable code from the loader's secret invariant
- Inline and pointer forms of the credential are mutually exclusive within the user-scoped file; declaring both fails startup with a stable double-source code
- The resolved authentication credential does not appear in any log line, HTTP response body, HTTP response header, or persisted disk write produced by Studio
- v1 emits exactly one supported authentication shape (token); the OAuth flow is not selectable at runtime — switching to OAuth is a v2-or-later spec revision
- Per-feature Figma file URLs are not part of Studio's configuration schema; the absence of such a key is enforced by a search that finds zero references in the dedicated configuration component

**Source**: @studio-config/figma-mcp-connection-configuration

**Backward-Compatible**: yes

**Notes**:
- The cross-feature question of where per-feature Figma file URLs live (page schema, layout artifact, separate spec layer) is referenced as an open question on this intent so it isn't forgotten, but its resolution lives in another feature
- The pointer form's path resolution is relative to the user-scoped configuration file's location when the path is relative; absolute paths are honored verbatim

---

## Web server runtime configuration keys

**Affects**: web-server-harness's startup behavior — port-binding strategy, idle-shutdown timing, browser-open behavior; the operator-visible failure modes for port conflicts and invalid timeout values; the headless/CI invocation path.

**Behavior**: Studio's configuration carries three keys consumed by the web-server harness at startup: the server port, the idle timeout, and a flag controlling whether Studio opens the operator's browser automatically. The server port defaults to a sentinel value that means "ask the operating system to allocate a free port"; a non-default value binds to that specific port and fails fast if the port is held by another process. The idle timeout is a duration; the default value sized for typical designer sessions is a thirty-minute window. A zero value disables the timeout entirely (Studio runs until explicitly closed); negative values fail startup with a stable code, because negative durations have no defensible meaning for an idle-shutdown timer. The browser-open flag defaults to true and is overridden to false by the operator for headless or CI invocations; with browser-open disabled, Studio still logs the bound URL so the operator can paste it into a browser manually. All three keys are project-scoped — they describe how Studio runs against a particular project, not per-user preferences — and the user-scoped configuration file is not the canonical home for them (a value found only in the user file still applies, but the loader emits a warning recommending the project file). The autoselected port is logged at startup with a stable line shape so testers and operators can find the URL even when the browser-open path is disabled.

**Invariants**:
- The server-port key defaults to the OS-allocate-a-free-port sentinel; a non-default value binds to that specific port and fails startup with a stable port-conflict code when the port is unavailable
- The idle-timeout key defaults to a thirty-minute window; a zero value disables the timeout; a negative value fails startup with a stable code naming the negative-duration error
- The browser-open key defaults to true; a false value disables the browser-open hook and triggers a startup log line containing the bound URL for manual use
- All three keys are project-scoped; a value present only in the user-scoped configuration file applies but generates a warning at startup recommending the project file
- The autoselected port is logged at startup with a stable line shape; the line is present even when the browser-open hook is disabled

**Source**: @studio-config/web-server-runtime-configuration

**Backward-Compatible**: yes

**Notes**:
- The semantics of "idle" for the timeout (last request vs last operator activity vs last on-disk mutation) is owned by the web-server-harness feature, not this one — Studio-config pins the duration knob; the harness pins the trigger event
- The exact log-line format for the browser-disabled URL hint is also a web-server-harness concern; this fragment requires that the URL be logged, not how

---

## Studio project root resolution

**Affects**: Studio's project-discovery layer; the precedence order between explicit overrides and cwd walk-up; the walk-up termination at the operator's home directory; the strict-root invariant for explicit overrides; the failure-mode shape when no parlay project can be located.

**Behavior**: Before Studio's configuration loader can find the project-scoped configuration file, it must know which parlay project the binary is operating against. A project-resolution layer determines this from three sources in deterministic precedence: an explicit command-line override, an explicit environment-variable override, and a cwd-based walk-up. The two explicit overrides are strict-root: they must point at the actual project root (the directory directly containing the parlay project marker subdirectory), not at a subdirectory of the project. The walk-up fallback walks from the current working directory toward the root of the filesystem, returning the first ancestor that contains the parlay project marker subdirectory; the walk terminates at the operator's home directory (when the cwd was inside the home directory at entry) and at the filesystem root in all cases. A successful resolution logs the resolved absolute path and the source that supplied it. A failure (no marker found from any source) fails startup with a stable code naming every source that was tried. The resolution runs before the configuration loader because the project-scoped configuration file's path depends on the resolved project root.

**Invariants**:
- Project root resolution sources are exactly three, in the precedence order: command-line override, then environment-variable override, then cwd-based walk-up
- The cwd-based walk-up terminates at the operator's home directory (when cwd was inside the home at entry) and at the filesystem root; the walk-up never crosses out of the home directory upward unless cwd was already outside the home directory
- Explicit overrides (command-line flag and environment variable) are strict-root: they MUST point at a directory that directly contains the parlay project marker subdirectory; pointing them at a subdirectory of a project fails startup with the project-root-invalid stable code
- The resolved project root is logged at startup with the source that supplied it
- Failure to resolve a project root (no marker found from any source) fails startup with a stable not-found code that names every source that was tried
- Project root resolution runs before the configuration loader; the project-scoped configuration file's path is derived from the resolved root by the resolver, not re-derived by the loader

**Source**: @studio-config/studio-project-root-resolution

**Backward-Compatible**: yes

**Notes**:
- The strict-root rule for explicit overrides is the load-bearing decision of this fragment: explicit invocations should be unambiguous; walk-up is reserved for the implicit cwd fallback where ambiguity is the norm
- How Core's CLI hook hands off the project root to Studio (command-line, environment, or by setting cwd) is owned by Core's hook surface feature, not this one — Studio-config accepts all three forms and applies the precedence order regardless of which one Core uses

---
