# Testcases Schema

File: `.parlay/build/<feature-name>/testcases.yaml`
Generated alongside buildfile.yaml by `/parlay build-feature`. Tool-internal — drives cross-validation and feeds spec generation, never handed off to engineering. Defines property-based tests that verify the prototype matches the buildfile contract.

Tests are specification-level — they verify what the user sees and can do, not implementation details. Any AI agent generating test code from this file must produce tests that pass against a correctly built prototype.

## Structure

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

## Step types

### Actions

| Action | Target | Value | Description |
|---|---|---|---|
| render | component-name | — | Render the component with the suite's fixture |
| click | action-name or element-name | — | Click a button, link, or interactive element |
| input | element-name | string | Type into an input field |
| select | element-name | option value | Select from a dropdown or option list |
| navigate | route path | — | Navigate to a URL |
| wait | condition description | — | Wait for async operation or animation |

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

## Suite organization

- One suite per component + fixture combination
- Component name must match a component in buildfile.yaml
- Fixture name must match a fixture in buildfile.yaml
- Intent must reference the source intent via `@feature/intent-slug` for traceability
- Each case tests one behavior or state

## Test categories

Tests should cover these categories (derived from buildfile):

1. **Rendering** — component displays correct data from fixture
2. **Elements** — all elements defined in buildfile are present and bound correctly
3. **Visibility** — conditional elements appear/hide based on `visible-when` conditions
4. **Actions** — each action triggers its defined effect
5. **State transitions** — entity state machines transition correctly
6. **Navigation** — route changes work as defined
7. **Edge cases** — derived from intent Verify and Questions (empty states, error conditions, boundary values)

## Determinism contract

Two AI agents reading the same testcases.yaml must produce tests that:
- Test the same behaviors in the same order
- Use the same fixtures
- Verify the same expected outcomes
- Pass against any prototype correctly built from the same buildfile

The test code may differ (assertion syntax, selector strategy), but the test coverage and expectations must be equivalent.

## Parsing

- YAML structure — standard YAML parsing
- Component references: match `components` keys in buildfile.yaml
- Fixture references: match `fixtures` keys in buildfile.yaml
- Element references: match `elements[].name` in buildfile components
- Action references: match `actions[].name` in buildfile components
- Model references: `EntityName.field` dot notation for state verification

## Schema version 2: discriminated suite kinds

<!-- parlay-extends: parlay-tool/multi-adapter/testcases-v2 -->

Multi-target projects bump `testcases.yaml` to `schema_version: 2` with a `kind:` discriminator over the closed set `{presentation, operation}`.

**Versioning policy** (see `schema-versioning.schema.md` for the house rule): **regenerate**. `testcases.yaml` is tool-generated by `/parlay-build-feature` from the buildfile and intent Verify bullets — there's nothing hand-edited in it worth migrating in place. The v1→v2 bump itself isn't a migrator-chain case either: a project adopting multi-target re-runs `build-feature`, which emits the v2 shape directly; there is no in-memory v1-to-v2 transform of an existing file.

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

**Coverage rule.** Every route in the merged route table needs at least one `scope: route` suite; the walker fires `testcases-route-uncovered` naming the route. Every multi-route flow named in a dialog needs a `scope: flow` suite, or the walker fires `testcases-flow-uncovered`. Both are warnings on a project that has never had them and errors once a project declares any scoped suite — a project mid-adoption should not be blocked, and one that has adopted the concept should not silently regress out of it.

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

**Why the field exists.** `build-feature` said "one test suite per component" and named no location. `generate-code` then had to write the file anyway, and its instruction was "tests live at the location the framework expects" — a convention it invented at emission time, invisible to the adapter and to the plan, and not necessarily the same convention the next run would infer. A question left open at the step that owns it does not stay open; it is answered downstream by whoever reaches it first, with less context than the step that should have decided it.

**Prefer `file:` over a new `kind:`.** The suite `kind:` set is closed at `{presentation, operation}`, and a suite whose code lives in a hand-authored unit is not a third kind of suite — it is an ordinary suite whose file someone else maintains.

#### Citing a hand-authored test

When a `file:` names a path inside a hand-authored unit's declared `tests:` globs, the suite is **cited, not generated**. Codegen refuses to write there (see `authored.schema.md`'s write fence), and the suite records that the invariant is covered by a test a person maintains.

The build phase should not emit such a suite at all when the unit's `satisfies:` already lists the invariant — a generated suite for an invariant a unit covers is either a duplicate or, because it cannot see the unit's internals, a vacuous one that asserts nothing and passes forever. Replacing a vacuous suite with a declared external test is the point of the mechanism.

Freshness of a cited test rides the existing hash machinery rather than a new one: `coverage-review.yaml` already pins `buildfile_hash` and `testcases_hash` and goes stale when either moves, and a unit's aggregate hash is already a `source-signatures:` input (`authored`), so an edit to the cited test invalidates the consuming buildfile through the same gate that catches any other source change. "The external test changed, re-review" therefore falls out of mechanisms that already exist, with nothing new to keep in sync.

### Coverage walker

For every canonical operation declared in the feature's `capabilities.yaml`, at least one `kind: operation` suite must reference it. The walker fires `testcases-operation-uncovered` for each missing operation. Coverage is computed against the `@<feature>/operation:<id>` normalized form.

### Source refs requirement

Every v2 suite must declare at least one `source_refs:` entry citing a real surface fragment (presentation suites) or capability operation (operation suites). Missing source_refs fail with `testcases-source-refs-missing`.

### Legacy v1 ingestion

Legacy v1 suites without explicit `kind:` load as `kind: presentation` and auto-populate `source_refs[0]` from the legacy `intent` string. The validator emits `testcases-source-refs-missing-legacy` as a warning so the designer knows to regenerate the v2 form.

| Code | When it fires |
|---|---|
| `testcases-operation-uncovered` | A canonical operation has no covering `kind: operation` suite. |
| `testcases-source-refs-missing` | A new v2 suite lacks `source_refs:`. |
| `testcases-file-missing` (warning) | A v2 suite lacks `file:`, so nothing has decided where its code goes and codegen would invent a path. Warning in both modes while the field lands — every testcases.yaml predates it. Rebuild to populate it from the plan. |
| `testcases-source-refs-missing-legacy` (warning) | A legacy v1 suite was loaded as v2 presentation; auto-populated source_refs would be approximate. |
| `testcases-suite-kind-unknown` | A suite declares `kind:` outside `{presentation, operation}`. |
| `testcases-operation-shape-mismatch` | An operation suite asserts `output.entity` that does not match the canonical operation. |
| `testcases-case-unnamed` | A `cases[]` entry declares no `name:`, or an empty one. The name is what a failing test reports, so an unnamed case fails anonymously. |
| `testcases-unknown-term` | A `cases[].steps[]` entry uses an `action:` outside `{render, click, input, select, navigate, wait}`, a `verify:` outside `{element, state, route, count, text, visible, hidden, enabled, disabled}`, or declares neither. |
| `testcases-route-uncovered` | A route in the merged route table has no `scope: route` suite. Warning on a project that has never had one, error once any scoped suite is declared. |
| `testcases-flow-uncovered` | A multi-route flow named in a dialog has no `scope: flow` suite. Same warning-then-error escalation as `testcases-route-uncovered`. |
