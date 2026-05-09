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
- A parent root MAY have an empty `spec/intents/` of its own — i.e. all features live in child roots and the parent exists solely to host the roots index, the shared schemas, the shared adapters, and the agent surface. This "feature-empty parent" topology must be a first-class supported configuration: every command run at the parent in this state behaves correctly (no features listed, no errors implying features must exist). Note: "feature-empty" means no features of its own; the parent still has a `config.yaml` declaring the project's `ai-agent`. A parent with NO `config.yaml` is the bare-parent state, which is a separate, deprecated topology — see Intents A, C, and D below
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
- A parent root with empty `spec/intents/` and one or more child roots is fully functional: `parlay status` at the parent lists zero parent features and the registered children; `parlay sync`, `parlay create-domain-model --root web`, and other commands work normally against child roots; no command errors with "no features found" at the parent


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
- `parlay create-domain-model` run in a child root produces a `domain-model.md` scoped to the child's intents only — does not include parent intents
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
- `--root <name>` is a separate flag retained for project-level commands that don't take a feature reference (`parlay create-domain-model --root web`, `parlay status --root web`); `--root` and a prefixed feature reference must agree, or the command errors

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
- `/parlay-create-domain-model --root web` works as a skill argument the same way it works as a CLI flag
- A skill invoked while the user is in a feature-empty parent project (no parent-owned features in `spec/intents/`) does not show "no features found" — it shows the registered children and prompts for the target root if needed
- When a skill auto-resolves to a non-cwd root (e.g. single child match for a bare reference), the visible response begins with "operating on root: <name>"
- Skill prompt text in `internal/embedded/skills/*.skill.md` contains no hardcoded `.parlay/` paths — every path reference is via the active-root resolver
- The same skill behavior is exercised on Claude Code, Cursor, and Generic CLI — no adapter-specific skill source

---

## Agent Identity Lives at the Parent in Multi-Root Projects

**Goal**: The `ai-agent` field — which adapter (Claude Code, Cursor, Generic CLI, ...) owns the deployed agent surface — belongs to the topology root: the parent in multi-root projects, the only root in single-root projects. Children carry only their own framework choices.
**Persona**: UX Designer
**Priority**: P0
**Context**: Intent #5 ("Single Repo-Level Agent Surface Drives All Roots") established that the agent SURFACE (`.claude/`, `CLAUDE.md`, `AGENT_INSTRUCTIONS.md`) lives at the parent. The agent IDENTITY — the `ai-agent` field that decides which adapter wrote that surface and will refresh it — is a strict extension of the same posture: there is one agent per repo, and it is declared once, at the same level the surface lives. A real bug surfaced this gap: `parlay upgrade` in a multi-root project where the parent had no `config.yaml` (and `ai-agent` lived in a child config) silently skipped skills because the upgrader looked for the agent identity at the parent and found nothing — the topology was structurally wrong but no command was checking for it.
**Action**: Treat `ai-agent` as a parent-only field in multi-root projects. The parent's `config.yaml` MUST declare `ai-agent`. Child `config.yaml` files MUST declare `sdd-framework` and `prototype-framework` (their own per-child choice) and a `parent: ..` pointer, but MUST NOT declare `ai-agent`. Single-root projects are unchanged: the one `config.yaml` carries all three fields.
**Objects**: ai-agent, config.yaml, sdd-framework, prototype-framework, parent-pointer, topology-root

**Constraints**:
- In a multi-root project, the parent root MUST have a `.parlay/config.yaml` containing `ai-agent: <adapter-name>`. A multi-root project where the parent has no `config.yaml` is structurally invalid (see "Detect and Migrate Legacy Topology Mismatches")
- In a multi-root project, child `.parlay/config.yaml` files MUST NOT contain an `ai-agent` field. The agent identity is defined exactly once per project; duplicating it at a child is ambiguous (whose adapter owns the surface?) and is rejected at config-load time with a clear "agent identity belongs at the parent root" error
- When `ai-agent` is present at BOTH parent and child (legacy state from before this model), the loader hard-errors and refuses to proceed. The error names both files and points the user at `parlay repair`. Preferring one side and warning would let the inconsistency persist silently — the model is "exactly one agent identity per project" and the loader enforces it
- Child `config.yaml` files MUST declare a `parent: <relative-path>` pointer. They MAY declare their own `sdd-framework` and `prototype-framework` (siblings need not match); when omitted, the child inherits the parent's value for the corresponding field
- The parent's `config.yaml` MAY declare `sdd-framework` / `prototype-framework` for two distinct purposes: (1) the parent itself hosts features and uses these for its own codegen, and (2) the parent provides defaults that children may inherit. Both purposes use the same field; presence in the parent does not require the parent to host features
- In a single-root project, the one `config.yaml` carries `ai-agent`, `sdd-framework`, and `prototype-framework` together — no behavior change from today, no migration required
- `parlay upgrade` reads `ai-agent` from the parent's `config.yaml` in multi-root projects and uses it to select the deployer for both schemas AND skills. The previous fallback that skipped skills when the parent had no config.yaml is removed (see "parlay upgrade Errors on Bare-Parent Topology")
- Config loading MUST report which file each effective field came from (`parlay --verbose` or equivalent), so the user can verify that `ai-agent` resolves from the parent and `sdd-framework` / `prototype-framework` resolve from the child (or from the parent when the child inherits)

**Verify**:
- `parlay upgrade` in a multi-root project where the parent has `config.yaml` with `ai-agent: Claude Code` redeploys both `.parlay/schemas/` and `.claude/skills/parlay-*/` — no silent skip
- A child `config.yaml` containing `ai-agent: Cursor` produces a config-load error with the message "agent identity belongs at the parent root; remove `ai-agent` from `<child>/.parlay/config.yaml`"
- A multi-root project where parent and child both declare `ai-agent` produces a config-load hard-error naming both file paths and pointing at `parlay repair`
- A multi-root project's parent `config.yaml` with only `ai-agent` (no framework fields) and child configs with `sdd-framework` / `prototype-framework` / `parent: ..` passes validation
- A child `config.yaml` that omits `sdd-framework` resolves to the parent's value when the parent declares it; produces a "no sdd-framework declared in child or parent" error otherwise
- A single-root project's `config.yaml` with all three fields continues to work unchanged
- `parlay --verbose status` (or equivalent) prints `ai-agent: Claude Code (from /repo/.parlay/config.yaml)` and `sdd-framework: parlay-spec (from /repo/core/.parlay/config.yaml)` when both fields are declared at their natural levels; prints `(inherited from /repo/.parlay/config.yaml)` when the child inherits
- Removing `ai-agent` from a parent `config.yaml` in a multi-root project causes the next `parlay upgrade` to error with "no agent identity declared at parent root" — never silently skip, never walk up

---

## parlay init Writes the Correct Topology Shape

**Goal**: `parlay init` produces a structurally-correct config topology on the first try — single-root projects get one `config.yaml` with all three fields; multi-root projects get a parent `config.yaml` with `ai-agent` and child `config.yaml` files with framework + parent pointer. Never writes a bare-parent layout.
**Persona**: UX Designer
**Priority**: P0
**Context**: `parlay init` is the entry point for new projects and the source of every fresh topology. If init writes the wrong shape, every downstream command compensates with fallbacks and every project drifts. This intent makes init the source of truth for "what a correct topology looks like" — same model as Intent A, just applied at the moment of creation.
**Action**: `parlay init` writes one `.parlay/config.yaml` at the directory it runs in. For a fresh single-root project this is the only file. Multi-root layouts are built by running `parlay init` once at the parent (creating the parent's `config.yaml` with `ai-agent`) followed by `parlay add-root <child>` per child (creating each child's `config.yaml` with `sdd-framework`, `prototype-framework`, `parent: <relative>`). There is no `--children` or `--multi-root` shortcut on `init`; `add-root` (already specified by Intent #2) is the canonical per-child path. When the user is running parlay through a known agent (Claude Code, Cursor, etc.), init detects this and pre-fills the `ai-agent` prompt; the user confirms or overrides — never silent.
**Objects**: parlay-init, parlay-add-root, config.yaml, parent-config, child-config, topology-shape

**Constraints**:
- `parlay init` writes ONLY the config.yaml at its invocation directory. It does not create child configs and does not invoke `parlay add-root` internally — the per-child step is explicit, run by the user
- `parlay init` MUST never produce a bare-parent state. When invoked at a directory that will host children (e.g. an existing repo with `roots.yaml`), it MUST write a `config.yaml` containing at minimum `ai-agent`. The agent value is prompted; if the user is running through a recognized agent (Claude Code, Cursor, etc.) the prompt is pre-filled with the detected value but the user must confirm — never silent
- `parlay init` MUST NOT write `ai-agent` into a child config. When invoked at a directory whose `parent: ..` resolves to an existing parent, init writes the child's `sdd-framework` / `prototype-framework` / `parent` only. If the parent has no `ai-agent` yet, init refuses and instructs the user to run `parlay init` at the parent first
- `parlay init` MUST NOT duplicate `ai-agent` between parent and child. The model has exactly one agent identity per project (Intent A); init enforces this by writing the field only at the parent
- Child `sdd-framework` / `prototype-framework` are prompted independently per child (or come from explicit flags). The parent's values are offered as defaults — the prompt is pre-filled with the parent's value when present, the user confirms or overrides
- Re-running `parlay init` against an existing project is idempotent for the topology layer: it preserves the agent identity at the parent, preserves framework choices, and never re-prompts for fields that are already set correctly
- `parlay init` for a single-root project is unchanged in behavior — single-root remains the default and most common shape

**Verify**:
- `parlay init` in an empty directory followed by selecting Claude Code / parlay-spec / parlay-prototype produces one `.parlay/config.yaml` containing all three fields — single-root, no parent pointer
- `parlay init` at an empty repo, then `parlay add-root core` and `parlay add-root studio`, produces `.parlay/config.yaml` with `ai-agent: <chosen>` at the repo and `core/.parlay/config.yaml` / `studio/.parlay/config.yaml` each with `sdd-framework`, `prototype-framework`, `parent: ..`. None of the child configs contain `ai-agent`
- `parlay init` invoked through Claude Code pre-fills the `ai-agent` prompt with `Claude Code` and waits for the user to confirm — pressing Enter accepts; typing a different value overrides; init never proceeds without an explicit choice
- `parlay add-root <child>` invoked when the parent has no `ai-agent` yet refuses with "parent is missing ai-agent — run `parlay init` at the parent first"
- `parlay init` invoked at a child whose `parent: ..` resolves to a parent with `ai-agent` already set writes the child's framework fields only — `ai-agent` is NOT written to the child
- `parlay init` re-run on a fully-configured multi-root project exits without modifying any `config.yaml` — pure idempotence
- The topology validator (the same checks Intent A and "Detect and Migrate Legacy Topology Mismatches" rely on) passes against every freshly-initialized project, single-root or multi-root, with no manual cleanup

---

## Detect and Migrate Legacy Topology Mismatches

**Goal**: `parlay repair` (and a one-line topology indicator in `parlay status`) detects three legacy config-topology mismatches and walks the user through a confirmed fix — never auto-corrects, never silently rewrites configs.
**Persona**: Tech Lead / Architect
**Priority**: P0
**Context**: Existing projects predate Intents A and B and may carry one of three structurally-wrong topologies: bare-parent (multi-root parent with `roots.yaml` but no `config.yaml`), agent-at-child (a child `config.yaml` declares `ai-agent`), or both-have-agent (parent and child both declare `ai-agent`, possibly disagreeing). These need a discoverable migration path. Per `feedback_parlay_vs_external_commands`, parlay-internal commands maintain invariants but **`parlay repair` is the explicit, user-facing channel for topology fixes** — it asks before writing, and `parlay status` only reports.
**Action**: `parlay repair` includes a topology-check pass that detects the three mismatches and, for each one, presents the user with a description of the problem, the proposed fix (which files will be created, modified, or have fields removed), and a confirm/skip prompt. `parlay status` includes a one-line topology indicator that says "topology: ok" or "topology: needs repair (run `parlay repair`)" — no detail at status-time, no auto-fix.
**Objects**: topology-check, bare-parent, agent-at-child, both-have-agent, parlay-repair, parlay-status

**Constraints**:
- `parlay repair` MUST detect four specific mismatches and report each one separately, with file paths and the concrete change it proposes:
  1. **Bare-parent**: parent has `.parlay/roots.yaml` (so it IS a multi-root parent) but no `.parlay/config.yaml`. Proposed fix: create `<parent>/.parlay/config.yaml` and prompt for the `ai-agent` value (default: detect from the agent currently running, or take from a child config if present)
  2. **Agent-at-child**: a child `.parlay/config.yaml` contains `ai-agent`. Proposed fix: remove `ai-agent` from the child config, write or update the parent's config with that value (after confirming with the user when the parent already has a different value)
  3. **Both-have-agent**: parent and child both declare `ai-agent`. If the values agree, the fix is to remove the child's copy. If they disagree, repair surfaces both values and asks the user which to keep at the parent — never picks silently
  4. **Single-root-missing-ai-agent**: a single-root project's `config.yaml` is missing `ai-agent`. Proposed fix: prompt for the `ai-agent` value (pre-filled with the detected agent when running through one) and write it
- `parlay repair` MUST walk the user through one mismatch at a time — prompt, apply the fix on confirmation, re-scan, surface the next mismatch. Mismatches may be independent (a project can have agent-at-child AND bare-parent simultaneously) and the user may want to skip some, so a single batched confirmation would lose granularity. No `--all` or `--yes` shortcut for topology fixes in v1
- `parlay repair` MUST NOT touch any config file without explicit confirmation. The existing repair philosophy (designer sees the mismatch and confirms the fix) applies uniformly
- `parlay status` includes one new line: `topology: ok` (when all four checks pass) or `topology: needs repair` (when any check fails), with the count of mismatches but NOT the per-file detail. Detail belongs in `parlay repair`. The check is uniform — single-root and multi-root projects use the same line; single-root projects with the correct three-field config simply report `topology: ok`
- The topology check MUST NOT run on every parlay command — it runs in `parlay repair` (full check, with prompts) and in `parlay status` (read-only, summary). Other commands are not slowed down or interrupted by topology checks; they may emit a one-line warning when they directly hit a topology error (e.g. `parlay upgrade` saying "no agent identity at parent — run `parlay repair`")
- After a successful repair, re-running `parlay repair` against the same project reports clean (no remaining mismatches), and re-running `parlay status` shows `topology: ok` — fixes are durable, not papered-over
- Migration MUST preserve user-authored content. If a child config has additional unrecognized fields beyond `ai-agent` / `sdd-framework` / `prototype-framework` / `parent`, those fields stay in the child file when `ai-agent` is moved out

**Verify**:
- A multi-root project with a bare-parent (parent has `roots.yaml`, no `config.yaml`) is detected by `parlay repair`, which prompts for `ai-agent` and creates the parent's `config.yaml` after the user confirms
- A multi-root project where a child config has `ai-agent: Claude Code` is detected by `parlay repair`, which proposes removing it from the child and writing it to the parent (creating the parent config if needed) — only after the user confirms
- A multi-root project where parent and child agree on `ai-agent: Claude Code` is detected by `parlay repair`, which proposes deleting the redundant child entry; the user confirms and repair completes
- A multi-root project where parent says `ai-agent: Claude Code` and a child says `ai-agent: Cursor` is detected by `parlay repair`, which surfaces both values and asks the user which to keep — never picks silently
- A single-root project where `config.yaml` is missing `ai-agent` is detected by both `parlay status` (`topology: needs repair`) and `parlay repair` (prompts for the value and writes it)
- A single-root project with `ai-agent` / `sdd-framework` / `prototype-framework` together in one config reports `topology: ok` from `parlay status` and is NEVER flagged by `parlay repair`
- A project with two simultaneous mismatches (e.g. agent-at-child AND a sibling child without sdd-framework) is walked one-at-a-time by `parlay repair`: fix the first, re-scan, surface the second, fix the second
- `parlay status` in a project with one mismatch prints a single `topology: needs repair (1 mismatch — run \`parlay repair\`)` line and no per-file detail
- After `parlay repair` resolves all four mismatch types in the same project, `parlay status` reports `topology: ok` and `parlay repair` re-run reports no remaining work

---

## parlay upgrade Errors on Bare-Parent Topology

**Goal**: Remove the bare-parent fallback from `parlay upgrade` outright. In bare-parent state, upgrade hard-errors and points the user at `parlay repair` — never silently skips skills, never prints a deprecation warning then continues. The fallback was a workaround for a missing spec; with Intents A, B, and C in place the spec exists and the workaround is no longer needed.
**Persona**: Tech Lead / Architect
**Priority**: P0
**Context**: The fallback in `core/internal/commands/upgrade.go` (commit `7ef27a7`) treats bare-parent as a valid topology and silently redeploys schemas while skipping skills. The fallback's existence is what allowed the original drift bug to compound undetected. Parlay has no stable release yet — no external projects depend on the bare-parent behavior — so the fallback can be removed in this release without a deprecation runway. Doing so collapses two paths (correct topology and bare-parent) into one (correct topology); future bugs do not have a "soft-fail" path to hide in.
**Action**: Delete the bare-parent branch in `deployToRoot` (the `case os.IsNotExist(err):` arm that proceeds with empty `cfg` and falls through to "deploy schemas, skip skills"). When `parlay upgrade` runs in a multi-root project where the parent has `roots.yaml` but no `config.yaml`, it errors immediately with `bare-parent topology: <parent>/.parlay/config.yaml is missing — run \`parlay repair\` to create it`. When it runs at a directory with neither `roots.yaml` nor `config.yaml`, it errors with the existing "run parlay init first" message. Single-root and correctly-configured multi-root projects are unaffected.
**Objects**: parlay-upgrade, bare-parent, deployToRoot

**Constraints**:
- `parlay upgrade` MUST hard-error in bare-parent state — no schemas deployed, no skills deployed, no partial work. Atomic: either the whole upgrade runs or none of it does
- The error message MUST name the missing file path AND point at `parlay repair` by name. No jargon-only message
- The pre-existing "run parlay init first" message is preserved for the case where neither `roots.yaml` nor `config.yaml` exists — that's a different failure (uninitialized project) than bare-parent
- `parlay upgrade` MUST NOT print any warning or info line in correctly-configured projects (single-root or multi-root with parent `config.yaml`) — quiet success path is preserved
- The fallback's removal MUST be reflected in the upgrade command's help text and any related documentation — no references to "bare-parent" as a supported state remain after this lands
- This change ships in the same release as Intents A, B, and C; the user-facing migration story is "run `parlay repair` once" rather than "wait for a deprecation cycle"

**Verify**:
- `parlay upgrade` in a multi-root project where the parent has `roots.yaml` but no `config.yaml` errors with `bare-parent topology: <parent>/.parlay/config.yaml is missing — run \`parlay repair\` to create it` and exits non-zero. Nothing is deployed
- `parlay upgrade` in a correctly-configured multi-root project (parent has `config.yaml` with `ai-agent`) deploys both schemas and skills with no warnings
- `parlay upgrade` in a single-root project deploys both schemas and skills with no warnings
- `parlay upgrade` in a directory with neither `roots.yaml` nor `config.yaml` errors with the pre-existing "run parlay init first" message — distinct from the bare-parent message
- After `parlay repair` migrates a bare-parent project, the next `parlay upgrade` runs cleanly
- The `parlay upgrade --help` text contains no reference to "bare-parent" as a supported state
