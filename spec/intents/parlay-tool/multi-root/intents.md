# Multi-root

> Support multiple parlay roots within a single repository — each subproject gets its own intents, dialogs, build artifacts, and (optionally) adapters, while sharing schemas, skills and other resources from a repo-level parent root.

---

## Discover Active Root via Cwd Walk-Up

**Goal**: Run any parlay command from anywhere inside the repo and have it operate on the closest enclosing root, without the user passing flags.
**Persona**: UX Designer
**Priority**: P0
**Context**: The user is in a monorepo with one or more parlay roots. They invoke `parlay build-feature ...` from a subfolder; the tool must locate the right `.parlay/` to operate against — and refuse to silently fall back to the wrong one.
**Action**: Walk upward from cwd until a `.parlay/` directory is found. Treat that directory as the active root for the invocation. Stop at the filesystem root if none is found and error out.
**Objects**: root, cwd, active-root, parlay-dir

**Constraints**:
- Walk-up must stop at the first `.parlay/` encountered — nearest wins, even if a parent root exists higher up
- Walk-up must stop at the git repository boundary — if a `.git/` directory is encountered before any `.parlay/`, resolution does NOT immediately error; the tool first attempts interactive disambiguation (next constraint)
- When walk-up fails but candidate roots are discoverable — i.e. one or more `.parlay/` directories exist below the current location, OR the user's cwd is at/inside a directory that has a `roots.yaml` listing registered child roots — the tool MUST prompt the user via the existing disambiguation mechanism (AskUserQuestion or the adapter's equivalent) to select the target root, rather than erroring
- Disambiguation only applies in interactive contexts. In non-interactive contexts (no TTY, CI, scripted invocation) the tool errors with a clear "no parlay root found; specify --root or PARLAY_ROOT" message — never blocks waiting for input
- A `PARLAY_ROOT` environment variable, when set to an absolute path containing a `.parlay/`, overrides cwd walk-up entirely and skips disambiguation; an invalid value errors rather than silently falling back
- Resolution must happen once at command entry; subsequent path operations within the command resolve relative to the chosen root, not cwd
- All existing single-root projects must continue to work without changes (a `.parlay/` at the repo root is itself a root, found by the same walk-up rule)
- The chosen root must be reported in `parlay --version` output and any verbose mode, so the user can confirm which root a command is acting on

**Verify**:
- `cd subproject/ && parlay sync` operates on `subproject/.parlay/` if it exists, else walks up to find the next ancestor (stopping at `.git`)
- `cd /tmp && parlay sync` errors with "no parlay root found" — does not walk past filesystem boundaries
- `cd repo/ && parlay sync` in a multi-root project where the repo root itself has no `.parlay/` but child roots exist below — the tool prompts (interactively) with the list of discovered roots and proceeds with the user's selection; in a non-interactive run the same case errors
- Disambiguation prompt is shown only once per invocation; the chosen root is recorded in verbose output but NOT persisted to disk
- `PARLAY_ROOT=/abs/path/to/repo parlay sync` (from any cwd) operates on the named root and skips any disambiguation
- `PARLAY_ROOT=/nonexistent parlay sync` errors rather than silently walking up
- `parlay --verbose sync` prints the resolved root path and the resolution source (cwd walk-up, disambiguation prompt, `PARLAY_ROOT`, or `--root` flag) before doing any work
- An existing single-root project at the repo root behaves identically before and after this feature ships
- A subfolder without its own `.parlay/` falls through to the repo-root `.parlay/` (sees only the repo-root root)

---

## Add a Child Root to an Existing Project

**Goal**: Opt an existing single-root project into multi-root by declaring a subfolder as its own parlay root, without disturbing the parent root or any of its features.
**Persona**: Tech Lead / Architect
**Priority**: P0
**Context**: The user already has a working `.parlay/` at the repo root. They want a subfolder (e.g. `apps/web/`) to have its own intents, dialogs, and build artifacts — independent from the rest of the repo.
**Action**: Run `parlay add-root <subdir>` from the parent root. The command creates `<subdir>/.parlay/` and `<subdir>/spec/`, links the new root to its parent, and registers it in the parent's root index.
**Objects**: parent-root, child-root, root-index, subdir

**Constraints**:
- The command must refuse if `<subdir>` already contains a `.parlay/` (use `parlay upgrade` instead)
- The command must refuse if invoked from a directory that is not itself a parlay root (no orphan child roots)
- The new child root must record a pointer to its parent (so inheritance resolution does not require walk-up at every read)
- The parent root must record the child in a roots index (e.g. `.parlay/roots.yaml`) so `parlay status` and project-level commands can enumerate all roots
- Adding a child root must not modify any existing intents, dialogs, surface, or build artifacts in the parent
- A child root cannot be added inside another child root (no nesting beyond one level for v1)
- A parent root MAY have an empty `spec/intents/` of its own — i.e. all features live in child roots and the parent exists solely to host the roots index, the shared schemas, the shared adapters, and the agent surface. This "bare-parent" topology must be a first-class supported configuration: every command run at the parent in this state behaves correctly (no features listed, no errors implying features must exist)
- If a child root's recorded parent path no longer resolves to a valid parlay root (parent removed, moved, or its `.parlay/` deleted), every command run against the child must error loudly with "parent root not found at <path>; restore it or run `parlay promote-root` to make this child standalone" — never auto-promote, never silently walk up to a different parent
- `parlay add-root` MUST automatically run the deployer/upgrade refresh at the repo root after creating the child, so the agent-rules file (`CLAUDE.md` or equivalent) reflects the new root immediately — without requiring a separate `parlay upgrade` invocation

**Verify**:
- After `parlay add-root apps/web`, the directory `apps/web/.parlay/` exists with a config that references the parent root by relative path
- The parent root's `.parlay/roots.yaml` lists `apps/web` as a registered child
- `cd apps/web && parlay sync` operates on the child root, not the parent
- `parlay add-root apps/web` (run twice) errors on the second invocation
- Running `parlay add-root apps/web` from a directory that is itself not a parlay root errors with a clear message
- Deleting the parent's `.parlay/` and running any command in the child root produces the loud "parent root not found" error and refuses to fall through to a different ancestor
- `parlay promote-root` run inside an orphaned child root makes it standalone (parent pointer removed, child becomes its own top-level root)
- After `parlay add-root apps/web`, no separate `parlay upgrade` step is required for the agent to see the new root — the repo-level agent-rules file lists it on first agent invocation
- A parent root with empty `spec/intents/` and one or more child roots is fully functional: `parlay status` at the parent lists zero parent features and the registered children; `parlay sync`, `parlay extract-domain-model --root web`, and other commands work normally against child roots; no command errors with "no features found" at the parent


---

## Inherit Resources from Parent Root

**Goal**: Avoid duplicating shared configuration across roots — schemas and the deployed agent surface live only at the repo-level root and are read directly from there; adapters can optionally be overridden per-child.
**Persona**: UX Designer
**Priority**: P0
**Context**: A child root has been added. The user runs commands inside the child root and expects schemas and the agent surface configured at the repo level to apply automatically, without copying files.
**Action**: When a command needs a schema, resolve from the parent root unconditionally. When a command needs an adapter, resolve from the child root if a same-named file is present, else fall back to the parent root. The deployed agent surface is loaded once from the repo-level root regardless of which root a command targets.
**Objects**: schema, adapter, deployed-skill, resolution-order, parent-root, child-root

**Constraints**:
- Schemas are repo-level only — child roots must not contain a `.parlay/schemas/` directory; if one is present, the tool errors with a clear message ("schemas live at the parent root only")
- Deployed agent skills (Claude Code: `.claude/skills/parlay-*/SKILL.md`; equivalents for other adapters) are repo-level only — child roots must not contain a deployed-skills directory; if one is present, the tool errors with the same root-only message
- Adapters are the ONLY resource a child root can override. A file at `<child>/.parlay/adapters/foo.adapter.yaml` fully replaces `<parent>/.parlay/adapters/foo.adapter.yaml` for commands run in the child (no key-by-key merging; the child file is loaded as-is, the parent file is ignored)
- The single `.claude/` at the repo root must dispatch each invocation to the correct root by walk-up (same rule as the CLI)
- Domain models are NOT inherited — each root has its own `domain-model.md`, extracted from its own intents
- Features are strictly root-scoped — a feature defined at the parent root (its intents, dialogs, surface, page manifests, build artifacts, and domain-model entries) does NOT contribute to or become visible from child roots in v1. Sharing parent-root features into children (auto-apply, import lists, or otherwise) is explicitly out of scope and reserved for a follow-up feature once a concrete use case appears
- Resolution order must be deterministic and printable: `parlay --verbose <cmd>` shows which file was loaded from which root

**Verify**:
- A child root with no `.parlay/adapters/` directory uses adapters from the parent root
- A child root with `.parlay/adapters/react.adapter.yaml` uses its own version, not the parent's, when commands run inside the child
- `parlay extract-domain-model` run in a child root produces a `domain-model.md` scoped to the child's intents only — does not include parent intents
- A feature defined at the parent root does NOT appear in `parlay status` / feature listings run inside a child root, and a feature reference resolved inside a child does not match parent-root features
- A child root with a `.parlay/schemas/` directory produces a clear startup error
- A child root with a deployed-skills directory (e.g. `.claude/skills/parlay-*`) produces a clear startup error
- `parlay --verbose build-feature` prints the resolution path of every loaded resource (which root provided it)
- Deployed skills at `.claude/skills/parlay-*/SKILL.md` (repo root) work identically when invoked from any subfolder of any root

---

## Target a Non-Active Root from Any Cwd

**Goal**: Run a command against a specific root without changing directories, so external tooling and scripts can address roots by name.
**Persona**: Tech Lead / Architect
**Priority**: P1
**Context**: The user is in the parent root (or some unrelated cwd) and wants to invoke a parlay command on a specific child root — e.g. running CI checks across all roots, or addressing a feature in another root by reference.
**Action**: Accept an optional root prefix on feature references (`web:@parlay-tool/multi-root`) and a `--root <name>` flag on commands. When present, override the cwd walk-up and operate against the named root instead.
**Objects**: root-prefix, root-name, feature-reference, root-index

**Constraints**:
- Root names are short identifiers declared in the parent's `.parlay/roots.yaml` — they are NOT directory paths
- A bare reference (`@parlay-tool/multi-root`, no prefix) resolves first against the active root from cwd walk-up; if the feature exists there, that match wins
- If a bare reference does NOT match in the active root but matches in multiple other roots (e.g. user is in the parent root, the feature does not exist in the parent, but exists in two child roots), the tool MUST prompt the user via the disambiguation mechanism with the list of matching roots and proceed with the chosen one — not error and not silently pick the first match
- If a bare reference matches in exactly one other root (and not the active root), the tool MAY proceed automatically with that match, but MUST print which root was chosen in normal (non-verbose) output so the user sees it
- Disambiguation only applies in interactive contexts. In non-interactive runs, ambiguous references error with the list of candidates and a hint to use the prefix or `--root` flag
- An unknown root name errors with a list of known roots — never falls through silently
- The root prefix is a CLI / cross-tool addressing mechanism only — intents.md content MUST NOT contain cross-root references in v1 (those are explicitly out of scope)
- Conflicts between `--root` and a prefixed reference must error rather than silently picking one
- The root prefix is accepted by every command that takes a feature reference, including write commands (`add-feature`, `build-feature`, `generate-code`) — there is no read-only restriction
- `--root <name>` is a separate flag retained for project-level commands that don't take a feature reference (`parlay extract-domain-model --root web`, `parlay status --root web`); `--root` and a prefixed feature reference must agree, or the command errors

**Verify**:
- `parlay sync web:@parlay-tool/multi-root` from any cwd operates on the `web` root, regardless of where the user is
- `parlay --root web sync` (no feature ref) operates on the `web` root
- `parlay sync unknown:@foo` errors with "unknown root 'unknown'; known roots: web, api"
- `parlay sync @foo` (no prefix) uses cwd walk-up — behavior identical to single-root projects
- `parlay build-feature @intent/A` run from the parent root, when the feature does not exist in the parent but exists in `apps/web` AND `apps/api` — prompts interactively with both candidates and proceeds with the user's selection
- `parlay build-feature @intent/A` run from the parent root, when the feature exists in only one other root (`apps/web`) — proceeds automatically and prints "using root: web" before doing the work
- The same ambiguous reference in a non-interactive run errors with "ambiguous: matches roots [web, api]; use prefix or --root"
- Authoring an intent that references `web:@some-feature/some-intent` produces a validation error (cross-root references blocked at v1)


---

## Single Repo-Level Agent Surface Drives All Roots

**Goal**: Keep the agent-side configuration (`.claude/skills/`, `CLAUDE.md`) in one place at the repo root, so the user does not maintain separate IDE / agent setups per subproject.
**Persona**: UX Designer
**Priority**: P0
**Context**: The user runs Claude Code from the repo root. Skills are deployed at `.claude/skills/parlay-*/SKILL.md`. When a skill is invoked, the agent's cwd may be in any subfolder — the skill needs to act on the right root.
**Action**: Deployed skills resolve the active root via the same cwd walk-up rule as the CLI. The repo-level `CLAUDE.md` describes the multi-root layout in a single section; per-root content is NOT generated.
**Objects**: deployed-skill, agent-surface, claude-md, root-resolution

**Constraints**:
- The rule is uniform across adapters — the agent-surface lives at the repo root regardless of which adapter is active. For Claude Code: `.claude/skills/` and `CLAUDE.md`. For Cursor: `.cursor/agents/` and any agent-rules file. For Generic CLI: `AGENT_INSTRUCTIONS.md`. None of these may exist inside a child root.
- Skills (and their adapter equivalents) must NOT hardcode `.parlay/` or `spec/intents/` as paths relative to a single root — they must use the active root resolved at invocation time via the same walk-up rule the CLI uses
- `parlay upgrade` operates on the repo-level root only (where the agent surface lives) and updates schemas at the repo root
- `CLAUDE.md` (or the adapter's equivalent agent-rules file) at the repo root must list all registered child roots and a one-line description of each, so the agent has top-of-context awareness of the multi-root layout
- Adding or removing a child root must trigger an update to the multi-root section of the repo-level agent-rules file, preserving user-authored sections per the existing claude-md-section-preservation rules
- `parlay add-root` and `parlay remove-root` MUST automatically run an upgrade-equivalent refresh at the repo root so the agent surface immediately reflects the new root set — the user does not need a separate `parlay upgrade` invocation

**Verify**:
- After `parlay add-root apps/web`, the repo-level agent-rules file (`CLAUDE.md` or equivalent) includes `apps/web` in its multi-root section without manual intervention
- A skill invoked while the agent's cwd is `apps/web/` writes to `apps/web/.parlay/build/...`, not the repo-level `.parlay/build/...`
- `parlay upgrade` run at the repo root refreshes the deployed agent surface once and updates schemas at the repo root only
- Running `parlay upgrade` from inside a child root errors with "run upgrade from the repo-level root"
- User-authored sections of the agent-rules file survive `parlay upgrade`, `parlay add-root`, and `parlay remove-root` invocations
- The same multi-root behavior applies for Cursor (`.cursor/agents/`) and Generic CLI (`AGENT_INSTRUCTIONS.md`) — agent surface is always at the repo root, never inside a child

---

## Skill Invocation in a Multi-Root Project

**Goal**: Make every `/parlay-*` skill behave correctly in a multi-root project — the user invokes a skill the same way they always have (e.g. `/parlay-sync @feat`) and the skill resolves the right root, disambiguates interactively when needed, and announces which root it's acting on.
**Persona**: UX Designer (working through the AI agent rather than the raw CLI)
**Priority**: P0
**Context**: The user works primarily through deployed skills, not the bare `parlay` binary. A skill is invoked while the agent's cwd is somewhere in the repo — possibly inside a child root, possibly at the parent. The skill must produce the same multi-root semantics as the CLI, but render any user prompts through the agent's interactive question mechanism (e.g. `AskUserQuestion` for Claude Code) and surface chosen-root announcements as visible agent text.
**Action**: Every deployed skill that operates on a feature reference or root-scoped resource invokes the underlying `parlay` CLI in a way that (a) inherits the agent's cwd as the resolution starting point, (b) when the CLI asks for disambiguation, the skill catches the candidate list and re-asks the user via the agent's native question mechanism, (c) when the active root differs from cwd-default, the skill surfaces the announcement in its visible response.
**Objects**: skill, agent-cwd, skill-argument, askuserquestion, announcement

**Constraints**:
- Skills must NOT bake assumptions about a single `.parlay/` location into their prompt text; every path reference must be relative to "the active root resolved at invocation time"
- Skills accept the same root-prefixed feature references as the CLI — `/parlay-sync web:@feat` resolves the `web` root, `/parlay-sync @feat` uses cwd walk-up
- When the underlying CLI invocation would prompt for disambiguation interactively, the skill MUST detect the prompt (via a structured signal — JSON output, exit code + stderr metadata, or the existing AskUserQuestion adapter contract) and re-render the same prompt through the agent's question mechanism, then re-invoke the CLI with the user's choice
- The skill MUST NOT auto-pick a root on disambiguation — it must surface the question, exactly as the CLI would in interactive mode
- When the auto-resolved root differs from the cwd-default root (single match in another root, or `--root` flag, or prefix), the skill includes a "operating on root: <name>" line in its visible response — same announcement requirement as the CLI
- Skills run in a single agent context where multiple roots may be referenced over the conversation; each skill invocation re-resolves the active root from scratch — no cached "we're using the web root for this conversation" state
- Skills MUST surface forbidden-directory errors (child has `.parlay/schemas/`, etc.) with actionable guidance, not just the raw CLI error string
- The same skill-side multi-root behavior must apply uniformly across adapters (Claude Code, Cursor, Generic CLI) — the skill source authored under `internal/embedded/skills/` is one file, deployed identically by every deployer

**Verify**:
- `/parlay-sync @feat` invoked while agent cwd is `apps/web/` operates on the `web` child root, with no special prompting
- `/parlay-build-feature @intent/A` invoked while agent cwd is the parent and the feature exists in two child roots — the skill surfaces an `AskUserQuestion` (or adapter equivalent) with the candidate roots, then re-invokes the CLI against the user's choice
- `/parlay-sync web:@feat` works regardless of agent cwd
- `/parlay-extract-domain-model --root web` works as a skill argument the same way it works as a CLI flag
- A skill invoked while the user is in a bare-parent project with empty parent `spec/intents/` does not show "no features found" — it shows the registered children and prompts for the target root if needed
- When a skill auto-resolves to a non-cwd root (e.g. single child match for a bare reference), the visible response begins with "operating on root: <name>"
- Skill prompt text in `internal/embedded/skills/*.skill.md` contains no hardcoded `.parlay/` paths — every path reference is via the active-root resolver
- The same skill behavior is exercised on Claude Code, Cursor, and Generic CLI — no adapter-specific skill source
