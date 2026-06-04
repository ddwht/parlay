# Vocabulary-validation

> Vocabulary validation is the static check that ensures a layout YAML's typed-tree nodes use only types and tokens drawn from the declared design-system vocabulary (e.g. `clarity@17`). It runs as a pre-flight gate inside the Design Loop before any Figma MCP write, and again during read-back to classify designer-authored Figma edits as in-vocabulary (merge back into the canonical layout) or out-of-vocabulary (refuse and warn). This feature pins the validation rules, the vocabulary-source resolution (adapter config), and the validation report shape the Design Loop consumes.

---

## Validation rules — types, tokens, and shape

**Goal**: Pin the rules a layout YAML must satisfy to pass vocabulary validation. Each typed-tree node's `type:` must resolve in the declared component vocabulary; spacing and layout values must be token names from the vocabulary (not raw pixels); component variants and properties must be from the per-component declared set.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The layout is a typed tree of design-system components with deliberately constrained expression: every node's `type:` is a vocabulary component, every spacing value is a token name (e.g. `spacing-lg`) rather than a raw pixel value, every variant selection is from the component's declared variant enum. Without enforcement, designer-authored layouts can drift into unconstrained expression (raw pixels, arbitrary components, unmapped properties) that the round-trip can't represent — the resulting Figma instantiation and the read-back would lose information. Validation runs in two contexts: pre-flight (before any Figma MCP write) and post-read-back (classifying designer-authored novelties). The rules are the same in both contexts; the skill invokes the same validator with different inputs.

**Action**: Define the validation rules as a small closed vocabulary of checks, each producing a structured result entry. Checks: (1) every node's `type:` resolves in the component vocabulary's component list; (2) every node's properties are drawn from that component's declared property set; (3) every node's variant selection is drawn from that component's declared variant enum; (4) every spacing/padding/gap value is a token name from the vocabulary's spacing-token set, not a raw pixel value; (5) every color value is a token name from the vocabulary's color-token set, not a raw hex; (6) layout containers (`clarity.region`-shaped nodes) carry layout parameters from the vocabulary's documented set (direction, alignment, etc.).

**Objects**: validation-rule-set, type-check, property-check, variant-check, spacing-token-check, color-token-check, layout-container-check, structured-validation-result

**Constraints**:
- The validation rule set is closed: exactly the six checks listed in the Action paragraph; adding a rule is a spec change against this intent
- Every check produces a structured result entry: `{node_path, rule, expected, actual, severity}` where severity is `error` (blocks the loop) or `warning` (logged but doesn't block)
- Type, property, and variant checks (1–3) produce errors; spacing, color, and layout-container checks (4–6) produce errors when the value is a raw literal, warnings when the value resolves but doesn't match the canonical token name (a defensive softness for token aliases)
- Checks run in order; an error in an earlier check does NOT short-circuit later checks — the validator collects all issues so the designer sees the full list in one report
- Validation is a pure function over the layout YAML and the resolved vocabulary; it does not call Figma MCP, does not access external network, does not read any file beyond the layout YAML and the vocabulary
- The validator's output is a structured report consumed by the Design Loop's pre-flight gate and conflict-classification step; the same report shape works in both contexts

**Verify**:
- A unit test passes a layout with a node whose `type:` is unknown and asserts the result entry has `rule: type-check, severity: error, expected: <component-list>, actual: <unknown-type>`
- A unit test passes a layout with a node using a raw pixel padding value (e.g. `padding: 16`) and asserts the result entry has `rule: spacing-token-check, severity: error`
- A unit test passes a layout with all valid types, properties, and tokens and asserts the result has zero entries
- A unit test passes a layout with both a type error AND a property error and asserts both appear in the result (no short-circuiting)
- A unit test asserts the validator makes no network calls and reads no files beyond the two inputs (layout YAML, vocabulary)

**Questions**:
- Should the rule set be extensible by adapter config (some design systems may need custom rules), or strictly closed to the six rules? Extensible is flexible; closed is more constrainable. Resolve during dialog authoring.

---

## Vocabulary source resolution

**Goal**: Pin where the component vocabulary comes from at validation time. Three candidates exist — declarative adapter config, live MCP variables fetch, or hardcoded — and this intent picks **adapter config** as the source: the simplest, the most predictable, and the only one available without a network call to Figma.

**Persona**: Parlay Studio maintainer

**Priority**: P0

**Context**: The three candidate vocabulary sources have different tradeoffs. Adapter config: declarative, deterministic, requires the adapter author to enumerate the component list, the spacing token set, the color token set, and the layout container configuration; cheap to consume at validation time because it's already on disk. MCP variables fetch: pulls the design system's tokens live from Figma's MCP server, which is accurate (Figma is the source of truth) but couples the validator to a network round-trip and requires Figma MCP access — Studio binary doesn't have that access under the host-agent architecture, and the skill having it doesn't help the binary-side validator. Hardcoded: only works when one design system is in scope; immediately fails when the project supports more than one. Adapter config wins because it's the only option that works for both the binary and the skill, supports multiple design systems, and avoids network coupling.

**Action**: Pin adapter config as the vocabulary source. Each design-system adapter declares its vocabulary in its adapter YAML: the component list with per-component property and variant sets, the spacing-token set, the color-token set, and the layout-container configuration set. The validator reads the vocabulary from the resolved adapter at startup (binary side) or at skill-invocation time (skill side); a layout's `componentVocabulary:` field names the adapter the vocabulary is sourced from (e.g. `clarity@17` resolves to `studio/.parlay/adapters/clarity-v17.adapter.yaml` or wherever the adapter resolver locates it).

**Objects**: vocabulary-source-adapter-config, component-list, property-and-variant-sets, spacing-token-set, color-token-set, layout-container-config-set, layout-componentVocabulary-field

**Constraints**:
- The component vocabulary's authoritative source is the design-system adapter referenced by the layout YAML's `componentVocabulary:` field; the adapter resolution follows the existing adapter-set resolution rules
- The adapter declares the vocabulary in its YAML: a `vocabulary:` block with subfields `components:` (list of component entries with name, properties, variants), `spacing_tokens:`, `color_tokens:`, `layout_containers:`
- Each adapter is responsible for the correctness of its declared vocabulary — there is no validation that the adapter's vocabulary matches the Figma source of truth (matching is an integration test concern, not a validator concern)
- The validator MUST be able to load the vocabulary without making any network calls or invoking Figma MCP — the entire vocabulary lives in the adapter YAML
- An adapter without a `vocabulary:` block cannot drive Design Loop validation; the validator fails with `vocabulary-missing-from-adapter` naming the adapter
- A layout YAML whose `componentVocabulary:` field references an unknown adapter fails validation with `vocabulary-unknown-adapter` naming the referenced value

**Verify**:
- A unit test loads a sample adapter with a `vocabulary:` block and asserts the validator reads the component list, spacing tokens, color tokens, and layout containers
- A unit test loads an adapter without a `vocabulary:` block and asserts the validator fails with `vocabulary-missing-from-adapter`
- A unit test loads a layout with `componentVocabulary: unknown-vocab@99` and asserts the validator fails with `vocabulary-unknown-adapter`
- A unit test asserts the validator opens no network sockets and makes no MCP calls during vocabulary load
- An adapter schema doc (`core/internal/embedded/schemas/adapter.schema.md` extension or a new `vocabulary.schema.md`) documents the `vocabulary:` block shape

**Questions**:
- Should the vocabulary live inline in the adapter YAML (current draft) or in a separate `vocabulary.yaml` next to the adapter (cleaner separation, two files)? Inline is fewer files; separate is more modular. Resolve during dialog authoring.

---
