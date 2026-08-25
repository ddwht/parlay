# Testcases Schema — authoring digest

Derived from `testcases.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

```yaml
feature: <feature-slug>
framework: <test framework — e.g., Cypress, Playwright, Jest + Testing Library>

suites:
  - name: <suite name>
    file: <path this suite's code is written to — the plan row scaffold-plan derived>
    component: <component-name from buildfile>
    fixture: <fixture-name from buildfile>
    intent: <@feature/intent-slug — the intent this suite validates>

    cases:
      - name: <test case name>
        description: <what this test verifies>
        criterion:
          ref: "@<feature>/<kind>:<name>"   # the verify: entry this case discharges
          text: <the criterion's text, pinned so drift is visible>
        exercises: [<target the steps must mutate>, ...]   # what this case acts on
        observes: [<target the expectations read>, ...]     # what this case asserts on
        coverage: full            # or state-only when a display criterion compiled to store assertions
        steps:
          - action: <render | click | input | select | navigate | wait>
            target: <element name, action name, or route path>
            value: <input value, selection value — when applicable>
          - verify: <one of the Verifications table below — element, text, state, route, ...>
            target: <element name, data property, or route path>
            expected: <expected value, count, or state>

  - name: <suite name>
    component: <component-name>
    fixture: <fixture-name>
    cases:
      - name: <state transition test>
        steps:
          - action: click
            target: <action-name>
          - verify: state
            target: <EntityName>.<state-field>
            expected: <new state value>
```

---

### Actions

| Action | Target | Value | Description |
|---|---|---|---|
| render | component-name | — | Render the component with the suite's fixture |
| click | action-name or element-name | — | Click a button, link, or interactive element |
| input | element-name | string | Type into an input field |
| select | element-name | option value | Select from a dropdown or option list |
| navigate | route path | — | Navigate to a URL |
| wait | condition description | — | Wait for async operation or animation |
| appears | component or element name | `mounted` \| `output` \| `content` | Assert the target reached the renderer at the given depth (see § The `appears` step). The level is the step's `value:`. |

### Verifications

| Verify | Target | Expected | Description |
|---|---|---|---|
| element | element-name | — | Element exists in the rendered output |
| text | element-name | string | Element displays this text content |
| visible | element-name | true/false | Element is visible/hidden |
| enabled | element-name | true/false | Element is enabled/disabled |
| count | element-name | number | Number of rendered instances (for lists/tables) |
| state | EntityName.field | value | Model state has this value |
| route | — | path | Current route matches this path |
| class | element-name | class-name | Element has this CSS class (for design system variants) |
| file-exists | file-path | — | File or directory exists at the specified path |
| file-content | file-path | value or object | File contains the expected content (string match or structured comparison) |
| directory-exists | directory-path | — | Directory exists at the specified path |
| error | action-name | error message | Action produces the expected error |
| hidden | element-name | — | Element is absent or not visible. The negative of `visible`; both spellings are accepted because generated suites use each. |
| disabled | element-name | — | Element is present but not interactive. The negative of `enabled`, same reasoning as `hidden`. |

**This table is the authoritative list.** The example above abbreviates it, and for a while the two disagreed: the example carried `hidden`/`disabled` that the table omitted, the table carried `class`, `file-exists`, `file-content`, `directory-exists` and `error` that the example omitted, and generated suites used terms from both — so neither list on its own described what the tool produces. `testcases-unknown-term` validates against this table. If a term belongs in the vocabulary, it goes here.

---

- One suite per component + fixture combination
- Component name must match a component in buildfile.yaml
- Fixture name must match a fixture in buildfile.yaml
- Intent must reference the source intent via `@feature/intent-slug` for traceability
- Each case tests one behavior or state

---

Tests should cover these categories (derived from buildfile):

1. **Rendering** — component displays correct data from fixture
2. **Elements** — all elements defined in buildfile are present and bound correctly
3. **Visibility** — conditional elements appear/hide based on `visible-when` conditions
4. **Actions** — each action triggers its defined effect
5. **State transitions** — entity state machines transition correctly
6. **Navigation** — route changes work as defined
7. **Edge cases** — derived from acceptance criteria and intent Questions (empty states, error conditions, boundary values)

---

A case exists because a criterion demands it. Three per-case fields make that reason machine-checkable rather than prose:

- **`criterion:`** — `{ref, text}`. `ref` names the contract entry, in `@<feature>/<kind>:<name>` form (`kind` is `operation` or `fragment`); `text` pins **which of that entry's `verify:` bullets** the case discharges, and a later edit to the contract shows up here as `testcases-criterion-text-drift`. Both halves are load-bearing: the pair is the criterion's identity, and a `ref` with no `text` cannot say which bullet it covers (`testcases-criterion-text-missing`). A case in a v2 suite with no `criterion:` at all draws `testcases-case-criterion-missing` — a **warning** while the field lands, because every testcases.yaml predates it.
- **`exercises:`** — the targets the case's steps must mutate. If a case declares `exercises:` and none of those targets appears as a step `target:`, the case acts on nothing it claims to and draws `testcases-case-vacuous`. This is what makes a ceremony test — one that satisfies a coverage count while asserting nothing — unauthorable rather than merely detectable.
- **`observes:`** — the targets the case's expectations read. If a `verify:` step reads a `target:` outside the declared `observes:`, the case asserts on something its declaration does not admit and draws `testcases-case-claims-unmet`. The declaration and the mechanics cannot silently diverge.

`exercises:` and `observes:` are optional — a case that declares neither is checked only for the criterion warning — but once declared they are held against the steps. This is the A+B half of the "green must mean verified" design: a test exists because a criterion demands it (A) and states its claim checkably (B).

**`coverage:`** — `full` (default) or `state-only`. A display-shaped criterion ("the viewport shows the mesh") whose adapter cannot deliver `appears` yet compiles down to a store-level assertion ("the store holds the mesh"). Stamping `coverage: state-only` records that the downgrade happened, so the coverage reviewer sees a weaker claim instead of a silent one (part E). An unknown value draws `testcases-coverage-unknown`. The stamp **lifts** to `full` the moment the adapter declares support for the level the criterion needs (see § The `appears` step) — the same criterion then compiles to a real `appears` assertion instead of a store proxy.

---

A store assertion cannot see composition. The four composition defects the benchmark surfaced — a cache serving a stale mesh, an unmounted component, a dead input, an importer that never presents — all left the store correct; only a render-level fact would have caught them. `appears` is that fact, at three depths:

| Level | Asserts | Catches |
|---|---|---|
| `mounted` | a mount point for the target exists on the page | importer-never-presents; a component nothing mounts |
| `output` | the target produced output (a non-empty render) | a component that mounts but renders nothing |
| `content` | the declared content reached the renderer — a rendered row count, a triangle count, the mesh vs the current fixture | a cache serving the first design forever |

Pixels (screenshot/A4 comparison) are deliberately out of scope: `appears` asserts scene-graph / DOM facts, not visual ones.

```yaml
steps:
  - action: render
    target: MeshViewport
  - action: appears
    target: MeshViewport
    value: content        # the mesh reached the renderer, not just the store
```

**Capability gating.** `appears` is adapter-gated the same way operation steps are (`adapter-supports-missing-step`): a presentation adapter declares which levels it can emit assertions for via `render-support:` (see adapter.schema.md). When the adapter declares the level the criterion needs, `build-feature` emits the `appears` step and the case is `coverage: full`. When it does not — every adapter that predates the field, which is all of them today — the criterion compiles to a store-level assertion and the case is stamped `coverage: state-only`. **The state-only path is the default**: an adapter with no `render-support:` block delivers no `appears` levels, and every display criterion against it downgrades honestly rather than failing.

---

One suite per page, generated by `build-feature` with **no author in the loop** from what the surface already declares — the composition defects are invisible to any per-component check, so nothing an author writes per feature can catch them. For each page a feature contributes to:

- every declared component asserts `appears: mounted`;
- every component carrying `actions:` asserts it is hit-reachable (interactive);
- every fragment marked `interactive: false` (surface.schema.md) asserts it is **not** hit-reachable — the adapter emits it as non-hit-testable output, so the "dead mouse" defect is impossible rather than merely visible.

The suite is `kind: presentation`, `scope: route`, and derives entirely from the surface + page manifest; it carries no hand-authored expectations. Where the adapter cannot deliver a level yet, the corresponding assertion is stamped `coverage: state-only` exactly as an ordinary case would be.

---

<!-- parlay-extends: parlay-tool/multi-adapter/testcases-v2 -->

Multi-target projects bump `testcases.yaml` to `schema_version: 2` with a `kind:` discriminator over the closed set `{presentation, operation}`.

**Versioning policy** (see `schema-versioning.schema.md` for the house rule): **regenerate**. `testcases.yaml` is tool-generated by `/parlay-build-feature` from the buildfile and the contract artifacts' `verify:` fields — there's nothing hand-edited in it worth migrating in place. Since v0.3 only the v2 shape is accepted; a leftover v1 file regenerates in one `build-feature` run.

```yaml
schema_version: 2
feature: <feature-slug>
framework: <test framework name>

suites:
  - kind: presentation
    name: <suite id>
    file: <path this suite's code is written to>
    component: <component reference into buildfile.components>
    source_refs:
      - "@<feature>/<surface-fragment>"
    fixture: <fixture reference>
    cases: [...]

  - kind: operation
    name: <suite id>
    operation: "@<feature>/operation:<id>"
    source_refs:
      - "@<feature>/operation:<id>"
    output_assertions: [...]
    error_assertions: [...]
    persistence_assertions: [...]
```

### Suite scope: what a suite composes

`kind: presentation` suites carry a `scope:`, over the closed set `{component, route, flow}`. Absent, it reads as `component` — the legacy shape.

| Scope | Unit under test | `component:` field |
|---|---|---|
| `component` | one component in isolation, against one fixture | required |
| `route` | everything a route renders, against one fixture | omitted; `route:` names the path |
| `flow` | a sequence of routes a persona walks | omitted; `flow:` lists the route path sequence |

Why this exists: `build-feature`'s unit of work is the component, and until now so was the testcase suite's. Nothing owned the composition. A regression run produced a wizard host that was unwired and untested (no spec fragment referenced it), four features owning four contradicting fixtures for the same report, and a login persona no fixture defined — and every one of those passed every gate and all 483 component-level assertions, because a component tested in isolation cannot observe what it composes with.

**What `scope:` feeds.** The scope distinction is organizational and feeds the composition checks below — a `scope: route` suite tells `scaffold-seed` which fixture boots the prototype, and a `scope: flow` suite is what `composition-flow-unsatisfiable` inspects. There is no per-route or per-flow *enumeration* gate: an earlier design promised a walker that fired `testcases-route-uncovered` / `testcases-flow-uncovered` for any uncovered route or flow, but it was never built — it would need the merged route table and the dialog-declared flow list, neither of which the testcases validator is handed. Coverage is instead driven from the contract artifacts' `verify:` criteria (see § Where assertions come from), not from route enumeration.

**Fixture coherence is checked separately.** `parlay internal check-composition` compares every feature's fixture data across the whole project. It deliberately ignores disagreements *within* one feature: alternative scenario fixtures are supposed to disagree, and reporting them buries the cross-feature findings that matter.

### The composing fixture

A feature's fixtures back its own suites, and several of them disagree on purpose. Exactly one is different: it holds the data the *running prototype* boots from, which every feature shares. Mark it.

```yaml
fixtures:
  three-reports-mixed-status:
    composes: true      # this is what the prototype boots with
    data:
      ExpenseReport:
        - id: aaaaaaaa-0001-4a01-8a01-aaaaaaaaaaaa
          status: submitted
  empty-state:          # a scenario, not the runtime
    data:
      ExpenseReport: []
```

When no fixture is marked, the one named by the feature's `scope: route` suite is used — a route suite is by definition "everything this route renders", which is the same question the seed asks. Zero or several disagreeing route suites is a real design question and is reported rather than guessed.

### Cross-feature flow assertions

A `scope: flow` suite that walks from one feature's route into another's and *then asserts on domain state* — approve on `/review`, then read "approved" on `/expenses` — is asking a question about a shared runtime. When each feature hydrates its own fixture, nothing carries the write across the boundary and the assertion cannot hold however the code is written.

Pure-navigation flows are unaffected. Clicking from the expense list into the submit wizard and verifying you landed there needs no shared state, and is not reported.

The discriminator is a `verify: state` step *after* the crossing. That is checkable before any code exists, which is the point: the first time this failed, the generating agent weakened the assertion it could not satisfy and left a ten-line comment explaining why. The suite went green and nothing upstream ever learned the journey did not work.

**The composed seed is the union of every feature's composing fixture**, computed by `parlay internal scaffold-seed`. Per `(entity, id)` it unions fields across contributors; a scalar two features disagree on is a contradiction and the derivation refuses — no last-writer-wins, because silently reconciling hides exactly the defect the seed exists to expose. Per-feature fixtures are unchanged and still back `scope: component` and `scope: route` suites. The composed seed is *additional*: it boots the app, and it is what `scope: flow` suites run against.

### Composition error codes

| Code | When it fires |
|---|---|
| `composition-fixture-contradiction` | Two features' **composing** fixtures give the same `(entity, id)` different values for the same scalar field. Both sides must be the fixture their feature contributes to the composed seed — that is what makes the two values coexist in the running prototype. |
| `composition-scenario-fixture-divergence` | The same disagreement, but at least one side is a fixture that never reaches the composed seed. Those two states never coexist at runtime, so this is a **note** and does not fail the check — like the other composition codes it is graded by which list it lands in, not by a severity marker. Mark a fixture `composes: true` if it is meant to reach the composed runtime; an undesignated fixture counts as non-composing. |
| `composition-dangling-reference` | A fixture references an entity id no feature defines |
| `composition-feature-unbuilt` | A feature has a spec but no `buildfile.yaml`, so its fixtures cannot be compared — reported rather than skipped, because an unexamined feature and a coherent one are not the same answer |
| `composition-seed-ambiguous` | No fixture is marked `composes: true` and the `scope: route` suites do not settle which one boots the prototype. Build mode fails; authoring mode warns. |
| `composition-buildfile-unreadable` | A `buildfile.yaml` exists but cannot be parsed, so the feature's contribution is unknown |
| `composition-cardinality-violated` | Two or more records in the composed seed point at the same parent through a relationship the domain model declares `one-to-one`. The prototype boots with data its own model forbids. |
| `composition-cardinality-unresolvable` | A `one-to-one` relationship could not be joined to the field that realises it — either nothing on the child entity targets the parent, or several fields do. A **note**: guessing which field implements the relationship would let the check fail a correct model. Add `relationship: <name>` to the intended field to settle it. |
| `composition-flow-unsatisfiable` | A `scope: flow` suite asserts on domain state after crossing from one feature's route into another's, and the project has no shared runtime that could carry the write across. An **error** when the adapter declares `file-conventions.paths.store` and a participating feature's plan does not wire it; a **note** when the adapter declares no store at all, since the framework may simply have no shared runtime and no better code would satisfy the assertion. |

### Where a suite's code goes: `file:`

Every suite declares `file:` — the path its generated test code is written to.

```yaml
suites:
  - kind: presentation
    name: expense-list-renders
    file: src/components/expense-list.spec.ts
```

The value is **not** decided when authoring testcases.yaml. `parlay internal scaffold-plan` expands the adapter's `file-conventions.paths.test` template once per component and emits the result as a `plan.creates` row; `file:` is that row. This makes three things true at once that were previously independent guesses: the path obeys the project's adapter, the path is inside the plan allowlist codegen enforces, and every component's tests land in the same place.

**Prefer `file:` over a new `kind:`.** The suite `kind:` set is closed at `{presentation, operation}`, and a suite whose code lives in a hand-authored unit is not a third kind of suite — it is an ordinary suite whose file someone else maintains.

#### Citing a hand-authored test

When a `file:` names a path inside a hand-authored unit's declared `tests:` globs, the suite is **cited, not generated**. Codegen refuses to write there (see `authored.schema.md`'s write fence), and the suite records that the invariant is covered by a test a person maintains.

The build phase should not emit such a suite at all when the unit's `satisfies:` already lists the invariant — a generated suite for an invariant a unit covers is either a duplicate or, because it cannot see the unit's internals, a vacuous one that asserts nothing and passes forever. Replacing a vacuous suite with a declared external test is the point of the mechanism.

Freshness of a cited test rides the existing hash machinery rather than a new one: `coverage-review.yaml` already pins `buildfile_hash` and `testcases_hash` and goes stale when either moves, and a unit's aggregate hash is already a `source-signatures:` input (`authored`), so an edit to the cited test invalidates the consuming buildfile through the same gate that catches any other source change. "The external test changed, re-review" therefore falls out of mechanisms that already exist, with nothing new to keep in sync.

### Coverage walker

For every canonical operation declared in the feature's `capabilities.yaml`, at least one `kind: operation` suite must reference it. The walker fires `testcases-operation-uncovered` for each missing operation. Coverage is computed against the `@<feature>/operation:<id>` normalized form.

The operation walker's subjects are fed from the feature's `capabilities.yaml`, resolved from the testcases.yaml's own build path (`.parlay/build/<feature>/`). A feature with no `capabilities.yaml` declares no operations, so the walker reports nothing — that is a feature with no backend contract, not a coverage failure.

### Criterion coverage walker

The operation walker asks whether a *suite* exists per operation; the criterion walker asks whether a *case* exists per stated acceptance criterion. Every **`verify:` bullet** a contract entry carries — an operation in `capabilities.yaml`, a fragment in `surface.yaml` — must be discharged by at least one case citing it, and that citation is the **pair** `criterion.ref` + `criterion.text`: the ref names the entry, in `@<feature>/<kind>:<name>` form (`kind` is `operation` or `fragment`), and the text pins which of that entry's bullets the case discharges.

**Criterion identity is the (ref, text) pair, not the ref.** The ref alone cannot be an identity, because an entry with five `verify:` bullets has one ref: counting coverage by ref meant a single case marked all five discharged, and "cases come 1:1 from `verify:` entries" was unenforceable in the direction that mattered. Identity is text rather than an index or hash because the contract this file already states is that a wording edit invalidates the case citing the old wording — an index would survive the re-wording that is precisely the drift worth surfacing.

Text comparison is normalized **narrowly**: surrounding whitespace and line endings only. It does not lowercase, collapse internal whitespace, or strip punctuation, since each of those can merge two materially distinct claims and report coverage the tests do not have.

Four diagnostics come out of this walker:

| code | condition | fix |
|---|---|---|
| `verify-criterion-uncovered` (warning) | a declared bullet no case discharges | write the case, or exempt the bullet |
| `testcases-criterion-ref-unknown` (warning) | a case cites a ref no contract entry declares | correct the ref against `capabilities.yaml` / `surface.yaml` |
| `testcases-criterion-text-missing` (warning) | a case cites a ref with no text | rebuild with `parlay build-feature`; this is what every pre-bullet-coverage file looks like |
| `testcases-criterion-text-drift` (warning) | a case cites a known entry with a text matching none of its current bullets | the contract was reworded after the case was written, or the criterion was invented |

### Cross-kind citation

A suite's `kind:` need not equal its criterion's owner. A presentation case may discharge an **operation's** criterion — an operation contract can legitimately be observed end-to-end through the UI — but only by **invoking that operation**: one of the case's steps must have the operation's ref as its `target:`. A presentation case citing an operation ref that no step targets draws `testcases-cross-kind-criterion-unexercised` (warning).

What this stops is the operation ref used as a **substitute** for a display criterion the fragment never stated — the tempting move when a fragment carries no `verify:` at all. It compiles every display claim down to a store assertion, permanently and without the `coverage: state-only` stamp that exists to record exactly that downgrade.

Only invocation is checkable. Whether the cited criterion is *contract-shaped*, and so suitable for the presentation case citing it, needs classification metadata criteria do not carry; that half is an authoring rule (`/parlay-build-feature`) and a job for review. Membership is tested directly against the case's step targets rather than through `exercises:`, because the vacuity walker fires only when *no* step targets *any* declared exercise — so listing the operation in `exercises:` proves nothing about the steps.

A fifth, `verify-criterion-duplicate`, reports the contract rather than the testcases file: two identical bullets on one entry are indistinguishable under text identity and cannot be discharged separately, so they are an authoring defect to fix rather than a case to index around.

A bullet may be excused by a `coverage-review.yaml` exemption. An exemption whose `item:` is the ref and whose `criterion_text:` is the bullet excuses exactly that bullet; an exemption with `item:` alone is **entry-wide**, which is how every exemption written before bullet-level coverage has to be read, since none could have recorded a text.

All five are **warnings** while criterion-driven cases land: every testcases.yaml was generated before `criterion:` existed, so its cases cite nothing yet, and erroring would fail every project at once over a fact none of them could have recorded. They graduate to errors once projects have rebuilt with criterion-carrying cases.

When no contract resolves at all, the citation checks are suppressed: with nothing declared every citation would look unknown, which is a fact about the missing input rather than about the file.

### Source refs requirement

Every v2 suite must declare at least one `source_refs:` entry citing a real surface fragment (presentation suites) or capability operation (operation suites). Missing source_refs fail with `testcases-source-refs-missing`.

### Legacy v1 ingestion

The v1 shape (a suite with no `kind:`) stopped being accepted in v0.3: it draws `testcases-v1-unsupported` (error). The policy has always been regenerate — one `/parlay-build-feature` run emits the current form — so there is no shim and no migrator.

| Code | When it fires |
|---|---|
| `testcases-operation-uncovered` | A canonical operation has no covering `kind: operation` suite. |
| `testcases-source-refs-missing` | A new v2 suite lacks `source_refs:`. |
| `testcases-file-missing` (warning) | A v2 suite lacks `file:`, so nothing has decided where its code goes and codegen would invent a path. Warning in both modes while the field lands — every testcases.yaml predates it. Rebuild to populate it from the plan. |
| `testcases-v1-unsupported` | A suite has no `kind:` — the v1 shape was removed in v0.3; regenerate via `/parlay-build-feature`. |
| `testcases-suite-kind-unknown` | A suite declares `kind:` outside `{presentation, operation}`. |
| `testcases-operation-shape-mismatch` | An operation suite asserts `output.entity` that does not match the canonical operation. |
| `testcases-case-unnamed` | A `cases[]` entry declares no `name:`, or an empty one. The name is what a failing test reports, so an unnamed case fails anonymously. |
| `testcases-unknown-term` | A `cases[].steps[]` entry uses an `action:` outside `{render, click, input, select, navigate, wait, appears}`, a `verify:` outside `{element, state, route, count, text, visible, hidden, enabled, disabled}`, or declares neither. |
| `testcases-appears-level-unknown` | An `appears` step's `value:` is missing or outside the level set `{mounted, output, content}` — the runner cannot decide what depth to assert. |
| `testcases-coverage-unknown` | A case declares `coverage:` outside `{full, state-only}` — an unknown value hides whether the claim was downgraded, the exact thing the marker exists to make visible. |
| `testcases-case-vacuous` | A case declares `exercises:` but none of those targets appears as a step `target:` — the case acts on nothing it claims to. |
| `testcases-case-claims-unmet` | A `verify:` step reads a `target:` outside the case's declared `observes:` — the case asserts on something its declaration does not admit. |
| `testcases-case-criterion-missing` (warning) | A case in a v2 suite declares no `criterion:`, so nothing records why it exists. Warning while the field lands — every testcases.yaml predates it. |
| `verify-criterion-uncovered` (warning) | A `verify:` bullet on a contract entry has no case whose `criterion.{ref,text}` discharges it and no `coverage-review.yaml` exemption. Counted per bullet, not per entry. Warning while criterion-driven cases land — every testcases.yaml predates `criterion:`. |
| `testcases-criterion-ref-unknown` (warning) | A case cites a `criterion.ref` no contract entry declares. |
| `testcases-criterion-text-missing` (warning) | A case cites a ref with no `criterion.text`, so which of the entry's bullets it discharges cannot be told. What every file written before bullet-level coverage looks like; the fix is a rebuild. |
| `testcases-criterion-text-drift` (warning) | A case cites a known entry with a `criterion.text` matching none of its current `verify:` bullets — the contract was reworded, or the criterion was invented. |
| `testcases-cross-kind-criterion-unexercised` (warning) | A presentation case cites an operation's criterion but no step targets that operation — the ref is standing in for a display criterion rather than discharging a contract one. |
| `verify-criterion-duplicate` (warning) | A contract entry declares the same `verify:` bullet twice. Two identical bullets cannot be discharged separately; fix the contract rather than indexing around them. |
