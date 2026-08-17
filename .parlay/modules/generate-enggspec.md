# generate-enggspec

_Generate engineering specification_

# Generate Engineering Specification

Translate feature design artifacts into a formal engineering specification for handoff.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Steps

1. **Load project config** — Read `.parlay/config.yaml` to determine the SDD framework format.

2. **Read all feature artifacts** — the four spec artifacts are co-equal — `surface.yaml` (or legacy `surface.md`), `capabilities.yaml`, `infrastructure.md`, and the project's `domain-model.yaml` — none is a stand-in for another; read whichever subset the feature has:
   - `spec/intents/{feature}/intents.md`
   - `spec/intents/{feature}/dialogs.md`
   - `spec/intents/{feature}/surface.yaml` or `surface.md` (if exists — YAML wins when both are present)
   - `spec/intents/{feature}/capabilities.yaml` (if exists)
   - `spec/intents/{feature}/infrastructure.md` (if exists)
   - `<activeRoot>/domain-model.yaml` (if exists — the project's one canonical domain model, not per-feature) or legacy `spec/intents/{feature}/domain-model.md`
   - `.parlay/build/{feature}/buildfile.yaml` (if exists — tool-internal)
   - `.parlay/build/{feature}/testcases.yaml` (if exists — tool-internal; project the observable-behavior assertions into the spec's Acceptance Criteria section)

3. **Generate specification** at `spec/handoff/{feature}/specification.md` (this is the only handoff artifact — engineering reads it and writes their own tests):
   - Feature overview and user stories (from intents — Goal becomes acceptance criteria)
   - Detailed interaction requirements (from dialog flows)
   - UI component specifications (from surface fragments)
   - Operation contracts (from `capabilities.yaml`, if present — each command/query's input, output shape, allowed errors, and policies)
   - Constraints and invariants (from `infrastructure.md`, if present — one entry per fragment's Behavior and Invariants)
   - Data models and API contracts (from buildfile models if available)
   - Test scenarios (from testcases if available)
   - Acceptance criteria (from intent constraints)
   - Format according to the configured SDD framework (e.g., GitHub SpecKit)

   **Ledger projects** (`.parlay/config.yaml` carries `ledger: true`): this
   document doubles as the ledger's **projection** — the always-derivable
   "current state in prose" the compaction model depends on. Three changes:
   - **The contract artifacts are the source of current truth**, not the
     founding docs: where the list above says "from intents" or "from dialog
     flows", read the frozen founding docs for the feature's original story,
     then apply every amendment in `spec/intents/{feature}/amendments/` in
     sequence order — an amendment's Change supersedes what it amends, and
     `supersedes:` chains resolve to the latest entry. Acceptance criteria
     come from the artifacts' `verify:` fields, which the apply step keeps
     current.
   - **Add a "History" section**: one line per amendment (`NNN <slug> —
     <date> — <trigger>`), so a reader sees how the feature got its shape
     without opening the ledger. **Supersession annotations are mandatory,
     both directions**: a superseded amendment's line ends with
     `(superseded by NNN)` and the superseder's with `(supersedes NNN)`.
     Run `parlay internal check-amendments @{feature}` and use its
     `superseded_by` map rather than deriving the links by hand — the
     amendment files themselves are immutable and carry only the backward
     link, so the projection is where readers get the forward direction.
   - **Disclose internal-state drift instead of asserting over it**: before
     stating any current behavior, grep the tool-internal
     `.parlay/build/{feature}/buildfile.yaml` and `testcases.yaml` for terms
     the amendments retired. Where the internal files still restate old
     behavior, add a "Known internal-state drift" note naming file and line
     rather than silently presenting the contract's answer as if the whole
     repo agreed. A handoff that asserts `✓ done` while three testcase
     descriptions still say `complete` is telling the reader less than it
     knows.
   - **Stamp the header generated-and-regenerable**: this file is a
     projection, never hand-edited, and regenerating it must always be safe.
     Include the `last-applied-amendment` sequence it was generated at, so a
     stale projection is detectable by eye.

4. **Report** — Print the specification path and the SDD format used. Remind the user to review before handoff.

## Error Handling

- Absent optional artifacts (`surface.yaml`/`surface.md`, `capabilities.yaml`, `infrastructure.md`, `domain-model.yaml`) are skipped silently — the corresponding specification section is simply omitted.
- `no-spec-artifacts` — the feature has none of the four spec artifacts. Tell the user to run `/parlay-create-artifacts @{feature}` first.
