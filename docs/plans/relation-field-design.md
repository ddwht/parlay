# Design: `relation:` + computed expectations + fixture oracle (WP9)

Status: **proposal, awaiting a decision** — WP9 implementation is gated on the choices below.
Scope: the C/D half of Theme 1 in [improvement-solutions.md](improvement-solutions.md), plus
the fixture-aware backstop lints that [WP4](improvement-implementation-plan.md) deliberately
deferred. This document proposes a design with options and a recommendation for each axis; it
implements nothing.

---

## 1. Why this is design-doc-first

Theme 1 ("green must mean verified") named five root causes. WP4 shipped the cures for three of
them and a transitional honesty marker for a fourth:

- **A — criterion-driven cases.** A case exists because a `verify:` criterion demands it; the
  never-built route/flow enumeration walker was retired (`661de2e`, `4309338`, `0abb5fc`).
- **B — machine-readable claims.** A case declares `exercises:` (targets its steps must mutate)
  and `observes:` (targets its expectations read); `testcases-case-vacuous` and
  `testcases-case-claims-unmet` make a ceremony test unauthorable rather than merely detectable
  (testcases.schema.md §"Criterion-driven cases").
- **E — expression-gap honesty.** `coverage: state-only` records that a display-shaped criterion
  compiled down to a store assertion (testcases.schema.md §"The `appears` step"); WP8 added the
  `appears` vocabulary that lifts the stamp when an adapter can deliver it.

Two causes remain **uncured**, and they are the two WP9 owns:

- **Cause 1 — one author, one pass.** The build phase still invents a case's steps *and* its
  expected values together. Self-agreement is treated as correctness. The only cure is to derive
  the expected value from an independent source (solutions option **C**).
- **Cause 4 — fixtures are invented literals with no oracle.** Nothing derives a fixture value
  from a declared rule, so an arithmetically impossible fixture passes every gate (solutions
  option **D**).

The solutions decision (improvement-solutions.md §"Decision") staged C and D behind a
`relation:` field because "they are the same investment (structured relations) seen from two
sides." C computes an *expectation* from a fixture; D computes a *fixture field* from other
fixture fields. Both need the same thing: a declared, machine-evaluable relation between values.
Once that relation exists, two of WP4's backstop lints gain their fixture-aware halves — the
`unsatisfiable case` and `assertion/fixture divergence` checks that WP4 explicitly deferred here
(improvement-implementation-plan.md WP4.5).

The reason to design before building: the relation is a **new expression language** entering the
spec surface, and its home, its grammar, and its evaluation model are one-way doors. A grammar
that is too weak silently downgrades (the exact failure Theme 1 is about); one that is too strong
becomes a second programming language the pipeline must interpret, and every adapter must agree
on. The decision below is which point on that spectrum WP9 commits to.

---

## 2. The four pieces and how they compose

```
                    ┌──────────────────────────────────────────┐
                    │  relation:  (a declared, pure expression) │
                    └───────────────┬───────────────┬──────────┘
                    evaluate against │               │ evaluate against
                    fixture inputs   │               │ fixture + step effects
                                     ▼               ▼
                        ┌────────────────────┐  ┌────────────────────────┐
                        │  D. fixture oracle │  │  C. computed expectation│
                        │  derived-from: /   │  │  a verify.expected that │
                        │  asserted:         │  │  the validator recomputes│
                        └─────────┬──────────┘  └──────────┬─────────────┘
                                  │                        │
                                  ▼                        ▼
                        fixture-unsatisfiable      testcases-case-unsatisfiable
                        (the D backstop)           testcases-assertion-divergence
                                                   (the WP4 fixture-aware backstop)
```

The relation is the shared primitive. D applies it *within* a fixture (a total field derived from
line-item fields). C applies it *across* a case (an expected count derived from the fixture the
suite renders plus the steps that mutate it). The backstop lints are what fires when a
hand-authored literal disagrees with what the relation computes.

---

## 3. Axis 1 — where a relation lives

The plan's one-line sketch says "structured `relation:` expressions on infrastructure fragments."
That is a defensible default but not the only home, and the choice materially changes what a
relation can reference.

### Option 1A — on infrastructure fragments (the plan's default)

A fragment's `**Invariants**:` bullets are already "declarative, testable properties"
(infrastructure.schema.md:50) — prose today. Add a structured sibling: a fragment may carry a
`**Relations**:` block whose entries are named, evaluable expressions.

```
## Report total integrity
**Affects**: expense report aggregation
**Behavior**: A report's total equals the sum of its line-item amounts.
**Relations**:
- report-total: ExpenseReport.total == sum(LineItem.amount where LineItem.report == ExpenseReport.id)
**Source**: @expenses/report-total
```

- **For:** matches the plan; infrastructure fragments are already the home for "properties that
  must hold" and already seed testcases (infrastructure.schema.md:151); no new artifact.
- **Against:** infrastructure fragments are framework-agnostic *architectural prose* and carry a
  portability lint that forbids implementation vocabulary. A relation is neither architectural nor
  prose — it is a domain fact about entities (a report's total). Housing arithmetic over
  domain entities in the "shape of the source tree" artifact is a category error that will
  confuse the very lint that keeps that artifact clean.

### Option 1B — on the domain model (derived fields)

A relation that says "total == sum(amounts)" is a statement about the `ExpenseReport` entity. The
domain model already owns entities, fields, and relationships (domain-model.schema.md), and
already reserves state-machine and list-field constructs as v2-deferred. A derived field is the
same shape of extension: a `DomainField` gains an optional `derived:` expression.

```yaml
entities:
  - name: ExpenseReport
    fields:
      - name: total
        type: float
        derived: sum(LineItem.amount where LineItem.report == self.id)
```

- **For:** a derived field is a domain fact and belongs where the entity is defined; the
  expression can reference declared relationships by name (the `relationship:` field already
  disambiguates edges, domain-model.schema.md:146-154); one home for both the fixture oracle (D)
  and any expectation that reduces to "this entity field equals this formula."
- **Against:** not every relation is a single-entity derived field. "The rendered row count equals
  the number of submitted reports" is a relation between a *view* and a *filtered entity set* — it
  is not a field on any entity. The domain model cannot express a relation whose left side is a
  surface fact.

### Option 1C — a per-scope relation, homed with the thing it constrains (recommended)

Neither "always infrastructure" nor "always domain" fits, because relations come in two shapes and
the right home differs by shape:

1. **Domain relations** — the left side is an entity field. `ExpenseReport.total == sum(...)`.
   Home: a `derived:` expression on the `DomainField` (Option 1B). This is the fixture oracle's
   natural anchor: a fixture field for a derived entity field is recomputed from that field's
   `derived:` rule.
2. **Surface/observation relations** — the left side is something a case *observes* (a rendered
   count, a displayed text, an `appears content` fact). Home: the `verify:` criterion on the
   surface fragment or capability operation, extended with an optional `compute:` expression that
   states how the observed value is derived from the fixture. This is where computed expectations
   (C) anchor, because a case already cites its criterion by `ref` (testcases.schema.md:114).

Under 1C, "infrastructure fragment relations" from the plan become the *third, residual* case: a
relation that is genuinely architectural (a probe result, an allowlist cardinality) and reduces to
neither an entity field nor an observation stays on the fragment as Option 1A describes — but that
is the rare case, not the default.

- **For:** each relation lives next to the thing it is a fact about, so it can reference that
  thing's vocabulary without a cross-artifact leak; the fixture oracle and computed expectations
  each get a clean anchor; the portability lint on infrastructure fragments stays meaningful.
- **Against:** three homes is more surface than one. The mitigation is that the *grammar* (Axis 2)
  is identical across all three — only the anchor differs — so an author learns one expression
  language, and the validator has one evaluator.

**Recommendation: 1C**, with 1B as the anchor that ships first (it is the fixture oracle's home and
the smallest coherent slice). If the decision is to minimize surface, fall back to **1B only** for
WP9 and leave surface/observation relations (C's full generality) to a follow-up — 1B alone still
cures cause 4 (the fixture oracle) and the domain half of cause 1. Recording this as the
conservative fallback because it is the slice with the clearest home and the least new vocabulary.

---

## 4. Axis 2 — the relation expression language

This is the one-way door. Options run from "almost nothing" to "a real engine."

### Option 2A — a closed, pure mini-DSL (recommended)

A deliberately small, side-effect-free expression grammar the validator evaluates itself:

- **References:** `Entity.field`, `self.field`, and (for surface relations) a criterion's declared
  `observes:` target.
- **Aggregations over a related set:** `sum`, `count`, `min`, `max`, `avg`, with a
  `where <field> == <value>` filter and relationship-name join (`where LineItem.report == self.id`
  resolves through the declared `DomainRelationship`).
- **Arithmetic:** `+ - * /` on numeric fields; string equality/concatenation for text.
- **Comparison:** the top-level of an *invariant* relation is a boolean (`==`, `!=`, `<`, `>`,
  `<=`, `>=`); the top-level of a *derivation* relation is a value expression.
- **No** conditionals beyond a single `where` filter, **no** user-defined functions, **no**
  iteration constructs, **no** date arithmetic beyond equality (deferred, like list-typed fields).

- **For:** deterministic and framework-independent — the validator computes the answer with no
  engine, no adapter cooperation, and no inference, which is exactly why the solutions doc rejected
  the two-agent adversarial pass ("pays inference cost to approximate what declared relations do
  deterministically") and mutation testing. It covers the observed defects: the benchmark's
  arithmetically-impossible fixture and the "rendered row count vs fixture" class both reduce to
  `count`/`sum` over a filtered set.
- **Against:** a closed grammar cannot express a genuinely computed value (a currency conversion, a
  tax table, a layout algorithm). Those must fall to 2C or stay `asserted:`. This is acceptable:
  the grammar's job is to catch self-agreement on the *checkable* majority, not to re-derive the
  whole engine.

The escape hatch is the honest one Theme 1 already uses: a value the DSL cannot derive is marked
`asserted:` with a reason (Axis 3), which is on the record for a reviewer rather than silently
downgraded.

### Option 2B — an existing embeddable expression library (e.g. CEL/expr)

Adopt a general expression evaluator instead of hand-rolling a grammar.

- **For:** more expressive out of the box (conditionals, macros); someone else maintains the
  parser.
- **Against:** a new third-party dependency in a toolchain that pins dependencies deliberately
  (WP-adjacent ground rules); the expressible set becomes "whatever the library does," which is
  unbounded and therefore un-reviewable — the spec surface stops being closed-vocabulary, which is
  the property every other Parlay artifact holds. The determinism argument for 2A holds for 2B
  too, but 2B trades a reviewable closed grammar for an open one. Rejected unless the closed DSL
  proves too weak in practice.

### Option 2C — a real engine call (solutions doc's "or a real engine")

`derived-from:` names an actual backend operation; the validator executes it to get the value.

- **For:** the only source that is *truly* independent of the spec author — the ground truth is
  running code.
- **Against:** there is no execution harness at validate time, and building one is a project of its
  own (the `appears content` work already drew the pixels/execution line deliberately at scene-
  graph facts, not runtime). It also inverts the dependency: the fixture would depend on generated
  code existing, but fixtures are authored at build-feature time before code exists. **Defer.**
  Reserve `derived-from:` syntax so that an engine-backed source is a future value of the same
  field, following the `subscription`/`job` reserved-not-unsupported posture
  (operation-kinds.schema.md, cited domain-model.schema.md:196).

**Recommendation: 2A**, with 2C's `derived-from:` shape reserved so a future engine oracle slots in
without a grammar change. Record `asserted:` as the explicit, reviewer-visible escape for anything
2A cannot compute.

---

## 5. Axis 3 — the fixture oracle (`derived-from:` / `asserted:`)

Cause 4's cure: every fixture field value is one of two things, and which one is on the record.

Today a fixture is a bag of literals (buildfile.schema.md:703-709, testcases.schema.md:227-238):

```yaml
fixtures:
  three-reports-mixed-status:
    composes: true
    data:
      ExpenseReport:
        - id: aaaa...
          total: 300.00        # is this right? nothing checks.
          submitted-count: 2   # does it match the rows? nothing checks.
```

### Proposed shape

A fixture field carries provenance. Two spellings, mutually exclusive:

```yaml
data:
  ExpenseReport:
    - id: aaaa...
      total:
        derived-from: report-total     # recomputed from the derived: rule; literal is a cache
      status:
        asserted: "root input — the scenario under test is a submitted report"
        value: submitted
```

- **`derived-from: <relation-name>`** — the validator recomputes the field from the named relation
  (Axis 1's `derived:` for a domain field) against the fixture's other fields and asserts the
  stored literal matches. A mismatch is `fixture-unsatisfiable` (D's backstop). The stored literal
  is retained as a cache so a human reading the fixture sees the value, but it is not trusted.
- **`asserted: <reason>`** — a hand-set root input. The reason is mandatory and on the record. A
  fixture made entirely of asserted roots is legal (some data has no derivation); the point is that
  *nothing derivable is silently hand-set*.
- **A derived field with a bare literal and no marker** draws `fixture-oracle-missing` — a
  **warning** during the transition (every fixture predates the field), escalating to error only
  by a later decision, mirroring how `testcases-case-criterion-missing` landed as a warning.

### Options within Axis 3

- **3A — verbose per-field mapping (above).** Every field that is derived or explicitly-asserted
  says so. Clearest provenance; most churn in fixture files.
- **3B — fixture-level default + per-field override.** A fixture declares `oracle: strict` and then
  only `asserted:` fields need a marker; everything else is assumed `derived-from` its field's
  `derived:` rule if one exists. Less churn; the default is invisible, which is the readability
  cost Theme 1 keeps warning about.
- **3C — marker only on divergence-eligible fields.** Only fields whose domain field carries a
  `derived:` rule are subject to the oracle; plain fields need no marker at all. Smallest surface —
  a field can only be `derived-from` if there is something to derive it from.

**Recommendation: 3C.** The oracle should bind exactly where a relation exists to check against: a
field with a `derived:` rule (Axis 1B) *must* be `derived-from` or carry an `asserted:` override
(with reason) explaining why this scenario deliberately breaks the derivation; a field with no rule
needs no marker. This keeps fixture churn proportional to the number of derived fields (few) rather
than to the number of fields (many), and it makes the oracle's scope exactly "the fields we can
check" — no ceremony markers on fields nothing could recompute.

---

## 6. Axis 4 — computed expectations in testcases

Cause 1's cure: a case's `expected:` value can be *computed*, not authored.

A case already declares `observes:` (the targets its expectations read) and cites a `criterion`
(testcases.schema.md:114-116). Extend a `verify:` step so its `expected:` may be derived:

```yaml
- verify: count
  target: report-row
  expected:
    derived-from: visible-report-count   # a surface relation: count(ExpenseReport where status==submitted)
```

The validator computes `visible-report-count` against the suite's fixture and asserts the
authored/generated `expected:` (if any) matches. Three sub-options:

- **4A — expectation is *only* derived.** The case names the relation; there is no authored
  literal at all; codegen materializes the computed value into the generated test. Strongest
  anti-self-agreement (there is no author-chosen number to agree with), but it means a suite is
  unreadable without running the evaluator, and a relation bug produces a wrong test that still
  "passes" its own derivation.
- **4B — expectation is authored *and* derived; validator checks agreement (recommended).** The
  case carries both a literal and `derived-from:`; the validator recomputes and fires
  `testcases-assertion-divergence` when they disagree. This is the direct catch for cause 1: the
  same agent that invented the fixture also wrote the expected literal, and now an *independent*
  recomputation from the fixture's own declared relation must agree with it. Self-agreement is no
  longer sufficient because the relation is the second, non-authored voice.
- **4C — no computed expectations; rely only on the fixture oracle (D) + WP4's A+B.** Cheapest;
  leaves cause 1 uncured for anything that is not a fixture field. Rejected as the primary path but
  worth naming as the floor if Axis 1 collapses to 1B-only.

**Recommendation: 4B.** Keep the authored literal (readability, and it is what codegen emits) but
require it to survive an independent recomputation. This is the smallest change that makes
"self-agreement accidentally failed" into "self-agreement is *checked* against a declared
relation." A case whose expectation genuinely cannot be derived keeps a bare literal and is exempt
from the divergence check — same honest escape as `asserted:`.

---

## 7. Axis 5 — the WP4 fixture-aware backstop lints

WP4.5 deferred "the fixture-aware unsatisfiable/divergence checks" here. With Axes 1–4 in place
they are the natural consequences, run at audit time over pre-existing testcases (the backstop
half, not the authoring half):

| Proposed code | Fires when | Severity |
|---|---|---|
| `fixture-unsatisfiable` | A `derived-from:` fixture field's stored literal does not equal the relation recomputed from the fixture's other fields — the fixture is arithmetically impossible. | **error** (a fixture that violates its own declared derivation is never intentional) |
| `testcases-assertion-divergence` | A `verify:` step carries both a literal `expected:` and a `derived-from:` relation, and they disagree — the authored expectation contradicts what the fixture+relation compute. | **error** |
| `testcases-case-unsatisfiable` | A case's asserted expectation cannot be produced by its declared `exercises:` steps applied to its fixture under the governing relation — no fixture value makes the assertion reachable. | **error** |
| `fixture-oracle-missing` | A fixture field for a domain field that carries a `derived:` rule has a bare literal with neither `derived-from:` nor `asserted:`. | **warning** (transitional, per §5) |
| `relation-unparseable` | A `derived:`/`derived-from:`/`compute:` expression is outside the closed grammar (Axis 2A). | **error** (an unparseable relation checks nothing and must not pass silently) |

Note the naming: `testcases-assertion-divergence` is distinct from the existing
`composition-scenario-fixture-divergence` (a cross-feature note, testcases.schema.md:257) — the
new code is intra-case, the existing one is cross-feature. Keep both names; do not overload
"divergence."

The **models-vs-contract gate** named in the solutions decision (improvement-solutions.md:68) is
the same machinery pointed at a third pair: a fixture field typed by the domain model against the
`verify:` contract that reads it. It is in scope as a backstop but adds no new *primitive* — it is
`fixture-unsatisfiable` evaluated with the domain model as the relation source. Recommend folding
it into `fixture-unsatisfiable` rather than minting a separate code, and recording that decision.

---

## 8. Meta-test and ground-rule obligations the implementation inherits

Whichever options are chosen, WP9 implementation must satisfy the binding ground rules. Recording
them here so the decision is made with the cost visible:

1. **New warning-severity codes** (`fixture-oracle-missing`, and any other warning) need a
   `ruleSeverityTable` entry in `core/internal/agent/validation_mode.go:44` **and** a `(warning)`
   marker on their schema table rows — `severity_doc_test.go` (validation_mode.go's companion,
   agent/severity_doc_test.go:50-60) fails otherwise.
2. **New error codes** must be *emitted by real source*, not just documented: `conformance_test.go`
   asserts every documented code is reachable (conformance_test.go:255-261), and
   `knownUnimplementedCodes` (conformance_test.go:50) **only shrinks**. A relation code documented
   ahead of its emitter would have to be implemented in the same commit — it may not be parked in
   the allowlist.
3. **Codes go in tables, not prose** — `repoSchemaCodes` reads markdown tables only
   (conformance_test.go:74-85). Every new code above is specified as a table row for exactly this
   reason.
4. **Meta-test lockstep** — any schema sentence an audit/conformance test pins moves in the same
   commit as the test's pin (ground rule 3).
5. **Source-first dogfooding** — the schema and skill edits land under
   `core/internal/embedded/{schemas,skills}/`, then `make build-noui && ./parlay upgrade &&
   make verify-skills`; after schema edits, regenerate the digest
   (`./parlay internal schema-digest --format md > .parlay/schemas/DIGEST.md`).
6. **build-feature.skill.md** gains the authoring instructions: derive expected values via
   `derived-from:` where a relation exists (§6), stamp `asserted:` with a reason where it does not
   (§5), and never hand-set a derived fixture field silently.

---

## 9. Transition posture

Every fixture and testcases file predates these fields. Follow the precedent WP4 set with
`testcases-case-criterion-missing` and `testcases-file-missing`: the *missing-marker* codes
(`fixture-oracle-missing`) land as **warnings** so existing files validate, while the
*contradiction* codes (`fixture-unsatisfiable`, `testcases-assertion-divergence`,
`testcases-case-unsatisfiable`) are **errors from day one** — a file that opts into a relation and
then contradicts it is not legacy, it is wrong. This mirrors how `composition-fixture-contradiction`
is an error while `...missing-legacy` is a warning: the presence of the new structure is optional;
its internal coherence, once present, is not.

---

## 10. Recommendation summary

| Axis | Recommendation | Conservative fallback if surface must shrink |
|---|---|---|
| 1 — relation home | 1C (home each relation with the thing it constrains) | 1B only (domain `derived:` fields) — cures cause 4 + domain half of cause 1 |
| 2 — expression language | 2A (closed pure mini-DSL), reserve 2C's `derived-from:` engine shape | 2A, unchanged |
| 3 — fixture oracle | 3C (marker only where a `derived:` rule exists) | 3C, unchanged |
| 4 — computed expectations | 4B (authored + derived, checked for agreement) | 4C (oracle only) if Axis 1 collapses to 1B |
| 5 — backstop lints | Five codes above; fold models-vs-contract into `fixture-unsatisfiable` | same |

The through-line: **one closed expression grammar, evaluated by the validator with no engine and no
inference, anchored next to the thing each relation is a fact about, with an explicit `asserted:`
escape for anything it cannot compute.** That is the minimum that turns "self-agreement is
correctness" into "self-agreement is checked against a declared, independent relation" — the exact
cure Theme 1 costed for causes 1 and 4.

---

## 11. Open questions for the decision gate

1. **Home (Axis 1):** full 1C, or 1B-only for WP9 with surface relations deferred? This is the
   single biggest scope lever.
2. **Grammar ceiling (Axis 2):** is `sum/count/min/max/avg + where-join + arithmetic + comparison`
   the right closed set, or is a narrower set (aggregation + comparison only, no free arithmetic)
   enough for the observed defects? Narrower is safer to widen later than to shrink.
3. **`fixture-oracle-missing` escalation:** ship as a permanent warning, or as a warning with a
   named future commit that promotes it to error once fixtures are backfilled (the WP4 precedent
   left this implicit)?
4. **Engine oracle (2C):** reserve the `derived-from:` syntax now even though no engine exists, or
   keep the field purely relation-valued until a harness is real? Reserving costs a sentence;
   not reserving costs a grammar change later.
5. **models-vs-contract:** separate code or folded into `fixture-unsatisfiable`? Recommended
   folded; the gate wants to confirm no reviewer needs the two distinguished in output.
