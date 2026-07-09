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
          - verify: <element | state | route | count | text | visible | hidden | enabled | disabled>
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
| `testcases-source-refs-missing-legacy` (warning) | A legacy v1 suite was loaded as v2 presentation; auto-populated source_refs would be approximate. |
| `testcases-suite-kind-unknown` | A suite declares `kind:` outside `{presentation, operation}`. |
| `testcases-operation-shape-mismatch` | An operation suite asserts `output.entity` that does not match the canonical operation. |
