# Figma-mcp-via-host-agent — Dialogs

---

### Studio defers Figma MCP to the host agent

**Persona**: Parlay Studio maintainer

**Trigger**: The maintainer has just absorbed the web-server-harness integration test result — Figma's MCP server is catalog-gated and Studio cannot register — and needs to pin a coherent architectural decision rather than leave the project in limbo.

#### Happy path: pin the host-agent-mediation decision

User: I've read the catalog-gating discovery. Studio cannot be a Figma MCP client. I want to capture the decision that the Design Loop becomes a host-agent skill and Studio's binary stops touching Figma MCP entirely.

System: Acknowledged. The decision shape is: (1) Studio's binary keeps the Domain Model Editor's web UI, the chi router harness, the file-I/O abstraction, and the boot/shutdown lifecycle; (2) Figma MCP operations move to a parlay-skill the host agent (Claude Code today, Cursor / VS Code / future catalog clients in principle) invokes; (3) Studio and the host communicate exclusively through on-disk artifacts (layouts, page schemas, domain-model.yaml). There is no IPC, no shared connection state, no protocol between them beyond the YAML files both already read and write. The decision is reversible: if Figma admits Studio to the catalog later, a follow-up feature can grow direct MCP capability without disturbing the host-agent path.

User: That matches my read. What does the retraction look like concretely?

System: The retraction is the second intent in this feature. In summary it deletes `studio/internal/mcpclient/` entirely, removes the Figma-specific keys from `studio/internal/config/`, strips the MCP probe from `studio/internal/server/boot.go`'s boot sequence, removes the `modelcontextprotocol/go-sdk` dependency from `go.mod`, and drops `studio/docs/figma-mcp-setup.md`. The historical intents/dialogs/infrastructure for figma-mcp-client, the Figma parts of studio-config, and web-server-harness's Intent 4 stay frozen on disk as design history — the retraction touches code, not spec.

User: Good. Also confirm the Design Loop skill side.

System: The Design Loop becomes a parlay-skill documented under `.claude/skills/parlay-design-loop/` after `parlay upgrade`. Its contract is: read named on-disk artifacts (page schemas, layout tree, domain-model.yaml) from the parlay project root, perform Figma MCP operations using the host agent's existing catalog connection, write results back to named on-disk artifacts. Authoring that skill is a separate feature; this feature pins the architectural decision and retracts the superseded code.

#### Branch: reversibility check

User: What if Figma admits Studio to the catalog in six months — does this decision foreclose that?

System: No. The retraction deletes the implementation; the historical spec stays on disk. If Figma admits Studio later, a new feature re-authors `studio/internal/mcpclient/` from the preserved figma-mcp-client spec. The host-agent skill path coexists — operators choose at runtime whether to use Studio-direct-MCP (when available) or the host agent (when not). The architectural decision documents host-agent-mediation as the default; admission to the catalog promotes Studio-direct-MCP from impossible to available, not from forbidden to default.

#### Branch: architecture v4 doc reconciliation

User: Architecture v4 §7 frames Studio and the Design Loop as sharing one web server harness. Does this decision break that?

System: Partially obsolete after this feature ships. The Design Loop never enters Studio's web server under the new architecture — it runs in the host agent's process. A future architecture-doc revision (v4.1 or v5) will reflect the split. This feature does not edit the architecture doc; the reconciliation is tracked separately so the decision lands first and the doc catches up.

---

### Retract the Studio-direct-MCP code

**Persona**: Parlay Studio maintainer

**Trigger**: The architectural decision is pinned (Intent 1 above). The maintainer now needs the concrete deletions executed as a single coordinated change so the binary cannot be in a half-retracted state.

#### Happy path: single-commit retraction lands cleanly

User: Run the retraction. One buildfile, one commit, atomic.

System: The buildfile will plan: delete `studio/internal/mcpclient/` (9 files); delete `studio/internal/config/figma.go` and `figma_test.go`; modify `studio/internal/config/config.go` to remove `FigmaMCPURL` and `FigmaToken` fields; modify `studio/internal/config/loader.go` and `loader_test.go` to remove references to those fields and to `STUDIO_FIGMA_TOKEN`; modify `studio/internal/server/boot.go` to remove the MCP-probe step (old step 4) and the construct-authenticated-MCP-client step (old step 5), renumbering the remaining 10 steps; modify `studio/internal/server/boot_test.go` to drop MCP-probe test cases; modify `studio/cmd/parlay-studio/main.go` to remove the `studio/internal/mcpclient` import and the MCP-related field passing to `server.Boot`; delete `studio/docs/figma-mcp-setup.md`; edit `studio/go.mod` to remove `github.com/modelcontextprotocol/go-sdk v1.6.0`; run `go mod tidy` to clean indirect dependencies (`jsonschema-go`, `segmentio`, `yosida95/uritemplate`, `golang.org/x/oauth2`); run `parlay save-build-state --root studio` to refresh the code-hash baselines.

User: Proceed.

System: After the buildfile runs and the commit lands, the verification step runs: `find studio/internal/mcpclient/ -type f` returns "no such directory"; `grep -rn 'FigmaMCPURL\|FigmaToken\|figma_mcp_url\|figma_token\|STUDIO_FIGMA_TOKEN' studio/internal/config/` returns zero matches; `grep -n 'mcpclient\|mcp.figma' studio/cmd/parlay-studio/main.go studio/internal/server/boot.go` returns zero matches; `grep 'modelcontextprotocol/go-sdk' studio/go.mod` returns no match; `find studio/docs/ -name 'figma-mcp-setup.md'` returns no match; `CGO_ENABLED=0 go build ./...` and `CGO_ENABLED=0 go test ./...` from `studio/` both pass; `parlay verify-generated --root studio` reports the new file set as stable.

#### Branch: verification greps return zero matches as required

User: The grep assertions in Intent 1 also cover `mcp.figma.com` and `modelcontextprotocol/go-sdk` across `studio/internal/`, `studio/cmd/`, `studio/go.mod`, `studio/go.sum`, `studio/docs/`. Confirm those pass too.

System: They do — those literal strings only survive in `studio/spec/intents/` (spec text describing the retraction's history) and in git history. The Verify item is explicit that matches inside `studio/spec/intents/` are expected and acceptable; matches anywhere else fail the gate.

#### Branch: `go mod tidy` cleans indirects

User: What does `go mod tidy` actually remove beyond the direct SDK entry?

System: The `modelcontextprotocol/go-sdk` pulls in `github.com/google/jsonschema-go`, `github.com/segmentio/asm` (or related), `github.com/yosida95/uritemplate/v3`, and `golang.org/x/oauth2` as indirect dependencies. Once the direct require is gone and nothing else in the `studio` module references those indirects, `go mod tidy` deletes them from both `go.mod` and `go.sum`. The resulting dependency graph is smaller; verifiable by a second `go mod tidy` run being a no-op.

#### Branch: cross-feature buildfile breakage is expected, not a regression

User: Once `studio/internal/mcpclient/` is gone, the buildfiles for figma-mcp-client and web-server-harness will fail check-buildfile — their source-signatures point at deleted files. How is that not a regression?

System: It is expected. Per Intent 1's supersession clause and Intent 2's Constraints, the historical spec files (intents, dialogs, infrastructure) for the retracted features stay frozen on disk as design history; their buildfiles are archival, not the build's source of truth for the live tree. `parlay verify-generated --root studio` is the authoritative check after this retraction lands — it runs against the new baseline written by `parlay save-build-state`, not the old per-feature buildfiles. The build sub-agent should be told not to attempt to "reconcile" the retracted features.

#### Branch: half-retracted state is forbidden

User: If the retraction is split into multiple commits, what breaks?

System: Several intermediate states are broken-by-construction: deleting `mcpclient/` without removing `main.go`'s import yields a non-compiling binary; removing `FigmaMCPURL` from the Config struct without updating `loader.go` yields a non-compiling config package; removing the boot-step-4 logic without renumbering steps 5–12 leaves comments out of sync with code. The atomic-commit constraint is load-bearing — the buildfile must emit one coordinated plan and the commit must land as a single unit. If a pre-commit hook fails mid-way, fix the issue and create a new commit; never partial-apply.

#### Branch: parlay save-build-state is in-scope

User: Is `parlay save-build-state` something I run after the commit, or is it part of the buildfile?

System: Part of this feature's Action (item 9). The buildfile's verify step includes the save-build-state invocation so the new code-hash baselines land in the same coordinated change as the deletions. After the commit, `parlay verify-generated --root studio` against those new baselines is a no-op.

---

