# Vocabulary-validation — Dialogs

---

### Validation rules — types, tokens, and shape

**Trigger**: The maintainer runs `parlay validate-vocabulary @<feature>` from the terminal against a feature's layout YAML, or the design-loop skill invokes the same command from step 2 (pre-flight on the full layout) or step 7 (read-back classification, once per designer-authored novelty node).

Maintainer: I edited the layout for `@checkout/cart-summary` and want to confirm it's still vocabulary-clean before I push to Figma.
Maintainer: `parlay validate-vocabulary @checkout/cart-summary`
System: Resolves the layout's `componentVocabulary:` field (`clarity@17`) to the active adapter, loads the `vocabulary:` block, walks every node in the layout's typed tree, and runs the six checks in order: type, property, variant, spacing-token, color-token, layout-container. Emits the structured report as JSON on stdout: `{report: [{node_path, rule, expected, actual, severity}, ...]}`. Exit code 0 if zero `error`-severity entries, 1 otherwise. Makes zero network calls and reads no files beyond the layout YAML and the resolved adapter YAML.

#### Happy path — clean layout, zero issues

Maintainer: The layout is clean — every type, property, variant, spacing token, color token, and container parameter resolves.
System: Returns an empty report: `{report: []}`. Exit code 0. The design-loop skill consumes the empty report as the pre-flight "go" signal and proceeds to step 3 (`get_metadata`). A CLI maintainer sees no stderr output and an empty JSON report on stdout.

#### Branch: unknown component type (check 1, type-check)

Maintainer: The layout has a node with `type: clarity.megabutton` — a typo that's not in the vocabulary.
System: Emits `{node_path: "root.children[2]", rule: "type-check", expected: ["clarity.button", "clarity.text", "clarity.region", ...the full component list...], actual: "clarity.megabutton", severity: "error"}`. Exit code 1. The maintainer sees the exact unknown identifier and the full admissible list as the `expected:` value — easy to spot the typo.

#### Branch: property not on component (check 2, property-check)

Maintainer: A `clarity.button` node carries a `glow: true` property; the button's declared properties are `label`, `kind`, `disabled`.
System: Emits `{node_path: "root.children[0]", rule: "property-check", expected: ["label", "kind", "disabled"], actual: "glow", severity: "error"}`. Exit code 1. The check fires once per offending property, so a node with three unknown props yields three report entries.

#### Branch: variant value outside enum (check 3, variant-check)

Maintainer: A `clarity.button` node has `kind: ghost` but the button's `kind` variant enum is `[primary, secondary, tertiary]`.
System: Emits `{node_path: "root.children[0]", rule: "variant-check", expected: ["primary", "secondary", "tertiary"], actual: "ghost", severity: "error"}`. Exit code 1. The `expected:` value carries the admissible enum so the maintainer can fix without consulting the adapter.

#### Branch: raw pixel spacing (check 4, spacing-token-check — error)

Maintainer: A container has `padding: 16` (raw integer) instead of `padding: spacing-md`.
System: Emits `{node_path: "root.children[1]", rule: "spacing-token-check", expected: ["spacing-xs", "spacing-sm", "spacing-md", "spacing-lg", "spacing-xl"], actual: 16, severity: "error"}`. Exit code 1. Raw literals are always errors — the intent is that designer expression goes through the token vocabulary, not raw values.

#### Branch: token alias that resolves but isn't canonical (check 4, spacing-token-check — warning)

Maintainer: A container has `padding: spc-md` and the adapter's vocabulary lists `spacing-md` as the canonical name. The value resolves through an alias.
System: Emits `{node_path: "root.children[1]", rule: "spacing-token-check", expected: "spacing-md", actual: "spc-md", severity: "warning"}`. Exit code 0 (warnings don't block). The design-loop pre-flight gate treats this as a "go" but the report still surfaces it so the maintainer can normalize at their leisure. The token-alias defensive softness only applies to checks 4–6 (spacing, color, layout-container); type, property, and variant checks (1–3) never warn — they error.

#### Branch: raw hex color (check 5, color-token-check)

Maintainer: A `clarity.text` node has `color: "#3B82F6"` instead of `color: brand-primary`.
System: Emits `{node_path: "root.children[3]", rule: "color-token-check", expected: ["brand-primary", "brand-secondary", "neutral-strong", ...], actual: "#3B82F6", severity: "error"}`. Exit code 1. Same rationale as spacing: raw hex is unconstrained expression that the round-trip cannot represent through the design system.

#### Branch: layout container with unknown parameter (check 6, layout-container-check)

Maintainer: A `clarity.region` container declares `direction: diagonal` but the adapter's `layout_containers:` entry for `clarity.region` lists `direction` as `parameter_constraints: {type: enum, allowed_values: [horizontal, vertical]}`.
System: Emits `{node_path: "root", rule: "layout-container-check", expected: ["horizontal", "vertical"], actual: "diagonal", severity: "error"}`. Exit code 1. If the parameter name itself is not in the container's `admissible_parameters`, the report names the parameter in `actual:` and the admissible parameter list in `expected:`.

#### Branch: multiple errors in one layout — no short-circuiting

Maintainer: A layout has both a `type-check` error AND a `property-check` error.
System: Emits both entries in the report, in walk order. The check sequence does not short-circuit on an earlier failure — the maintainer sees the full list in one report, fixes everything, and re-runs once.

#### Branch: pre-flight invocation mode (full layout)

Maintainer: The design-loop skill at step 2 runs `parlay validate-vocabulary @checkout/cart-summary` with no extra arguments. The validator reads the entire layout YAML and walks every node.
System: Returns the structured report covering all nodes. The skill computes the derived signal — `in-vocabulary` when the report has zero `error`-severity entries, `out-of-vocabulary` otherwise — from the report itself. The validator never emits the derived signal; the caller computes it.

#### Branch: read-back classification mode (single node)

Maintainer: The design-loop skill at step 7 classifies a designer-authored novelty captured in step 4's diff. It runs `parlay validate-vocabulary @checkout/cart-summary --node <node_path>` (or pipes a single-node fragment as stdin — the CLI accepts both).
System: Returns the same `{report: [...]}` shape, scoped to the named node and its subtree. Zero `error`-severity entries derives to `in-vocabulary` and the skill stages the novelty for merge-back into the canonical layout YAML; any `error`-severity entry derives to `out-of-vocabulary` and the skill stages the novelty for `design-loop-conflicts.yaml` as `kind: out-of-vocabulary-node`. Report shape is identical to the full-layout mode — same fields, same severity semantics.

#### Branch: CLI vs Go library invocation produces identical reports

Maintainer: A future Studio binary feature (e.g. Domain Model Editor pre-save) needs to validate a layout in-process without shelling out.
System: The binary imports `studio/internal/vocabulary` and calls the library's `Validate(layout, vocabulary) Report` function directly. The library returns the same `Report` shape the CLI marshals to JSON; the two paths produce byte-identical reports for the same inputs (modulo JSON marshaling whitespace). A unit test cross-checks: same layout + same adapter → same report from both paths.

#### Branch: rule set is closed at six (Q1.1 resolution)

Maintainer: I work on a design system that needs a custom check — say, "all `clarity.icon` nodes must reference an icon from the adapter's icon set." Can I add a rule via adapter config?
System: No, the rule set is closed at the six checks listed in Intent 1's Action paragraph. Adding a rule is a spec change against this intent, not an adapter-config knob. The reasoning: an extensible rule set makes the validator's behavior depend on per-adapter logic, which fragments the contract the design-loop skill relies on. A custom check belongs in a separate validator (the maintainer's design-system-specific pre-commit hook, for example) that runs alongside `parlay validate-vocabulary`, not inside it. **Q1.1 resolved: closed.**

---

### Vocabulary source resolution

**Trigger**: The validator (Go library or CLI) starts a validation run. Before walking any node, it must resolve the layout YAML's `componentVocabulary:` field to a concrete adapter and load that adapter's `vocabulary:` block.

Maintainer: I'm authoring the `clarity-v17.adapter.yaml` for the Clarity design system at v17 and need to add the vocabulary block so the design-loop validator picks it up.
System: The vocabulary lives **inline** in the adapter YAML — same file as the rest of the adapter's slot/widget mappings — in a top-level `vocabulary:` block with four subfields: `components:` (list of `{name, properties, variants}` records), `spacing_tokens:` (flat list of admissible names), `color_tokens:` (flat list of admissible names), and `layout_containers:` (list of `{container_type, admissible_parameters, parameter_constraints}` records). The maintainer adds the block once; the adapter is self-contained.

#### Happy path — vocabulary resolves cleanly from adapter

Maintainer: A layout has `componentVocabulary: clarity@17`. The adapter resolver maps `clarity@17` to `studio/.parlay/adapters/clarity-v17.adapter.yaml`. The file has a `vocabulary:` block.
System: Reads the adapter YAML, parses the four subfields into the in-memory vocabulary representation, and hands it to the validator. The maintainer never sees this — it's silent setup. Subsequent validation runs hit the resolved vocabulary directly. Zero network calls; zero MCP calls; one filesystem read of the adapter YAML.

#### Branch: adapter exists but has no `vocabulary:` block

Maintainer: I added `componentVocabulary: clarity@17` to a layout but forgot to add the `vocabulary:` block to the Clarity adapter YAML.
System: Validation fails with the error code `vocabulary-missing-from-adapter` and names the adapter (`clarity-v17`). The error message: "Adapter `clarity-v17` has no `vocabulary:` block; the layout's `componentVocabulary:` reference cannot resolve. Add the block following the schema documented at `core/internal/embedded/schemas/vocabulary.schema.md`." Exit code 1. The validator does not fall back to any default vocabulary — there's no implicit vocabulary, only the one the adapter declares.

#### Branch: layout references an unknown adapter

Maintainer: A layout has `componentVocabulary: clarity@99` but no adapter named `clarity-v99` is registered in the adapter-set.
System: Validation fails with the error code `vocabulary-unknown-adapter` and names the referenced value (`clarity@99`). The error message: "Layout references vocabulary `clarity@99` but no adapter resolves that reference. Registered adapters: [react-vite-radix-tailwind, go-studio-app, ...]." Exit code 1. The error does not suggest a fix — the maintainer either typo'd the version or hasn't authored that adapter yet.

#### Branch: vocabulary inline in adapter YAML vs separate file (Q2.1 resolution)

Maintainer: A `vocabulary:` block in a busy adapter YAML is going to be long — hundreds of components plus tokens. Should it live in a separate `clarity-v17.vocabulary.yaml` next to the adapter?
System: **Inline**, in the adapter YAML. Reasoning: the adapter is the unit of design-system identity at the adapter-set layer; splitting the vocabulary into a sibling file means the adapter resolver has to find two files instead of one, the schema has to document both, and a future migration that re-keys vocabularies (e.g. `clarity@17` → `clarity@18`) has to touch two files instead of one. Inline keeps the adapter self-contained and the resolution path single-file. Length is the only cost; YAML anchors and references mitigate that, and a long vocabulary block scrolls past — it doesn't structurally hurt anything. **Q2.1 resolved: inline.**

#### Branch: adapter resolution is ambiguous in a multi-root project

Maintainer: My multi-root project has two adapters named `clarity-v17` — one at the repo-level root and one inside a child root.
System: Adapter resolution follows the existing adapter-set resolution rules: child-root adapters shadow repo-level-root adapters of the same name. The validator inherits this behavior — it doesn't add new resolution logic. If the project's adapter-set declares only one as active, that's the one the validator uses. If the active root is ambiguous (CLI exited with code 11), the maintainer re-invokes with `--root <chosen>` per the standard active-root protocol; the validator never silently picks one over the other.

#### Branch: adapter vocabulary is wrong relative to the real Figma source of truth

Maintainer: The adapter's `components:` list says `clarity.button` has variants `[primary, secondary, tertiary]` but the actual Figma library now has `[primary, secondary, ghost]`. A layout using `kind: ghost` validates as an error even though Figma would accept it.
System: That's the adapter author's responsibility, not the validator's. The validator's contract is "validate against whatever vocabulary the adapter declares." Detecting drift between the adapter and the real Figma library is an integration-test concern — a separate feature could compare the adapter's vocabulary against a live Figma variables fetch and report drift. The validator itself does no such comparison and never calls Figma MCP.

---
