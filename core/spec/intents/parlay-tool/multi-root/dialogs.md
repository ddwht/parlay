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

#### Branch: Bare parent — features only in children

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

#### Branch: Bare-parent — skill at repo root with no parent features prompts

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

