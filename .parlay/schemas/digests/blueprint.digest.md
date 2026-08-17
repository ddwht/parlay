# Application Blueprint Schema — authoring digest

Derived from `blueprint.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

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

---

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

---

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

---

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

**Not the same vocabulary as capabilities' `policies:`.** This `authorization.policies` block (named, free-form business rules — `owner or admin`, `task deletion`) is a different vocabulary from `capabilities.yaml`'s closed three-value `policies:` enum (`auth-required`/`permission-required`/`transaction-required`; see `capabilities.schema.md`'s "Policy-step-error tie rules" and "Relationship to blueprint's `authorization.policies`" sections). The two are related in spirit — both are "policy" in the everyday sense — but they're not the same field, don't share an identifier space, and a capability operation declaring `permission-required` does not currently reference a specific entry here by name. Don't conflate the two when reading either schema.

---

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

---

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

---

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

---

Native-app-only section for OS-level integration points. Omit entirely for web and CLI apps.

| Field | Required | Description |
|---|---|---|
| `push-notifications.enabled` | Yes | Whether the app uses push notifications |
| `push-notifications.categories` | No | Notification types and their tap actions |
| `background-tasks` | No | Scheduled or event-driven background work |
| `widgets` | No | Home screen / lock screen widgets |
| `extensions` | No | App extensions (share sheets, today widgets, Siri intents) |
