# Figma-mcp-via-host-agent — Infrastructure

---

## Host-agent mediation of Figma MCP

**Affects**: package import boundary, outbound network allowlist, scope-of-binary boundary
**Behavior**: The Studio binary does not authenticate against or call Figma's MCP server. Figma MCP operations are delegated to whichever host agent the operator is already running (Claude Code, Cursor, future MCP-catalog clients). The Design Loop is documented as a parlay-skill the host agent invokes, not as a Studio binary feature. Studio and the host agent communicate exclusively through on-disk artifacts under the parlay project root — page schemas, the layout typed-tree, the domain-model artifact, and other YAML files both already read and write. There is no IPC, no shared connection state, and no protocol between them beyond the on-disk artifact contract. The Studio binary's responsibilities are bounded to: serving the Domain Model Editor's web UI, owning the file-I/O abstraction that writes parlay project paths atomically, hosting the chi-style router harness for the web UI, and the binary-local boot/shutdown/idle lifecycle. The decision is reversible: if Figma admits Studio to the catalog at a later date, a follow-up feature may grow direct MCP capability in the Studio binary without invalidating the host-agent path.
**Invariants**:
- A repository-wide search for `mcp.figma.com` or for imports of the upstream MCP SDK under the Studio root returns matches only inside the spec tree (intents, dialogs, infrastructure prose describing the decision's history). No matches survive under the binary's source directories, the binary's command directory, or the Studio module manifest.
- The Design Loop's Figma MCP operations are documented as a parlay-skill executed by the host agent. The skill's contract names the on-disk artifacts it reads and writes; the skill is the only sanctioned execution path for Studio-related Figma MCP work.
- The Studio binary's startup sequence contains no MCP-probe step and constructs no MCP client. A new operator invoking Studio sees the Domain Model Editor's web UI and nothing else.
- Studio-to-host-agent and host-agent-to-Studio coordination uses only on-disk artifacts. No shared in-memory state, no IPC sockets, no shared process handles, no in-protocol RPC between the two.
- The Studio binary's outbound network allowlist excludes Figma's MCP server endpoint entirely. Outbound calls to that endpoint, if attempted, would be a regression and must fail a repository-wide lint or grep gate.
**Source**: @studio-foundation/figma-mcp-via-host-agent/studio-defers-figma-mcp-to-the-host-agent
**Caching**: none
**Backward-Compatible**: no

**Notes**:
- This decision supersedes four prior specs: figma-mcp-client (entire feature), studio-config's Figma-MCP-connection-configuration intent, web-server-harness's Figma-MCP-Phase-0-wiring intent, and the MCP-probe step in web-server-harness's boot-sequence intent. The historical spec files for those features stay frozen on disk as design history; this fragment does not edit them.
- The triggering observation: the web-server-harness integration test surfaced a hard blocker — Figma's remote MCP server requires OAuth-issued bearer tokens with the `mcp:connect` scope, and the dynamic client registration endpoint returns 403 for non-catalog clients. Figma's documentation confirms the catalog gating.
- Architecture v4 §7's framing of "shared web server harness for both tools" is partially obsolete after this fragment lands — the Design Loop never enters Studio's web server under the new architecture. A future architecture-doc revision will reflect the split; the reconciliation is tracked separately and is not part of this feature.

---

## Retraction of Studio-direct-MCP source-tree shape

**Affects**: source-tree shape, dependency baseline, startup-sequence shape, configuration vocabulary, distributed setup documentation
**Behavior**: The source-tree shape that implemented the Studio-direct-MCP architecture is removed in a single coordinated change so the binary cannot exist in a half-retracted state. The dedicated MCP-client package under the Studio binary's internal tree is removed entirely; no scaffolding or stub remains. The Figma-specific configuration keys (the MCP endpoint URL field, the Figma-token field, the corresponding environment-variable binding) are removed from the configuration struct and from the loader; the loader retains its remaining web-server keys and project-root resolver, and continues to honor secret-key invariants for any future secret keys. The startup sequence is reshaped: the MCP-probe step and the construct-authenticated-MCP-client step are removed; the remaining steps are renumbered into a contiguous shorter sequence with no gaps in commentary or stable-code naming; the graceful-shutdown sequence no longer closes an MCP client. The binary's command entry point removes its import of the MCP-client package and stops passing MCP-related fields to the server boot routine. The upstream MCP SDK is removed from the Studio module's direct dependency list, and a dependency-tidy pass removes the indirect dependencies it pulled in, leaving a smaller dependency graph. The setup document describing the now-defunct environment-variable-based Figma-MCP setup is removed; the setup story for the host-agent skill path is documented under the parlay-design-loop skill (a separate feature). After deletions land, the code-hash baseline is refreshed so the post-retraction file set is recorded as stable.
**Invariants**:
- The dedicated MCP-client package directory under the Studio binary's internal tree no longer exists; a recursive listing returns "no such directory." The deletion is not a stub-leaving rename or a comment-out; the directory is gone.
- A grep for the removed Figma-specific configuration field names and the removed environment-variable binding under the configuration directory returns zero matches. The remaining configuration tests do not reference those fields.
- A grep for references to the deleted MCP-client package or to the Figma MCP endpoint host under the binary's command directory and under the server-startup file returns zero matches.
- The Studio module's manifest no longer declares the upstream MCP SDK as a direct dependency. A dependency-tidy pass after the deletions is a no-op — the indirect dependencies the SDK pulled in are gone as well.
- The Figma-MCP setup document no longer exists under the Studio documentation directory. A find for its filename returns no match.
- The Studio module builds and its test suite passes after the retraction lands. The remaining tests cover the configuration package (minus the removed Figma tests) and the server-startup logic (minus the removed MCP-probe tests). Total test count is lower than before; all remaining tests are green.
- The verified-generated-files gate against the refreshed code-hash baseline reports the new file set as stable. No file is reported missing or modified.
- The retraction lands as a single coordinated change — one buildfile run, one commit. Half-retracted intermediate states (package deleted but import not removed; configuration field removed but loader not updated; startup-step removed but neighboring steps not renumbered) are forbidden by construction.
**Source**: @studio-foundation/figma-mcp-via-host-agent/retract-the-studio-direct-mcp-code
**Caching**: none
**Backward-Compatible**: no

**Notes**:
- This fragment is deletion-heavy. The downstream buildfile will be dominated by `plan.deletes` rather than `plan.creates` or `plan.modifies`. This is a new pattern relative to prior loops in the project; the build sub-agent should expect substantial deletion plans and the verify gate should treat zero-match grep assertions as success rather than as missing-content errors.
- Cross-feature buildfile breakage is expected, not a regression: once the deletions land, the buildfiles for the superseded features (figma-mcp-client; the Figma parts of studio-config and web-server-harness) will fail their own `check-buildfile` against the new tree because their source-signatures point at files being deleted. Per the superseding fragment above, those historical buildfiles are archival — they are not the build's source of truth after this fragment ships. The authoritative post-retraction gate is the verify-generated check against the refreshed code-hash baseline.
- The refreshed code-hash baseline (`parlay save-build-state` in the project's tooling) is part of this feature's scope, not a separate post-step the operator runs by hand. The buildfile's verify step runs the save and the subsequent verify-generated together in the same coordinated change as the deletions.

---
