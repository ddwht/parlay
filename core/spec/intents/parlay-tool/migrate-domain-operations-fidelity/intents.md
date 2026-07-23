# Migrate domain-operations fidelity

> `parlay migrate-domain-operations` walks each entry under a project's deprecated `domain-model.operations:` and writes a `kind: unknown` stub into a chosen feature's `capabilities.yaml`. Two residual gaps reduce its usefulness in a real migration. First, the migrated stub's `notes:` carries only the boilerplate line `Migrated from domain-model.operations: <name>` and drops the operation's substantive `effects:` prose — yet the migrate skill tells the designer to review "the prose carried over under `notes:`," so the prose the designer is asked to review is not actually there. Second, `--feature` is a single global flag applied to every ambiguous operation in the run, so a headless migration cannot route different operations to different owning features — every ambiguous operation lands in the one named feature, or the migration must be run interactively. (The earlier "nested slugs are unroutable / namespace dirs get orphaned stubs" problem is already fixed — candidate features now come from `cfg.AllFeatures()`, which returns qualified `initiative/feature` slugs and excludes bare namespace directories.)

---

## Migrated stubs carry the operation's `effects:` prose

**Goal**: Carry each operation's `effects:` prose into the migrated stub so the designer reviewing `capabilities.yaml` actually sees the substantive description the migrate skill promises — not just the boilerplate provenance line — before classifying the stub as `kind: command` or `kind: query`.
**Persona**: Parlay tool maintainer
**Priority**: P2
**Context**: In `core/internal/commands/migrate_domain_operations.go`, the per-operation loop reads only `op["entity"]`, `op["title"]`, and `op["name"]` and emits a stub whose `notes:` is exactly `Migrated from domain-model.operations: <title>` (around line 146). The operation's `effects:` list is never read. But `domain-model.yaml` operations carry real prose there — e.g. the `add-feature` operation's `effects:` reads "parlay add-feature creates a Feature with empty intents.md and dialogs.md," and `bootstrap-project`'s reads "creates Project, ProjectConfig, and FrameworkAdapter; initializes .parlay/ (initial setup, no CLI command)." Meanwhile `core/internal/embedded/skills/migrate-domain-operations.skill.md` step 2 instructs: "For each stub, the designer reviews the prose carried over under `notes:` and sets `kind: command` or `kind: query` explicitly." The prose the skill points the designer at is dropped on the floor — the designer must open the original `domain-model.yaml` to recover what the migration was supposed to carry.
**Action**: In the stub-emission path, read the operation's `effects:` (and any other substantive prose fields the source operation carries) and include them under the stub's `notes:` alongside the provenance line, so a migrated stub reads as `Migrated from domain-model.operations: <title>` followed by the operation's effects. Preserve the existing provenance line; append the carried prose rather than replacing it. The carry is mechanical field-copying, no AI.
**Objects**: migrate-domain-operations, operation-effects, capability-stub, stub-notes, provenance-line, domain-model-operations

**Constraints**:
- The existing provenance line (`Migrated from domain-model.operations: <title>`) is retained — the change adds the effects prose, it does not replace the provenance.
- An operation with an empty or absent `effects:` produces a stub identical to today's (just the provenance line) — no empty "effects:" scaffolding written for operations that have none.
- Multi-line / multi-entry `effects:` are carried readably under `notes:` (a YAML block scalar or bulleted list), preserving each effect string verbatim — no lossy flattening that drops entries.
- `kind:` still lands as `unknown` for every stub (this feature changes what prose the stub carries, not the classification contract the skill's step 2 governs).
- The emitted `capabilities.yaml` remains schema-valid — the enriched `notes:` is still a single `notes:` field on the stub, correctly quoted/escaped for arbitrary effect text.

**Verify**:
- Migrating an operation whose `effects:` reads "parlay add-feature creates a Feature with empty intents.md and dialogs.md" produces a stub whose `notes:` contains that sentence, not just the provenance line.
- An operation with multiple `effects:` entries carries every entry into the stub's `notes:` verbatim.
- An operation with no `effects:` produces a stub byte-identical to today's provenance-only stub.
- The resulting `capabilities.yaml` validates cleanly regardless of punctuation or special characters in the carried effect text.

---

## Per-operation feature routing in a headless run

**Goal**: Let a headless (non-interactive) migration route different ambiguous operations to different owning features in one run, instead of forcing every ambiguous operation into the single feature named by the one global `--feature` flag or requiring an interactive session.
**Persona**: Parlay tool maintainer
**Priority**: P2
**Context**: In `core/internal/commands/migrate_domain_operations.go`, `migrateDomainOperationsFeature` is one string set by `--feature` for the whole run. In the per-operation loop, every operation is offered the same full candidate list (`candidates := candidateFeatures`), and when ambiguous the code routes to the single `explicitFeature` (around line 118) — so `--feature @task-list` sends every ambiguous operation to `task-list`. In headless mode (no TTY or `--non-interactive`), the only alternatives are the interactive stdin prompt (unavailable) or an `ambiguous-target` hard error. There is no way to say "operation A goes to feature X, operation B goes to feature Y" without a TTY. A real project (this repo's `core/domain-model.yaml` carries a dozen-plus operations across many features) cannot be migrated headlessly to its correct per-operation homes in a single pass. The skill documents `--feature` as the headless disambiguation lever but it is a single global lever.
**Action**: Add a per-operation routing mechanism usable headlessly — for example a repeatable mapping flag (`--route <operation>=<feature>`), a routing manifest file the command reads, or per-operation narrowing of the candidate set by the operation's `entity` (features that reference the entity become the natural candidates, shrinking or resolving the ambiguity). Keep the single global `--feature` as the "send all ambiguous to one home" shortcut it is today. Headless ambiguity that the per-operation routing does not resolve still hard-errors with `ambiguous-target`, now naming the specific unrouted operation(s). Decide the exact shape in dialogs.
**Objects**: migrate-domain-operations, feature-routing, ambiguous-target, per-operation-route, routing-manifest, candidate-features, headless-mode

**Constraints**:
- The single global `--feature` flag keeps working as today (route all ambiguous operations to one feature); the new mechanism is additive, not a replacement.
- Per-operation routing works in headless/non-interactive mode — that is the whole point; it must not require a TTY.
- An operation that remains ambiguous after per-operation routing is applied still hard-errors with `ambiguous-target` in headless mode (never a silent guess), and the error names the specific operation left unrouted, per the skill's headless contract.
- A route that names a non-candidate (or non-existent) feature fails loudly, consistent with the current `--feature` validation that rejects a feature outside the candidate set.
- Candidate features continue to come from `cfg.AllFeatures()` (qualified `initiative/feature` slugs, namespace directories excluded) — this feature builds on that fix, it does not reintroduce top-level-only scanning.

**Verify**:
- A headless run with per-operation routing sends operation A to feature X and operation B to feature Y in a single pass, writing each stub to the correct `capabilities.yaml`.
- A single global `--feature` run behaves byte-identically to today (all ambiguous operations to the one named feature).
- An operation left ambiguous after routing hard-errors with `ambiguous-target` naming that specific operation, in headless mode — no silent default.
- A route naming a feature outside the candidate set fails loudly with a message naming the bad target.
