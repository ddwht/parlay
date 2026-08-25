---
name: create-artifacts
description: "Determine and create any subset of surface, capabilities, infrastructure, and domain-model artifacts a feature needs"
surface: module
---

# Create Artifacts

Determine which spec artifacts a feature needs, based on its intents and dialogs, then create the appropriate ones — <!-- parlay:expand-co-equal-artifacts -->. The four artifacts cover orthogonal concerns: what the user sees, what operations the backend exposes, what architectural prose constrains the codebase, and what entities and vocabulary the feature shares. A feature picks whichever subset it needs.

## Arguments

- `feature`: The feature slug (e.g., `initiatives`)

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

<!-- parlay:expand-decision-protocol -->

<!-- parlay:expand-feedback -->

## Steps

1. **Read feature files**:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`

1.5. **Amendment pre-flight — this module authors artifacts at birth only.** Run `parlay internal check-applied @{feature}` before analyzing anything. This module derives the four contract artifacts from the founding documents (`intents.md`, `dialogs.md`). That is only correct at birth, before the founding docs freeze. Once a feature has been built, the founding docs are frozen and the contract artifacts are the current truth — regenerating them here from `intents.md`/`dialogs.md` would silently revert every amendment the ledger has since applied, because the founding docs never absorbed them.

   So stop, and do not author, when EITHER holds:
   - the feature has a non-empty `amendments/` ledger (`check-applied` returns any entries in `amendments`), or
   - `ComputeFeaturePhase` reports phase ≥ build (a buildfile exists — the founding docs are frozen). `check-applied`'s `has_baseline: true` is the cheap signal for this.

   Return a `kind: impasse` decision block (see **Asking the user**) routing the change to `/parlay-refine`: post-birth change to a contract goes through the amendment ledger, one amendment at a time, not through a wholesale re-derivation from frozen founding docs. Name what you found — the ledger entries or the baseline — in `context:`, and offer `refine` (hand off to `/parlay-refine`) and `explain` (the user believes this genuinely is a fresh feature) as options. No `default:` — silently regenerating over an amended contract is exactly the data loss this guard exists to prevent, so an unattended run must abort rather than pick one.

   Proceed only when the feature is at birth: no ledger, no baseline. That is the state this module is for.

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
   - **If surface**: run the existing create-surface flow (load schemas, analyze for ambiguities, generate surface.yaml, validate)
     - **Populate `verify:` on each fragment** (YAML form) with the owning intent's presentation claims, following **Routing acceptance criteria** below. Operation coverage of the intent does **not** exempt a fragment: an intent that produces an operation still contributes its presentation claims here. This is what testcase derivation reads for presentation suites; there is no fallback — a fragment whose criteria never land here has none, and every case written against it will cite nothing.
   - **If capabilities**: guide the designer to author `capabilities.yaml` — show the closed-vocabulary structure, the operation kinds, and an example operation derived from the feature's intents
     - **Set `source:` on every operation** to the `@{feature}/{intent-slug}` refs it came from, the same way a surface fragment carries one. This is the only record of which intent an operation implements, and it is what lets a later change described in prose be routed to the operation that owns it. An operation without it can be found only by name similarity, which misses renames and matches things that merely sound alike.
     - **Populate `verify:` on every operation** with the owning intent's contract claims, following **Routing acceptance criteria** below. The acceptance criteria live on the operation from birth — testcase derivation reads them from here, not from intents.md.
   - **If infrastructure**: guide the designer to author `infrastructure.md` — show the fragment format, the field set (Name, Source intent, Affects, Behavior, Invariants), and a worked example drawn from the matching architectural category (boundary, probe, allowlist, dependency pin)

   **Routing acceptance criteria.** An intent's **Verify** bullets are split
   between the fragments and the operations that source that intent. Route
   **atomic claims, not whole bullets**: a real bullet routinely packs a
   stimulus, a backend result and a piece of visible evidence into one
   sentence, and routing the sentence by its dominant flavour either places it
   arbitrarily or duplicates it wholesale.

   1. Extract the independently testable claims from the bullet.
   2. A claim about **user-observable presentation or output**, attributable to
      a specific fragment, goes on that fragment.
   3. A claim about the **transport-independent contract** — input validation,
      state change, output shape, allowed errors — goes on the operation.
   4. A sentence carrying both is **rewritten into separate criteria**, one per
      destination. Never relocate the same sentence verbatim to both places: a
      contract-shaped claim sitting on a fragment demands a display case that
      cannot be written honestly, and the build phase will write a vacuous one
      to discharge it.

   *Whether an operation covers the intent is not an input to this.* Routing by
   operation coverage is what produced features specified to have zero
   presentation criteria: every intent produced an operation, so every claim
   went to the operation, so every presentation case had nothing to cite.

   **"Visible" does not imply a fragment.** A CLI or TUI feature with no
   surface artifact has observable output and no fragment to carry it; there
   its output claims stay on the operation. The rule places presentation claims
   on a fragment *when the feature has fragments*, not wherever something is
   observable.
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
- `artifacts-already-exist` — one or more of surface.yaml / capabilities.yaml / infrastructure.md / domain-model.yaml already exists. Ask whether to regenerate (overwrite) the affected ones or skip. **But first re-check the step 1.5 pre-flight:** if this feature has a ledger or a baseline, regenerating is not an overwrite the designer should be offered — it silently reverts every applied amendment, because the fresh artifacts derive from frozen founding docs that never absorbed the ledger. In that state the answer is not regenerate-vs-skip; it is `/parlay-refine`, and step 1.5 has already stopped with the impasse. Only offer regenerate-vs-skip for a feature genuinely still at birth.
