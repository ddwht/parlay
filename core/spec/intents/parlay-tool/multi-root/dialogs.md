# Multi-root — Dialogs

---

### Discover Active Root via Cwd Walk-Up

**Trigger**: User runs any `parlay` command from a directory inside the repo

User: parlay sync @some-feature
System (background): Walks up from cwd looking for `.parlay/`, stopping at the first `.parlay/` encountered or at a `.git/` boundary.
System (background): Resolves to `==/abs/path/to/active-root/.parlay/==` and proceeds with the command.

#### Branch: Verbose mode shows resolution

User: parlay --verbose sync @some-feature
System: resolved root: ==/abs/path/to/active-root== (source: cwd walk-up)
System (background): Proceeds with sync.

#### Branch: PARLAY_ROOT override

User: PARLAY_ROOT=/abs/path/to/repo parlay sync @some-feature
System (background): Detects `PARLAY_ROOT`; skips cwd walk-up; verifies the path contains a `.parlay/` directory.
System (condition: --verbose): resolved root: ==/abs/path/to/repo== (source: PARLAY_ROOT)
System (background): Proceeds with sync.

#### Branch: PARLAY_ROOT points at a non-existent path

User: PARLAY_ROOT=/nonexistent parlay sync @some-feature
System: error: PARLAY_ROOT=/nonexistent does not contain a .parlay/ directory
System (background): Exits non-zero. Does not fall back to cwd walk-up.

#### Branch: No root found (walk-up hits .git)

User: parlay sync @some-feature
System (background): Walks up from cwd. Encounters `.git/` before any `.parlay/`.
System: error: no parlay root found in this repo (stopped at .git boundary)
System: Run `parlay init` to create one, or `parlay add-root <subdir>` if you have a parent root elsewhere.

#### Branch: No root found (walk-up hits filesystem boundary)

User: cd /tmp && parlay sync @some-feature
System (background): Walks up from `/tmp` to `/`. No `.parlay/`, no `.git/` either.
System: error: no parlay root found

#### Branch: Ambiguous resolution — interactive disambiguation

User: parlay sync @some-feature
System (background): Walk-up fails at the current location. Discovers candidate roots — either `.parlay/` directories below cwd or entries in a parent's `roots.yaml`.
System: No active root at the current location, but found candidate roots:
  A: ==web== (apps/web)
  B: ==api== (apps/api)
  C: Cancel
User: Selects A
System (background): Uses `web` as the active root; proceeds with sync.

#### Branch: Ambiguous resolution — non-interactive run errors

User: parlay sync @some-feature   (running in CI / no TTY)
System: error: no parlay root found at this location; candidate roots: web, api
System: specify --root <name> or set PARLAY_ROOT=<abs-path> for non-interactive runs.

---

### Add a Child Root to an Existing Project

**Trigger**: /parlay add-root ==subdir==

User: parlay add-root apps/web
System (background): Verifies cwd is itself a registered parlay root.
System (background): Verifies `apps/web/` does not already contain a `.parlay/`.
System (background): Verifies `apps/web/` is not nested inside another child root.
System (background): Creates `apps/web/.parlay/` with a parent pointer; creates `apps/web/spec/`.
System (background): Appends `apps/web` to the parent's `.parlay/roots.yaml`.
System (background): Runs the deployer/upgrade refresh at the parent root so the agent-rules file (`CLAUDE.md` or equivalent) lists the new root.
System: Created child root at apps/web/.
System: Registered in parent's roots index as ==web==.
System: Agent surface refreshed (==CLAUDE.md== now lists apps/web).

#### Branch: Subdir already has a .parlay/

User: parlay add-root apps/web
System: error: apps/web/ already contains a .parlay/ directory.
System: If this is an existing parlay root, no action is needed. To upgrade it, run `parlay upgrade` from the parent root.

#### Branch: Cwd is not a parlay root

User: cd /elsewhere && parlay add-root apps/web
System: error: must be run from inside an existing parlay root (cwd has no `.parlay/` and no parent root via walk-up).
System: Run `parlay init` first to create a parent root.

#### Branch: Subdir is nested inside another child root

User: parlay add-root apps/web/inner
System: error: apps/web is already a registered child root; nesting child roots is not supported in v1.

#### Branch: Adding the same root twice

User: parlay add-root apps/web   (second invocation)
System: error: apps/web/ already contains a .parlay/ directory.

#### Branch: Orphaned child — parent removed

User: cd apps/web && parlay sync @feat
System (background): Reads `apps/web/.parlay/config` parent pointer; tries to resolve the parent path.
System: error: parent root not found at ==/abs/path/to/parent==; restore it or run `parlay promote-root` to make this child standalone.
System (background): Refuses to fall through to a different ancestor.

#### Branch: Promote orphaned child to standalone

User: cd apps/web && parlay promote-root
System (background): Removes parent pointer from `apps/web/.parlay/config`; child becomes its own top-level root.
System: apps/web is now a standalone parlay root.

#### Branch: Feature-empty parent — features only in children

User: parlay init   (in repo root)
System (background): Creates `.parlay/` (schemas, adapters, roots.yaml), `spec/intents/`, `.claude/skills/`, `CLAUDE.md`. The parent root exists; it has no features yet.
User: parlay add-root apps/web
System (background): Creates child root at apps/web/.
User: parlay add-root apps/api
System (background): Creates child root at apps/api/.
User: parlay status   (run at repo root)
System: root: ==parent== (==/abs/path/to/repo==)
System:   features: ==(none)==
System:   child roots:
System:     - ==web== (apps/web)
System:     - ==api== (apps/api)
System (condition: --verbose): the parent has an empty spec/intents/ — this is a supported configuration. Features live in child roots.

#### Branch: Parent has no features — commands still work

User: cd /repo && parlay extract-domain-model --root web
System (background): Operates on `apps/web` exclusively; the empty parent `spec/intents/` is irrelevant.
System: Domain model extracted for root ==web==.

User: cd /repo && parlay status
System (background): Walks up to parent root; lists zero parent features and the registered children. No "no features found" error.
System: root: ==parent==; features: 0; child roots: ==web, api==

---

### Inherit Resources from Parent Root

**Trigger**: User runs a `parlay` command inside a child root

User: cd apps/web && parlay --verbose build-feature @my-feature
System (background): Resolves active root to `apps/web` via walk-up.
System (background): For each resource type, records which root provided it.
System: Loaded resources:
System:   - schemas: ==parent root== (==/abs/path/to/repo==)
System:   - adapter (react): ==child root== (apps/web — overrides parent)
System:   - adapter (storybook): ==parent root== (no override at child)
System:   - deployed skills: ==parent root==
System (background): Proceeds with build-feature against apps/web.

#### Branch: Adapter override at child

User: cd apps/web && parlay build-feature @my-feature
System (background): Loads `apps/web/.parlay/adapters/react.adapter.yaml` instead of the parent's.
System: Loaded react adapter from child root (parent's react adapter ignored).

#### Branch: No adapter override — falls back to parent

User: cd apps/web && parlay build-feature @my-feature
System (background): No `apps/web/.parlay/adapters/react.adapter.yaml` exists.
System (background): Loads `<parent>/.parlay/adapters/react.adapter.yaml` instead.

#### Branch: Forbidden — child has its own .parlay/schemas/

User: cd apps/web && parlay sync @feat
System: error: schemas live at the parent root only. Found `apps/web/.parlay/schemas/`.
System: Remove this directory; schemas are loaded from the parent automatically.
System (background): Refuses to run.

#### Branch: Forbidden — child has its own deployed-skills directory

User: cd apps/web && parlay sync @feat
System: error: deployed agent skills live at the repo-level root only. Found `apps/web/.claude/skills/`.
System: Remove this directory; skills are loaded from the parent automatically.

#### Branch: Per-root domain model

User: cd apps/web && parlay extract-domain-model
System (background): Reads only `apps/web/spec/intents/`; does not include parent intents.
System: Extracted domain model for root ==web== (==/abs/path/to/repo/apps/web==).
System: Domain model saved to apps/web/spec/intents/domain-model.md (==12== entities).

#### Branch: Feature visibility is root-scoped

User: cd apps/web && parlay status
System (background): Lists features only from `apps/web/spec/intents/`.
System: Features in root ==web==:
System:   - ==feature-1==
System:   - ==feature-2==
System (condition: parent has features): Note: features defined at the parent root are not visible from this child (cross-root inheritance is out of scope for v1).

---

### Target a Non-Active Root from Any Cwd

**Trigger**: User runs a `parlay` command with a root prefix on the feature reference, or with the `--root` flag

User: parlay sync web:@parlay-tool/multi-root
System (background): Reads parent's `.parlay/roots.yaml`; resolves `web` to its absolute path.
System: Operating on root: ==web== (==/abs/path/to/repo/apps/web==)
System (background): Runs sync against the named root, regardless of cwd.

#### Branch: --root flag for project-level commands

User: parlay extract-domain-model --root web
System (background): No feature reference, but `--root web` selects the target root.
System: Operating on root: ==web==
System (background): Extracts domain model for that root only.

#### Branch: Unknown root name

User: parlay sync unknown:@feat
System: error: unknown root 'unknown'; known roots: ==web, api==
System: Use one of the known root names, or omit the prefix to use the active root from cwd.

#### Branch: Bare reference, exactly one match in another root

User: cd /repo-root && parlay build-feature @intent/A
System (background): Active root is the parent. Feature does not exist at parent.
System (background): Searches registered child roots; finds exactly one match in `apps/web`.
System: using root: ==web== (only match for @intent/A)
System (background): Proceeds with build-feature.

#### Branch: Bare reference, multiple matches — interactive disambiguation

User: cd /repo-root && parlay build-feature @intent/A
System (background): Active root is the parent. Feature does not exist at parent.
System (background): Finds matches in two child roots.
System: @intent/A matches in multiple roots. Which one?
  A: ==web== (apps/web)
  B: ==api== (apps/api)
  C: Cancel
User: Selects A
System (background): Proceeds with build-feature against apps/web.

#### Branch: Bare reference, multiple matches — non-interactive errors

User: parlay build-feature @intent/A   (running in CI / no TTY)
System: error: ambiguous: @intent/A matches roots [web, api]
System: use prefix (web:@intent/A) or `--root <name>` for non-interactive runs.

#### Branch: Conflict between --root and prefix

User: parlay --root api sync web:@intent/A
System: error: --root=api conflicts with prefixed reference web:@intent/A
System: pass one or the other, not both.

#### Branch: Cross-root reference in intent body — validation error

User: parlay validate spec/intents/some-feature/intents.md
System (background): Parses intents; finds `web:@another-feature/intent` inside an intent block.
System: error: cross-root references in intent content are not supported in v1
System:   at spec/intents/some-feature/intents.md:==42==

---

### Single Repo-Level Agent Surface Drives All Roots

**Trigger**: User runs `parlay upgrade`, or an agent invokes a deployed skill while its cwd is inside a child root

User: parlay upgrade
System (background): Verifies cwd is the repo-level root (the root that owns `.claude/`).
System (background): Re-deploys `.claude/skills/parlay-*/SKILL.md` from the embedded source.
System (background): Updates schemas at the repo-level root (`.parlay/schemas/`).
System (background): Updates the multi-root section of `CLAUDE.md`, listing all registered child roots; preserves user-authored sections.
System: Agent surface refreshed at ==/abs/path/to/repo==.
System: Skills deployed: ==18== Schemas updated: ==12==
System: Roots listed in CLAUDE.md: ==parent, web, api==

#### Branch: Run upgrade from inside a child root

User: cd apps/web && parlay upgrade
System: error: run upgrade from the repo-level root, not from a child root.
System: cd to ==/abs/path/to/repo== and try again.

#### Branch: Skill invoked from agent cwd inside a child root

User: /parlay-sync @feat   (agent cwd is apps/web/)
System (background): Skill is loaded from `<repo>/.claude/skills/parlay-sync/SKILL.md`.
System (background): Skill resolves active root via walk-up — finds `apps/web/.parlay/`.
System (background): Operates on the child root, not the repo root.
System: synced @feat in root ==web==.

#### Branch: User-authored CLAUDE.md sections survive add-root

User: parlay add-root apps/web
System (background): Reads existing `CLAUDE.md`. Identifies user-authored sections vs. tool-generated sections.
System (background): Updates only the multi-root section to include `apps/web`. Preserves user-authored sections verbatim.
System: CLAUDE.md updated. Your custom sections are preserved.

#### Branch: Cursor adapter — same rule

User: parlay upgrade   (project uses Cursor adapter)
System (background): Re-deploys `.cursor/agents/parlay-*.md` at the repo root.
System (background): Updates the multi-root section of the Cursor agent-rules file.
System: Cursor agent surface refreshed at ==/abs/path/to/repo==.

#### Branch: Generic CLI adapter — same rule

User: parlay upgrade   (project uses Generic CLI adapter)
System (background): Re-deploys `AGENT_INSTRUCTIONS.md` at the repo root.
System: Agent instructions refreshed at ==/abs/path/to/repo==.

#### Branch: Forbidden — agent-surface directory inside a child root

User: cd apps/web && parlay sync @feat
System: error: deployed agent surface lives at the repo-level root only. Found `apps/web/.claude/`.
System (background): Refuses to run; user must remove the directory.

---
### Skill Invocation in a Multi-Root Project

**Trigger**: User invokes any deployed `/parlay-*` skill from the agent (Claude Code, Cursor, Generic CLI)

User: /parlay-sync @feat   (agent cwd is apps/web/)
System (background): Skill is loaded from `<repo-root>/.claude/skills/parlay-sync/SKILL.md`.
System (background): Skill invokes `parlay sync @feat` from the agent's cwd (`apps/web/`).
System (background): CLI walk-up finds `apps/web/.parlay/`. Skill receives output scoped to root ==web==.
System: Synced @feat in root ==web==.

#### Branch: Skill receives ambiguous-reference signal — re-prompts via AskUserQuestion

User: /parlay-build-feature @intent/A   (agent cwd is repo root, parent has no @intent/A, two children do)
System (background): Skill invokes `parlay build-feature @intent/A` non-interactively (with a flag asking for structured output on ambiguity).
System (background): CLI returns ambiguous-reference signal with candidate roots [web, api].
System: @intent/A matches in multiple roots. Which one?
  A: ==web== (apps/web)
  B: ==api== (apps/api)
  C: Cancel
User: Selects A
System (background): Skill re-invokes `parlay build-feature web:@intent/A` with the chosen prefix.
System: operating on root: ==web==
System (background): Build proceeds against the chosen root.

#### Branch: Skill receives single-match auto-resolution — surfaces the announcement

User: /parlay-sync @intent/A   (agent cwd is repo root, parent has no match, exactly one child has it)
System (background): Skill invokes the CLI; the CLI auto-selects the single matching child root and prints "using root: web (only match)".
System: operating on root: ==web== (auto-resolved — only match)
System (background): Skill displays the rest of the CLI output unchanged.

#### Branch: Explicit root-prefix in skill argument

User: /parlay-sync web:@feat   (agent cwd unimportant)
System (background): Skill passes the prefix through to the CLI verbatim.
System: operating on root: ==web== (prefix)
System (background): Sync proceeds against root ==web==.

#### Branch: --root flag passed as skill argument

User: /parlay-extract-domain-model --root web
System (background): Skill passes `--root web` through to the CLI verbatim.
System: operating on root: ==web== (--root flag)
System (background): Domain model extracted for the chosen root only.

#### Branch: Feature-empty parent — skill at repo root with no parent features prompts

User: /parlay-sync @feat   (agent cwd is repo root, parent spec/intents/ is empty, two child roots exist)
System (background): Skill invokes the CLI; the CLI returns "feature not found in active root; matches roots [web, api]".
System: @feat does not exist in this root. Which root would you like to use?
  A: ==web== (apps/web)
  B: ==api== (apps/api)
  C: Cancel
User: Selects A
System (background): Skill re-invokes against the chosen root.

#### Branch: Forbidden directory — skill renders actionable guidance

User: /parlay-sync @feat   (agent cwd is apps/web/, which has a stray .parlay/schemas/)
System (background): Skill invokes the CLI; the CLI errors with "schemas live at the parent root only".
System: This child root has a `.parlay/schemas/` directory, but schemas are managed at the repo-level root.
System: Run `rm -rf apps/web/.parlay/schemas/` to remove it; schemas will be loaded from the parent automatically.
System (condition: agent supports tool execution): Want me to remove the directory now?
  A: Yes
  B: No, I'll do it myself

#### Branch: Forbidden agent-surface directory inside a child

User: /parlay-sync @feat   (agent cwd is apps/web/, which has a stray .claude/)
System (background): Skill catches the CLI's "deployed agent surface lives at the repo-level root only" error.
System: This child root has a `.claude/` directory, but the agent surface lives only at the repo root.
System: Remove `apps/web/.claude/`; the parent's `.claude/` is shared by all roots.

#### Branch: Skill works identically across adapters

User: /parlay-sync @feat   (project uses Cursor)
System (background): Cursor invokes its `parlay-sync` skill via the same CLI invocation pattern. Walk-up resolution and disambiguation behave identically.
System: Synced @feat in root ==web==.

User: /parlay-sync @feat   (project uses Generic CLI)
System (background): Generic CLI's `AGENT_INSTRUCTIONS.md` describes the same invocation pattern; skill content is identical to Claude/Cursor.
System: Synced @feat in root ==web==.

---

### Agent Identity Lives at the Parent in Multi-Root Projects

**Trigger**: User runs any `parlay` command that loads config in a multi-root project (e.g. `parlay upgrade`, `parlay status`, `parlay build-feature`)

User: parlay upgrade   (multi-root project, parent has `config.yaml` with `ai-agent: Claude Code`)
System (background): Loads parent `config.yaml`; reads `ai-agent: Claude Code`.
System (background): Selects the Claude Code deployer for both schemas AND skills.
System: Deployed schemas to ==/abs/path/to/repo/.parlay/schemas/== (==12==).
System: Deployed skills to ==/abs/path/to/repo/.claude/skills/parlay-*/== (==18==).

#### Branch: Child config declares ai-agent (forbidden)

User: parlay status   (multi-root project; `apps/web/.parlay/config.yaml` contains `ai-agent: Cursor`)
System (background): Loads child config; sees `ai-agent` field.
System: error: agent identity belongs at the parent root; remove `ai-agent` from ==apps/web/.parlay/config.yaml==.
System (background): Refuses to proceed.

#### Branch: Both parent and child declare ai-agent — agreeing or disagreeing

User: parlay status   (parent has `ai-agent: Claude Code`, `apps/web/.parlay/config.yaml` also has `ai-agent: Claude Code`)
System: error: agent identity declared at multiple levels:
System:   - ==/abs/path/to/repo/.parlay/config.yaml== (ai-agent: Claude Code)
System:   - ==/abs/path/to/repo/apps/web/.parlay/config.yaml== (ai-agent: Claude Code)
System: The model is exactly one agent identity per project. Run `parlay repair` to migrate.
System (background): Refuses to proceed even when the values agree — silent preference would let the inconsistency persist.

User: parlay status   (parent has `ai-agent: Claude Code`, `apps/web/.parlay/config.yaml` has `ai-agent: Cursor`)
System: error: agent identity declared at multiple levels with conflicting values:
System:   - ==/abs/path/to/repo/.parlay/config.yaml== (ai-agent: Claude Code)
System:   - ==/abs/path/to/repo/apps/web/.parlay/config.yaml== (ai-agent: Cursor)
System: Run `parlay repair` to choose which to keep at the parent.

#### Branch: Child omits sdd-framework, parent declares it — silent inheritance

User: cd apps/web && parlay build-feature @feat   (child config has no `sdd-framework`; parent declares `sdd-framework: parlay-spec`)
System (background): Loads child config; field absent. Walks parent pointer; reads parent's `sdd-framework: parlay-spec`.
System (background): Proceeds with build using the parent's value. No warning.

User: cd apps/web && parlay --verbose status
System: ai-agent: ==Claude Code== (from ==/abs/path/to/repo/.parlay/config.yaml==)
System: sdd-framework: ==parlay-spec== (inherited from ==/abs/path/to/repo/.parlay/config.yaml==)
System: prototype-framework: ==parlay-prototype== (from ==/abs/path/to/repo/apps/web/.parlay/config.yaml==)

#### Branch: Child omits sdd-framework AND parent does not declare it

User: cd apps/web && parlay build-feature @feat
System: error: no sdd-framework declared in child or parent.
System: Add `sdd-framework: <name>` to ==apps/web/.parlay/config.yaml== or to ==/abs/path/to/repo/.parlay/config.yaml== (parent default).
System (background): Refuses to proceed.

#### Branch: parlay upgrade with parent missing ai-agent (declared, then removed)

User: parlay upgrade   (parent `config.yaml` exists but the user removed the `ai-agent: ...` line)
System: error: no agent identity declared at parent root.
System:   - ==/abs/path/to/repo/.parlay/config.yaml== has no `ai-agent` field.
System: Add `ai-agent: <Claude Code|Cursor|Generic CLI>` and re-run, or run `parlay repair`.
System (background): Refuses to deploy. Never walks up, never silently skips.

#### Branch: Single-root project — no behavior change

User: parlay upgrade   (single-root project; one `config.yaml` with all three fields)
System (background): Loads the one config; reads `ai-agent`, `sdd-framework`, `prototype-framework`.
System: Deployed schemas (==12==) and skills (==18==) to ==/abs/path/to/repo==.

#### Branch: Verbose mode shows resolution source for every effective field

User: parlay --verbose status   (multi-root, parent declares ai-agent and sdd-framework, child overrides prototype-framework)
System: ai-agent: ==Claude Code== (from ==/abs/path/to/repo/.parlay/config.yaml==)
System: sdd-framework: ==parlay-spec== (from ==/abs/path/to/repo/.parlay/config.yaml==)
System: prototype-framework: ==react== (from ==/abs/path/to/repo/apps/web/.parlay/config.yaml==)

---

### parlay init Writes the Correct Topology Shape

**Trigger**: User runs `parlay init` (single-root or as the first step of bootstrapping a multi-root project), or `parlay add-root <child>` after the parent is initialized

User: parlay init   (empty directory, NOT invoked through a known agent)
System: ai-agent? [Claude Code | Cursor | Generic CLI]
User: Claude Code
System: sdd-framework? [parlay-spec | ...]
User: parlay-spec
System: prototype-framework? [parlay-prototype | react | ...]
User: parlay-prototype
System (background): Writes ==.parlay/config.yaml== with `ai-agent: Claude Code`, `sdd-framework: parlay-spec`, `prototype-framework: parlay-prototype`. No `parent:` field. Single-root project.
System: Initialized parlay project at ==/abs/path/to/dir==.

#### Branch: parlay init invoked through Claude Code — prompt is pre-filled

User: parlay init   (running inside Claude Code; env signals detected)
System: ai-agent? [==Claude Code== (detected)] — press Enter to confirm or type to override
User: <press Enter>
System (background): Confirms `ai-agent: Claude Code`.
System: sdd-framework? ...
User: ...
System (background): Writes config. Init never proceeds without explicit confirmation, even with a detected default.

#### Branch: parlay init invoked through Claude Code — user overrides the detected agent

User: parlay init   (running inside Claude Code)
System: ai-agent? [==Claude Code== (detected)]
User: Cursor
System (background): Records `ai-agent: Cursor` (override accepted).
System: sdd-framework? ...

#### Branch: Bootstrap multi-root — init at parent, then add-root per child

User: parlay init   (in repo root that will host children)
System (background): Prompts for `ai-agent` (pre-filled if detected). User confirms.
System (background): Optionally prompts for `sdd-framework` / `prototype-framework` defaults (used by parent's own features OR offered as defaults to children).
System (background): Writes ==/abs/path/to/repo/.parlay/config.yaml==.
User: parlay add-root core
System (background): Prompts for `sdd-framework` (default: parent's value). User confirms.
System (background): Prompts for `prototype-framework` (default: parent's value). User confirms.
System (background): Writes ==core/.parlay/config.yaml== containing `sdd-framework`, `prototype-framework`, `parent: ..`. Does NOT write `ai-agent` to the child.
User: parlay add-root studio
System (background): Same flow. Writes ==studio/.parlay/config.yaml== — no `ai-agent`.

#### Branch: parlay add-root invoked when parent has no ai-agent

User: parlay add-root core   (parent's `config.yaml` exists but has no `ai-agent`, OR parent has no `config.yaml` at all)
System: error: parent is missing ai-agent — run `parlay init` at the parent first.
System:   parent path: ==/abs/path/to/repo==
System (background): Refuses to create the child root. No partial work.

#### Branch: parlay init invoked at a child whose parent already has ai-agent

User: cd apps/web && parlay init   (child dir; `parent: ..` resolves to parent that has `ai-agent: Claude Code` set)
System: sdd-framework? [==parlay-spec== (default from parent)]
User: <press Enter>
System: prototype-framework? [==parlay-prototype== (default from parent)]
User: <press Enter>
System (background): Writes ==apps/web/.parlay/config.yaml== with `sdd-framework`, `prototype-framework`, `parent: ..`. ai-agent is NOT prompted and NOT written to the child.
System: Initialized child root at ==/abs/path/to/repo/apps/web==.

#### Branch: parlay init re-run on already-configured project — idempotent

User: parlay init   (project already has `.parlay/config.yaml` with all three fields set correctly)
System (background): Reads existing config. All required fields present and topology is correct.
System: Project already initialized. No changes made.
System (background): Exits zero. Does not re-prompt for any field.

User: parlay init   (multi-root project: parent has `ai-agent`, all children have framework + parent pointer)
System (background): Reads parent and child configs. Topology valid.
System: Project already initialized. No changes made.

#### Branch: Single-root project unchanged

User: parlay init   (empty directory, single-root use case)
System: ai-agent? ...; sdd-framework? ...; prototype-framework? ...
User: ...
System (background): Writes one ==.parlay/config.yaml== with all three fields. No `parent:` field, no `roots.yaml`. Behavior identical to today.

---

### Detect and Migrate Legacy Topology Mismatches

**Trigger**: User runs `parlay repair` (full topology check with prompts) or `parlay status` (read-only topology summary)

User: parlay repair   (project has bare-parent topology: `roots.yaml` at parent, no `config.yaml` at parent)
System (background): Runs topology-check pass; detects bare-parent mismatch.
System: Topology mismatch detected (1 of 1):
System:   ==bare-parent==: ==/abs/path/to/repo== has `roots.yaml` but no `config.yaml`.
System: Proposed fix: create ==/abs/path/to/repo/.parlay/config.yaml== with `ai-agent: <value>`.
System: ai-agent? [==Claude Code== (detected from current agent, or from a child config if present)]
User: <press Enter>
System (background): Writes parent config; re-scans.
System: Fix applied. No remaining mismatches.

#### Branch: Bare-parent — agent value comes from a child config

User: parlay repair   (parent has `roots.yaml` no `config.yaml`; `apps/web/.parlay/config.yaml` has `ai-agent: Cursor`)
System: Topology mismatch detected (1 of 2):
System:   ==bare-parent==: ==/abs/path/to/repo== has `roots.yaml` but no `config.yaml`.
System: Proposed fix: create ==/abs/path/to/repo/.parlay/config.yaml== with `ai-agent: ==Cursor==` (from ==apps/web/.parlay/config.yaml==).
System: Confirm? [Y/n]
User: Y
System (background): Writes parent config. Re-scans.
System: Topology mismatch detected (1 of 1):   (note: re-scan surfaced agent-at-child as the next mismatch)
System:   ==agent-at-child==: ==apps/web/.parlay/config.yaml== contains `ai-agent: Cursor`.
System: Proposed fix: remove `ai-agent` from ==apps/web/.parlay/config.yaml==. Parent already has `ai-agent: Cursor` (matches), so no parent change needed.
System: Confirm? [Y/n]
User: Y
System (background): Removes `ai-agent` from child. Re-scans. No further mismatches.
System: All topology mismatches resolved.

#### Branch: Agent-at-child — child config has ai-agent, parent does not

User: parlay repair   (parent `config.yaml` exists with no `ai-agent`; `apps/web/.parlay/config.yaml` has `ai-agent: Claude Code`)
System: Topology mismatch detected (1 of 1):
System:   ==agent-at-child==: ==apps/web/.parlay/config.yaml== contains `ai-agent: Claude Code`.
System: Proposed fix: remove `ai-agent` from the child config; write `ai-agent: Claude Code` to ==/abs/path/to/repo/.parlay/config.yaml==.
System: Confirm? [Y/n]
User: Y
System (background): Updates parent config (adds `ai-agent: Claude Code`); removes `ai-agent` from child. Re-scans. No remaining mismatches.

#### Branch: Both-have-agent, values agree

User: parlay repair   (parent and child both declare `ai-agent: Claude Code`)
System: Topology mismatch detected (1 of 1):
System:   ==both-have-agent==: ai-agent declared at both levels with matching value.
System:     - ==/abs/path/to/repo/.parlay/config.yaml== (ai-agent: Claude Code)
System:     - ==apps/web/.parlay/config.yaml== (ai-agent: Claude Code)
System: Proposed fix: remove `ai-agent` from ==apps/web/.parlay/config.yaml== (parent's value is authoritative).
System: Confirm? [Y/n]
User: Y
System (background): Removes child entry. No further mismatches.

#### Branch: Both-have-agent, values disagree

User: parlay repair   (parent has `ai-agent: Claude Code`, child has `ai-agent: Cursor`)
System: Topology mismatch detected (1 of 1):
System:   ==both-have-agent==: ai-agent declared at both levels with conflicting values.
System:     - ==/abs/path/to/repo/.parlay/config.yaml== (ai-agent: Claude Code)
System:     - ==apps/web/.parlay/config.yaml== (ai-agent: Cursor)
System: Which value should be kept at the parent?
  A: ==Claude Code== (parent's current value)
  B: ==Cursor== (child's current value)
  C: Skip
User: Selects A
System (background): Keeps parent's `ai-agent: Claude Code`; removes `ai-agent` from child. No silent pick.
System: Resolved.

#### Branch: Single-root missing ai-agent

User: parlay repair   (single-root project; one `config.yaml` with `sdd-framework` and `prototype-framework` but no `ai-agent`)
System: Topology mismatch detected (1 of 1):
System:   ==single-root-missing-ai-agent==: ==/abs/path/to/repo/.parlay/config.yaml== is missing `ai-agent`.
System: ai-agent? [==Claude Code== (detected from current agent)]
User: <press Enter>
System (background): Writes `ai-agent: Claude Code` to the config. No remaining mismatches.

#### Branch: Two simultaneous mismatches — walked one at a time

User: parlay repair   (project has agent-at-child AND a sibling child missing `sdd-framework`)
System: Topology mismatch detected (1 of 2):
System:   ==agent-at-child==: ==apps/web/.parlay/config.yaml== contains `ai-agent: Claude Code`.
System: Proposed fix: ...
User: Y
System (background): Applies fix. Re-scans.
System: Topology mismatch detected (1 of 1):   (note: count decremented after re-scan)
System:   ==missing-sdd-framework==: ==apps/api/.parlay/config.yaml== has no `sdd-framework` and parent does not declare one either.
System: Proposed fix: ...
User: Selects skip
System (background): Skips this fix. Re-scan still shows it.
System: 1 mismatch remaining (skipped). Re-run `parlay repair` to address.

#### Branch: User-authored fields preserved when ai-agent is moved out

User: parlay repair   (child config has `ai-agent: Cursor`, `sdd-framework: parlay-spec`, `prototype-framework: parlay-prototype`, `parent: ..`, AND a custom field `team: web`)
System: Topology mismatch detected (1 of 1):
System:   ==agent-at-child==: ==apps/web/.parlay/config.yaml== contains `ai-agent: Cursor`.
System: Proposed fix: remove `ai-agent` from child; write to parent.
System: Note: ==team: web== and other unrecognized fields will be preserved in the child config.
User: Y
System (background): Removes only `ai-agent`; preserves `team: web` and all other fields verbatim.

#### Branch: parlay status — clean topology

User: parlay status
System: root: ==parent== (==/abs/path/to/repo==)
System:   topology: ==ok==
System:   features: 5
System:   child roots: ==web, api==

#### Branch: parlay status — needs repair, summary only

User: parlay status   (project has 2 topology mismatches)
System: root: ==parent== (==/abs/path/to/repo==)
System:   topology: ==needs repair== (==2 mismatches== — run `parlay repair`)
System:   features: 5
System:   child roots: ==web, api==
System (background): Does NOT enumerate which mismatches; per-file detail belongs in `parlay repair`.

#### Branch: After successful repair — durable fix

User: parlay repair   (after a previous run resolved all mismatches)
System (background): Topology check passes cleanly.
System: No topology mismatches found.

User: parlay status
System: root: ==parent==
System:   topology: ==ok==

#### Branch: Other commands surface a one-line topology hint, never block on it

User: parlay upgrade   (topology has agent-at-child, but user is running an unrelated command)
System: error: no agent identity at parent — run `parlay repair`.
System (background): Refuses to deploy. Topology checks do not run on every command — they only fire when a command directly hits the failure (e.g. upgrade needs the parent's `ai-agent`).

---

### parlay upgrade Errors on Bare-Parent Topology

**Trigger**: User runs `parlay upgrade` in any project state — bare-parent, correctly-configured (single or multi-root), or uninitialized

User: parlay upgrade   (bare-parent topology: parent has `.parlay/roots.yaml`, no `.parlay/config.yaml`)
System: error: bare-parent topology: ==/abs/path/to/repo/.parlay/config.yaml== is missing — run `parlay repair` to create it.
System (background): Exits non-zero. Nothing is deployed: no schemas, no skills, no partial work. Atomic.

#### Branch: Correctly-configured multi-root — quiet success

User: parlay upgrade   (parent `config.yaml` declares `ai-agent: Claude Code`)
System (background): Loads parent config. Selects deployer.
System (background): Deploys schemas to ==/abs/path/to/repo/.parlay/schemas/== and skills to ==/abs/path/to/repo/.claude/skills/parlay-*/==.
System: Upgraded to ==v1.2.3==. Schemas: ==12==. Skills: ==18==.
System (background): No warnings, no info lines about topology.

#### Branch: Correctly-configured single-root — quiet success

User: parlay upgrade   (one `config.yaml` with all three fields)
System (background): Loads config; deploys schemas and skills.
System: Upgraded to ==v1.2.3==. Schemas: ==12==. Skills: ==18==.

#### Branch: Uninitialized directory — distinct error from bare-parent

User: parlay upgrade   (directory with neither `.parlay/roots.yaml` nor `.parlay/config.yaml`)
System: error: not a parlay project — run `parlay init` first.
System (background): Distinct message from the bare-parent error; this case is "uninitialized," that one is "structurally invalid."

#### Branch: After parlay repair fixes a bare-parent project

User: parlay repair   (bare-parent project; user accepts the prompted fix)
System: ... (see Detect and Migrate Legacy Topology Mismatches dialog)
User: parlay upgrade
System (background): Parent now has `config.yaml` with `ai-agent`. Upgrade runs cleanly.
System: Upgraded to ==v1.2.3==. Schemas: ==12==. Skills: ==18==.

#### Branch: parlay upgrade --help text contains no "bare-parent" as supported state

User: parlay upgrade --help
System: Re-deploy schemas, skills, and agent config to match the current parlay version.
System: ...
System (background): Help text mentions bare-parent only in the context of the error message, never as a supported configuration. The previous fallback's documentation is gone.

#### Branch: deployToRoot fallback removed — code path no longer exists

User: parlay upgrade   (bare-parent project)
System: error: bare-parent topology: ==/abs/path/to/repo/.parlay/config.yaml== is missing — run `parlay repair` to create it.
System (background): The previous `case os.IsNotExist(err):` arm in `deployToRoot` is gone. There is no soft-fail path that proceeds with empty config and skips skills. Single code path: correct topology or hard-error.

---

