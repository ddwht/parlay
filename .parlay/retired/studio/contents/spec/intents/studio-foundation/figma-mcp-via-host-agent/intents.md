# Figma-mcp-via-host-agent

> Figma's remote MCP server is gated by a catalog admission process — only pre-approved clients (VS Code, Cursor, Claude Code) can authenticate. Studio is not on the catalog and the registration endpoint returns 403 for non-catalog clients. Rather than block on Figma's waitlist or pivot Studio entirely into a Claude Code skill, this feature establishes a surgical architectural decision: **Studio defers Figma MCP traffic to whichever host agent (Claude Code, Cursor, future MCP-catalog clients) the operator is already running**. Studio's binary keeps the web server + harness + lifecycle for the Domain Model Editor; the Design Loop becomes a host-agent skill that uses the host's existing Figma MCP connection and reads/writes the same on-disk artifacts (page schemas, layouts, domain-model.yaml) the Studio binary owns. This feature retracts the Studio-direct-MCP work: the `figma-mcp-client` Go package, the Figma-related keys in `studio-config`, the MCP probe in `web-server-harness`'s boot sequence, and Intent 4 of `web-server-harness` (Figma MCP Phase 0 wiring) are all removed. The Studio binary no longer authenticates against Figma MCP at all.

---

## Studio defers Figma MCP to the host agent

**Goal**: Pin the architectural decision that Studio's binary never talks to Figma's MCP server directly. Figma MCP operations belong to whichever host agent the operator is already running (Claude Code today; Cursor / VS Code / future catalog clients in principle). The Design Loop is a host-agent skill, not a Studio binary feature. Studio and the host agent collaborate via on-disk artifacts — the host writes layouts and metadata to paths Studio's file-I/O layer reads, and reads paths Studio writes.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The web-server-harness integration test surfaced a hard blocker: Figma's remote MCP server requires OAuth-issued bearer tokens with the `mcp:connect` scope, and the dynamic client registration endpoint returns 403 for non-catalog clients. The developer documentation confirms: *"Only clients listed in the Figma MCP Catalog like VS Code, Cursor, or Claude Code can connect to the Figma MCP Server."* Three options were considered: (A) wait for Figma to admit Studio to the catalog and ship the OAuth flow once it's possible; (B) pivot Studio entirely into a Claude Code skill (no binary); (D) park the OAuth work and ship the Domain Model Editor first. This feature picks a surgical hybrid: keep the Studio binary for the work that doesn't need Figma MCP (Domain Model Editor, file I/O, project resolution, the web UI harness), and delegate the work that does need Figma MCP (the Design Loop's round-trip) to whichever host agent already has the connection. Studio and the host agent communicate exclusively through the parlay project's on-disk artifacts; there is no IPC, no shared connection state, no protocol between them beyond the YAML files Studio and the host already both read and write. The decision is reversible: if Figma admits Studio to the catalog later, the Studio binary can grow direct MCP capability without disturbing the host-agent integration.

**Action**: Establish the architectural decision in the spec layer and update the deployed parlay skill documentation to reflect it. Specifically: the Design Loop feature becomes a parlay-skill that the host agent (Claude Code, Cursor) invokes; the skill's documented behavior describes the read/write contract against on-disk artifacts (layouts, page schemas, domain-model.yaml); the host agent's Figma MCP connection is the assumed transport. The Studio binary's responsibilities are limited to: serving the Domain Model Editor's web UI, owning the file-I/O abstraction that writes parlay project paths, hosting the chi router harness, and providing the binary-local lifecycle (boot, idle, signal, shutdown). No code under `studio/` makes outbound calls to `mcp.figma.com` or to any other MCP server. The supersession of figma-mcp-client, the Figma keys in studio-config, web-server-harness Intent 4, and web-server-harness Intent 1's MCP-probe step is documented in this intent's Notes; the retraction is implemented by the retraction intent below.

**Objects**: host-agent-mediation, design-loop-as-skill, on-disk-artifact-contract, studio-binary-scope-without-figma-mcp, host-agent-figma-mcp-connection, supersession-of-direct-mcp-features

**Constraints**:
- No code under `studio/` makes outbound network calls to Figma's MCP server, to `mcp.figma.com`, or to any MCP server. A repository-wide search for `mcp.figma.com`, `modelcontextprotocol/go-sdk` imports, or MCP-protocol JSON-RPC method names (`initialize`, `tools/call`) under `studio/` returns matches only in spec text (intents/dialogs/infrastructure) and historical commit messages
- The Design Loop's Figma MCP operations are documented as a parlay-skill (executed by the host agent); the skill's contract is "read the named on-disk artifacts, perform Figma MCP operations, write results back to the named on-disk artifacts." The skill is the only sanctioned execution path for Studio-related Figma MCP work
- Studio and the host agent communicate exclusively through on-disk artifacts (YAML files, the layout typed-tree, domain-model.yaml, page schemas); no shared process state, no IPC sockets, no shared in-memory caches
- The Studio binary's responsibilities are limited to: the Domain Model Editor's web UI; the chi router harness for the Domain Model Editor; the file-I/O abstraction that writes parlay project paths atomically; the binary-local boot/shutdown/idle lifecycle. The binary does NOT host the Design Loop's HTTP surface
- This feature supersedes: figma-mcp-client (entire feature), studio-config Intent 2 (Figma MCP connection configuration), web-server-harness Intent 4 (Figma MCP Phase 0 wiring), and the MCP-probe step in web-server-harness Intent 1's boot sequence. The supersession is documented here; the prior features' specs stay frozen at-time-of-shipment per the established project pattern
- The decision is reversible: if Figma admits Studio to the catalog at a later date, a follow-up feature can grow direct MCP capability in the Studio binary without invalidating the host-agent skill path

**Verify**:
- `git grep -nE 'mcp\.figma\.com|modelcontextprotocol/go-sdk' -- studio/` returns matches only inside `studio/spec/intents/` (spec text describing the history) — no matches in `studio/internal/`, `studio/cmd/`, or `studio/go.mod` after the retraction lands
- The Design Loop feature's intents (currently unauthored stubs under `studio-foundation/design-loop/`) describe a parlay-skill, not a Studio binary feature; the skill's contract names the on-disk artifacts it reads and writes
- A new operator invoking Studio sees the Domain Model Editor's web UI; nothing in the binary's startup attempts to authenticate against Figma or probe an MCP endpoint
- `studio/internal/server/boot.go`'s boot sequence has no MCP-probe step; the boot list goes parse-flags → resolve-project-root → load-config → build-router → start-server → log-URL → open-browser → install-signals → start-idle-timer → block (10 steps)
- The deployed parlay-skill catalog (under `.claude/skills/` after `parlay upgrade`) includes a `parlay-design-loop` skill that documents the Figma MCP delegation pattern; the skill is invokable from Claude Code with the operator's existing Figma MCP catalog connection

**Notes**:
- This decision was made on 2026-05-13 after the web-server-harness integration test revealed Figma's catalog gating. The full reasoning, the rejected alternatives (Option A: wait for catalog; Option B-maximal: pivot Studio entirely to skills; Option C: REST API), and the decision rationale are in the conversation that produced this feature
- Architecture v4 §7's "shared web server harness for both tools" framing is partially obsolete after this feature ships — the Design Loop never enters Studio's web server. A future architecture-doc revision (v4.1 or v5) will reflect the split

---

## Retract the Studio-direct-MCP code

**Goal**: Remove the Go code, configuration keys, and tests that implemented the Studio-direct-MCP architecture that this feature retracts. The deletions are the concrete consequence of the architectural decision above; specifying them here as a dedicated retraction intent makes the cleanup auditable and reviewable as a single change.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: figma-mcp-client shipped 9 Go files under `studio/internal/mcpclient/` plus a setup doc; web-server-harness shipped 3 more files (transport.go, client_test.go, integration_test.go) and modified 3 existing ones (client.go, tools.go, probe.go) to fill the Phase 0 stubs; studio-config shipped figma.go and figma_test.go under `studio/internal/config/` plus the `STUDIO_FIGMA_TOKEN` env binding. All of that code is dead under the new architecture — Studio does not call Figma MCP at all. Carrying dead code accumulates entropy (imports drift, tests run for no reason, readers spend time understanding code that doesn't matter). The cleanest retraction is to delete the package, the config keys, and the boot-step entries that referenced them. The historical intents, dialogs, infrastructure.md, buildfiles, and testcases stay on disk as design history; only the runtime artifacts get removed.

**Action**: In a single coordinated change (one buildfile run, one commit), remove the following: (1) the entire `studio/internal/mcpclient/` directory (client.go, tools.go, probe.go, errors.go, transport.go, all `_test.go` files, integration_test.go); (2) `studio/internal/config/figma.go` and `studio/internal/config/figma_test.go`; (3) the Figma-related fields on the `Config` struct in `studio/internal/config/config.go` (FigmaMCPURL, FigmaToken); (4) the references to those fields and to `STUDIO_FIGMA_TOKEN` in `studio/internal/config/loader.go` and `studio/internal/config/loader_test.go`; (5) the MCP probe step in `studio/internal/server/boot.go` (boot step 4 in the old 12-step sequence becomes absent in the new 10-step sequence; the boot result struct loses the `MCPClient`, `ChiRouterConstructed` fields if they referenced MCP-specific state); (6) the import of `studio/internal/mcpclient` from `studio/cmd/parlay-studio/main.go` and from any other location; (7) the entries in `go.mod` and `go.sum` for `github.com/modelcontextprotocol/go-sdk` (run `go mod tidy` after the deletions); (8) the `studio/docs/figma-mcp-setup.md` setup doc (it described the now-defunct env-var-based setup); (9) update the relevant baselines and code hashes via `parlay save-build-state` after the deletions land.

**Objects**: mcpclient-package-deletion, config-figma-key-removal, boot-step-removal, sdk-dependency-removal, setup-doc-removal, atomic-retraction-commit

**Constraints**:
- The retraction lands as a single coordinated change (one buildfile run, one commit) so reviewers can see the entire cleanup at once and the binary cannot be in a half-retracted state
- `studio/internal/mcpclient/` is removed entirely; no scaffolding or stub left behind. If Figma admits Studio to the catalog later, the package is re-authored from the (preserved) historical spec
- `studio/internal/config/` retains its loader infrastructure, the web-server keys, and the project-root resolver. Only the Figma-specific keys (`FigmaMCPURL`, `FigmaToken`) and their tests are removed. `studio-config`'s loader continues to honor the secret-key invariants for any future secret keys
- `studio/internal/server/boot.go` loses boot step 4 (MCP probe) and boot step 5 (construct authenticated MCP client). The remaining boot steps are renumbered 1–10 in the comments and stable-code naming. The graceful-shutdown sequence no longer closes the MCP client (there isn't one)
- `studio/cmd/parlay-studio/main.go` is updated to not import `studio/internal/mcpclient` and to not pass MCP-related fields to `server.Boot`
- `go.mod`'s `require github.com/modelcontextprotocol/go-sdk v1.6.0` is removed; `go mod tidy` runs to clean up indirect dependencies the SDK pulled in (jsonschema-go, segmentio, yosida95/uritemplate, golang.org/x/oauth2). The resulting `go.mod` is smaller and the dependency graph cleaner
- `studio/docs/figma-mcp-setup.md` is removed; the setup story for the host-agent skill path is documented under the parlay-design-loop skill (a separate feature, not this one)
- The historical intents/dialogs/infrastructure for the retracted features (figma-mcp-client, the Figma parts of studio-config and web-server-harness) stay on disk as design history; this retraction does NOT edit those spec files
- After the retraction, `parlay save-build-state --root studio` records the new (smaller) set of code hashes; `parlay verify-generated` reports no missing or modified files

**Verify**:
- `find studio/internal/mcpclient/ -type f` after the retraction returns "no such directory"
- `grep -rn 'FigmaMCPURL\|FigmaToken\|figma_mcp_url\|figma_token\|STUDIO_FIGMA_TOKEN' studio/internal/config/` returns zero matches
- `grep -n 'mcpclient\|mcp.figma' studio/cmd/parlay-studio/main.go studio/internal/server/boot.go` returns zero matches
- `grep 'modelcontextprotocol/go-sdk' studio/go.mod` returns no match; `go.sum` no longer contains entries for the SDK or its indirect dependencies (verifiable via `go mod tidy` being a no-op after the retraction)
- `find studio/docs/ -name 'figma-mcp-setup.md'` returns no match
- `CGO_ENABLED=0 go build ./...` and `CGO_ENABLED=0 go test ./...` from `studio/` both pass; the remaining test suite covers `studio/internal/config/` (minus the Figma tests) and `studio/internal/server/` (minus the MCP-probe boot test) — total test count is lower than before, all green
- `parlay verify-generated --root studio` reports the new file set as stable (after the post-retraction `parlay save-build-state` run)
- `git grep -nE 'mcp\.figma\.com|modelcontextprotocol/go-sdk' -- studio/internal/ studio/cmd/ studio/go.mod studio/go.sum studio/docs/` returns zero matches

---
