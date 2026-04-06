# Testcases Schema

File: `spec/intents/<feature-name>/devspec/testcases.yaml`
Generated alongside buildfile.yaml by `/parlay build-feature`. Defines property-based tests that verify the prototype matches the buildfile contract.

Tests are specification-level — they verify what the user sees and can do, not implementation details. Any AI agent generating test code from this file must produce tests that pass against a correctly built prototype.

## Structure

```yaml
feature: <feature-slug>
framework: <test framework — e.g., Cypress, Playwright, Jest + Testing Library>

suites:
  - name: <suite name>
    component: <component-name from buildfile>
    fixture: <fixture-name from buildfile>

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

## Suite organization

- One suite per component + fixture combination
- Component name must match a component in buildfile.yaml
- Fixture name must match a fixture in buildfile.yaml
- Each case tests one behavior or state

## Test categories

Tests should cover these categories (derived from buildfile):

1. **Rendering** — component displays correct data from fixture
2. **Elements** — all elements defined in buildfile are present and bound correctly
3. **Visibility** — conditional elements appear/hide based on `visible-when` conditions
4. **Actions** — each action triggers its defined effect
5. **State transitions** — entity state machines transition correctly
6. **Navigation** — route changes work as defined
7. **Edge cases** — derived from intent Hints (empty states, error conditions, boundary values)

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
