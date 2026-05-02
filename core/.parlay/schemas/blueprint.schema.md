# Application Blueprint Schema

File: `.parlay/blueprint.yaml`
Created during project setup or before first code generation.

The application blueprint captures **per-app architectural decisions** that are too app-specific for the framework adapter and too cross-cutting for any single feature. The adapter answers "how does our framework work?" — the blueprint answers "how is this app wired together?"

The blueprint is a **project-level singleton** — one per app, not per feature. It is **team-authored** (by the tech lead or architect) and lives in the tool zone (`.parlay/`), not in `spec/intents/`.

Every section is optional. A CLI app may have only `navigation.strategy: cli-subcommands`. A simple web app may need only `shells` and `navigation`. Native apps typically use all sections including `platform`.

## Structure

```yaml
app: <application name>

# --- Section 1: Layout hierarchy ---

shells:
  <shell-name>:
    description: <what this shell provides>
    chrome:
      - region: <region-name>
        widget: <framework widget from adapter>
        content: <what goes here>
    wraps: <"all" | [page-name, ...]>

# --- Section 2: Navigation ---

navigation:
  strategy: <hash | browser | native-stack | native-tab | cli-subcommands>
  default-route: <path>
  routes:
    - path: <route path>
      shell: <shell-name>
      guard: <guard-name | "none">
      lazy: <boolean>
  deep-links:
    - pattern: <URL pattern>
      target: <route path + action>
  not-found: <route-path | "render-404">

# --- Section 3: Authorization ---

authorization:
  strategy: <role-based | permission-based | attribute-based | none>
  roles:
    - name: <role identifier>
      description: <what this role can do>
  guards:
    <guard-name>:
      requires: <role-name | permission expression>
      redirect: <route path when unauthorized>
  policies:
    <policy-name>:
      controls: <what this policy governs>
      rule: <structured rule>

# --- Section 4: Data architecture ---

data:
  fetching: <on-mount | prefetch | stale-while-revalidate | graphql | none>
  caching:
    strategy: <none | in-memory | local-storage | service-worker>
    invalidation:
      - trigger: <what causes invalidation>
        scope: <what gets invalidated>
  offline:
    strategy: <none | read-only-cache | optimistic-writes>
  prefetch:
    - route: <path>
      data: [<what to prefetch>]

# --- Section 5: Error architecture ---

errors:
  boundaries:
    - scope: <app | shell | route | component>
      fallback: <what to show>
  http:
    "401": <action>
    "403": <action>
    "404": <action>
    "5xx": <action>
  retry:
    strategy: <none | exponential-backoff | immediate-once>
    applies-to: <reads | writes | all>

# --- Section 6: State architecture ---

state:
  global:
    - name: <state slice name>
      type: <model name or primitive type>
      source: <where it comes from>
  propagation: <context | props | url | global-store>
  url-state:
    - param: <query parameter name>
      controls: <what it drives>

# --- Section 7: Platform integration (native apps only) ---

platform:
  push-notifications:
    enabled: <boolean>
    categories:
      - name: <notification category>
        action: <what tapping it does>
  background-tasks:
    - name: <task name>
      trigger: <schedule or event>
      action: <what it does>
  widgets:
    - name: <widget name>
      shows: <what data it displays>
      refresh: <interval or event>
  extensions:
    - name: <extension point>
      type: <share-extension | today-widget | intent-extension>
```

## Section 1: Layout hierarchy (shells)

Shells describe the persistent chrome that wraps groups of pages. A shell has a name, a list of chrome regions (each mapped to a framework widget from the adapter), and a list of pages it wraps.

| Field | Required | Description |
|---|---|---|
| `<shell-name>` | Yes | Unique identifier for the shell |
| `description` | Yes | What this shell provides (e.g., "sidebar + header for authenticated pages") |
| `chrome` | Yes | List of chrome regions the shell renders |
| `chrome[].region` | Yes | Region name: `header`, `sidebar`, `footer`, `tab-bar`, `nav-bar`, `toolbar` |
| `chrome[].widget` | Yes | Framework widget name from the adapter (e.g., `Sider`, `Header`, `UITabBarController`) |
| `chrome[].content` | Yes | Human description of what goes in this region (e.g., "primary navigation", "user menu") |
| `wraps` | Yes | Either `"all"` or a list of page names from surface fragment `**Page**:` targets |

The first shell listed is the default — routes not explicitly assigned to a shell in `navigation.routes` inherit this one.

## Section 2: Navigation

Describes the app's route tree and how routes are wired together.

| Field | Required | Description |
|---|---|---|
| `strategy` | Yes | One of: `hash`, `browser`, `native-stack`, `native-tab`, `cli-subcommands` |
| `default-route` | No | Where `/` redirects to. Omit if `/` is a real page. |
| `routes` | No | List of route entries with shell, guard, and lazy assignments |
| `routes[].path` | Yes | Route path — must match a buildfile route `path:` value |
| `routes[].shell` | No | Shell name from `shells:`. Defaults to the first shell. |
| `routes[].guard` | No | Guard name from `authorization.guards:`, or `"none"`. Defaults to `"none"`. |
| `routes[].lazy` | No | Whether to lazy-load this route's bundle. Defaults to `false`. |
| `deep-links` | No | URL patterns for deep linking (native apps, universal links) |
| `deep-links[].pattern` | Yes | URL pattern with `:param` placeholders |
| `deep-links[].target` | Yes | Route path (with optional action hint) the deep link resolves to |
| `not-found` | No | What to show for unmatched routes: a route path or `"render-404"` |

Route entries annotate — not duplicate — buildfile routes. The buildfile route says "path `/tasks`, components: [task-board, ...]" (the **what**). The blueprint route says "path `/tasks` uses `app-shell`, requires `auth` guard, is lazy-loaded" (the **how**). Code generation joins on `path`.

## Section 3: Authorization

Describes the app's access control model.

| Field | Required | Description |
|---|---|---|
| `strategy` | Yes | One of: `role-based`, `permission-based`, `attribute-based`, `none` |
| `roles` | No | List of roles (required when strategy is `role-based`) |
| `roles[].name` | Yes | Role identifier (e.g., `admin`, `user`, `anonymous`) |
| `roles[].description` | Yes | What this role can do |
| `guards` | No | Named guard definitions |
| `guards.<name>.requires` | Yes | Role name or permission expression required to pass |
| `guards.<name>.redirect` | Yes | Route path to redirect to when guard rejects |
| `policies` | No | Fine-grained resource-level policies |
| `policies.<name>.controls` | Yes | What the policy governs (e.g., "task deletion") |
| `policies.<name>.rule` | Yes | Structured rule (e.g., "owner or admin") |

Guards are referenced by name in `navigation.routes[].guard`. They produce route-level protection. Policies are used by components for action-level checks (e.g., showing/hiding a delete button based on ownership).

## Section 4: Data architecture

Describes the app's data fetching, caching, and offline strategy.

| Field | Required | Description |
|---|---|---|
| `fetching` | Yes | Default fetch strategy: `on-mount`, `prefetch`, `stale-while-revalidate`, `graphql`, `none` |
| `caching.strategy` | No | Cache location: `none`, `in-memory`, `local-storage`, `service-worker` |
| `caching.invalidation` | No | Rules for when cached data becomes stale |
| `caching.invalidation[].trigger` | Yes | What causes invalidation (e.g., "mutation on Task") |
| `caching.invalidation[].scope` | Yes | What gets invalidated (e.g., "task-list, dashboard-metrics") |
| `offline.strategy` | No | Offline capability: `none`, `read-only-cache`, `optimistic-writes` |
| `prefetch` | No | Route-specific data prefetch rules |
| `prefetch[].route` | Yes | Route path to prefetch for |
| `prefetch[].data` | Yes | List of data to prefetch |

## Section 5: Error architecture

Describes error boundary placement and HTTP error handling.

| Field | Required | Description |
|---|---|---|
| `boundaries` | No | List of error boundary scopes |
| `boundaries[].scope` | Yes | Granularity: `app`, `shell`, `route`, `component` |
| `boundaries[].fallback` | Yes | What to show: "error page", "inline retry", "toast" |
| `http` | No | Map of HTTP status codes to actions |
| `http."401"` | No | Action for unauthorized (e.g., "redirect:/login") |
| `http."403"` | No | Action for forbidden |
| `http."404"` | No | Action for not found |
| `http."5xx"` | No | Action for server errors |
| `retry.strategy` | No | Retry approach: `none`, `exponential-backoff`, `immediate-once` |
| `retry.applies-to` | No | Which operations to retry: `reads`, `writes`, `all` |

## Section 6: State architecture

Describes global state slices and how state propagates through the app.

| Field | Required | Description |
|---|---|---|
| `global` | No | List of global state slices |
| `global[].name` | Yes | State slice name (e.g., "currentUser", "theme") |
| `global[].type` | Yes | Model name or primitive type |
| `global[].source` | Yes | Where the data comes from: `auth-flow`, `local-storage`, `api` |
| `propagation` | No | How global state reaches components: `context`, `props`, `url`, `global-store` |
| `url-state` | No | Query parameters that drive app state |
| `url-state[].param` | Yes | Query parameter name |
| `url-state[].controls` | Yes | What it drives (e.g., "active tab", "filter preset") |

## Section 7: Platform integration

Native-app-only section for OS-level integration points. Omit entirely for web and CLI apps.

| Field | Required | Description |
|---|---|---|
| `push-notifications.enabled` | Yes | Whether the app uses push notifications |
| `push-notifications.categories` | No | Notification types and their tap actions |
| `background-tasks` | No | Scheduled or event-driven background work |
| `widgets` | No | Home screen / lock screen widgets |
| `extensions` | No | App extensions (share sheets, today widgets, Siri intents) |

## Validation

When a blueprint file is loaded, the tool verifies:
- Valid YAML syntax
- Every shell name referenced in `navigation.routes[].shell` exists in `shells:`
- Every guard name referenced in `navigation.routes[].guard` exists in `authorization.guards:`
- `navigation.strategy` is one of: `hash`, `browser`, `native-stack`, `native-tab`, `cli-subcommands`
- `authorization.strategy` (if present) is one of: `role-based`, `permission-based`, `attribute-based`, `none`
- No duplicate route paths in `navigation.routes`
- `navigation.default-route` (if present) corresponds to a valid route path
- Page names in `shells[].wraps` (when not `"all"`) have corresponding `**Page**:` values in at least one feature surface (warning, not error — surfaces may not exist yet)

## Relationship to other artifacts

| Artifact | Relationship |
|---|---|
| **Adapter** | Sibling. Adapter says HOW to implement (framework conventions). Blueprint says WHAT to implement at the app level. Both feed into code generation. |
| **Buildfile** | Consumer. Buildfile routes JOIN with blueprint routes on `path`. Blueprint adds shell, guard, lazy metadata to each route. |
| **Surface** | Upstream. Surface fragments declare `**Page**:` targets. Blueprint shells reference those page names via `wraps`. |
| **Page manifest** | Parallel. Page manifest locks fragment ordering within a page. Blueprint assigns the page to a shell. Neither replaces the other. |
| **Config** | Neighbor in `.parlay/`. Config says which framework. Blueprint says how the app is structured. |

## Ownership model

| Aspect | Owner |
|---|---|
| Blueprint content | Tech lead / architect |
| Blueprint file location | Parlay (always `.parlay/blueprint.yaml`) |
| When it changes | App structure changes (new shell, new role, data strategy shift) |
| Effect of change | `parlay diff` reports `sections.blueprint: "changed"`, triggering regeneration of cross-cutting files (shells, guards, providers, error boundaries) |

## Pipeline consumption

**build-feature** reads the blueprint for:
- Guard-related elements: if a route has a guard, the buildfile component may need unauthorized/forbidden elements
- Role-aware fixtures: if authorization defines roles, fixtures should include users with different roles

**generate-code** reads the blueprint for:
- Generating shell/layout components from `shells:`
- Wiring the route tree with strategy, guards, lazy loading, redirects, and 404 handling
- Placing error boundaries at specified scopes
- Setting up global state providers from `state.global`
- Configuring data fetching infrastructure from `data:`
- Platform integration setup from `platform:` (native only)

The codegen boundary is preserved: the blueprint lives in `.parlay/`, so generate-code never needs to read `spec/intents/`.
