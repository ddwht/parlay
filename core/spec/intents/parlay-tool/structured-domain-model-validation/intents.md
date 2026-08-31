# Structured Domain-Model Validation Output

> Expose Core's domain-model deep validation as a machine-readable CLI mode so out-of-process consumers — Parlay Studio's domain-model editor first among them — get the same findings the build path enforces, with stable codes, element paths, and severity, without importing Core or reimplementing a rule. Also closes a latent gap: `domain-operations-deprecated` is declared in the severity table but never emitted, so no caller can act on it today. This is the Core half of the parity contract that `studio/domain-model-editor/domain-model-editor-validation` consumes.

---

## JSON validation mode for `parlay validate --type domain-model`

**Goal**: Give `parlay validate --type domain-model` a structured `--json` mode that emits the full finding list (not a collapsed single error), reading the model from stdin as well as from a path, so an out-of-process caller can validate an in-memory draft and get one finding per violation.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: `ValidateDomainModelStructured(path, content) []ValidationError` already returns per-violation findings, but the CLI's `validate` command routes `--type domain-model` through the plain `ValidateDomainModel` variant and collapses the result into a single synthetic `schema-validation-failed` error — fine for a human reading a terminal, useless to a program that must anchor each finding to an element. Studio's editor validates a draft that lives only in memory and never touches disk, so the command must also accept the model on stdin rather than requiring a file. The `--json` project-validation path already exists (the `validateProject` output carries JSON tags), so this extends an established output convention rather than inventing one.

**Action**: Add a `--json` flag to `parlay validate`. For `--type domain-model`, route through `ValidateDomainModelStructured` and emit its `[]ValidationError` as a JSON array (empty array + exit 0 when the model is clean). Accept `-` as the path argument to read the model bytes from stdin, so `parlay validate --type domain-model --json -` validates a piped draft. The command exits 0 whether or not findings are present when `--json` is set — a finding list is a query result, not a command failure — while the non-`--json` human path keeps its current non-zero-on-invalid behavior.

**Objects**: json-flag, structured-finding-output, stdin-model-input, domain-model-validate-path, exit-code-policy

**Constraints**:
- `--json` emits the exact `[]ValidationError` findings from `ValidateDomainModelStructured`, one entry per violation, never collapsed into a single aggregate error
- A clean model under `--json` emits `[]` and exits 0; a model with findings under `--json` still exits 0 (the list is the result), so callers branch on the list contents, not the exit code
- The path argument accepts `-` to read the model from stdin; stdin and a real path are mutually exclusive and produce the same finding set for identical bytes
- Each finding in the `--json` output carries its severity resolved for authoring mode (the editor's context, where `domain-operations-deprecated` is a warning); the build path applies build-mode severity separately and is unaffected by this flag
- `--json` is additive: the existing human-readable `validate` output and its non-zero-on-invalid exit behavior are unchanged when the flag is absent
- The JSON shape reuses the existing `ValidationError` field convention (code, message, path/context, fix, severity) so it matches the project-validation JSON already emitted elsewhere — no parallel finding schema

**Verify**:
- `echo '<clean model yaml>' | parlay validate --type domain-model --json -` prints `[]` and exits 0
- `echo '<model with two distinct violations>' | parlay validate --type domain-model --json -` prints a two-element JSON array, one entry per violation, and exits 0
- `parlay validate --type domain-model --json <path-to-invalid-file>` prints the same finding set as piping the same file's bytes on stdin
- `parlay validate --type domain-model <path-to-invalid-file>` (no `--json`) preserves today's human output and non-zero exit
- A malformed (unparseable) model under `--json` emits a finding with a stable parse-failure code rather than crashing or printing a non-JSON error

---

## Machine-usable element path on every finding

**Goal**: Ensure every structured finding carries an element path precise enough for a UI to anchor it to the exact entity, field, enum, enum value, or relationship — so a consumer can navigate a designer straight to the offending element, not just show a message.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: `ValidationError` today carries `Context` (a free-text locator) and `Fix`. A message and a fix answer "what's wrong" and "what to do," but a visual editor also needs "where" as a structured path it can resolve to a form control or diagram node — e.g. `entities.Order.fields.status`, `relationships.customer-orders.to`, `enums.Priority.values.2`. Whole-model findings (e.g. `missing-schema-version`) have no owning element; they need a distinguished top-level token rather than a blank locator. This path is what Studio's inline markers and finding-navigation are built on, so it is a contract, not a convenience.

**Action**: Give each finding a machine-usable element path — a dedicated field, or a `Context` guaranteed to hold the structured path — populated at every emission site in `ValidateDomainModelStructured` with a dotted path from the model root to the offending element. Findings that apply to the whole model use a single distinguished top-level token. Document the path grammar alongside the domain-model schema's validation table so consumers and Core agree on the shape.

**Objects**: element-path, path-grammar, whole-model-path-token, finding-emission-site

**Constraints**:
- Every finding emitted by `ValidateDomainModelStructured` carries an element path; none is blank
- The path is a stable dotted grammar rooted at the model (`entities.<name>.fields.<name>`, `relationships.<name>.<end>`, `enums.<name>.values.<index>`, etc.) documented with the schema's validation table
- Whole-model findings (no owning element) carry a single distinguished top-level token, not an empty string or a guessed element
- The path grammar is closed and versioned with the schema: a new rule that points at a new element kind extends the grammar in the same change that adds the rule
- Existing human-readable output stays legible — adding the path does not remove or garble the message and fix a terminal user already reads

**Verify**:
- A finding for a bad field type reports a path resolving to that field (`entities.<E>.fields.<F>`)
- A finding for an unknown relationship cardinality reports a path resolving to the relationship end (`relationships.<R>.<end>`)
- A `missing-schema-version` finding reports the distinguished whole-model token, not a blank or fabricated element path
- Two runs over the same model produce identical paths for the same violations (paths are deterministic)

---

## Emit `domain-operations-deprecated` in authoring mode

**Goal**: Actually raise `domain-operations-deprecated` when a domain model still carries a populated `operations:` block, so the warning the severity table already classifies becomes a finding a caller can surface — closing the gap where the code exists in policy but never fires.

**Persona**: Parlay Studio maintainer

**Priority**: P1

**Context**: `domain-operations-deprecated` is declared in Core's per-mode severity table (authoring = warning, build = error) but has no emission site — grep finds it only in the table and in spec markdown. So a model with a populated deprecated `operations:` block validates clean today, and no consumer (Studio's editor included) can warn a designer that the block needs migrating via `parlay migrate-domain-operations`. The deprecated block is otherwise preserved untouched (the editor's passthrough contract); the missing piece is only the warning that it is there.

**Action**: Add the check to the domain-model validator: when the parsed model's `operations:` block is present and non-empty, emit a `domain-operations-deprecated` finding with the schema's actionable fix message (migrate via `parlay migrate-domain-operations`). Its severity follows the existing table — warning in authoring mode, error in build mode — with no new severity logic; the finding flows through the classification already defined.

**Objects**: domain-operations-deprecated, operations-block-presence-check, authoring-mode-severity, migration-fix-message

**Constraints**:
- A model with a populated `operations:` block emits exactly one `domain-operations-deprecated` finding; a model without the block, or with an empty one, emits none
- The finding's severity is taken from the existing per-mode table (authoring = warning, build = error) — this intent adds an emission site, not a new severity rule
- The finding's fix message names the migration path (`parlay migrate-domain-operations`), matching the schema's actionable-fix convention
- Emitting the warning does not mutate, reorder, or drop the `operations:` block — detection is read-only, consistent with the block's structural-passthrough contract
- In build mode the finding is an error, so a populated deprecated block blocks the build exactly as the severity table already promises — this intent makes that promise real rather than latent

**Verify**:
- A model with a populated `operations:` block validated in authoring mode emits one `domain-operations-deprecated` warning finding
- The same model validated in build mode emits the same code at error severity, failing the build
- A model with no `operations:` block, or an empty one, emits no `domain-operations-deprecated` finding
- The `operations:` block is byte-for-byte unchanged after validation (detection is read-only)

---
