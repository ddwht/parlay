# Create Artifacts

Determine which of the four co-equal spec artifacts a feature needs — `surface.md` / `surface.yaml`, `capabilities.yaml`, `infrastructure.md`, `domain-model.yaml` — based on its intents and dialogs, then create the appropriate artifacts. The four artifacts cover orthogonal concerns: what the user sees, what operations the backend exposes, what architectural prose constrains the codebase, and what entities and vocabulary the feature shares. A feature picks whichever subset it needs; none of the artifacts is a stand-in for another.

## Arguments

- `feature`: The feature slug (e.g., `initiatives`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), re-prompt the user via AskUserQuestion with the listed candidate roots, then re-invoke with `--root <chosen>`.

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
   - Any ambiguous intents → **ask the designer** (step 4)

4. **Present the decision** — Show the designer:
   - The decision (which subset of the four artifacts)
   - Per-intent breakdown: which intent maps to which artifact(s) and what signals drove the classification
   - Override options:
     ```
     A: Proceed with this recommendation
     B: Also add [the missing artifact]
     C: Drop [an artifact from the recommendation]
     D: Let me explain what this feature does (for ambiguous cases)
     ```
   - Wait for the designer's confirmation or override via AskUserQuestion

5. **Create the artifacts**:
   - **If surface**: run the existing create-surface flow (load schemas, analyze for ambiguities, generate surface.md or surface.yaml, validate)
   - **If capabilities**: guide the designer to author `capabilities.yaml` — show the closed-vocabulary structure, the operation kinds, and an example operation derived from the feature's intents
   - **If infrastructure**: guide the designer to author `infrastructure.md` — show the fragment format, the field set (Name, Source intent, Affects, Behavior, Invariants), and a worked example drawn from the matching architectural category (boundary, probe, allowlist, dependency pin)
   - **If domain-model**: guide the designer to author or extend `domain-model.yaml` with the new entities, relationships, or vocabulary
   - When multiple artifacts are required, author them in order: domain-model first (so other artifacts can reference its entities), then surface and capabilities and infrastructure in any order

6. **Report** — Confirm which artifacts were created and what the next pipeline step is (`/parlay-build-feature @{feature}`).

## Error Handling

- `no-intents` — intents.md is empty or missing. Tell user to author intents first.
- `no-dialogs` — dialogs.md doesn't exist. Warn that the decision will be based on intents only (less signal). Ask whether to proceed or scaffold dialogs first.
- `artifacts-already-exist` — one or more of surface.md / surface.yaml / capabilities.yaml / infrastructure.md / domain-model.yaml already exists. Ask whether to regenerate (overwrite) the affected ones or skip.
