# generate-code

_Generate prototype code from buildfile_

<!--
parlay-section: cross-cutting
parlay-extends: studio-support/layout-aware-codegen/layout-block-reader
parlay-extends: studio-support/layout-aware-codegen/resolved-binding-consumer
parlay-extends: studio-support/layout-aware-codegen/buildfile-freshness-gate
parlay-extends: studio-support/layout-aware-codegen/layout-validation-precheck-surfacer
parlay-extends: studio-support/layout-aware-codegen/non-interactive-codegen-pipeline
parlay-extends: parlay-tool/cross-cutting-target-paths/generate-code-skill-topological-emission-order
-->

# Generate Code

Translate ALL features' buildfiles into working prototype source code at the project level. Reads every feature's buildfile, merges cross-cutting concerns (models, routes), and generates code for the entire project incrementally.

## Arguments

None — this skill operates at the project level, not per-feature.

## Inputs (and the strict isolation rule)

This skill reads ONLY from these locations:

- `.parlay/schemas/buildfile.schema.md`
- `.parlay/schemas/adapter.schema.md`
- `.parlay/schemas/blueprint.schema.md`
- `.parlay/config.yaml`
- `.parlay/blueprint.yaml` — application blueprint (optional for CLIs, recommended for web/mobile)
- `.parlay/adapters/{framework}.adapter.yaml`
- `.parlay/build/{feature}/buildfile.yaml` — the features the project diff names as having work (step 4 scopes this; the allowlist is every feature, the read-set is not)
- The existing prototype source tree (for incremental updates)
- `.parlay/build/{feature}/testcases.yaml` — read **only** at the test execution phase, and only for the features you regenerated

**You must NOT read anything under `spec/intents/{feature}/`.** This includes intents.md, dialogs.md, surface.yaml, capabilities.yaml, and infrastructure.md. The buildfile is the deterministic intermediate; if you find yourself wanting to read source-of-truth design files to make a decision, the buildfile is leaking detail and the right fix is to enrich the buildfile schema, not to cross the boundary.

This isolation rule is the load-bearing test for whether the buildfile is doing its job. If a code generator can produce a working, test-passing prototype using only buildfile + adapter, the buildfile is correct.

**When the buildfile is not enough, report what is missing — and offer the way out.** The rule turns "make it pass" into "report what is missing", which is the most valuable behaviour this skill has: a gap named against the buildfile is fixable upstream, where a gap papered over with a guess is not. Keep reporting it.

But a report is a dead end if the gap is one no buildfile could close. Some things cannot be generated from a specification at all — a numerical kernel, a codec, a solver — and enriching the buildfile schema will not change that. When you conclude the gap is of that kind rather than a buildfile that is merely thin, raise `kind: impasse` (see **Asking the user**) offering the hand-authored unit described in `authored.schema.md`, pre-filled with the components you could not write and the paths you would have written them to. On acceptance the work becomes a declared unit: written by a person, tracked by parlay, and fenced off from this skill by step 11.4. Omit `default:` — accepting a unit is a permanent scope reduction and no flag authorizes it unattended.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Project-pass mode

This skill operates in **project-pass mode** by default — it walks every feature under the resolved root and emits all features in one pass. When invoked via `--project` (or the equivalent agent argument), behavior is identical: project-pass mode is the default, single-feature invocations are a special case where the cross-feature plannedCreates union is empty and the cross-feature dependency graph has at most one node.

Project-pass mode introduces two cross-feature contracts:

**The plan allowlist is audited, not intercepted.** Writing outside `plan.creates` / `plan.modifies` is an obligation on you: codegen's writes are performed by you, not by the tool, so nothing stops one at the moment it happens. What does exist is `parlay internal check-write-set`, which compares `.code-hashes.yaml` against every declared plan afterwards and reports `codegen-wrote-outside-plan` for anything undeclared. Emitting a file you have not declared is therefore detectable, not invisible — declare it or do not write it.

1. **Sibling-create satisfies modify.** A `plan.modifies` row in feature B is allowed to name a path that does not yet exist on disk, provided some other feature A in the same pass declares that path in its `plan.creates`. The validator (`parlay validate --project`) enforces this; codegen relies on the topological emission order (step 11.6 below) to make sure A's create has actually happened before B's modify runs.
2. **Cycles are rejected.** If A's `plan.modifies` matches B's `plan.creates` AND B's `plan.modifies` matches A's `plan.creates`, neither feature can run first. The validator surfaces `plan-create-modify-cycle`; codegen refuses to start emission and stops.

Single-feature invocations remain valid for tooling that wants to drive one feature at a time. In that mode the plannedCreates union is empty, every modify must point at an on-disk file, and the topological order has a single trivial node.

## Non-interactive, CI-safe by construction

Codegen has **no TTY-conditional code paths and no interactive prompts** in any path. The skill reads the buildfile, runs the freshness gate (see step 11.6), runs the layout-validation precheck (see step 11.7), and emits framework code via an AI agent — but the agent has no decisions left to make at codegen time. Wiring inference, disambiguation, and layout validation all live upstream (in build-feature and the layout-creation flow) and have already produced their verdicts before codegen runs.

Concretely:

- **No TTY checks.** Codegen behaves identically with and without a controlling terminal. A no-TTY container produces output behaviorally equivalent to a local TTY run on the same source state.
- **No interactive prompts.** Codegen never calls AskUserQuestion or any equivalent. Every prompt described in this skill (mount-strategy ambiguity, hand-edited file, etc.) is a **decision request returned to the orchestrator**, not a call codegen makes itself — the codegen execution is prompt-free. Where a step says "STOP and surface the situation" or "let the user choose", codegen halts and returns the choice to its caller, which owns all user interaction. This matters because codegen commonly runs inside a sub-agent where no interactive tool exists at all; a skill that tried to prompt from there would silently skip the gate instead of honoring it.

  Codegen takes no `--non-interactive` flag of its own. It used to claim it accepted one "for compatibility" with no observable effect, which was worth deleting on its own terms: a flag documented as doing nothing is still a promise that something reads it, and nothing did. The property that sentence reached for is the one stated above — codegen has no interactive path to disable — and it holds without a flag to assert it.

  The loop's `--non-interactive` is a different thing at a different layer, and it does reach this phase: it is threaded in so the decision requests raised here carry a `default:` where one is safe, and so the driver aborts rather than answers the `overwrite` and `failure` decisions this phase raises. That governs what the **orchestrator** does with a decision request. It adds no prompt here, because there was never one to add.
- **Atomic output.** On any per-page failure within a run (stale buildfile, layout precheck refusal, missing binding), no new files are written for the run — a half-written prototype never reaches CI's verification step.
- **Exit-code is the source of truth.** Process exit code is non-zero on any error path (stale buildfile, layout precheck refusal); zero on success. CI's pass/fail is derived from exit code, not from stdout pattern matching. Two CI workers running against the same source state produce identical exit codes and behaviorally-equivalent output (same testcases pass, same component tree emitted); lexical text may vary because the emitting AI agent is non-deterministic on text, but the CI pass/fail signal stays consistent. This governs generate-code's own output only — `create-domain-model`'s greenfield-stub message is a deliberate, narrow exception with pinned-stable wording that its own feature's testcases assert; see that skill's step 6.

## Asking the user

This skill runs as a **phase module** — normally inside a parlay-loop subagent, where no interactive tool exists. A question asked there is written into a transcript nobody reads, and you then answer it yourself; that is not a confirmation, it is a decision made on the user's behalf. So do not prompt. **Stop and return a decision request** as your final output. The driver prompts and resumes you with the chosen `id`, with your context intact, so you continue exactly where you stopped.

````
```yaml parlay-decision
kind: phase-boundary        # phase-boundary | override | overwrite | failure | ambiguity | impasse
phase: <the phase you are in>
question: "<the one question, in the user's terms>"
context: |
  <what you found, and what is already on disk>
options:
  - id: <slug>
    label: "<what the user picks>"
    detail: "<the consequence, when it isn't obvious>"
default: <id>               # advancement kinds ONLY — see below
resume: "Re-enter with decision: <id>. <what is written so far>"
```
````

**The `default:` field.** It names the one option id a driver running `--non-interactive` may take without asking. It exists so an unattended run has a defined answer rather than an inferred one, and it must be an id from your own `options:` list.

Only the two advancement kinds may carry a default: `phase-boundary` (normally `proceed`) and `override` (your recommended set). Those are decisions where one answer is the recommendation and the others are the user electing to intervene — taking the recommendation unattended is what the user asked for by passing the flag.

The other four kinds must NOT carry one, and a driver must abort rather than invent one, because on each of them every available answer is wrong in a way the user would want to know about:

- `ambiguity` — the protocol already forbids resolving one by taking the cheapest reading. A flag must not become the exception that makes it allowed.
- `overwrite` — one answer destroys work that may have been hand-edited; the other ships a prototype that diverges from its spec. There is no safe default, only a choice about which loss is acceptable.
- `failure` — the safe-looking answer proceeds past a suite that did not pass, which is the one outcome a CI run exists to prevent.
- `impasse` — the pipeline cannot express what the spec asks for, and the offered way forward hands the work to a person permanently. Accepting that is a scope reduction nobody can consent to on the user's behalf.

So: when you raise one of those four, omit `default:`. Adding one does not make the run smoother; it makes an unattended run take an action nobody authorized.

**`impasse` vs `ambiguity`.** An ambiguity has two readings and you cannot pick between them; an impasse has none — the pipeline has no way to express what the spec asks for, whichever reading you take. They are separate kinds because their resolutions differ in kind: an ambiguity is settled by the user choosing a reading, an impasse by the user agreeing that this part of the system will be written by hand, declared as a unit, and never generated. Filing an impasse as an ambiguity offers the user a choice between readings that all fail.

Leave the filesystem coherent before you stop — a decision is a pause, not a half-write. If you genuinely cannot pause at that point, take the option that preserves the user's work, never the one that destroys it, and say so in your report.

Two things not to do: never narrow the options to spare the user a question, and never resolve an ambiguity by taking the reading that is cheapest to implement. Both turn a decision the user should own into one you made quietly.

## Recording what happened (feedback mode)

When feedback mode is on, this project records what actually happened during a run so the toolkit can be improved from evidence rather than recollection. It is **off by default**; when it is off every command below is a silent no-op, so call them unconditionally and never branch on whether it is enabled.

**The log is written to be sent.** A user turns this on, reproduces a problem, and forwards the file to whoever maintains the toolkit. So nothing you pass can be free text: every flag below takes a value from a closed vocabulary, and anything else is replaced with `redacted` before it reaches the file. Do not try to describe a situation in words — pick the closest vocabulary value and, if none fits, use `other`. How often `other` shows up is itself the signal that a vocabulary needs a new member.

The CLI already records its own half: every command's outcome and duration, and every diagnostic any validator produced. **Do not re-report those.** Record only what the CLI cannot see — what you did and why:

```
parlay internal feedback-record --kind <kind> --skill <this-skill> [--phase P] [--artifact A] [...]
```

| Kind | Record when | Flags |
|---|---|---|
| `phase` | You enter or leave a pipeline phase | `--phase intents\|dialogs\|artifacts\|build\|code` |
| `decision` | You raised a `parlay-decision` block, and again when it resolves. The CLI never sees these | `--decision <kind>` and, on resolution, `--option <id>` |
| `retry` | **The important one.** You authored something, had it refused, and tried a different shape | `--code <the error code>` and `--changed added-field\|removed-field\|changed-shape\|changed-version\|changed-artifact\|reordered\|other` |
| `improvised` | You proceeded without a rule you needed — invented a path, guessed a convention, weakened an assertion | `--needed schema-rule\|path-convention\|naming-convention\|adapter-capability\|example\|decision\|other` |
| `note` | Anything else worth a future reader knowing. Use sparingly | — |

`--subject` optionally names the feature, unit or operation concerned. Pass it in **plaintext**; the CLI hashes it on receipt with a per-project salt. Never hash it yourself.

**`retry` and `improvised` are the two the log exists for.** A validator that teaches by rejection looks exactly like one that teaches by documentation unless the retries are counted, and an agent that guessed a convention leaves no other trace at all — the run passes, and the guess surfaces later as an inconsistency nobody can date. Recording them is not an admission of failure; it is the only way the gap that forced them gets closed.

**Correlation is automatic — do not manage it.** Events are tied together by `PARLAY_RUN_ID`, which the loop driver sets once per pipeline run and every CLI call inherits from the environment. The CLI hashes it before writing, so the value never appears in the log. You do not need to read it, pass it, or thread it through; `--run` exists only to override it and is almost never the right thing to reach for.

## Step 0 — Gate

This step is injected at deploy time and runs before every other step in this module. Gate the phase boundary before doing any work in it. For the feature this phase acts on, run:

```
parlay internal gate @{feature} --stage code
```

**If this run carries `--authorize-criteria=machine`, pass it here too.** Without it this gate refuses a feature whose criteria nobody has approved, which is right for a run that was not authorized and wrong for one that was — and the flag is how a run says which it is. Both the project setting and this argument are required; neither alone counts.

(When this phase operates on more than one feature — a project-level pass emits several — run the gate once per feature in scope.) The gate is a **recomputation** over what is on disk: it aggregates the boundary's checkers into one verdict, so re-running it after a fix re-derives the answer with no stale state to clear. It writes nothing **except** when an authorized machine run passes the code boundary, where it records the waiver — that record is the only trace that generation proceeded against a standard nobody approved, so it is written by the run that actually advanced rather than inferred later.

**If any invocation exits non-zero, stop.** Do not proceed to the steps below, and do not quietly fix-and-retry: each entry in the gate's `blockers[]` names its own `fix`, and resolving a blocker is the driver's call, not this phase's. Surface the blockers as a `failure` decision request (see **Asking the user**) with them in `context:`, and let the driver decide. A passing gate (exit zero) is the only condition under which the rest of this module runs.

## Steps

1. **Load schema digests** — Read these before generating:
   - `.parlay/schemas/digests/buildfile.digest.md`
   - `.parlay/schemas/digests/adapter.digest.md`
   - `.parlay/schemas/digests/blueprint.digest.md`

   Each digest is derived from its schema at deploy time and carries the
   authoring-normative content — field tables, closed vocabularies, required
   shapes, invariants — without the schema's rationale and history. It is
   what you need to READ a buildfile and an adapter correctly.

   **Open the full `.parlay/schemas/<name>.schema.md` when** a validator
   finding routes you there (`.parlay/schemas/DIGEST.md` maps codes to
   schemas), when the digest does not answer a question you actually have, or
   when you are changing the schema itself. Do not open one out of caution:
   the digest is derived, so anything it states is what the schema states.

2. **Load project config** — Read `.parlay/config.yaml` for project settings. Resolve the adapter from `.parlay/adapter-set.yaml` — the presentation slot for UI work, the kind-appropriate slot otherwise. (`prototype-framework:` was removed in v0.3 — nothing reads it; a project without an adapter-set converts via `parlay migrate-config`.)

3. **Load framework adapter** — Read `.parlay/adapters/{framework-slug}.adapter.yaml` for framework-specific vocabulary, file conventions, and patterns.

4. **Scope the read-set, then load only what it names** — Run `parlay internal diff` (no @feature) FIRST, before opening any buildfile. Its JSON (documented at step 8) names exactly which features this run has work in:

   - a feature with entries in `components.dirty[]` or `components.removed[]`,
   - a feature with `first_build: true`,
   - plus any feature named in a `sections` entry that reads `"changed"`/`"new"`, when you need its rows to regenerate the project-scoped file.

   **Read `.parlay/build/{feature}/buildfile.yaml` for those features only.** A feature whose components are all `stable` is not read: its entries are preserved verbatim by construction — the same preserve-stable rule this skill already applies when writing — so loading it buys nothing and costs its whole file. On a mature project that is the difference between reading one buildfile and reading every buildfile, and it is why a one-feature change used to cost the same as a whole-project one.

   If the diff reports no features at all, stop and tell the user to run `/parlay-build-feature @{feature}` for at least one feature.

   **Require `plan:` on every buildfile — from the diff, not by reading them.** The project diff's `missing_plan[]` names every built feature whose buildfile declares no `plan:` section. If it is non-empty, STOP and tell the user to regenerate those features via `/parlay-build-feature @{feature}` — do NOT fall back to deriving paths on the fly. The plan is the executable contract for which files a feature touches; missing it means the integration intent was never captured. (Answering this from the diff is what lets the gate cover every feature while you read only a few.)

   **Widening is allowed, and it is an escalation — never a silent fallback.** If generation surfaces a cross-feature contradiction the scoped set cannot explain — a component referencing an entry you have not loaded, a composition finding naming a feature you skipped — open *that named feature's* buildfile and say in the step-11 report that you widened and why. What you must not do is respond to uncertainty by loading everything again: that reintroduces the cost this step exists to remove, and hides the contradiction instead of naming it.

   **Fallback.** If the project has no baseline yet (first generation) or the diff errors, read every buildfile as before — with no recorded state there is no scope to trust, and correctness outranks the saving.

5. **Compute the merged model and routes** — Across the project:
   - The model layer comes from the project's `domain-model.yaml` — the one canonical entity set every feature shares. (Buildfile `models:` sections were removed in v0.3; a buildfile still carrying one fails validation before codegen starts.)
   - The route dispatch table comes from `parlay internal merged-routes`, not from reading every buildfile's `routes:` section. It emits every route with its owning `feature`, the blueprint's `shell`/`guard` join already applied, `strategy` and `default_route`, and `conflicts[]` for any path two features both claim. Resolve a conflict before emitting the entry point — two unordered writers on one path is a composition question (`parlay internal check-composition`), not a tie for this skill to break.
   - These merged artifacts drive the cross-cutting files (model definitions, entry point).
   - **External type resolution** (brownfield): for each entity in the merged model set, grep the source tree (under `file-conventions.source-root`) for existing type/interface/struct definitions matching the entity name (e.g., `interface User`, `type User struct`, `export type User`).
     - If exactly **one match** is found: record it as an external type (entity name → import path). In step 14, generate an import statement for this entity instead of a type declaration.
     - If **multiple matches** are found: raise an `ambiguity` decision request naming each candidate:
       ```
       Found multiple existing definitions for "User":
       A: src/types/user.ts (line 14) — interface User { id: string; name: string; }
       B: src/models/auth.ts (line 42) — interface User { id: number; email: string; }
       C: Generate a new type (ignore existing definitions)
       ```
     - If **no match** is found: proceed as before (generate the type declaration).
     - Store the external type map (`{ entityName: importPath }`) for use in step 14.

6. **Load and merge blueprint** — Read `.parlay/blueprint.yaml` if it exists. The blueprint provides app-level structural decisions that complement the per-feature buildfiles:
   - The route-level join (`shell`, `guard`) and `navigation.strategy`/`default-route` already arrived with step 5's merged table — that lookup is a deterministic join on `path` and is done for you. Routes the blueprint does not list come back unjoined: they get the default shell (first shell in `shells:`) and no guard. Read the blueprint here for what the join does NOT cover, below.
   - `lazy` loading per route, where the blueprint declares it.
   - Record `authorization.guards` — each guard becomes a wrapper component.
   - Record `errors.boundaries` — each boundary scope becomes an error boundary component.
   - Record `state.global` — each global state slice becomes a context provider.
   - Record `data` settings — the fetching strategy and caching config drive the data infrastructure setup.
   - If the blueprint doesn't exist, proceed without it — the agent uses its own judgment for these decisions (as it did before the blueprint existed). This is the backwards-compatible path.

7. **Determine source root(s)** —
   - **Single-target project** (no `.parlay/adapter-set.yaml`, or one with only the presentation slot): from the adapter's `file-conventions.source-root`. All features share one source root since they compile into one project.
   - **Multi-target project** (adapter-set with a non-presentation slot): there is **one source root per target**, taken from `adapter-set.yaml`'s `targets.<kind>.root` — NOT the adapter's own `source-root`, which the target root overrides. Codegen loops the filled slots and emits each target's files under its own root using the kind-appropriate adapter: `presentation` (e.g. React under `apps/web`), `application` (e.g. NestJS controllers/services/modules under `apps/api`), `persistence` (e.g. the Prisma schema under `apps/api`). The buildfile's `plan.targets.<kind>.creates` is authoritative for which files land where — emit exactly those paths, per target, and never recompute them from the adapter's `source-root`. Run `parlay internal scaffold-plan @{feature}` to see the derived per-target plan.

8. **The project-level diff** — already run at step 4, where it scoped the read-set; this is the reference for what its JSON carries. Do not re-run it: nothing between step 4 and here writes to the project, so a second call would return the same answer at the cost of another round trip.
   - `features.<name>.components.stable/dirty/removed` — per-feature component status based on source changes. On `first_build: true` for a feature, treat all its components as new.
   - `sections` — `models`, `routes`, `fixtures` compared across ALL features' merged buildfile sections. Values: `"changed"`, `"stable"`, `"new"`. Used to determine which project-scoped cross-cutting files need regeneration. Note the scope difference: these hashes are computed in Go across every feature's buildfile, so `sections` stays a whole-project answer even though you read only a few buildfiles — that is exactly why the gate can be trusted while the read-set is narrow.
   - `missing_plan[]` — built features whose buildfile declares no `plan:` section (step 4's hard stop).

9. **Scan generated files** — Run: `parlay internal scan-generated {source-root}` to map each file to its owner.
   - Files with `parlay-feature: X + parlay-component: Y` belong to feature X's component Y.
   - Files with `parlay-scope: project + parlay-section: Z` are project-scoped cross-cutting files.
   - Files with `parlay-artifact: test` are test files for their parent component.
   - Files without ANY parlay marker are user-owned; never modify or delete them.

10. **Verify generated files haven't been hand-edited** — Run: `parlay internal verify-generated` (no @feature, project-level) to compare each recorded generated file against its stored content hash. Returns JSON `{has_hashes, stable, modified, missing}`.
   - If `has_hashes` is `false`, this is the very first generation — treat everything as new and skip the modified-file check.
   - Otherwise, check **every** component that has a generated file — dirty and stable alike — against `verify.modified[]`. If a file is listed there, the user has hand-edited it — STOP and surface the situation:
     ```
     <file> has been edited since the last generation.
     A: Overwrite (lose my edits)
     B: Skip this file (keep my edits, possibly diverging from the buildfile)
     C: Show me the diff first
     ```
     **Do not scope this check to stable components.** A hand-edit is most likely to be lost
     precisely when the upstream spec also changed — that is, when the component is *dirty* and
     codegen is about to rewrite the file anyway. Checking only stable components inverts the
     safety property: it fires when nothing was going to be overwritten, and stands down in the
     one case where an overwrite is imminent. A dirty component whose file is `modified` is the
     highest-risk case in this skill, not an exempt one.
   - If a generated file is in `verify.missing[]`, the user deleted it — ask whether to regenerate or to drop the component.

11. **Tell the user what's about to happen** — Before regenerating, summarize: "Regenerating N component files: ... . Keeping M stable files. Deleting K removed component files."

    Also print the merged plan derived from the `plan:` sections of the buildfiles you loaded: list every path the run will create, modify, or delete, with the producing component or cross-cutting id. The plan is the contract; the user sees it before any file write. Features you did not load contribute no rows because this run writes none of their files — say how many features were in scope, so "three paths" reads as a scoped run rather than a suspiciously short whole-project plan. If you widened the read-set at step 4, name that here too.

11.4. **Load the hand-authored denylist** — Read `.parlay/build/_project/authored-files.yaml`. It lists every file a hand-authored unit declares: code a person wrote, which this skill must never write, modify, delete or merge into. An absent file means the project declares no units; that is a normal state, not an error.

   **Read it from `.parlay/build/`, never from `spec/intents/`.** The declarations live at `spec/intents/<unit>/authored.yaml`, and `spec/intents/**` is off-limits here — that isolation is the load-bearing test for whether the buildfile is doing its job, and a filename carve-out would make it negotiable. `parlay internal save-build-state` projects the resolved file list into the build tree for exactly this reason, the same way capabilities are compiled into the buildfile rather than read from the spec.

   Build an in-memory **denylist** from every `sources:` and `tests:` path in that file. The denylist is checked **before every write, in every step below**, and it **outranks the plan allowlist and both exempt classes**. A path in the denylist is refused even when a plan row names it, even when it carries a `parlay-section:` marker, and even when it is a test file.

   That ranking is the whole point of loading this before the allowlist rather than auditing afterwards. The two exempt classes in 11.5 are marker-bearing categories with no path bound — a test file "at the location the framework expects", a section file "where file-conventions dictate". Those are precisely the writes that would land inside a unit: a unit's own test directory is where the framework expects tests to go. The post-hoc write-set audit exempts both categories by design, so a write that leaked through it would be reported as authorized. Refusing at the write is the only placement that catches it.

   On a refusal: do not write the file, do not silently skip it either. Stop and report `unit-write-refused` naming the path and the owning unit, exactly as you would report anything else the buildfile failed to authorize. A unit's code changing is a person's decision, and a codegen run that wanted to change it has found a real disagreement between the buildfile and the declaration — surface it rather than resolving it.

11.5. **Lock the plan as the file-write allowlist** — Build an in-memory set of permitted paths from `plan.creates ∪ plan.modifies ∪ plan.deletes` across all loaded buildfiles. **For a multi-target (adapter-set) buildfile the plan rows are nested under `plan.targets.<kind>.creates` / `.modifies` / `.deletes` — union every filled target's rows into the same allowlist.** Every subsequent write or delete in steps 12–14.7 MUST resolve to a path in this set, **plus the two exempt classes below**. Refuse any write to a path that is neither in the plan nor exempt — the buildfile didn't authorize it. Refuse any skip of a `plan` path that the diff doesn't classify as stable. Violations are bugs — STOP and surface the offending entry.

   **Exempt classes (authorized by this skill, not by the buildfile).** `build-feature` emits `plan:` rows for component implementation files and cross-cutting targets only. It emits none for the two categories this skill separately *requires* you to write, so a literal reading of the allowlist forbids exactly the files steps 14 and 15 mandate. Both are therefore exempt:

   1. **Section-derived project files** — the models/types, routes/entry-point, shell, guard, error-boundary and state-provider files generated in step 14 from `blueprint` + merged `sections`. Each is identified by its `parlay-section:` marker, not by a plan row.
   2. **Test files** — the per-component spec files generated in step 15, identified by `parlay-artifact: test`.

   Exempt does not mean unbounded: a write still has to be one of these two marker-bearing categories, at the path the adapter's `file-conventions` dictate. Anything else remains a violation. **Neither exemption reaches into the hand-authored denylist from step 11.4** — that check runs first and wins. Record every exempt write in the run summary so the set stays auditable — if a file is being written repeatedly under an exemption, that is a signal `build-feature` should be emitting a plan row for it instead.

   **Cross-feature dependency graph (project-pass mode).** Run `parlay internal emission-groups`. It reads every feature's build state in Go — not just the buildfiles you loaded — and returns the dependency graph already scheduled: `waves` (features grouped so every wave's creates precede the next wave's modifies), `shared_paths`, and `cycles`. On a non-empty `cycles`, surface `plan-create-modify-cycle` and stop without writing any files.

   **Do not build this graph by hand from the buildfiles you loaded.** An edge runs from a modifying feature to the feature that creates the file, and with a scoped read-set the creating feature is frequently one you did not load — so a hand-built graph is missing exactly the edges that matter, and it is missing them silently. The command exists to answer this project-wide; this step is a call, not a construction.

11.5.5. **Order features topologically for emission** — take the order from `emission-groups`' `waves` (step 11.5). Creates run before the modifies they satisfy; the sort is stable, with feature-slug alphabetization breaking ties, so re-running the same project pass produces byte-identical emission order. The runtime invariant: every path appearing in BOTH another feature's `plan.creates` AND this feature's `plan.modifies` is emitted by the producing feature first.

   You emit only the features step 4 scoped, but you order them within the project-wide schedule — a scoped run may sit in a later wave than a feature it never touches, and the modifies/creates rule still holds because the producing file is already on disk from the run that created it.

   In single-feature invocations the topological order is trivial — there is one node and no edges; the order is the identity. In project-pass mode with N features and a DAG of dependencies, emission walks the features in topological order; the strict-target rule (step 14.7) still applies AT EMISSION TIME for each feature — by the time a modifying feature runs, the topological order guarantees the file is on disk.

11.6. **Buildfile freshness gate** (per feature, before emission) — For every feature whose buildfile is loaded, run the freshness gate **before any emission begins for that feature**:

   1. Read the buildfile's `source-signatures:` section (see buildfile.schema.md).
   2. Recompute content signatures for every source artifact the buildfile consumed: `intents`, `dialogs`, `surface`, `domain`, `layout` (where applicable), and `adapter-version`. Signatures are **content hashes**, not timestamps — filesystem mtime never enters the comparison.
   3. Compare each recorded signature against the freshly-computed signature.
   4. **On match**: proceed with that feature's emission.
   5. **On mismatch (or absent `source-signatures:` section)**: refuse to run for that feature, surface the error verbatim:
      ```
      stale-buildfile at <feature>: buildfile reflects <prior-signature>; current sources are <current-signature>. To fix: run `parlay build-feature <feature>` to refresh the buildfile, then re-run codegen.
      ```
      and exit with a non-zero status. Continue to run the gate for other features in the same project — the gate is **per-feature**, not per-project. A stale buildfile in feature A does not block feature B from generating, but the overall process exit code remains non-zero whenever any feature fails the gate.

   The check is mechanical: signature comparison only — no AI invocation, no prompts. Codegen does NOT auto-run `parlay build-feature` and does NOT auto-rewrite the layout on stale-buildfile; it points the author at the offending feature and the human (or `parlay build-feature`) makes the change. Missing bindings inside an otherwise-fresh buildfile (i.e., a layout node that has no binding entry) are also classified as `stale-buildfile` — codegen does not surface a separate "missing-binding" error class. `stale-buildfile` is the only codegen-owned content-error category.

   `stale-buildfile` is suppressed for any page that ALSO fails the layout-validation precheck (step 11.7) — the precheck refusal wins because the layout itself is invalid.

11.7. **Layout-validation precheck surfacer** (per page, after freshness gate, before emission) — For every layout-bearing page in a feature whose freshness gate passed, consult the layout-validation precheck owned by the layout-creation feature:

   1. Read the precheck verdict for the page from its layout artifact. The precheck implementation itself is the layout-creation feature's concern; this step only covers codegen's role of consulting the verdict.
   2. **On precheck pass**: proceed to per-page emission.
   3. **On precheck refusal** (unknown component type, vocabulary version mismatch, malformed block, etc.):
      - Surface the precheck's refusal **verbatim**. Do NOT augment the message with codegen-internal vocabulary. Do NOT re-classify the failure as `stale-buildfile`. Do NOT silently fall back to the layout-free emission path — silent fallback would mask a real authoring error and is forbidden.
      - Refuse codegen for THAT PAGE only. Other pages in the same project (including layout-free pages and other layout-bearing pages whose prechecks pass) continue to generate normally.
      - Exit code remains non-zero for the run because at least one page failed.

   The surfaced message points the author back at the layout-creation flow, not at codegen internals. When a page fails BOTH this precheck and the freshness gate, the precheck refusal wins and `stale-buildfile` is suppressed for that page.

11.8. **Detect and consume layout block** (per page) — After the freshness gate (11.6) and the layout-validation precheck (11.7) have passed, dispatch each page to one of two emission paths based on whether its page artifact carries a `layout:` block. Activation is **per-page**, not per-project — pages with and without layout coexist in the same run, and the same project may mix layout-aware and layout-free pages.

   For each page to be emitted:

   1. **Detect the layout block.** Inspect the page artifact (the `*.page.md`/`*.page.yaml` resolved through the buildfile) for a `layout:` block.

   2. **Layout-aware emission path** (when a layout block is present): the typed layout tree is the **structural source of truth** for that page's emission. The skill walks the typed tree and produces **one component instance per node, in declaration order**, with the structural shape of the layout tree preserved one-for-one in the emitted component tree (parent/child relationships and sibling order match the layout exactly). For each layout node:
      - **Look up the binding entry in the buildfile** for that node.
      - **Emit framework-specific wiring code** for the node:
        - Pass entity data into the component (the `data.inputs:` resolved by the binding).
        - Attach operation calls to action handlers (the `actions:` resolved by the binding).
        - Apply presentation hints from the binding to the component's render properties (e.g. `presentation: badge` on a status column → render the column as a Clarity badge with the bound entity field flowing through unchanged).
      - **Pass tokens through as adapter-defined token references**, never as raw pixel values. A binding referencing `spacing-lg` emits a token reference recognized by the adapter (e.g. a CSS variable or a framework token import), not the underlying numeric value.
      - **Emit a traceability annotation** carrying the `(layout-node, surface-fragment, domain-element)` source triple recorded on the binding. The triple appears as a comment or framework-idiomatic annotation alongside the wired component, so traceability survives into the framework output.
      - **Missing binding handling.** If the buildfile lacks a binding for a layout node codegen reaches, codegen treats it as a **freshness-gate failure** — it surfaces `stale-buildfile` and points the author at `parlay build-feature`. There is no separate "missing-binding" error class in codegen.

   3. **Layout-free emission path** (when no layout block is present): the page falls through to the existing surface-and-domain emission path described in step 12 onward, **unchanged**. Output for layout-free pages is behaviorally equivalent to its pre-feature output given the same adapter version and source state. A project with zero layouts at all generates output indistinguishable in behavior from a pre-feature run.

   **Codegen MUST NOT invoke the rules engine, the AI matcher, or any disambiguation prompt during emission.** Those calls live in the build phase and have already produced the bindings before codegen runs. The codegen execution trace must show zero invocations of any of those subsystems. If a binding is missing, the response is `stale-buildfile`, never an inline disambiguation prompt.

   **Re-running codegen on identical inputs produces behaviorally-equivalent output.** Same `(layout, buildfile, adapter)` input produces output that passes the same testcases and emits the same component tree. Lexical details (whitespace, identifier casing in non-load-bearing places, comment ordering) may differ — the AI emitter is non-deterministic on text — but behavior, structure, and bindings are stable.

11.9. **Run pre-emit toolchain tools** — Adapters may declare external skills and MCP servers in their `toolchain:` block (adapter.schema.md Section 10). Run `parlay internal toolchain-plan @{feature} --phase code --stage pre-emit` to get the entries as JSON — never parse the adapter YAML yourself. For a multi-target project, run each entry whose `target` matches a layer **immediately before that layer emits** (persistence's pre-emit tools before persistence, etc., per the layered emission order below); single-target entries (`target: ""`) run once before step 12. For each entry, in the order returned:

   1. **Resolve availability.** Skill: is its `invoke` slash command available to you? MCP: are ALL the `mcp__{server}__{tool}` names in `tools[]` present in your tool namespace? (`tools[]` is a closed allowlist — never call a tool on that server that is not listed.)
   2. **Absent + `required: false`** → apply the `fallback` verbatim (e.g. emit from adapter templates), note it in the run summary, continue. The build MUST succeed with the tool uninstalled — this is the graceful-absence contract.
   3. **Absent + `required: true`** → STOP. Return a `failure` decision request with code `toolchain-required-tool-absent` naming the entry; write nothing (same CI-safe refusal shape as `stale-buildfile`).
   4. **Present** → invoke it (skill via its `invoke` command; MCP calling only the allowlisted `mcp__{server}__{tool}`). Never pass any tool a path under `spec/intents/**` — the codegen boundary holds for tools too (their `read_set` was already rejected at registration if it crossed it).
      - **`authority: advisory`** (empty `write_set` by contract): treat the output as guidance for the code you are about to write. The tool must not have written files; if it did, that is a contract violation — STOP.
      - **`authority: mutating`**: the tool may write within `write_set`. After it runs, apply the **owns-markers bookkeeping**: if `owns_markers: parlay`, the written file stays in parlay's marker/hash chain — give it the normal marker and record it in `.parlay/build/_project/.emitted` like any generated file. If `owns_markers: tool`, the file LEAVES parlay's chain: do NOT append it to `.emitted`, do NOT add a parlay marker, and exclude it from `save-build-state`/`scan-generated` reconciliation; list it under "tool-owned files" in the run summary. `parlay internal check-write-set` admits writes within an active mutating tool's `write_set`, so a `owns_markers: parlay` write inside `write_set` is not flagged `codegen-wrote-outside-plan`.

12. **Generate code per dirty/new component** — For each component the diff classifies as dirty or new:
    - **Read the component's `decisions:` entries first.** Before regenerating a component, read every `decisions:` entry (see buildfile.schema.md) whose `component:` names it. Each records an implementation judgment a prior emission made and why. Honor them: re-derive the same choice unless an entry's `obsolete-when:` condition now holds, in which case append a superseding entry (below) rather than silently reverting. This is the mechanism that stops a bug fixed during codegen from returning on the next regeneration — the reason is on disk in the buildfile, in codegen's read allowlist, not in an expiring transcript. The block is not regenerated by build-feature; it accumulates across runs and you are its only author.
    - **Append a `decisions:` entry for every judgment call you make.** When emitting this component forces a non-obvious choice that no test encodes and the next regeneration would otherwise re-derive or undo — a workaround for a framework quirk, a deliberate deviation from the obvious translation, a fix you discovered while generating — record it: `{id, component, decided, why, enforced-by: [the files you wrote it into], obsolete-when, supersedes}`. Write the `id` verbatim into each file listed in `enforced-by:` (a comment naming it is enough) so `parlay internal check-buildfile` can confirm the reason reached the code; a file that carries the decision but not its id is reported as `rationale-stranded`. Do not record ordinary, obvious translations — the block is for the choices a later reader would otherwise delete as unexplained, and inflating it with the routine is exactly the comment spam the scoping avoids.
    - Look up the component's file path in `plan` (the entry whose `sources` references this component). The plan is authoritative — do NOT recompute the path from the adapter's `component-pattern` + `naming` rules at this step. The adapter's conventions were already applied when build-feature emitted the plan.
    - Translate the component's abstract `type`, `elements`, `actions`, and `file-operations` into framework-specific code using the adapter's widget mappings
    - Honor the adapter's `patterns:` section (interaction style, information density, error placement, confirmation style, content rules)
    - Add the marker at the top of every generated file. Use the comment style appropriate for the file type (`//` for Go/TS/JS, `#` for YAML/Python/shell).
    - **Component implementation files** get a two-line marker:
      ```
      // parlay-feature: {feature}
      // parlay-component: {component-name}
      ```
    - **Component test files** get a three-line marker:
      ```
      // parlay-feature: {feature}
      // parlay-component: {component-name}
      // parlay-artifact: test
      ```
      Test files ride the same component's dirty/stable status. When a component is dirty, regenerate BOTH its implementation and its test file.
    - **Multi-component (extended) files** result from intelligent merge (see step 14.5 Tier 2 — when one component's behavior is layered into a file already owned by another component). They carry the primary owner's two-line marker plus one `parlay-extends:` line per additional component:
      ```
      // parlay-feature: {primary-feature}
      // parlay-component: {primary-component}
      // parlay-extends: {extending-feature}/{extending-component}
      ```
      Multiple `parlay-extends:` lines may appear if more than one feature has extended the file. The primary owner is whichever component first claimed the file (or, in brownfield, the component that semantically matches the original user-authored implementation). Optional per-function markers may appear above each extending function for human-readability:
      ```
      // parlay-feature: {extending-feature}
      // parlay-component: {extending-component}
      <function or class declaration for the extending behavior>
      ```
      These per-function markers are documentation only; the file-level marker block is what scan-generated reads.
    - **Record each file as you write it:** append its path to `.parlay/build/_project/.emitted`, one path per line, immediately after writing it — implementation files, test files and merged files alike. Not at the end of the run. Step 17 explains what the manifest is for and what goes wrong without it.

13. **Delete removed-component files** — For each component in `components.removed[]`, look up the file path from the scan-generated output and delete the file. Only delete files that have a `parlay-component:` or `parlay-section:` marker — never touch user-owned files.
    - Deletions do **not** go into `.parlay/build/_project/.emitted`. The manifest declares what this run wrote; `save-build-state` drops the record for a file that no longer exists.

14. **Regenerate cross-cutting files (section-derived)** — Consult `diff.sections` to determine which cross-cutting files need regeneration:
    - If `sections.models` is `"changed"` or `"new"`: regenerate the models/types file from `buildfile.models`. For each entity in the merged model set, check the external type map (from step 5): if the entity is external, emit an import statement pointing to the existing file instead of a type declaration; if the entity is not external, generate the type declaration as before. The resulting models file may contain a mix of imports and declarations. Mark it with `parlay-section: models`.
    - If `sections.models` is `"changed"` or `"new"` **and the adapter declares `file-conventions.paths.seed`**: regenerate the composed runtime seed. Run `parlay internal scaffold-seed` and write its records to the declared path, in whatever shape the framework wants — a module exporting a const, an embedded JSON file, a fixtures package. Mark it with `parlay-section: seed`.
      - The command emits canonical JSON and **refuses** rather than guessing: a non-zero exit with `composition-fixture-contradiction` means two features' composing fixtures disagree about the same record, and that is a question for the designer, not something to reconcile here. Stop and report it. A `composition-scenario-fixture-divergence` note is different — at least one side is a fixture that never reaches the composed seed, so the two states never coexist and the derivation proceeds. Do not edit a scenario fixture to silence it.
      - Write what the command gives you. Do not add records, drop records, or reorder them — the seed is compared against the derivation on later runs, and a hand-adjusted seed reads as drift forever after.
      - This file is one per project, not one per feature. Regenerating it from a second feature's run is expected and correct; it is derived from the same entity set every time.
      - If the adapter declares no `paths.seed`, skip this — most frameworks have no single boot-time dataset, and its absence is not a gap.
    - If `sections.models` is `"changed"` or `"new"` **and the adapter declares `file-conventions.paths.store`**: regenerate the shared runtime store at the declared path, holding the domain-model entity set. Mark it with `parlay-section: store`. The store is what carries domain state between two user actions, so a write on one feature's screen is visible on another's — without it a cross-feature journey silently does not work, however correct each feature is on its own.
      - **The migration invariant when a project adopts a store: preserve the feature-level accessor surface; change only what backs it.** Per-suite test code addresses the feature's own API, so keeping that API and swapping its backing keeps every existing suite green. Move the entity state to the store, leave genuine view state (sort order, which row is expanded, wizard step) feature-local, and keep the feature's hydrate entry point's signature — it becomes a delegating write-through.
      - Boot the store from the composed seed, not from a per-feature fixture. That is what makes the prototype tell one story from any entry point.
      - If the adapter declares no `paths.store`, skip this. Most frameworks have no shared runtime between user actions and its absence is not a gap; `parlay internal check-composition` will note any cross-feature assertion that consequently cannot hold.
    - If `sections.routes` is `"changed"` or `"new"`: regenerate the entry point from **`parlay internal merged-routes`** (step 5's table), never from the `routes:` of the buildfiles you happen to have loaded. The entry point is a whole-project file: writing it from a scoped read-set would silently delete every route belonging to a feature this run did not touch. The merged table is computed across all buildfiles in Go precisely so this file can be regenerated correctly from a narrow read. Mark it with `parlay-section: routes`.
    - If `sections.blueprint` is `"changed"` or `"new"`: regenerate the cross-cutting blueprint-derived files:
      - **Shell components**: One layout component per shell in `blueprint.shells`. Mark each with `parlay-section: shell-{name}`.
      - **Guard components**: One route guard per guard in `blueprint.authorization.guards`. Mark each with `parlay-section: guard-{name}`.
      - **Error boundaries**: Error boundary components per scope in `blueprint.errors.boundaries`. Mark with `parlay-section: errors`.
      - **State providers**: Context providers per global state slice in `blueprint.state.global`. Mark with `parlay-section: state`.
      - **Route wiring**: The entry point / router file must reflect `navigation.strategy`, `navigation.default-route`, `navigation.not-found`, and the shell→route→guard assignments. This file is also marked `parlay-section: routes`, so it is regenerated whenever routes OR blueprint changes.

        **Emit the route table in match-precedence order, and verify `default-route` actually resolves.** Most routers match top-down and take the first hit, so a redirect for the empty path placed *after* the shells that also match the empty path is unreachable — the shell matches first, finds no child for the empty remainder, and renders an empty outlet. The symptom is a blank page at the app root with no console error, and `navigation.default-route` silently doing nothing.

        Order the emitted table so that, for each path, the most specific match precedes the more general one — in particular the `default-route` redirect must precede any shell or layout route that also matches the empty path. After emitting, confirm the root path resolves to `default-route` rather than to an empty shell; if the target framework offers a full-match qualifier (Angular's `pathMatch: 'full'`, React Router's `index`), use it rather than relying on ordering alone.
    - If a section is `"stable"`: leave the corresponding file untouched (look it up via scan-generated by its `parlay-section:` marker).
    - If a section is `"removed"`: delete the corresponding file.
    - Cross-cutting files use a two-line marker:
      ```
      // parlay-scope: project
      // parlay-section: models
      ```
    - **Record each file as you write it:** append its path to `.parlay/build/_project/.emitted`, one path per line, immediately after writing it. The models file, the seed, the store, the routes file and every blueprint-derived file each get a line. Step 17 explains what the manifest is for and what goes wrong without it.

14.5. **Mount into existing files (brownfield)** — This step runs when the project has existing source files that are not Parlay-generated (i.e., files without `parlay-component:` or `parlay-section:` markers). It has two tiers:

   - **Tier 1 — Templated mount**: structural insertion via adapter `mount-strategies:` (the fast path — adding a tab, a menu item, a route registration, a prompt step into known scaffolds).
   - **Tier 2 — Intelligent merge** (fallback): when the existing file IS the surface the route targets but no mount-strategy template fits the change, the agent reads the file and produces a merge diff that layers new behavior in alongside existing code (the slow path — adding a flag to an existing command, extending a function with new branching, etc.).

   For each route in the merged route table that references a page:

   1. **Find the target file**: search the source tree for the file implementing the page component, using the page name from the buildfile route and the adapter's `file-conventions.naming` and `component-pattern`. If the file has a `parlay-section:` marker, it is Parlay-owned — skip (step 14 already handles it). If the file is not found, skip (new page — step 14 creates it). **If the file is in the step 11.4 denylist, refuse and report `unit-write-refused`** — do not skip silently.

      This step is the largest single risk to a unit, and the reason the denylist exists as a pre-write check rather than an audit. Its whole premise is "an existing source file that is not Parlay-generated", which is the exact description of every file in a hand-authored unit — a unit's sources carry no marker, by definition and on purpose. Tier 2's intelligent merge then reads such a file and rewrites it in place. Nothing else in this skill comes closer to editing code a person owns.

   2. **Read the file**: read the full content of the target file.

   3. **Tier 1 — Templated mount**: scan each strategy in the adapter's `mount-strategies:` (if any) for a `detection` pattern that appears in the file content.
      - **1 match**: proceed with this strategy → step 5.
      - **Multiple matches**: raise an `ambiguity` decision request:
        ```
        <file> has multiple integration points:
        A: New <strategy-1-name> (found <detection-1> on line N)
        B: New <strategy-2-name> (found <detection-2> on line M)
        C: Skip — I'll integrate manually
        ```
        → step 5.
      - **0 matches**: proceed to Tier 2 (step 4) before falling back to a decision request.

   4. **Tier 2 — Intelligent merge** (only if Tier 1 found 0 matches): determine whether the existing file is **the same surface** as the route's component before giving up.

      The file is the same surface if BOTH:
      - **Naming match**: the file name corresponds to the route path under the adapter's `file-conventions.naming` and `component-pattern` rules (e.g., with `naming: snake_case`, route `add-feature` maps to a file named `add_feature.*`).
      - **Purpose match**: the file declares a primary entity whose identifier matches the route. The adapter MAY declare an optional `purpose-marker:` regex per file pattern to identify these declarations rigorously; without one, naming-match alone is the fallback signal.

      If the file is the same surface, perform an intelligent merge:

      a. Read the existing file fully (already done in step 2).
      b. Read the component spec from the buildfile (data.inputs, elements, actions, file-operations, computed values).
      c. Identify which behaviors the existing file already implements and which are new in the component spec.
      d. Generate a merge that:
         - **Preserves all existing user-owned code paths verbatim** (do not rewrite working code).
         - **Adds new behaviors as additional functions, branches, or flag declarations** rather than inline edits to existing functions. New entry-point logic typically takes the form of an early-return guard at the top of the existing function (e.g., `if newFlag != "" { return runExtendedBehavior(...) }`) followed by the original function body unchanged.
         - **Wires new inputs via the adapter's idiomatic mechanism** (flags, props, arguments — whatever the framework uses for parameterization).
         - **Renders error/status output via framework-idiomatic mechanisms**, not literal element-by-element translation. Buildfile elements like `[ERR]`-prefixed messages may not translate verbatim if the framework already has its own error channel.
         - **Updates the file's marker block**: replace the original two-line marker (or add one if the file had only legacy comments) with the multi-component form documented in step 12: primary's `parlay-feature:` + `parlay-component:` lines, plus a `parlay-extends: {feature}/{component}` line for the new owner.
      e. Continue to step 6.

      If the file is NOT the same surface (naming or purpose mismatch), fall back to an `ambiguity` decision request:
      ```
      <file> exists in the source tree but doesn't match any mount strategy in the adapter, and its purpose differs from <Component>'s route.
      How should <Component> be added?
      A: Show me the file so I can describe the pattern
      B: Skip — I'll integrate manually
      C: Add as a new standalone route instead (generates a new file, route registration via mount-strategy)
      ```

   5. **Find existing instances** (Tier 1 only): search the file for existing instances of the chosen strategy's template pattern. These serve as style examples for indentation, prop naming, and code conventions.

   6. **Generate diff**:
      - **Tier 1**: use the strategy template with placeholders filled from the buildfile component, and existing instances as style guides; produce an insertion diff.
      - **Tier 2**: produce the merge diff from step 4d (additive changes only — new flag declarations, new functions, dispatch line, expanded marker block).

   7. **Present diff for review**: show the user a unified diff of the target file:
      ```
      Proposed change to <file>:

      <unified diff showing added/modified lines>

      A: Apply this change
      B: Skip — I'll integrate manually
      C: Edit the proposed change
      ```

   8. **Apply or skip**: on approval, write the modified file. On skip, continue to the next route. On edit, accept the user's modification and apply it.

   9. **Record each file as you write it**: append the path of every file you modify here to `.parlay/build/_project/.emitted`, one path per line, immediately after applying the diff. A mounted file counts as emitted — this run wrote its current bytes. A skipped file does not. Step 17 explains what the manifest is for and what goes wrong without it.

   Tier 1 diffs are typically small (1-3 files, a few lines each — adding tabs, panels, section, route entries, menu items). Tier 2 diffs are typically larger (a new function plus a dispatch line plus flag declarations) but still additive — the agent does not rewrite existing logic, it layers new logic alongside.

14.7. **Process cross-cutting entries** — process these AFTER component generation (step 12–14) and brownfield mount (step 14.5), but BEFORE tests (step 15). This ensures infrastructure changes are in place when tests exercise the components that depend on them.

   **Which entries.** Two sets, and the second is what keeps a scoped read-set honest:

   1. The `cross-cutting:` entries of the features you loaded at step 4 — this run changed them, so they are (re-)applied.
   2. **Entries belonging to features you did NOT load, whose targets this run regenerated wholesale.** Run `parlay internal cross-cutting-index --target <path>` once with every project-scoped file step 14 just rewrote (models, seed, store, the entry point, blueprint-derived files). It answers with the entries whose targets — explicit paths, or a `target-pattern` it resolves against those files' actual content — land in what you rewrote, carrying id, feature, targets and the buildfile path, but never the transform prose. For each hit, open **that entry's** buildfile, read its `Behavior:`/`transform:`, and re-apply it.

   Run it AFTER writing the files, so a `target-pattern` resolves against real content. An unreadable path (a file this run creates fresh) keeps every pattern entry in the answer — the safe direction, and a signal to check rather than a bug.

   Skipping set 2 is a silent data-loss bug, not an optimisation: feature A's middleware merged into the entry point, feature B's change regenerates that entry point, and A's infrastructure disappears from a run that reported success. The index exists so this costs a lookup instead of every buildfile — on a project where no unloaded feature targets a regenerated file, which is the common case, it costs nothing at all.

   For each entry in scope:

   1. **Resolve targets**: if the entry has `target-files:`, use the explicit paths. If it has `target-pattern:`, grep the source tree under `file-conventions.source-root` to find matching files. If zero files match, warn but don't error (the pattern may be ahead of the codebase). If the entry has both, resolve both and take the union.

   2. **For each resolved target file** — strict-target rule:
      - **If the entry has non-empty `Affects:` or `target-files:` naming files**: those exact paths MUST be the targets. The file MUST already exist on disk **OR appear in another feature's `plan.creates` set that has already run earlier in the same project pass** (per the topological order from step 11.5.5). If the file is neither on disk nor a satisfied sibling-create, error — the buildfile names a file that isn't there. Apply Tier 2 intelligent merge: read the file, read the entry's `Behavior:`/`transform:` description and `introduces:` list, produce a diff that adds new behavior while preserving existing code. If the file already has a `parlay-component:` marker, add a `parlay-extends:` line for the cross-cutting entry. If the file has no marker, add a `parlay-section: cross-cutting` marker.
      - **If a Tier 2 merge is too risky** (e.g. the file is large, the integration spans many sites, the agent isn't confident the diff preserves existing behavior): return the proposed diff as an `ambiguity` decision request (Apply / Skip / Edit) — **do NOT silently invent a new file path under the source root**. Writing a file at any path not named in the entry's `Affects:`/`target-files:` is a bug — STOP and surface it.
      - **If the entry declares `target-creates:`** (two-kinded shape): paths in `target-creates:` are introducing — generate new files at those exact paths with a `parlay-section: cross-cutting` marker, never invent alternate paths. Paths in `target-files:` are still strict-modifies — they must exist on disk (or be satisfied by a sibling-create earlier in the topological order).
      - **Only when the entry has NO `Affects:`/`target-files:`/`target-creates:`** (purely-introducing entries that genuinely add a new package via grep-pattern fan-out): create a new file with a `parlay-section: cross-cutting` marker and generate the introduced functions/types. Present the new file for review. The file path is computed from the adapter's conventions; the agent must NOT pick an arbitrary path.

   3. **Present diff for review**: same A/B/C menu as brownfield mount:
      ```
      Cross-cutting change: <entry-id> (source: <source-ref>)
      Target: <file-path>

      <unified diff>

      A: Apply this change
      B: Skip — I'll integrate manually
      C: Edit the proposed change
      ```

   4. **Apply or skip**: on approval, write the modified file. On skip, continue.

   5. **Record each file as you write it**: append the path of every file you write or modify here to `.parlay/build/_project/.emitted`, one path per line, immediately after applying the diff — both the strict-modify targets and any `target-creates:` files. Step 17 explains what the manifest is for and what goes wrong without it.

   Cross-cutting entries follow the same diff lifecycle as components. On subsequent runs, `parlay internal diff` classifies each entry as stable/dirty/removed. Stable entries are skipped; dirty entries are re-applied; removed entries have their claims revoked from the target files.

15. **Generate test code** — Read `.parlay/build/{feature}/testcases.yaml` **for the features this run regenerated** (step 4's scope) and translate each suite into framework-appropriate test code. Use the test framework specified in `testcases.yaml` `framework:` field. A stable feature's suites are already generated and already passing; re-reading them to emit the same file is the same wasted read as its buildfile. Step 16 still RUNS the whole suite — what is scoped here is generation, never execution.

    **The suite's `file:` is where its code goes.** `build-feature` set it from the plan row that `scaffold-plan` derived from the adapter's `file-conventions.paths.test` template, so the path is already decided, already in the plan allowlist, and already consistent with every other component's tests. Write there.

    Do **not** infer a location from framework habit ("`*_test.go` next to the source"). That instruction used to live here, and it was the downstream half of a question step 9 of `build-feature` never answered: a convention this step invents is invisible to the adapter, invisible to the plan, and differs from whatever the next run infers. If a suite has no `file:`, that is a stale testcases.yaml — say so and stop, rather than restoring the guess.
    - **A suite citing a hand-authored unit's test is not generated.** Its `file:` names a test a person maintains; the denylist from step 11.4 covers that path and writing it would overwrite their suite with a generated one. Report it as covered-by-citation in the run summary.
    - **Check the step 11.4 denylist before every test file you write.** "Where the framework expects" is next to the source, and for a unit's source that is inside the unit's own test directory — a path the unit declares in `tests:` and a person already maintains. The test-file exemption in 11.5 does not authorize it. Refuse with `unit-write-refused` rather than overwriting someone's test suite with a generated one.
    - **A suite whose invariant a unit already satisfies should not exist.** If `testcases.yaml` still carries one, that is a build-phase problem, not something to fix by writing the file elsewhere — report it and leave the suite ungenerated.
    - **Record each file as you write it:** append its path to `.parlay/build/_project/.emitted`, one path per line, immediately after writing it. Test files are generated files and are recorded like any other. Step 17 explains what the manifest is for and what goes wrong without it.

15.5. **Run post-emit toolchain tools** — Run `parlay internal toolchain-plan @{feature} --phase code --stage post-emit`. Same availability / `required` / `fallback` handling as step 11.9. This step sits deliberately **before** step 16 so step 16's test run is the `preserves: [testcases]` enforcement.

   - **`authority: advisory`** (e.g. a `/react-review` skill): invoke it, capture its findings, and surface them in the step-18 report. It writes nothing.
   - **`authority: mutating`** (e.g. a formatter): invoke it within `write_set`, then enforce its `preserves:` list:
     - **`testcases`** → enforced by step 16 below. If a `preserves: [testcases]` tool ran and step 16 then fails, STOP with `toolchain-preserves-violated` naming the tool — the provenance matters, so do not report it as a plain test failure.
     - **`markers`** → snapshot `parlay internal scan-generated {source-root}` before invoking, re-run it after, and diff. A parlay marker that vanished from an `owns_markers: parlay` file the tool touched is a violation → STOP with `toolchain-preserves-violated`.
     - **`declared-elements`** → confirm every declared element/action from each touched component's buildfile entry is still present in the tool's output.
     - Apply the same `owns-markers` bookkeeping as step 11.9.

16. **Run tests** — Execute the generated tests against the generated prototype. Capture the result.
    - **If any test fails, STOP.** Do not proceed to step 17. Report the failures and ask the user how to proceed (show details / regenerate failing components / stop). The build state must NOT be committed when tests are failing — see step 15.

17. **Commit the build state** — Only if all tests passed in step 16: run `parlay internal save-build-state --source-root {source-root} --emitted .parlay/build/_project/.emitted`. This atomically writes:
    - Per-feature baselines for ALL features (source hashes for per-feature diff)
    - Project-level baseline at `.parlay/build/_project/.baseline.yaml` (merged section hashes)
    - Project-level code-hashes at `.parlay/build/_project/.code-hashes.yaml` (all generated files)
    - This is the **only** sanctioned write path for these files. No @feature argument — the command operates at project level.
    - **`--emitted` is how you declare what you wrote.** `save-build-state` cannot tell a regeneration from a hand-edit by looking at bytes — the determinism contract is functional, not byte-identity, so the same file legitimately differs between two runs. Without the manifest every file is recorded with unknown provenance and `verify-generated` can no longer say whether anyone hand-edited anything.
    - **Append to `.parlay/build/_project/.emitted` as you go**, one path per line, immediately after writing each file in steps 12–15. Do not reconstruct the list here from memory: "now list everything you wrote", asked at the end of a long run, is exactly the recall that goes wrong, and a file you forget is recorded as a hand-edit. `save-build-state` deletes the manifest on success, so nothing needs cleaning up.
    - If the command reports files as **adopted**, they were changed outside codegen since the last emission. It records them and warns rather than refusing — the save still succeeds. Report them to the user by name; do not silently overwrite them on the next run.

18. **Report** —
    - On success: list the generated files (one per component + cross-cutting files), confirm tests passed, confirm that `save-build-state` succeeded, and tell the user how to run the prototype.
    - On test failure (stopped at step 16): list the failing tests with summaries, and ask the user how to proceed. **Do not call `save-build-state` when tests have failed** — the whole point of running tests before committing state is to avoid committing a broken state.
    - On generation failure (stopped before step 16): report the underlying error and stop.

## Determinism contract

Two AI agents reading the same buildfile + adapter must produce code that passes the same testcases. The code itself does NOT need to be byte-equivalent or even structurally identical — the contract is functional determinism, measured at the testcase boundary. Agents have latitude on naming, file organization, idiomatic style, and framework-specific helpers, as long as observable behavior matches.

If two agents produce code that diverges on testcase pass/fail, that is either:
- A buildfile schema bug (missing detail) — fix the schema
- A testcase observability bug (testing implementation details) — fix the testcases.yaml generation in build-feature
- An agent bug (not following the buildfile faithfully) — fix the skill instructions

It is never a "minor difference" to be ignored.

## Incremental regeneration

Three read helpers and one write helper cooperate to make incremental rebuilds safe:

- **`parlay internal diff @{feature}`** — compares current sources to the saved baseline and classifies each buildfile component as `stable`, `dirty`, or `removed`. Source-of-truth for "what changed in design land."
- **`parlay internal scan-generated {source-root}`** — walks the source tree, finds every file with a `parlay-component:` marker, returns `path → component` map. Source-of-truth for "which file belongs to which component." Files without a marker are user-owned and excluded.
- **`parlay internal verify-generated @{feature}`** — compares each recorded generated file against its stored content hash and its recorded provenance. Classifies as `stable`, `modified`, `missing`, `adopted`, or `unknown`. Source-of-truth for "did the user hand-edit a generated file."
  - `modified` is honestly ambiguous — the bytes differ from the snapshot, and because re-emission is not byte-stable that could be either a hand-edit or a regeneration.
  - `adopted` is not ambiguous: a previous save found the file in a state no emission declared, so something other than codegen wrote it. Treat it as user-owned and surface it before overwriting.
  - `unknown` means provenance was never declared — most often a snapshot written before `--emitted` was used. It is reported separately from `stable` so an uncertified snapshot cannot read as a clean bill of health. A snapshot with no `schema-version` predates provenance entirely and grades **every** file `unknown`, whatever its entries claim: that version could not have written a provenance value, so any value found in one is unreadable rather than authoritative.
  - Exit code stays 0 whatever it finds; parse the JSON and decide. `--strict` exits non-zero on any `adopted`, `unknown`, or `modified`, for CI — it answers "is every recorded file confirmed safe to overwrite", and all three of those buckets mean *no*. `modified` is included precisely *because* it is ambiguous: a caller with no user to ask cannot resolve the ambiguity, and resolving it toward "carry on" is what overwrites an unsaved hand-edit.
- **`parlay internal save-build-state --source-root {source-root} --emitted .parlay/build/_project/.emitted`** — atomically commits the source baselines, the code hashes and their emission provenance after a successful end-to-end generation. This is the **only** sanctioned write path for those files. It takes no `@feature` argument: the command operates at project level.

The skill calls the three read helpers before regenerating, then `parlay internal save-build-state` after writing files AND running tests successfully. The saves happen exactly once per successful e2e run and represent the state at that point in time.

**Multi-component (extended) files** — files produced by intelligent merge (step 14.5 Tier 2) carry a primary `parlay-component:` marker plus one or more `parlay-extends:` lines. These files belong to multiple components at once, with consequences for the read helpers:

- `parlay internal scan-generated` reports the file's primary component AND its extending components. A single file path appears once but maps to multiple `(feature, component)` owners.
- `parlay internal verify-generated` hashes the file as a unit; the file is `stable` only if every component named in its marker block (primary + all `parlay-extends:`) is currently `stable` per `parlay internal diff`. If ANY claimed component is dirty in the diff, the file requires regeneration via re-merge.
- Re-merge re-runs step 14.5 for the dirty component(s) against the current state of the file (which includes the other components' contributions). The agent must preserve all currently-claimed components in the resulting file; dropping any without explicit removal would silently un-extend the file.
- A component being `removed` in the diff means its claim on the file should be revoked: drop its `parlay-extends:` line and remove the spans it owned (identified by per-function markers if present). If the removed component was the primary owner, ownership transfers to the first remaining `parlay-extends:` line, which is promoted to the primary marker.

**The very first generation** of a feature is detected by `parlay internal verify-generated` returning `has_hashes: false`. In that case there are no stable components to preserve and nothing to verify — treat every component as new and regenerate everything. `parlay internal diff` may report components as `stable` on a first run (if `parlay build-feature` left a baseline behind, which it shouldn't anymore but might from older runs) — `verify-generated`'s `has_hashes` field is the authoritative signal for "is there committed code state?"

If **any** generated file is reported as `modified` by verify-generated — whether its component is stable or dirty — the user has hand-edited it. **Do not** silently overwrite it. Surface the situation and let the user choose: overwrite, skip, or diff. The `parlay-component:` marker is the source of truth for "this file is generated"; absence of the marker means the file is user-owned and must never be touched.

The dirty case is the one that matters most and it is easy to get wrong: a component whose upstream spec changed is exactly the component codegen is about to rewrite, so scoping the check to stable components alone guarantees the hand-edit is destroyed without warning. Stable components, by contrast, are not rewritten at all — the check is nearly free there.

Because re-emission is only *functionally* deterministic (see "Determinism contract"), a content hash alone cannot distinguish "the user edited this" from "we regenerated it". `verify-generated` reports both as `modified`. Until emission provenance is recorded alongside the hash, treat `modified` as "needs a human decision" rather than as proof of a hand-edit, and prefer showing the diff (option C) when the component is dirty.

## Why save-build-state is at the end (and only at the end)

The baseline (`.baseline.yaml`) and the code-hashes sidecar (`.code-hashes.yaml`) have a **consistency invariant**: they must always represent the same point in time — the end of a successful end-to-end generation. If either file is updated independently of the other, subsequent `parlay internal diff` and `parlay internal verify-generated` calls describe inconsistent states and the agent gets stuck (e.g., diff says "stable" but no code exists).

For a full generate-code run the "same point in time" is the whole project's — every feature is regenerated and blessed at one instant. A **partial** save (`save-build-state --partial --emitted`, the form `/parlay-refine` runs) relaxes this to *per-feature* instants: it advances the baseline only for the features it actually emitted and leaves every other feature's baseline — and its dirty flags — exactly as they were. The baseline/code-hashes pair still moves atomically, at the finer per-feature grain; a feature the partial run did not touch keeps a consistent instant of its own from its last full blessing. See `schema-versioning.schema.md`, "Per-feature blessing instants".

Earlier versions of the skill saved the baseline at the end of `build-feature`, before code generation. That broke the invariant: after build-feature ran but before generate-code ran, the baseline said "this source state is committed" but no code state existed for that source state. The next run would see all components as stable and skip everything.

The fix is structural: the baseline and code-hashes are written together by a single command (`parlay internal save-build-state`) at the end of `generate-code`, only after tests pass. The two underlying writes use the write-then-rename pattern for atomicity, so a partial failure leaves the previous state intact. If tests fail, neither file is written — the next run starts from the same state as before, so retrying is safe and deterministic.

## Error Handling

- `buildfile-not-found` — `.parlay/build/{feature}/buildfile.yaml` does not exist. Tell the user to run `/parlay-build-feature @{feature}` first.
- `adapter-not-found` — `.parlay/adapters/{framework}.adapter.yaml` does not exist. Tell the user to run `parlay register-adapter <path>` or `parlay init`.
- `invalid-buildfile-yaml` — YAML parse error. Show the error and ask the user to regenerate via `/parlay-build-feature`.
- `unknown-component-type` — buildfile uses a component type not in the adapter. Either the buildfile is stale (regenerate it) or the adapter needs extending.
- `source-root-collision` — adapter's source root conflicts with existing non-generated files. Ask the user how to proceed.
- `test-execution-failed` — generated tests don't pass. Show summaries and offer the menu (show details / regenerate failing components / stop).
- `stale-buildfile` — the freshness gate (step 11.6) detected that the buildfile's `source-signatures:` no longer match current source state. The error message format is `stale-buildfile at <feature>: buildfile reflects <prior-signature>; current sources are <current-signature>. To fix: run `parlay build-feature <feature>` to refresh the buildfile, then re-run codegen.` Process exit code is non-zero. No new files are written for the affected feature, but other features in the same run continue. This is the **only codegen-owned content-error category** — missing bindings inside a buildfile are also surfaced as `stale-buildfile`, not as a separate "missing-binding" class.
- `precheck-refusal` — the layout-validation precheck (step 11.7) refused a layout-bearing page. Codegen surfaces the precheck's message **verbatim**, refuses for that page only, lets other pages in the same project continue, and exits non-zero overall. The precheck refusal wins over `stale-buildfile` when both apply for the same page.
- `toolchain-required-tool-absent` — a `required: true` toolchain entry (step 11.9 / 15.5) names a skill or MCP server that is not available in this session. Codegen STOPs for the affected feature, writes nothing, and exits non-zero; other features continue. (A `required: false` tool that is absent is NOT this error — it falls back gracefully.)
- `toolchain-preserves-violated` — a `mutating` toolchain tool ran and broke one of its declared `preserves:` guarantees: the testcases regressed (caught by step 16), a parlay marker vanished from an `owns_markers: parlay` file, or a declared element/action disappeared. Codegen STOPs and names the tool. Distinct from `test-execution-failed` on purpose — the provenance (a tool broke it, not the emission) is the actionable fact.
- `spec-leak` — if you (the agent) find yourself wanting to read a file under `spec/intents/`, **do not**. Stop and report which buildfile field is missing the information you need. This is a buildfile schema bug, not an excuse to cross the boundary.

## Section: Boundary gate

<!-- parlay-extends: parlay-tool/multi-adapter/coverage-review-gate — provenance: this section began as the coverage-review gate, now retired -->
<!-- parlay-extends: parlay-tool/multi-adapter/codegen-flow-ordered-layer-generation-and-fixed-read-set -->

Codegen does not consult a coverage review. That gate is retired: it asked a
person to approve a list of suite NAMES with the default set to yes, recorded
whoever the environment said was running, and proved that somebody answered
rather than that anybody looked.

What guards this boundary now is the injected **Step 0 — Gate**, which
aggregates it: a person approved the criteria this feature is graded against
(`criteria-authority`), the tests mechanically discharge that standard
(`testcases-readiness`), and any recorded exception is still bound to the
contract it was granted against (`coverage-decisions`). A non-zero exit stops
this module exactly as before, and its `blockers[]` name what to fix.

Two things follow. Regenerating testcases no longer invalidates anything: what
was approved is the standard, not the suites derived from it. And a run carrying
`--authorize-criteria=machine`, in a project that has opted in, advances without
human approval and records that waiver at this boundary — the record says the
separation between authoring a standard and grading against it was waived, not
satisfied.

### Codegen read-set

The skill is permitted to read ONLY:

- `.parlay/build/<feature>/{buildfile,testcases}.yaml`

  `criteria-authority.yaml` and `coverage-decisions.yaml` are deliberately NOT
  in this set. Codegen does not read them; the injected **Step 0 — Gate** does,
  and that command's reads are its own — a boundary check running before this
  module is not this module widening what it may open. Keeping them out is what
  stops "the gate consults it" from becoming "codegen may read anything the gate
  reads".
- `.parlay/{config,blueprint,adapter-set}.yaml`
- `.parlay/adapters/<slug>.adapter.yaml` (referenced from adapter-set)
- `.parlay/domain-model.yaml`
- The source tree under each adapter's declared root

Reads of `spec/intents/**` are forbidden and mechanically enforced. Attempts surface `codegen-spec-read-forbidden`. Reads of paths outside the read-set surface `codegen-input-out-of-scope`.

### Layered emission order

Default emission order: persistence → application → transport → presentation. Each layer fully completes before the next starts; freshly-emitted outputs feed the next layer's prompt context. This ordering ensures that downstream layers can consult the shape upstream layers committed to.

### Step ownership (`owns:`)

Each backend target's `targets.<kind>.operations."@f/op:id"` carries an `owns:` list — the steps that layer implements. When emitting a target, implement the steps in its `owns:` list using the adapter's conventions (the persistence target implements `create-one` as `prisma.<entity>.create`; the application target implements `validate-input`/`authorize`/`return-*` via its pipes/guards/return values). For a step owned by a **downstream** target (e.g. the application service orchestrating a persistence-owned `create-one`), do not re-implement it — **call** the downstream layer's output across the authorized `links` edge. The persistence-first emission order guarantees the owned code exists before the orchestrator that calls it. This makes the layer split a pre-decided contract rather than a codegen-time judgment. **Backward-compatible:** if `owns:` is absent (older buildfiles), fall back to emitting each backend layer from the adapter conventions + the plan paths as before.
