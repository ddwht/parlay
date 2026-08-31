# Domain Model Schema (`domain-model.yaml`) — authoring digest

Derived from `domain-model.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

```yaml
schema_version: 1     # required integer; first released value is 1
enums:        []      # optional list of DomainEnum
entities:     []      # optional list of DomainEntity
relationships: []     # optional list of DomainRelationship
operations:   []      # REMOVED in v0.3 — a populated block fails; see below
```

`schema_version` is **required** and **must be an integer**. Never
semver, never a string tag. Missing it fails parse with code
`missing-schema-version`. Newer-than-binary fails with
`schema-version-newer-than-binary` (run `parlay upgrade`).
Older-than-binary is routed in-memory through the per-version migrator
chain (`v1 → v2 → ... → binary`); the on-disk file is unchanged.
Unreachably-old (no migrator chain reaches binary) fails with
`schema-version-unreachable`.

---

```yaml
name: OrderStatus
values:
  - value: pending
    label: Pending          # optional, presentation
    tone: warning           # optional; closed set: neutral|info|warning|danger|success
```

- `name` (required) — referenced from entity field types via the
  `enum:` key on a `DomainField`.
- `values[].value` (required) — raw enum value (e.g. `paid`).
- `values[].label` (optional) — human-readable label.
- `values[].tone` (optional) — semantic tone for rendering. Closed set
  per the `enum-tone-vocabulary` decision; unknown tones fail with
  `enum-tone-outside-closed-set`. Adding a tone is a `schema_version`
  bump.

Presentation metadata (`label`, `tone`) lives on enums deliberately, as
an exception to the "domain is logic-only" rule, so designers can
preview rendering without an adapter round-trip.

---

```yaml
name: Order
fields:
  - name: id
    type: uuid
    required: true
  - name: status
    type: OrderStatus       # named-enum reference
    enum: OrderStatus
    required: true
  - name: customer_id
    type: ref
    target: Customer
    required: true
```

- `name` (required) — entity name; referenced by relationship endpoints
  and `ref`-typed fields.
- `fields` (required, may be empty) — list of `DomainField`.

---

```yaml
name: status
type: <closed-set>          # see below
target: Customer            # only when type=ref
enum: OrderStatus           # only when type names an enum
relationship: customer-orders  # optional; only on a ref field
required: true
```

`relationship:` names the declared `DomainRelationship` this ref field
realises. It is optional and carries no validation of its own — its use is to
settle which field implements which edge when two relationships connect the
same pair of entities. See "Cardinality is checked against the project's own
data" under `DomainRelationship`.

**Closed field-type set** (per the `entity-field-shape` decision):

| `type:`        | Notes                                                  |
| -------------- | ------------------------------------------------------ |
| `uuid`         | UUID v4 string                                         |
| `string`       | Free text                                              |
| `int`          | Signed integer                                         |
| `float`        | Floating-point number                                  |
| `bool`         | True/false                                             |
| `datetime`     | RFC 3339 timestamp                                     |
| `ref`          | Foreign-key reference; **must** also set `target:`     |
| `<enum-name>`  | Names a declared `DomainEnum`; **must** also set `enum:` |

Anything else fails deep validation with code
`field-type-outside-closed-set`. Inline object literals (nested
struct-shaped fields) are rejected with the same code; lift the nested
shape into a separate entity joined by a `ref`-typed field.

### v2-deferred: list-typed scalar/enum fields

The closed field-type set above has no way to express a field that holds
a **list of scalars or enum values** — e.g. `Task.tags: string[]` (a list
of free-text tags) or `Task.labels: [Priority]` (a list of enum values).
This surfaced as a real gap during Phase 4's domain-model migration: a
feature with a genuinely list-shaped scalar field had no expressible
target and had to be flattened or dropped rather than migrated
faithfully. It is **v2-deferred**, following the same posture as
`operation-kinds.schema.md`'s `subscription`/`job` deferral — reserved,
not silently unsupported: a `type: string[]`-style declaration fails
`field-type-outside-closed-set` today, and that failure should read as
"deferred to v2," not "malformed."

This is narrower than it might first look: a list of **entity
references** (e.g. `Task.watchers` naming several `User` entities) is
already expressible today — as a `DomainRelationship` with
`cardinality: one-to-many` or `many-to-many`, not as a field. The gap is
specifically scalar/enum lists, which have no relationship-based
workaround because there's no second entity to relate to.

### v2-deferred: state-machine constructs

There is no way to declare which transitions between a `DomainEnum`'s
values are valid — `DomainEnum` declares the closed set of values
(`pending`, `paid`, `shipped`, ...) but not which transitions between
them are allowed (`pending → paid` yes, `paid → pending` no). The legacy
buildfile shape (see `buildfile.schema.md`'s frozen v1 appendix) had an
informal `models.<Entity>.states.{values,transitions}` construct that
covered this; `domain-model.yaml` has no equivalent, and Phase 4's
migration hit features that relied on it with no expressible target.
**V2-deferred**, same posture as the list-typed-fields gap above: this
is a recognized, reserved gap, not an oversight. A future schema version
would need a new construct (something like a `transitions:` block on
`DomainEnum`, declaring the allowed `{from, to}` pairs) — sketching that
shape is out of scope for this consolidation; naming the gap explicitly
is what's in scope.

---

```yaml
name: customer-orders
from: Customer
to: Order
cardinality: one-to-many
```

- `from` and `to` must resolve to declared entities. Endpoints that
  reference an undeclared entity fail with `undeclared-entity-reference`.
- `cardinality` is drawn from the closed set
  `{one-to-one, one-to-many, many-to-one, many-to-many}`. Unknown values
  fail with `relationship-cardinality-unknown`.

### Cardinality is checked against the project's own data

`parlay internal check-composition` holds the composed runtime seed against the
cardinalities declared here. **Only `one-to-one` is checkable**, and the limit
is worth stating plainly, because a check whose reach is overstated gets
trusted further than it earns:

- `one-to-one` yields a constraint a scalar ref field can violate — at most one
  child may point at any given parent. Two that do is
  `composition-cardinality-violated`.
- `many-to-one` and `many-to-many` cannot be violated by counting at all.
- The "one" side of `one-to-many` is automatic: a scalar field holds one value.

**The join is inferred, and the inference can be declined.** Nothing here links
a relationship to the field that realises it — a relationship names no field,
and a field names no relationship — so the check looks for ref fields on the
`to` entity whose `target:` is the `from` entity. Exactly one candidate is
unambiguous. Zero or several produce `composition-cardinality-unresolvable`, a
note, rather than a guess: a check that misfires on a correct model is worse
than no check.

**`relationship:` settles it.** A ref field may name the relationship it
realises, which is how an author disambiguates when two relationships connect
the same pair of entities:

```yaml
entities:
  - name: Approval
    fields:
      - name: report
        type: ref
        target: ExpenseReport
        relationship: report-approval   # this field realises that edge
      - name: supersedes
        type: ref
        target: ExpenseReport           # a second edge between the same pair
```

The field is optional and only meaningful on a ref field. Without it the
target-based inference applies; with it, it wins.

Only records from each feature's **composing** fixture are counted — the data
the prototype actually boots with. A scenario fixture describes a state the
running app never enters, so its records cannot coexist with the seed's.

---

<!-- parlay-feature: parlay-tool/structured-domain-model-validation -->
<!-- parlay-component: cross-cutting/element-path-on-every-finding -->

Every finding emitted by deep validation carries a **machine-usable
element path** in its `context` field — a deterministic, dotted locator
for the offending value. The path is computed from the model position of
the offending value, so two runs over identical bytes produce identical
paths (no map-iteration order leaks into a path). No finding carries a
blank path.

The grammar is **closed and versioned** — it is extended only in the same
change that adds a validation rule pointing at a new element kind, and
that extension is a `schema_version` bump alongside the schema, exactly
like the closed type / tone / cardinality sets above.

| Element kind            | Path form                                          |
| ----------------------- | -------------------------------------------------- |
| Entity field type       | `entities.<name>.fields.<name>.type`               |
| Entity field ref target | `entities.<name>.fields.<name>.target`             |
| Entity field enum key   | `entities.<name>.fields.<name>.enum`               |
| Relationship end        | `relationships.<name>.<end>` — `<end>` ∈ {`from`, `to`, `cardinality`} |
| Enum value tone         | `enums.<name>.values.<value>.tone`                 |
| Enum (whole)            | `enums.<name>`                                     |
| Entity (whole)          | `entities.<name>`                                  |
| Operation input         | `operations.<name>.input[<index>]`                 |
| Whole model (ownerless) | `<domain-model>`                                   |

The dotted paths always root at `entities.` / `relationships.` / `enums.`
/ `operations.`. The whole-model token `<domain-model>` is
angle-bracketed so it can never collide with a real dotted path; it is
used for findings that are not attributable to a single element — parse
failure (`invalid-yaml`), a missing or non-integer `schema_version`
(`missing-schema-version`), a newer-than-binary or unreachable version
(`schema-version-newer-than-binary`, `schema-version-unreachable`), and
the removed-operations-block finding (`domain-operations-unsupported`),
whose block spans the whole model.
