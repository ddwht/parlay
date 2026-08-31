# Studio-deployer — Dialogs

---

### Studio binary embeds its skills and deploys them via init/upgrade subcommands

**Trigger**: Studio maintainer is preparing a parlay-studio binary release that needs to ship its own skills (starting with `parlay-design-loop`) to end-user projects via a `parlay-studio init` / `parlay-studio upgrade` deployer that mirrors parlay's own embed-and-deploy pattern.

User: I'm cutting the first parlay-studio release. The current monorepo has `parlay-design-loop` checked in at `.claude/skills/parlay-design-loop/SKILL.md` as a development convenience. Before I ship, where does that skill actually live, and how does an end user with parlay-studio installed get it into their project?
System: The canonical source-of-truth for every Studio-owned skill is `studio/internal/embedded/skills/<skill-name>.skill.md`, embedded into the parlay-studio binary at compile time via `//go:embed` directives in `studio/internal/embedded/embed.go`. The embedded surface mirrors parlay's `core/internal/embedded/` layout — subdirectories `skills/` today, and `agents/`, `schemas/`, `adapters/` as future Studio features need them. End users get the skill by running `parlay-studio init` once per parlay project (first-time bootstrap, may emit a one-time banner) or `parlay-studio upgrade` after a Studio binary update (idempotent re-deploy). Both subcommands resolve the active parlay project root using studio-config's existing resolver (the `--project` flag / `STUDIO_PROJECT_ROOT` env / cwd walk-up precedence is unchanged), refuse to operate against a directory without a `.parlay/` subdirectory, detect every present agent surface, and write each embedded skill to every detected agent's target path. Both report the deployed file list to stdout, one line per file, with path + source-component attribution mirroring parlay's deployer reporting. Studio's deployer does NOT shell out to parlay or call into parlay's binary — Studio reads its own embedded source and writes directly; parlay's deployer and Studio's deployer are independent.

User: What happens to the current project-local copy at `.claude/skills/parlay-design-loop/SKILL.md` in this repo? It's been there for dogfooding.
System: The project-local *source* copy is removed by this feature. The canonical source-of-truth is `studio/internal/embedded/skills/parlay-design-loop.skill.md` — that's the file you edit when you change the skill. After the move, the studio repo itself becomes one of the projects that runs `parlay-studio init` against itself (the dogfooding loop), and the same file path `.claude/skills/parlay-design-loop/SKILL.md` reappears — but now as *deployer output*, regenerated from the embedded source on every Studio build + deploy cycle, not as a checked-in source artifact. Treat the working-tree presence of that path post-deploy as a build product, not as a source-of-truth violation; the grep verify checks the *source surface* (and git history for the move), not the working-tree output of a deployed self-install.

#### Branch: Studio source skill is missing required YAML frontmatter

User: I dropped a new skill source at `studio/internal/embedded/skills/parlay-foo.skill.md` but the build is failing.
System: Studio's embedded skill source files follow the same frontmatter convention as parlay's: every `.skill.md` source must begin with a YAML frontmatter block declaring `name:` and `description:` keys (the skill-frontmatter rule applies to Studio's skills, not just parlay's). The Studio build refuses to embed a skill source that fails this shape check. Fix by adding `---` / `name: parlay-foo` / `description: <one-line summary>` / `---` at the top of the file and rebuilding.

#### Branch: Question — should `parlay-studio init` chain to `parlay init` when the project lacks `.parlay/`?

User: I tested `parlay-studio init` against a fresh directory that doesn't have parlay initialized yet. It failed. Should it have offered to bootstrap parlay first?
System: Resolved during dialog authoring: **strict** — `parlay-studio init` fails with a stable error code (`studio-deployer-parlay-not-initialized`) naming the missing `.parlay/` marker, and instructs the operator to run `parlay init` first. Chain-to-parlay-init was considered (friendlier UX) but rejected because (a) it couples Studio's binary to parlay's binary being on PATH, which isn't required today for Studio's operation (Studio reads its own embedded source); (b) it muddles the two binaries' independence — operators learning the pipeline benefit from seeing the bootstrap step explicitly; (c) "strict" is the more predictable behavior in scripted environments. If a future feature adds an opt-in `--bootstrap` flag, that's a separate intent.

#### Branch: `parlay-studio init` invoked twice on the same project

User: I ran `parlay-studio init` twice by accident. Did the second invocation break anything?
System: No — `init` and `upgrade` produce the same deployed file set when run against the same embedded source. The second `init` behaves like an `upgrade`: every file is content-hash-compared against the existing on-disk file, and matched files are reported as `unchanged` (no write). The first-run banner (if any) is suppressed when the deployer detects a prior install. See the file-ownership/idempotency intent for the full status enum.

#### Branch: `parlay-studio upgrade` invoked before any `parlay-studio init` has run

User: I ran `parlay-studio upgrade` in a project where I never ran `parlay-studio init` first. What happened?
System: `upgrade` is identical to `init` minus the first-run banner — it does not require a prior install to exist. The first-time deploy proceeds normally and every file is reported as `written`. The two subcommands exist for UX differentiation (operators know which phase they're in), not because the underlying operation differs.

---

### Multi-agent target resolution

**Trigger**: Studio maintainer or end user invokes `parlay-studio init` / `upgrade` in a project whose agent surface configuration varies — Claude Code only, Cursor only, both present, Generic CLI only, none detected, or parlay not yet initialized. Each combination has a defined deployer behavior.

User: parlay supports several agents — Claude Code, Cursor, future MCP-catalog clients, and the Generic CLI fallback. How does Studio's deployer decide which one(s) to write to?
System: Studio's deployer detects every agent surface present in the active parlay project and writes its skills to each detected surface — no preferred-agent selection, no single-target assumption. The detection marker and per-agent target path conventions are parlay's existing convention; Studio reuses them rather than redefining. Concretely: Claude Code surfaces are detected by `.claude/` presence and target `.claude/skills/parlay-<name>/SKILL.md`; Cursor surfaces are detected by `.cursor/` presence and target the Cursor convention path; Generic CLI is detected via parlay's headless-mode marker and targets the CLI convention. When a project has multiple agent surfaces, the deployer fans out and writes to each one independently. When none is detected, the deployer fails with a stable error code instructing the operator to initialize at least one agent first. When `.parlay/` is missing, the deployer fails earlier with the parlay-not-initialized error code.

#### Branch: Claude-Code-only project

User: My project has `.claude/` but no `.cursor/`. What does the deployer write?
System: Only the Claude Code targets. For each embedded skill, the deployer writes one file at `.claude/skills/parlay-<name>/SKILL.md`. The stdout summary lists one `written` (or `unchanged` on a repeat) line per skill, all under `.claude/skills/`. No Cursor paths appear in the manifest because no Cursor surface was detected.

#### Branch: Cursor-only project

User: My project has `.cursor/` but no `.claude/`. What does the deployer write?
System: Only the Cursor targets. For each embedded skill, the deployer writes one file at the Cursor convention path (`.cursor/agents/parlay-<name>.md` per parlay's current convention; if parlay updates the convention, Studio picks up the change). The stdout summary lists one entry per skill under `.cursor/agents/`. No Claude Code paths appear in the manifest.

#### Branch: Both Claude Code and Cursor present

User: My project has both `.claude/` and `.cursor/`. What does the deployer write?
System: Both target sets. For each embedded skill, the deployer writes two files — one at the Claude Code path, one at the Cursor path — and the stdout summary lists both. The per-agent fan-out is atomic per agent (all files for one agent surface land in one coordinated write), but a failure on one agent surface does not block writes to the other agent surface. If the Claude Code write fails partway through, the Cursor writes still proceed; the exit code is non-zero (one or more `failed` statuses), and the operator can re-run after fixing the underlying issue.

#### Branch: Generic-CLI-only (headless) project

User: I'm in a CI environment with no `.claude/` or `.cursor/` — only parlay's Generic CLI fallback. Does the deployer work?
System: Yes. The Generic CLI is a first-class agent surface in parlay's detection — it's detected when parlay's headless-mode marker is present and no editor-specific surface is. Studio's deployer fans out to the Generic CLI convention path the same way it would to Claude Code or Cursor. The stdout summary lists one entry per skill under the CLI convention path. This is the canonical CI/headless flow.

#### Branch: No agent surface detected

User: I ran `parlay-studio init` in a parlay-initialized project, but I haven't set up any agent yet. What happened?
System: The deployer failed with stable error code `studio-deployer-no-agent-detected`. The error message names the agent surfaces parlay currently knows about (Claude Code via `.claude/`, Cursor via `.cursor/`, Generic CLI via parlay's headless marker) and instructs the operator to initialize at least one of them before running `parlay-studio init` or `upgrade`. The deployer wrote zero files and exited non-zero; no partial state is on disk.

#### Branch: parlay not initialized

User: I ran `parlay-studio init` in a directory that has no `.parlay/` subdirectory. What happened?
System: The deployer failed earlier than the no-agent-detected check, with stable error code `studio-deployer-parlay-not-initialized`. The error names the missing `.parlay/` marker and instructs the operator to run `parlay init` first. This is the resolution of the strict-vs-chain Question from the prior intent — Studio does not auto-bootstrap parlay; the operator's bootstrap step is explicit. The deployer wrote zero files and exited non-zero.

#### Branch: Question — does Studio import parlay's agent detection or duplicate it?

User: How does Studio actually know about parlay's agent surfaces? Does it import parlay's detection code, or maintain its own?
System: Resolved during dialog authoring: **duplicate the detection logic locally in `studio/internal/deployer/`**. Importing from a hypothetical `core/pkg/agent/` was considered — that would be DRY but would require parlay to promote its detection from `internal/` to `pkg/`, exposing API surface for stability commitments parlay hasn't taken on yet. Duplication is independent: Studio's deployer carries its own small detection package that matches parlay's current conventions exactly, and a parlay-studio binary update is required when parlay adds a new agent surface. The duplication footprint is small (detection is essentially "directory exists?" checks), the drift risk is mitigated by both binaries living in the same monorepo with shared CI, and the independence is worth the duplication. If a future refactor of parlay does expose `core/pkg/agent/` with stability guarantees, a follow-up feature can switch Studio over.

#### Branch: Partial agent surface — parent marker present but skill subdirectory missing

User: My project has a `.claude/` directory but no `.claude/skills/` subdirectory yet. What does the deployer do?
System: The deployer treats "parent marker present" as "agent detected" and creates the missing subdirectory as part of the per-agent atomic write. `mkdir -p` on the target's parent path is part of the same coordinated write step that produces the skill file — the operator doesn't need to pre-create skill directories. The same applies to `.cursor/` without `.cursor/agents/`, and to the Generic CLI convention. If creating the subdirectory fails (permissions, disk full), the deployer reports that agent's writes as `failed` and continues to other detected agents per the per-agent atomicity rule.

#### Branch: Future agent surface added to parlay

User: parlay just added support for a new agent (say, an MCP-catalog editor). Studio's deployer didn't pick it up. Why not?
System: Because Studio duplicates parlay's detection logic locally (per the Question resolved above) rather than importing it, a parlay-studio binary update is required to add support for a new agent surface. The Studio maintainer adds the new detection check + target-path convention to `studio/internal/deployer/` and ships a new Studio binary. End users get the new surface support by upgrading their `parlay-studio` binary and re-running `parlay-studio upgrade`. Until then, Studio's deployer ignores the new surface — it doesn't fail, but it also doesn't fan out to it. This is the documented trade-off of the duplication choice; the alternative (auto-pickup via shared library) requires parlay's `core/pkg/agent/` API to exist and stabilize first.

---

### File ownership, atomic writes, and idempotency

**Trigger**: Studio maintainer or operator runs `parlay-studio upgrade` (or `init`) against a project; the deployer must own only the files on its current manifest, use atomic writes, skip writes when content is unchanged, report orphans from previous Studio versions, and never touch user-authored content even when filenames collide with Studio's naming convention.

User: parlay's deployer owns every file under `.claude/skills/parlay-*/`. Studio's deployer needs the same discipline. How does Studio decide what it owns?
System: Studio's deployer maintains an explicit **owned-files manifest derived from the embedded source surface at build time**. Every entry in `studio/internal/embedded/skills/*.skill.md` (and any future embedded subdirectories) maps to one or more deployed-file paths — one per detected agent surface. A file is owned by Studio if and only if its path appears on the manifest produced for the current Studio binary build. Ownership is **by manifest, not by name prefix**. The `parlay-*` naming prefix is a documented convention for Studio-owned files (and matches parlay's own prefix convention so the visual boundary is consistent), but the prefix alone does not grant ownership — a user-authored skill that happens to use that prefix is not touched. Every output file is written atomically (write `<target>.tmp`, fsync, rename over `<target>`); a content hash of each embedded source is compared against the on-disk file before writing, and the write is skipped when hashes match. The stdout summary lists every file with one of four statuses: `written` (content changed), `unchanged` (content matched, write skipped), `orphan` (on disk but not on current manifest; logged WARN, untouched), `failed` (an error occurred). Exit code is 0 when every file is `written` / `unchanged` / `orphan`, non-zero when any file is `failed`.

#### Branch: User-authored skill with the `parlay-` naming-convention prefix

User: A teammate hand-authored a skill at `.claude/skills/parlay-my-custom/SKILL.md` to fit our project's naming conventions. Will `parlay-studio upgrade` overwrite or delete it?
System: No — it will be left completely untouched. Studio's manifest is derived from the embedded source surface, and `parlay-my-custom` is not in the embedded source. Because ownership is by manifest (not by prefix), the file's `parlay-` prefix does not grant Studio ownership. The deployer never reads, modifies, or deletes paths outside the manifest, regardless of name or location. The file does not appear in the stdout summary at all (Studio doesn't enumerate non-manifest paths; orphan reporting is specifically about *previously-Studio-owned* files no longer on the current manifest, not about arbitrary user files). This is the key safety property of manifest-based ownership over prefix-based ownership.

#### Branch: Stale skill from a previous Studio binary version (orphan)

User: I just upgraded to a new parlay-studio binary that removed an old skill, `parlay-old-thing`. The file still exists at `.claude/skills/parlay-old-thing/SKILL.md` from before. What happens when I run `parlay-studio upgrade`?
System: The deployer reports it as **`orphan`** in the stdout summary, logs a WARN line `studio-deployer-orphan-detected` naming the file path and (if discoverable from the file's content or accompanying metadata) the previously-owning Studio version, and **takes no further action**. The file is left on disk. Studio NEVER auto-deletes orphans — the operator decides whether to remove them after confirming they're no longer needed (the file may still be useful, or the operator may not have realized a skill was retired). The exit code is unaffected (`orphan` does not count as `failed`). If the operator confirms removal, they delete the file manually; the next `upgrade` no longer reports it because it's not on disk.

#### Branch: Repeated `upgrade` runs (idempotency)

User: I ran `parlay-studio upgrade` twice in a row over the same on-disk state. The second run was nearly instant and reported every file as `unchanged`. Is that the expected behavior?
System: Yes. Idempotency is a hard invariant: a second `upgrade` run over an unchanged embedded source against an unchanged on-disk state produces zero writes — every file is reported as `unchanged` (content hash matched, write skipped), the on-disk content hashes match the embedded source hashes, and no `.tmp` files appear in the file system at any point during the run. This invariant is unit-tested. If you ever see a second run report `written` for a file whose source and target are byte-equivalent, that's a bug in the content-hash-skip check.

#### Branch: Crash mid-write

User: The deployer crashed mid-run (power loss, kill signal, panic). I see a `.tmp` file alongside the target. What state is the original target in?
System: Intact. The atomic-write pattern is write `<target>.tmp` (with fsync), then `rename` over `<target>`. The rename is atomic at the filesystem level; the deployer either completes it (target updated) or doesn't (target untouched, `.tmp` is orphaned debris). The original target file is never in a half-written state observable from outside the deployer. On the next `upgrade` run, the deployer re-evaluates the target against the embedded source: if the prior run completed the rename before crashing, the file is `unchanged`; if it didn't, the file is `written` fresh. Orphan `.tmp` files are cleaned up at the start of each run (any `<target>.tmp` adjacent to a manifest path is removed before the new write begins). The on-disk invariant "every Studio-owned path is either byte-equivalent to a prior manifest's embedded source or to the current manifest's embedded source" holds across crashes.

#### Branch: Shared file write (CLAUDE.md, adapter-set.yaml, future)

User: Studio doesn't write to any shared files today, but if a future Studio feature needs to add a section to `CLAUDE.md`, how does that work?
System: Marker-based section preservation, matching parlay's pattern. The shared file is read; the region between `<!-- parlay-studio:begin -->` and `<!-- parlay-studio:end -->` HTML comment markers is replaced with Studio's content; everything outside the markers is preserved verbatim. If the markers don't exist yet (first time Studio writes to the shared file), Studio appends the new section with markers at the end of the file (or at a documented insertion point, depending on the shared file's conventions). Studio NEVER reads, modifies, or deletes content outside its markers. The four-status enum applies (`written` / `unchanged` / `failed` — `orphan` does not apply to marker-managed sections; a removed-from-manifest shared-file section is handled by a separate documented removal flow). No shared files are written today; this is forward-looking documentation only.

#### Branch: Manifest mismatch (drift between embedded source and manifest)

User: I'm debugging a deployer bug where the manifest seems to list a file that doesn't have a corresponding embedded source, or vice versa. How is the manifest produced?
System: The manifest is byte-equivalent to the file list derived from the embedded source surface at build time. The derivation is deterministic: every `studio/internal/embedded/skills/*.skill.md` source produces exactly the documented set of per-agent target paths (one path per detected agent surface, using the per-agent convention), and no other paths. A unit test asserts this byte-equivalence — every embedded source produces its expected targets, and no manifest entry exists without a corresponding embedded source. If you see drift, it's a bug in the manifest derivation logic, not a state issue; the manifest is reconstructed on every deployer invocation from the embedded source surface and never persisted between runs.

#### Branch: Non-atomic write primitive in deployer source

User: I just added a new helper to the deployer that calls `os.WriteFile` directly. Is that ok?
System: No. The deployer's atomic-write invariant requires every output file go through the canonical atomic-write helper (write `<target>.tmp`, fsync, rename). Direct calls to `os.WriteFile` or other non-atomic primitives are not allowed outside the helper itself. A grep across Studio's deployer source for direct `os.WriteFile` (and other non-atomic write primitives like `ioutil.WriteFile`, `os.Create` + `Write` patterns that don't go through the helper) must return zero matches outside the canonical atomic-write helper. CI enforces this. Replace your direct call with a call to the helper; if the helper doesn't cover your use case, extend the helper rather than bypassing it.

---
