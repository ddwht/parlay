# Create Domain Model

> Rename `parlay extract-domain-model` to `parlay create-domain-model` and broaden the command to cover both modes a designer actually encounters: **brownfield** (extract from existing intents/dialogs) and **greenfield** (write a schema-valid empty stub so the designer — typically in Parlay Studio's Domain Model Editor — can author one from scratch). The verb shifts from "extract" to "create" because the command now spans both modes against a single entry point.

---

## Rename to `parlay create-domain-model` and Preserve Brownfield Behavior

**Goal**: Replace the existing `parlay extract-domain-model` CLI command with `parlay create-domain-model`, keeping the current extraction-from-signals behavior bit-for-bit identical for projects that have intents and dialogs with extractable entities, relationships, and operations.
**Persona**: UX Designer
**Priority**: P0
**Context**: The current `extract-domain-model` command produces `domain-model.yaml` at the project root by reading entities, relationships, and operations out of every feature's intents and dialogs. That behavior is correct for projects with content and is the baseline this feature must preserve. The rename is motivated by the new greenfield mode (next intent) and by `studio-support/studio-cli-hooks`, whose hook trio refers to the new name.
**Action**: Rename the command from `extract-domain-model` to `create-domain-model` across the CLI command registration, the embedded skill source, the deployed `.claude/skills` entry, the deployer's embedded-skills list, and the generic-CLI hardcoded command list. Brownfield-mode output (a populated `domain-model.yaml` produced from extractable signals) is unchanged: the command reads the same inputs, applies the same extractor, and writes to the same path. The only externally observable difference for brownfield runs is the command name and the help text.
**Objects**: cli-command, parlay-create-domain-model, brownfield-mode, domain-model-yaml, embedded-skill, deployer-registration

**Constraints**:
- Brownfield behavior is preserved exactly: same inputs, same extractor, same output path (`{root}/domain-model.yaml`), same schema, same exit codes
- The old command name `parlay extract-domain-model` is a **hard cutover**: after this feature ships, invoking the old name produces an "unknown command" error from the CLI's standard handler. No alias, no deprecation shim. (Rationale: the project rule is to change the code rather than carry compatibility shims, and every in-repo caller is updated as part of this feature's rollout.)
- The embedded skill source file is renamed from `core/internal/embedded/skills/parlay-extract-domain-model.skill.md` to `core/internal/embedded/skills/parlay-create-domain-model.skill.md`. The deployed copy at `.claude/skills/parlay-extract-domain-model/SKILL.md` is removed and `.claude/skills/parlay-create-domain-model/SKILL.md` is written in its place by `parlay upgrade`.
- The deployer's embedded-skills list (whatever enumerates which skills to materialize on `parlay init` / `parlay upgrade`) and the generic-CLI hardcoded command list are both updated to the new name. Both must agree, so the deployer never tries to write a stale file and the CLI never advertises a stale command in `parlay --help`.
- In-repo references to the old command name in skill bodies, intents, dialogs, surface, and infrastructure files of OTHER features are updated as part of this feature's rollout pass — but the spec edits to those other features happen in their own loops, not in this feature's specs. (This intent owns: the rename itself. It does not own: rewording other features' intents.)
- Tests covering brownfield extraction (unit + integration) are renamed alongside but their assertions are unchanged.

**Verify**:
- `parlay create-domain-model` on a project with extractable signals produces the same `domain-model.yaml` (byte-equivalent) that `parlay extract-domain-model` produced before this feature
- `parlay extract-domain-model` after this feature ships exits non-zero with the CLI's standard "unknown command" error — no alias, no warning, no fallback
- `parlay --help` lists `create-domain-model` and does not list `extract-domain-model`
- After `parlay upgrade`, `.claude/skills/parlay-create-domain-model/SKILL.md` exists and `.claude/skills/parlay-extract-domain-model/SKILL.md` does not
- The deployer's embedded-skills list contains `parlay-create-domain-model` and not `parlay-extract-domain-model`
- The generic-CLI hardcoded command list contains `create-domain-model` and not `extract-domain-model`
- Brownfield unit tests pass with the renamed command, exercising the same fixtures the old tests used

---

## Greenfield Mode: Write a Schema-Valid Empty Stub When There Is Nothing to Extract

**Goal**: When the project has no extractable signals — no features with entities, relationships, or operations the extractor recognizes — `parlay create-domain-model` writes a schema-valid empty `domain-model.yaml` stub so a designer (typically in Parlay Studio's Domain Model Editor) can author the model from scratch.
**Persona**: UX Designer starting a new project, or a designer on a project whose intents have not yet introduced any extractable entities
**Priority**: P0
**Context**: Today, on a project with no extractable content, `extract-domain-model` produces nothing useful — at best an empty file, at worst an error. This dead-ends the workflow at exactly the moment a designer most needs scaffolding. The `studio-support/studio-cli-hooks` feature already commits to a "ready to author — open Studio?" prompt against the greenfield path; this intent provides the on-disk artifact that prompt is offering to edit. With this intent, "create" becomes the right verb against a single command: it produces a domain model in either case, populated or empty.
**Action**: When `parlay create-domain-model` runs and the extractor finds no entities, no relationships, and no operations across all features, the command writes a schema-valid empty `domain-model.yaml` to the project root containing only the required top-level scaffolding (e.g., empty `entities`, `relationships`, `operations` collections plus whatever metadata the schema mandates). The stub validates against the domain-model schema cleanly. The command exits 0 and prints a one-line message clarifying that the stub is empty and ready to author.
**Objects**: greenfield-mode, empty-stub, domain-model-yaml, schema-validation, extractable-signals

**Constraints**:
- Greenfield is **detected automatically** from the absence of extractable signals. There is no `--greenfield` or `--empty` flag. (Rationale: the studio-cli-hooks feature already describes greenfield as "no model and no extractable signals", and a flag would let the two definitions drift.)
- The extractor's "no signals" check is the same check brownfield mode uses — there is exactly one source of truth for "did extraction find anything." The branch between brownfield and greenfield happens after that check returns its verdict, not in a parallel detector.
- The empty stub validates against the same schema brownfield output validates against. A designer who opens it in Studio sees a well-formed-but-empty model, not a syntax error.
- If `domain-model.yaml` already exists at the project root, the command does NOT clobber it with an empty stub even if the extractor finds no signals. Existing-file handling is the same as brownfield: the existing file is read, the extracted (or empty) result is reconciled with it, and the merge rules are unchanged. Greenfield is fundamentally about the empty-project case where there is nothing on disk yet.
- The user-visible output for greenfield is a single line distinguishing the empty stub from a populated extraction (e.g., "Created empty domain-model.yaml stub at {path} — ready to author."). This wording is what the studio-cli-hooks "ready to author — open Studio?" prompt chains off of.
- Greenfield mode is interactive-agnostic: it works the same in CI/non-TTY runs as in interactive runs. Only the Studio hook (a separate feature) cares about TTY.

**Verify**:
- On a project with zero features, `parlay create-domain-model` writes a `domain-model.yaml` at the project root that validates clean against the domain-model schema
- On a project with features but where no feature has any extractable entity, relationship, or operation, `parlay create-domain-model` produces the same empty stub as the zero-features case (greenfield)
- On a project with at least one extractable signal anywhere, `parlay create-domain-model` produces the populated brownfield output (NOT a stub) — confirming the brownfield/greenfield branch threshold is "any signal at all"
- On a project where `domain-model.yaml` already exists and there are no extractable signals, the existing file is preserved unchanged — the stub does not overwrite it
- The greenfield stub passes `parlay validate --type domain-model {path}` (or whatever the equivalent schema-validation entry point is) without modification
- The greenfield run prints a single-line message that distinguishes itself from a brownfield run, so a downstream caller (Studio hook, scripted pipeline) can disambiguate from stdout
- The greenfield run exits 0
- A unit test exercises a synthetic project with zero extractable signals and asserts the produced YAML matches a fixture for the empty stub
- A unit test exercises a synthetic project with one minimal extractable signal and asserts the produced YAML is populated rather than the empty stub — pinning the brownfield/greenfield boundary at "any signal"

---
