# create-artifacts

_Determine and create any subset of surface, capabilities, infrastructure, and domain-model artifacts a feature needs_

# Create Artifacts

Determine which spec artifacts a feature needs, based on its intents and dialogs, then create the appropriate ones — the four spec artifacts are co-equal — `surface.yaml` (or legacy `surface.md`), `capabilities.yaml`, `infrastructure.md`, and the project's `domain-model.yaml` — none is a stand-in for another. The four artifacts cover orthogonal concerns: what the user sees, what operations the backend exposes, what architectural prose constrains the codebase, and what entities and vocabulary the feature shares. A feature picks whichever subset it needs.

## Arguments

- `feature`: The feature slug (e.g., `initiatives`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

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

When feedback mode is on, this project keeps a local record of what actually happened during a run, so the toolkit can be improved from evidence rather than from recollection. It is **off by default**; when it is off every command below is a silent no-op, so you may call them unconditionally and must not branch on whether it is enabled.

The CLI already records its own half — every invocation, its exit code, its duration, and every diagnostic any validator produced. **Do not re-report those.** Record only what the CLI cannot see, which is what you did and why:

```
parlay internal feedback-record --kind <kind> --skill <this-skill> --data key=value
```

| Kind | Record when |
|---|---|
| `phase` | You enter or leave a pipeline phase. `--data phase=build --data at=start`. |
| `decision` | You raised a `parlay-decision` block, and again when it resolves. Include `kind=`, and on resolution the chosen `option=`. The CLI never sees these. |
| `retry` | **The important one.** You re-attempted something after a rejection: authored a shape, had it refused, and tried a different one. Include `after=<the error code>` and `changed=<what you did differently>`. |
| `improvised` | You proceeded without a rule you needed — invented a path, guessed a convention, weakened an assertion you could not satisfy. Include `needed=<what was missing>`. |
| `note` | Anything else worth a future reader knowing. Use sparingly; a log full of notes means a kind is missing. |

**`retry` and `improvised` are the two the log exists for.** A validator that teaches by rejection looks exactly like one that teaches by documentation unless the retries are counted, and an agent that guessed a convention leaves no other trace at all — the run passes, and the guess is discovered later as an inconsistency nobody can date. Recording them is not an admission of failure; it is the only way the gap that forced them gets closed.

**Correlation is automatic — do not manage it.** Events are tied together by `PARLAY_RUN_ID`, which the loop driver sets once per pipeline run and every CLI call inherits from the environment. That is what lets a retry be tied to the diagnostic that caused it. You do not need to read it, pass it, or thread it through; `--run` exists only to override it and is almost never the right thing to reach for.

## Steps

1. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`

2. **Analyze intents for artifact signals** — For each intent, classify which artifact (or combination of artifacts) it contributes to. The classification is based on what the intent DESCRIBES, not on persona names (which are project-specific). The four artifacts are co-equal — an architectural intent flows to `infrastructure.md` directly, not via `capabilities.yaml`, and vice versa.

   **Surface signals** (the intent describes visible output):
   - Dialog System turns show visible output — rendered results, prompts, status messages, formatted data
   - The intent's Goal describes what someone sees or interacts with when the feature runs
   - Objects reference output-facing concepts (reports, prompts, displays, confirmations)

   **Capabilities signals** (the intent describes operation-shaped backend behavior):
   - The intent introduces a closed-vocabulary command or query — create, read, update, delete, search, validate-input — acting on a domain entity
   - Dialog System turns describe an operation the backend performs (with input, steps, errors, output shape) rather than a structural constraint
   - The intent's Verify bullets describe operation contracts: input validation, persistence side-effects, output presence, allowed errors

   **Infrastructure signals** (the intent describes architectural prose — constraints on how the codebase is shaped, not operations the backend performs):
   - **Boundaries**: package-import rules, layered-architecture constraints, allowed-callers limits
   - **Probes**: startup health checks, readiness probes, dependency-availability assertions
   - **Allowlists**: bounded vocabularies of external tools, SDK calls, environment variables, host names
   - **Dependency pins**: required library versions, runtime baselines, build-time toolchain constraints
   - **Feature-stable error codes** outside the closed errors vocabulary defined in `errors.schema.md`
   - **Build-time constraints**: lints, compile-time assertions, code-generation invariants
   - The Goal describes "the codebase is shaped such that …" or "X is constrained to …" rather than "the backend performs …"
   - Dialog turns describe a property of the source tree, the build pipeline, or the runtime environment — not an operation a user triggers

   An intent whose entire content is architectural — for example, "the figma SDK may only be imported from internal/sdk" — flows to `infrastructure.md` without authoring a `capabilities.yaml` entry. Architectural prose does not need to be paraphrased as a fake operation; the two artifacts are co-equal, and either is a valid backend destination depending on the intent's shape.

   **Domain-model signals** (the intent introduces entities, relationships, or shared vocabulary):
   - The Objects field names new entities, value types, or enums not already in `domain-model.yaml`
   - The intent introduces relationships between existing entities (containment, ownership, cardinality)
   - The intent introduces vocabulary used across multiple features

   **Ambiguous signals** (conflicting indicators):
   - The intent describes both visible output AND backend behavior
   - Dialog shows both rendered results and code-shape constraints
   - Objects mix output-facing, operation-shaped, and architectural concepts

   **Blueprint check** (after per-intent analysis): If `.parlay/blueprint.yaml` exists, check whether the feature's intents imply changes to any cross-cutting system documented there (e.g., deployers, registries, shared layers). Features that appear surface-only from their intents may also need infrastructure or capabilities to integrate with the project's shared architecture. When in doubt, recommend the additional artifact and explain the blueprint-derived reason to the designer.

3. **Determine the artifact set** — Any non-empty subset of {surface, capabilities, infrastructure, domain-model} is valid. Common combinations:
   - All intents are surface-only → **surface only**
   - All intents are operation-shaped → **capabilities only**
   - All intents are architectural prose → **infrastructure only** (no `capabilities.yaml` is authored — architectural prose is a co-equal artifact)
   - Mix of surface + operations → **surface + capabilities**
   - Mix of surface + architectural → **surface + infrastructure**
   - Mix of operations + architectural prose → **capabilities + infrastructure**
   - New entities or vocabulary → **add domain-model**
   - Surface intents with blueprint-derived backend implications → **surface + the implied backend artifact** (explain why)
   - Any ambiguous intents → **raise the decision** (step 4) rather than picking the reading that is easiest to build
   - **No subset fits** → this is not an artifact-set question. See step 3.5.

3.5. **When no artifact set fits: offer the hand-authored unit** — Sometimes an intent is not surface, not operation-shaped, and not architectural prose. It describes a computation: a geometry kernel, a codec, a solver, a parser. The four artifacts can describe what such a thing is *for* and what must hold of it, but none of them can express *it*, and every subset you could pick would be a container you fill with prose that generates nothing.

   Do not resolve this by picking the closest artifact. That is the same failure the ambiguity rule forbids one step up, and it costs more here: the designer gets a `capabilities.yaml` full of operations no adapter supports, or an `infrastructure.md` of specificity warnings, and the pipeline carries the fiction forward until codegen reports it cannot write the thing.

   Raise `kind: impasse` instead, offering to declare the work a **hand-authored unit** — code a person writes, which the pipeline never generates but does track, hash and depend on. See `authored.schema.md`. Pre-fill from what you already know:

   - `context:` — which intents no artifact set expresses, and *why* each resists: the operation whose `kind:` is outside the closed vocabulary, the invariant no assertion vocabulary can state, the term the domain model has no shape for. Name them. "This looks hard to generate" is not a finding the designer can act on.
   - `options:` — at minimum `declare-unit` (scope it: which intents move into the unit, and which stay as ordinary artifacts — a unit rarely swallows a whole feature), and `keep-trying` with a note on what would have to change for the pipeline to express it. Add `explain` when you are not certain the intent resists expression at all.
   - **No `default:`.** Accepting a unit is a permanent scope reduction — that code will never be generated, by design — and no flag authorizes taking it unattended. Under `--non-interactive` the driver aborts with exit 11.

   On `declare-unit`, run `parlay add-feature "<name>" --authored --sources "<glob>" --summary "<one line>"`, then tell the designer to write the code and re-run `parlay validate --type authored`. The unit's `satisfies:` is where its invariants go, so the build phase stops generating suites for them.

   **Only when it genuinely does not fit.** A unit is the right answer for a computation nobody should generate; it is the wrong answer for an intent you have not read carefully, and offering it as an escape from a hard artifact decision trades a solvable problem for a permanent one.

4. **Raise the decision** — Return an `override` decision request (see **Asking the user**) carrying:
   - `context:` — the recommended artifact subset, and a per-intent breakdown naming which intent maps to which artifact(s) and what signal drove each classification. The reasoning is the point; a bare verdict gives the designer nothing to override *with*.
   - `options:` — at minimum:
     - `proceed` — author the recommended set
     - `add-<artifact>` — one per artifact not in the recommendation
     - `drop-<artifact>` — one per artifact in the recommendation
     - `explain` — "Let me describe what this feature does" (for intents you classified as ambiguous)
   - **Write nothing before this is answered.** The override menu exists so the designer can change the artifact set; authoring first makes it a notification.

5. **Create the artifacts**:
   - **If surface**: run the existing create-surface flow (load schemas, analyze for ambiguities, generate surface.md or surface.yaml, validate)
   - **If capabilities**: guide the designer to author `capabilities.yaml` — show the closed-vocabulary structure, the operation kinds, and an example operation derived from the feature's intents
     - **Set `source:` on every operation** to the `@{feature}/{intent-slug}` refs it came from, the same way a surface fragment carries one. This is the only record of which intent an operation implements, and it is what lets a later change described in prose be routed to the operation that owns it. An operation without it can be found only by name similarity, which misses renames and matches things that merely sound alike.
   - **If infrastructure**: guide the designer to author `infrastructure.md` — show the fragment format, the field set (Name, Source intent, Affects, Behavior, Invariants), and a worked example drawn from the matching architectural category (boundary, probe, allowlist, dependency pin)
   - **If domain-model**: write what this feature needs into the **feature's own** `spec/intents/{feature}/domain-model.yaml` — a *contribution*. Do **not** edit the project's root `domain-model.yaml` from a feature phase.
     - The contribution uses the same schema as the root model and holds **only what this feature proposes** — the new entities, the new fields on existing entities, the new enum values, the new relationships. It is not a copy of the root with edits.
     - The root model stays the source of truth. A contribution is a proposal: the loop reports it at the artifacts→build boundary, names which other features it affects, and the designer accepts it there. Editing the root directly from a feature phase is how one feature's need silently becomes every other feature's problem.
     - **Referencing an entity that only another feature's contribution proposes is fine.** `capabilities.yaml` reports it as `capabilities-entity-pending` — a warning naming the proposer — rather than failing. Do not invent a placeholder entity to work around it.
     - A project that has no contributions and only ever edits the root model still works exactly as before; the contribution file is optional.
   - When multiple artifacts are required, author them in order: domain-model first (so other artifacts can reference its entities), then surface and capabilities and infrastructure in any order

6. **Report** — Confirm which artifacts were created and what the next pipeline step is (`/parlay-build-feature @{feature}`).

   When you are running as a phase module, the report is the `phase-boundary` decision request you return. Carry an `artifacts:` list on it naming exactly what you wrote:

   ````
   ```yaml parlay-decision
   kind: phase-boundary
   phase: artifacts
   artifacts: [domain-model, surface]
   question: "Artifacts phase complete. Advance to build?"
   ...
   ```
   ````

   The list is not decoration and it is not the same as the recommended set from step 4 — the designer may have overridden it, and what matters downstream is what is on disk. The driver reads it to decide which follow-on options to offer at the boundary; `domain-model` in particular earns an offer to open the editor before the build phase reads the model. Report the names, not the filenames: `domain-model`, `surface`, `capabilities`, `infrastructure`.

## Error Handling

- `no-intents` — intents.md is empty or missing. Tell user to author intents first.
- `no-dialogs` — dialogs.md doesn't exist. Warn that the decision will be based on intents only (less signal). Ask whether to proceed or scaffold dialogs first.
- `artifacts-already-exist` — one or more of surface.md / surface.yaml / capabilities.yaml / infrastructure.md / domain-model.yaml already exists. Ask whether to regenerate (overwrite) the affected ones or skip.
