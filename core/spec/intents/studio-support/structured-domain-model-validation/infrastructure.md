# Structured Domain-Model Validation Output — Infrastructure

---

## JSON validation mode for domain-model validate

**Affects**: validate command output mode, domain-model validation routing, stdin model input, exit-code policy
**Behavior**: The `parlay validate --type domain-model` command gains an additive `--json` mode. When the flag is set, the command routes the model through the structured (per-violation) domain-model validation path rather than the human path that collapses every violation into one synthetic aggregate error, and emits the full finding list as a JSON array — one entry per violation, never collapsed. Each finding reuses the existing structured-finding field convention (code, message, structured path, fix, severity) so the shape matches the JSON already emitted by project validation; no parallel finding schema is introduced. The path argument accepts `-` to read the model bytes from standard input, so a caller can validate an in-memory draft that never touches disk; stdin and a real path are mutually exclusive and produce identical findings for identical bytes. Under `--json`, findings carry severity resolved for authoring mode. Exit is `0` whether or not findings are present — the finding list is a query result, not a command failure. The human-readable path (flag absent) is unchanged: its collapsed output and non-zero-on-invalid exit behavior are preserved. Unparseable input under `--json` produces a finding with a stable parse-failure code rather than a crash or a non-JSON error on stdout.
**Invariants**:
- A clean model piped on stdin under `--json` prints `[]` and exits `0`
- A model with two distinct violations piped on stdin under `--json` prints a two-element JSON array (one entry per violation) and exits `0`
- Validating a model from a real path under `--json` produces the same finding set as piping the same bytes on stdin
- The path argument `-` reads the model from stdin; a real path and stdin are mutually exclusive
- With `--json` absent, the command preserves today's human-readable output and its non-zero exit on an invalid model
- An unparseable model under `--json` emits a finding carrying a stable parse-failure code, and the output remains valid JSON
- Findings emitted under `--json` carry authoring-mode severity; the build path resolves build-mode severity separately and is unaffected by this flag
**Source**: @studio-support/structured-domain-model-validation/json-validation-mode-for-parlay-validate-type-domain-model
**Caching**: none — validation runs per invocation over the supplied bytes
**Backward-Compatible**: yes — `--json` is purely additive; the existing human path, its output, and its exit behavior are unchanged when the flag is absent.

**Notes**:
- This extends an established output convention: the `--json` project-validation path already emits structured findings with the same field convention. The gap this closes is that `--type domain-model` previously routed through the collapsing single-error variant even when `--json` was requested.
- The stdin path (`-`) is the contract Studio's domain-model editor consumes: its draft lives only in memory and must be validatable without a file write.

---

## Machine-usable element path on every finding

**Affects**: structured-finding element path, domain-model path grammar, finding emission sites, schema validation-table documentation
**Behavior**: Every finding emitted by the structured domain-model validation path carries a machine-usable element path — a dotted locator, rooted at the model, precise enough for a consumer to anchor the finding to the exact entity, field, enum, enum value, or relationship end it concerns (for example `entities.<name>.fields.<name>`, `relationships.<name>.<end>`, `enums.<name>.values.<index>`). The path is populated at every emission site; none is blank. Findings that apply to the whole model and have no owning element carry a single distinguished top-level token rather than an empty string or a guessed element. The path grammar is closed and versioned alongside the domain-model schema: it is documented next to the schema's validation table so consumers and Core agree on its shape, and a new rule that points at a new element kind extends the grammar in the same change that adds the rule. Adding the structured path does not remove or garble the message and fix that a terminal user already reads.
**Invariants**:
- A finding for a bad field type reports a path resolving to that field (`entities.<E>.fields.<F>`)
- A finding for an unknown relationship cardinality reports a path resolving to the relationship end (`relationships.<R>.<end>`)
- A whole-model finding (for example a missing schema version) reports the distinguished top-level token, not a blank or fabricated element path
- Two runs over the same model produce identical paths for the same violations (paths are deterministic)
- No finding emitted by the structured path carries a blank element path
- The path grammar is documented adjacent to the domain-model schema's validation table; extending it to a new element kind is part of the same change that adds the rule pointing there
**Source**: @studio-support/structured-domain-model-validation/machine-usable-element-path-on-every-finding
**Caching**: none — paths are computed at emission time from the model position of the offending value
**Backward-Compatible**: yes — the existing message and fix are retained; the element path is added alongside them.

**Notes**:
- The path grammar is a contract, not a convenience: Studio's inline markers and finding-navigation resolve it to form controls and diagram nodes. Closing and versioning it with the schema is what keeps consumer and producer from drifting.
- Whether the path lives in a dedicated field or in a `Context` guaranteed to hold the structured path is a build-time decision; the invariant is that the structured path is present and resolvable on every finding.

---

## Emit domain-operations-deprecated in authoring mode

**Affects**: domain-model validation pipeline, deprecated-operations detection, per-mode severity classification
**Behavior**: The domain-model validation path gains a check that raises the `domain-operations-deprecated` finding when a parsed model still carries a populated deprecated `operations:` block. The finding's fix message names the migration path (`parlay migrate-domain-operations`), matching the schema's actionable-fix convention. The finding's severity is taken from the existing per-mode severity table — a warning in authoring mode, an error in build mode — with no new severity logic; this change adds an emission site, not a new severity rule. Detection is read-only: emitting the finding does not mutate, reorder, or drop the `operations:` block, consistent with the block's structural-passthrough contract. Because build mode classifies the code as an error, a populated deprecated block blocks the build exactly as the severity table already promises — making a promise that was previously latent (the code was declared in the table but had no emission site) real.
**Invariants**:
- A model with a populated `operations:` block emits exactly one `domain-operations-deprecated` finding; a model without the block, or with an empty one, emits none
- The same model validated in authoring mode emits the finding at warning severity, and in build mode at error severity, failing the build
- The finding's fix message names `parlay migrate-domain-operations`
- The `operations:` block is byte-for-byte unchanged after validation — detection is read-only
- The severity is drawn from the existing per-mode table; no new severity rule is introduced by this emission site
**Source**: @studio-support/structured-domain-model-validation/emit-domain-operations-deprecated-in-authoring-mode
**Caching**: none — the presence check runs as part of validation over the supplied model
**Backward-Compatible**: yes — the code was already declared in the severity table with no emitter; this adds the emitter. A model that validated clean only because the check was missing will now surface the finding it always should have, which is the intended correction rather than a regression.

**Notes**:
- The severity classification lives in Core's existing per-mode severity table; this fragment supplies the missing emission site, not a change to the table.
- This is the Core half of a parity contract: Studio's domain-model editor validation relies on the warning firing in authoring mode so it can prompt a designer to migrate the deprecated block.

---
