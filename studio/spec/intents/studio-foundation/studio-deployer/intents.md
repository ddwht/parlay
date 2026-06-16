# Studio-deployer

> Parlay Studio is an extension to parlay: it installs on top of the parlay binary, ships its own embedded skills (parlay-design-loop today; future Studio skills later), and deploys them to a parlay project's agent surface(s) via `parlay-studio init` and `parlay-studio upgrade` subcommands. This feature pins how Studio mirrors parlay's deployer pattern: an embedded source surface inside the studio binary, a deployer subcommand that fans out to per-agent target conventions, and the file-ownership rules that let Studio idempotently overwrite its own deployed files without touching user content. Parlay supports multiple agents (Claude Code, Cursor, future MCP-catalog clients, plus the Generic CLI fallback); Studio's deployer matches that multi-agent surface — it does not assume Claude Code. Brew formula authoring, GitHub release pipelines, cross-compilation, and versioning schemes are external infrastructure concerns and are explicitly out of scope; this feature covers only the internal embedded-source + deployer-subcommand surface that makes Studio brew-installable as an extension once those external pieces exist.

---

## Studio binary embeds its skills and deploys them via init/upgrade subcommands

**Goal**: Pin that the Studio binary owns an embedded source surface (mirroring parlay's `core/internal/embedded/`) and exposes `parlay-studio init` and `parlay-studio upgrade` subcommands that fan the embedded skills out to the active parlay project's agent surface(s). The first concrete skill embedded is `parlay-design-loop` (currently project-local in `.claude/skills/parlay-design-loop/SKILL.md`); the relocation to embedded source is part of this intent.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: parlay's brew-installable binary contains everything it ships in a single executable via Go's `//go:embed` directive: skills, schemas, agent persona docs, adapters. End users run `parlay init` (first time in a project) or `parlay upgrade` (after a parlay binary upgrade) and the deployer reads from the embedded set and writes to the project's per-agent surface files. parlay-studio, as an extension to parlay, must follow the same pattern for its own skills: the `parlay-design-loop` skill (and any future Studio skills) lives in `studio/internal/embedded/skills/` as a `//go:embed`-backed source, and ships inside the `parlay-studio` binary itself. End users get the skill by running `parlay-studio init` once per project (or `parlay-studio upgrade` after a Studio binary update). The current project-local copy at `.claude/skills/parlay-design-loop/SKILL.md` is a development convenience for this monorepo; under this feature it relocates to the embedded source and is regenerated on every `parlay-studio` build via Go's embed mechanism. End users without parlay-studio installed never see the skill (correct — they aren't running Studio); users with parlay-studio installed but who haven't run `init` in their project see no skill either (correct — explicit opt-in is the right UX).

**Action**: Create `studio/internal/embedded/` with subdirectories `skills/` and (when future intents add them) `agents/`, `schemas/`, `adapters/` — mirroring parlay's `core/internal/embedded/` layout. Embed each subdirectory's contents via `//go:embed` directives in a new `studio/internal/embedded/embed.go`. Move `parlay-design-loop`'s source from `.claude/skills/parlay-design-loop/SKILL.md` to `studio/internal/embedded/skills/parlay-design-loop.skill.md` (canonical source-of-truth location); delete the project-local copy. Add `parlay-studio init` and `parlay-studio upgrade` subcommands to `studio/cmd/parlay-studio/main.go` (or in a dedicated `studio/internal/deployer/` package). The `init` subcommand resolves the parlay project root (using the existing project-root resolver), detects active agent surfaces, reads from the embedded skills set, and writes each skill to every detected agent's target path (per the multi-agent intent below). `upgrade` is identical except it runs unconditionally over an existing install (whereas `init` may add a one-time first-run banner). On every successful deploy, both subcommands report which files were written.

**Objects**: studio-embedded-source-surface, embedded-skills-subdirectory, go-embed-directives, parlay-design-loop-source-of-truth, parlay-studio-init-subcommand, parlay-studio-upgrade-subcommand, project-root-resolution-reuse, embedded-skill-source-singular-location

**Constraints**:
- Studio's embedded source surface lives at `studio/internal/embedded/`, with subdirectories matching parlay's convention (`skills/`, eventually `agents/`, `schemas/`, `adapters/` when future Studio features need them)
- Studio's embedded skill source files use the same naming and frontmatter convention as parlay's: `studio/internal/embedded/skills/<skill-name>.skill.md` with YAML frontmatter declaring `name:` and `description:` (the skill-frontmatter rule applies to Studio's skills, not just parlay's)
- The `parlay-design-loop` skill source lives exclusively at `studio/internal/embedded/skills/parlay-design-loop.skill.md`; the project-local copy at `.claude/skills/parlay-design-loop/SKILL.md` is removed by this feature (it was a development convenience that obscured the proper source-of-truth)
- `parlay-studio init` is invoked once per parlay project; `parlay-studio upgrade` is invoked after a Studio binary update; both produce the same deployed file set when run against the same embedded source
- Both subcommands report the deployed file list to stdout with one line per file written; the format mirrors parlay's deployer reporting (path + source-component attribution)
- Both subcommands resolve the active parlay project root using studio-config's existing project-root resolver (the `--project` flag / `STUDIO_PROJECT_ROOT` env / cwd walk-up precedence is unchanged); they refuse to operate against a directory that is not a parlay project (no `.parlay/` subdirectory)
- The Studio binary's `init` and `upgrade` commands do NOT call into parlay's binary or shell out — Studio reads its own embedded source and writes directly. parlay's deployer and Studio's deployer are independent

**Verify**:
- `studio/internal/embedded/skills/parlay-design-loop.skill.md` exists with proper YAML frontmatter (`name: parlay-design-loop`, descriptive one-liner); the `//go:embed` directive in `studio/internal/embedded/embed.go` includes it
- `.claude/skills/parlay-design-loop/SKILL.md` does NOT exist in the studio repo after this feature ships (the project-local copy is removed; the canonical source is the embedded one)
- A unit test invokes `parlay-studio init` against a fixture parlay project and asserts the project's `.claude/skills/parlay-design-loop/SKILL.md` (or each detected agent's equivalent) is written with content identical to the embedded source
- A unit test asserts `parlay-studio upgrade` over an existing install produces the same file set as a fresh `parlay-studio init` (idempotency)
- A unit test asserts `parlay-studio init` against a non-parlay directory (no `.parlay/` subdirectory) fails with a stable error code naming the missing marker
- The `parlay-studio init --help` and `parlay-studio upgrade --help` text describes the deployer behavior, the per-agent fan-out, and the file-ownership rules
- A grep across this repo for `\.claude/skills/parlay-design-loop` after the feature ships returns matches only in spec text and historical commit messages — the actual skill source is at the embedded location

**Questions**:
- Should `parlay-studio init` chain to `parlay init` if the project root has no `.parlay/` (i.e. offer to bootstrap parlay first), or strictly require parlay to be initialized before Studio runs? Chain is friendlier; strict is more predictable. Resolve during dialog authoring.

---

## Multi-agent target resolution

**Goal**: Pin that Studio's deployer writes its skills to every agent surface present in the active parlay project. parlay supports multiple agents — Claude Code (`.claude/skills/`), Cursor (`.cursor/agents/`), and the Generic CLI fallback for headless environments — and Studio matches that surface, never assuming Claude Code as the sole target.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: parlay's deployer detects the active agent(s) in a project — typically by checking for the presence of marker directories or config files (`.claude/`, `.cursor/`, etc.) — and writes its embedded artifacts to each detected surface with the appropriate per-agent file shape. A skill embedded in parlay produces `.claude/skills/parlay-<name>/SKILL.md` on Claude Code, `.cursor/agents/parlay-<name>.md` on Cursor, and the Generic CLI equivalent. Studio's deployer must mirror this exactly. If a project has both Claude Code and Cursor agent surfaces present, Studio deploys to both. If a project has only the Generic CLI fallback, Studio deploys to that. The detection logic and per-agent target paths are parlay's existing convention; Studio's deployer reuses those conventions rather than redefining them. The implementation may either import a shared agent-detection library from parlay (if parlay exposes it via `core/pkg/`) or duplicate the detection logic locally — both are acceptable; the build phase of this feature decides which is cleaner.

**Action**: Implement agent surface detection in `studio/internal/deployer/` (or wherever the Studio deployer lives). For each detected agent surface, the deployer resolves the per-agent target path for each embedded skill using parlay's documented convention: Claude Code skills land at `.claude/skills/parlay-<name>/SKILL.md`; Cursor skills land at the Cursor convention path (`.cursor/agents/parlay-<name>.md` or whatever parlay's deployer uses); the Generic CLI lands at the CLI convention. If no agent surface is detected, the deployer fails with a stable error code instructing the operator to initialize at least one agent first. If parlay has not been initialized in the project (no `.parlay/` directory), the deployer fails with a stable error code instructing the operator to run `parlay init` first (resolving the Question on the prior intent toward "strict" if that path is chosen).

**Objects**: agent-surface-detection, claude-code-target-path, cursor-target-path, generic-cli-target-path, multi-agent-fan-out, per-agent-skill-shape, no-agent-detected-failure-code, parlay-not-initialized-failure-code

**Constraints**:
- The Studio deployer detects every agent surface present in the active parlay project and writes its skills to each; it does not select one preferred agent and skip the others
- The per-agent target paths and per-agent file shapes are parlay's existing convention, not parlay-studio-specific: a Studio skill named `parlay-design-loop` lands at the same per-agent paths a hypothetical parlay-deployed skill of the same name would land at
- A project with no detected agent surface fails the deployer with stable code `studio-deployer-no-agent-detected` naming the agent surfaces parlay knows about
- A project without a `.parlay/` directory fails the deployer with stable code `studio-deployer-parlay-not-initialized` naming the missing marker
- The deployer's per-agent fan-out is atomic per agent — all skills for one agent surface land in one coordinated write, but failures on one agent surface do not block writes to other agent surfaces (each agent gets its own atomic write)
- The deployer respects parlay's file-ownership rules (next intent) on every per-agent target it writes to; the fan-out does not invent new ownership conventions per agent
- When parlay introduces support for a new agent surface (a future MCP-catalog client, a new editor's skill convention, etc.), Studio's deployer picks up the new surface automatically if it reuses parlay's detection library, or requires a parlay-studio update if it duplicates the logic — the build phase decides which approach is used

**Verify**:
- A unit test sets up a fixture project with only a `.claude/` directory present and asserts the deployer writes the skill only to `.claude/skills/parlay-design-loop/SKILL.md`
- A unit test sets up a fixture project with only a `.cursor/` directory and asserts the deployer writes to the Cursor convention path
- A unit test sets up a fixture project with both `.claude/` and `.cursor/` present and asserts both target paths receive the skill
- A unit test sets up a fixture project with no agent surface marker and asserts the deployer fails with `studio-deployer-no-agent-detected`
- A unit test sets up a fixture project with agent surface(s) but no `.parlay/` directory and asserts the deployer fails with `studio-deployer-parlay-not-initialized`
- A unit test invokes the deployer twice in succession and asserts each per-agent target receives identical content on both runs (multi-agent idempotency)

**Questions**:
- Should the Studio deployer reuse parlay's agent-detection logic by importing from `core/pkg/agent/` (a path that doesn't exist yet — parlay would need to expose its detection from `internal/` to `pkg/`), or duplicate the detection logic locally in `studio/internal/deployer/`? Importing is DRY but couples Studio's build to core's API stability; duplication is independent but drifts when parlay adds a new agent surface. Resolve during dialog authoring.

---

## File ownership, atomic writes, and idempotency

**Goal**: Pin that Studio's deployer owns every file it writes (mirroring parlay's file-ownership rules), uses atomic writes (write-temp + rename), and is idempotent — running `parlay-studio upgrade` twice over the same embedded source produces no on-disk changes on the second run.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: parlay's deployer takes ownership of every file under the project's `.claude/skills/parlay-*/`, `.parlay/schemas/`, etc. — running `parlay upgrade` is expected to overwrite those files because they're machine-generated from parlay's embedded source. User-authored content that lives next to those files (the user's own custom skills, sections of `CLAUDE.md` outside parlay's markers, project-specific schemas) is never touched. Studio's deployer needs the same discipline: own its files completely, treat user content as off-limits. Studio's deployed files all live under `parlay-*` named paths (matching parlay's prefix convention) so the boundary is visually obvious. Atomic writes (write-temp + rename) ensure a crashed deployer cannot leave a half-written skill file. Idempotency means re-running upgrade is safe — the deployer compares the embedded source against the on-disk file and either writes (when they differ) or skips (when identical); a second run on an unchanged embedded source produces zero writes.

**Action**: Implement the file-ownership rules in Studio's deployer mirroring parlay's: every file Studio writes carries an implicit "owned by parlay-studio" label via its `parlay-*` naming prefix; user files (everything not under that prefix) are never read or modified by Studio's deployer. Use the standard write-temp + rename pattern (`<target>.tmp` written, fsync'd, rename'd over `<target>`) for every output file. Compute a content hash of each embedded source against the existing on-disk file before writing; skip the write when they match. Report which files were written and which were skipped in the deployer's stdout summary. If Studio ever needs to write to a shared file (e.g. `CLAUDE.md` if Studio adds a section), use the same marker-based section preservation pattern parlay uses for `CLAUDE.md` (`<!-- parlay-studio:begin -->` / `<!-- parlay-studio:end -->` markers around Studio's content; everything outside markers preserved verbatim).

**Objects**: parlay-studio-file-ownership-prefix, atomic-write-temp-rename, content-hash-skip-on-match, idempotent-upgrade, marker-based-section-preservation, deployer-stdout-summary

**Constraints**:
- Studio's deployer maintains an explicit owned-files manifest derived from the embedded source surface at build time: every entry in `studio/internal/embedded/skills/*.skill.md` (and any future embedded subdirectories) maps to one or more deployed-file paths per detected agent surface. A file is owned by the deployer if and only if its path appears on the manifest produced for the current Studio binary build
- Ownership is by manifest, not by name prefix. A user-authored file whose path happens to match the `parlay-*` naming convention (e.g. a hand-written `<agent-skill-dir>/parlay-my-custom/SKILL.md`) is NOT owned by Studio; the prefix is a documented convention for Studio-owned files but does not by itself grant ownership. Conversely, if a future Studio skill ever ships without the `parlay-` prefix (unlikely but allowed), the manifest still owns it
- The deployer reads, writes, or skips only paths on the current manifest; it never touches paths outside the manifest under any condition, regardless of name or location
- Orphan handling: when a previous Studio binary version shipped a skill that the current binary no longer includes (the file exists on disk but the current build's manifest does not include it), the deployer logs a `studio-deployer-orphan-detected` WARN line naming the file path and the previously-owning Studio version (if discoverable), and takes no further action. The file is preserved on disk; the operator can remove it manually after confirming it is no longer needed. The deployer NEVER auto-deletes orphans
- Every output file is written atomically (write to `<target>.tmp`, fsync, rename over `<target>`); a deployer crash mid-write cannot leave a partially-written skill on disk
- The deployer computes the content hash of each embedded source and compares against the existing on-disk file (when present); when hashes match, the write is skipped and the file is reported as `unchanged`
- A second `parlay-studio upgrade` run over an unchanged embedded source produces zero on-disk changes; the stdout summary reports every file as `unchanged`
- If Studio writes to any shared file (CLAUDE.md, an adapter-set.yaml, etc.) — none today, but future Studio features may — the write uses marker-based section preservation (`<!-- parlay-studio:begin -->` / `<!-- parlay-studio:end -->` HTML comment markers) so user content is preserved
- The deployer's stdout summary lists every file with one of four statuses: `written` (content changed), `unchanged` (content matched, write skipped), `orphan` (on disk but not on current manifest; logged WARN, untouched), `failed` (an error occurred); the exit code is 0 if every file is `written`, `unchanged`, or `orphan`, non-zero if any file is `failed`

**Verify**:
- A unit test runs `parlay-studio upgrade` against a fresh fixture and asserts every output file is reported as `written`
- A unit test runs `parlay-studio upgrade` a second time over the same on-disk state and asserts every file is reported as `unchanged`; the on-disk content hashes match the embedded source hashes
- A unit test places a user-authored file at `<agent-skill-dir>/my-custom-skill/SKILL.md` (a non-parlay-prefixed skill) and asserts the deployer does not read, modify, or delete it
- A unit test places a user-authored file at `<agent-skill-dir>/parlay-my-custom/SKILL.md` (a parlay-prefixed user skill — collision with Studio's naming convention) and asserts the deployer does not read, modify, or delete it; the file is not on the manifest, so it is not owned
- A unit test places a stale skill at `<agent-skill-dir>/parlay-old-studio-skill/SKILL.md` (was shipped by a previous Studio version, not on the current manifest) and asserts the deployer reports it as `orphan` in the stdout summary, logs `studio-deployer-orphan-detected` at WARN, and leaves the file on disk
- A unit test asserts the deployer's manifest is byte-equivalent to the file list derived from the embedded source surface: every embedded `skill.md` source produces exactly the documented set of per-agent target paths, and no other paths
- A unit test simulates a crash mid-write (the `.tmp` file exists but the rename hasn't run) and asserts the original target file is intact (no half-written state observable)
- A unit test confirms the deployer's stdout summary has the documented status format for each output file (the four-status enum: `written`, `unchanged`, `orphan`, `failed`)
- A grep across Studio's deployer source for direct `os.WriteFile` or non-atomic write primitives returns zero matches outside the canonical atomic-write helper

**Notes**:
- Brew formula authoring, GitHub release pipelines, cross-compilation matrices, and versioning schemes are external infrastructure concerns and are explicitly out of scope for this feature. This feature covers only the internal embedded-source surface and the deployer subcommands that make Studio brew-installable as an extension once those external pieces exist.
- The decision to keep the three Studio-related schemas (`vocabulary.schema.md`, `design-loop-result.schema.md`, `design-loop-conflicts.schema.md`) in core's embedded set rather than relocating them to Studio's embedded set is preserved: those schemas describe spec-layer shapes any parlay project might use, even projects without parlay-studio installed; shipping them with parlay is the lower-friction choice. If a future feature relocates them to Studio's embedded set, this intent's file-ownership rules apply identically.

---
