# Vocabulary-validation — Infrastructure

---

## Validator is a pure function with a stable structured report shape

**Affects**: validator purity boundary and external-call allowlist
**Behavior**: Vocabulary validation is a pure function over two inputs — the layout YAML being validated and the resolved design-system vocabulary. The validator makes zero network calls, invokes zero Figma MCP tools, and reads zero files beyond those two inputs. Its only output is a structured report: an ordered list of result entries each shaped `{node_path, rule, expected, actual, severity}`, where `rule` names which of the six checks fired (type, property, variant, spacing-token, color-token, layout-container), `node_path` locates the offending node in the layout's typed tree, `expected` carries the admissible set or canonical value, `actual` carries what the layout had, and `severity` is either `error` (blocks the design-loop pre-flight gate) or `warning` (logged but doesn't block). The report shape is identical regardless of invocation mode: a full-layout validation produces one report covering all nodes, a single-node classification produces one report scoped to that node. The validator never short-circuits — every check fires against every applicable node so the operator sees the full issue list in one report. The validator never emits a derived signal like "in-vocabulary" or "out-of-vocabulary"; callers compute that signal themselves from the report (zero error-severity entries derives to "in-vocabulary"; one or more derives to "out-of-vocabulary").
**Invariants**:
- The validator opens zero network sockets during a validation run; a network-syscall trace across a validation call shows nothing.
- The validator invokes zero Figma MCP tools; a grep across its source for any Figma MCP tool name returns zero matches outside comment blocks documenting what the validator deliberately excludes.
- The validator reads exactly two filesystem inputs per invocation — the layout YAML and the resolved adapter YAML; a filesystem-syscall trace shows no other reads.
- The structured report's entries are exactly the closed shape `{node_path, rule, expected, actual, severity}` with no additional or renamed fields; a fixture test loads a sample report and asserts the field set is exactly that quintuple.
- `severity` is exactly one of `error` or `warning` for every entry; no other severity values exist in the validator's source.
- The validator's report is the same in pre-flight invocation mode (full layout) and read-back classification mode (single node); a unit test cross-checks the two modes against the same node and asserts identical entry shapes.
- The validator never computes or emits a derived "in-vocabulary" / "out-of-vocabulary" signal; that derivation lives in callers, and a grep across the validator's source for those literal strings returns zero matches outside test fixtures and documentation.
- Checks 1-3 (type, property, variant) only ever produce `error` severity; a unit test asserts a report containing any of those three rules with `severity: warning` is impossible from the validator's output path.
- Checks 4-6 (spacing-token, color-token, layout-container) produce `error` when the value is a raw literal and `warning` when the value resolves through an alias to a non-canonical token name; a unit test exercises both severities per check.
- The rule set is closed at exactly six checks; adding a seventh is a spec change against intent 1, not a runtime extension point or adapter-config knob.
**Source**: @design-loop/vocabulary-validation/validation-rules-types-tokens-and-shape
**Backward-Compatible**: yes

**Notes**:
- The validator is implemented as a library at a stable location inside the Studio application target, and the `parlay validate-vocabulary @<feature>` CLI command is the operator-facing entry that wraps the library. The two surfaces share the same library code; the CLI marshals the library's report to JSON on stdout, the library returns the same report shape in-process to other binary callers. A unit test cross-checks: same layout plus same adapter against both paths produces byte-identical reports (modulo JSON marshaling whitespace).
- A seventh check — container-child-type restriction (validating that a layout container's children are admissible child types per the container's component spec) — was considered and deliberately deferred from v1. A future feature adds the seventh check when there is concrete design-system evidence that child-type restrictions need enforcement; until that evidence exists the closed six-check set holds.
- The design-loop skill invokes the CLI from step 2 (pre-flight, full-layout mode, no `--node` flag) and step 7 (read-back classification, single-node mode, one CLI call per designer-authored novelty captured in step 4's diff). The skill parses the JSON output and computes the `in-vocabulary | out-of-vocabulary` derived signal itself. Per the design-loop infrastructure, the skill calls the validator exactly once per novelty in step 7.

---

## Vocabulary source resolution is adapter config; vocabulary block lives inline in the adapter YAML

**Affects**: vocabulary source allowlist and adapter YAML shape
**Behavior**: The design-system vocabulary the validator consumes comes from exactly one source: the design-system adapter referenced by the layout YAML's `componentVocabulary:` field. The adapter declares its vocabulary inline in its YAML file under a top-level `vocabulary:` block with four subfields whose shapes are pinned by this feature: (1) `components:` is a list of records each shaped `{name, properties, variants}` — `name` is the canonical component identifier (e.g. `clarity.button`), `properties` enumerates the admissible property names for that component, `variants` maps each variant axis name to the list of admissible enum values; (2) `spacing_tokens:` is a flat list of admissible spacing token name strings (the validator checks set membership, not the resolved pixel value); (3) `color_tokens:` is a flat list of admissible color token name strings (same rationale — set membership, not the resolved hex); (4) `layout_containers:` is a list of records each shaped `{container_type, admissible_parameters, parameter_constraints}` — `container_type` matches a name from `components`, `admissible_parameters` enumerates which layout parameters that container honors, `parameter_constraints` documents per-parameter typed constraints. The vocabulary lives inline in the adapter YAML and NOT in a separate sibling file. Two alternative vocabulary sources were considered and rejected: a live MCP variables fetch (couples the validator to a network round-trip and requires Figma MCP access the Studio binary doesn't have under the host-agent architecture) and a hardcoded vocabulary (only works when one design system is in scope; fails immediately when the project supports more than one). Adapter resolution follows the existing adapter-set resolution rules — the validator inherits this behavior and does not add new resolution logic.
**Invariants**:
- The validator's vocabulary load path opens zero network sockets and invokes zero Figma MCP tools; a runtime trace asserts both.
- The validator's vocabulary load reads exactly one filesystem input — the resolved adapter YAML; it does not read a sibling `vocabulary.yaml` or any other file.
- A layout YAML whose `componentVocabulary:` field is absent fails validation with a clear error naming the missing field; no implicit default vocabulary exists.
- The four `vocabulary:` subfields are exactly `components:`, `spacing_tokens:`, `color_tokens:`, `layout_containers:`; the validator rejects an adapter declaring additional or renamed top-level subfields under `vocabulary:` with a schema error.
- A `components:` entry that is not shaped `{name, properties, variants}` (missing field, extra field, wrong type) fails adapter load with a schema error naming the offending entry.
- A `layout_containers:` entry whose `container_type` does not match any `name` in `components:` fails adapter load with a schema error naming both the dangling container type and the component list.
- `spacing_tokens:` and `color_tokens:` are flat lists of strings; a map shape under either subfield fails adapter load with a schema error.
- The vocabulary block lives inline in the adapter YAML at the top level under `vocabulary:`; the validator does not look for a sibling `<adapter>.vocabulary.yaml` file, and a fixture test with such a sibling file asserts the validator ignores it.
- An adapter schema document at the repo-level embedded schema surface documents the `vocabulary:` block shape; a doc-existence test asserts the document is present and names the four subfields.
**Source**: @design-loop/vocabulary-validation/vocabulary-source-resolution
**Caching**: on-first-access
**Backward-Compatible**: yes

**Notes**:
- The inline-vs-separate-file decision was resolved during dialog authoring in favor of inline. Splitting the vocabulary into a sibling file would force the adapter resolver to find two files instead of one, force the schema to document both, and force any future re-keying migration (e.g. `clarity@17` → `clarity@18`) to touch two files instead of one. Inline keeps the adapter self-contained and the resolution path single-file. Length is the only cost; YAML anchors and references mitigate it.
- The adapter is responsible for the correctness of its declared vocabulary relative to the real Figma source of truth. The validator does not compare the adapter's vocabulary against a live Figma variables fetch — that drift-detection concern is reserved for a separate integration-test feature. The validator's contract is "validate against whatever vocabulary the adapter declares," nothing more.
- Vocabulary load is cached on first access during a validation run so a multi-node read-back classification loop does not re-parse the adapter YAML per node. The cache lives in the validator's process-local memory and has no persistence layer; restarting the binary or re-running the CLI re-reads the adapter.

---

## Stable error codes for vocabulary resolution failures

**Affects**: feature-stable error code surface consumed across feature boundaries
**Behavior**: Two failure modes during vocabulary resolution emit named, feature-stable error codes that cross-feature consumers — primarily the design-loop skill at step 2 and step 7 — match against textually. The codes are: (1) `vocabulary-missing-from-adapter`, emitted when the resolved adapter exists but has no `vocabulary:` block; the error names the adapter and points the operator at the schema documentation for the `vocabulary:` block; the validator does not fall back to any default vocabulary, since no implicit vocabulary exists. (2) `vocabulary-unknown-adapter`, emitted when the layout YAML's `componentVocabulary:` field references a value that no registered adapter resolves; the error names the referenced value and lists the registered adapter names; the error does not suggest a fix because the operator either typo'd the version or hasn't authored that adapter yet. Both error codes are stable across versions — renaming either is a breaking change that requires updating the design-loop skill's conflict-classification logic and the conflicts YAML schema in lockstep. These codes live in `infrastructure.md` rather than the closed errors vocabulary at `errors.schema.md` because they are vocabulary-validation-feature-specific failures, not generic framework errors; the closed errors vocabulary is intentionally narrow and excludes feature-specific failure modes.
**Invariants**:
- The validator emits exactly the two error codes `vocabulary-missing-from-adapter` and `vocabulary-unknown-adapter` for the two resolution failure modes; no third resolution-failure code exists, and a unit test asserts the closed pair by exercising both code paths.
- Each error includes the relevant identifier in its message — `vocabulary-missing-from-adapter` names the adapter; `vocabulary-unknown-adapter` names the referenced `componentVocabulary:` value and the registered adapter list.
- Renaming either code is a breaking change tracked by a fixture test in the design-loop feature that pins the exact code strings the skill matches against; a rename without coordinated updates to that fixture fails the build.
- The two codes do not appear in `core/internal/embedded/schemas/errors.schema.md`; they are feature-stable codes documented inside this infrastructure fragment and surfaced to operators through the CLI's stderr output and JSON-result error field.
- Neither code is emitted from any path other than vocabulary resolution; a grep across the validator's source for either code string returns matches only inside the two resolution-failure code paths and their tests.
**Source**: @design-loop/vocabulary-validation/vocabulary-source-resolution
**Backward-Compatible**: yes

**Notes**:
- The design-loop skill at step 2 maps `vocabulary-missing-from-adapter` and `vocabulary-unknown-adapter` to a `kind: pre-flight-vocabulary-failure` entry in `design-loop-conflicts.yaml`, aborts the loop before any Figma MCP call, and exits. The skill text matches the code strings literally — the codes are part of the skill-to-validator wire contract.
- The pair is deliberately closed at two. A future "adapter exists but its vocabulary block has a schema error" failure mode was considered for a third code and folded into the existing adapter-schema-validation path instead — schema errors on the `vocabulary:` block are caught at adapter load time, not at validator invocation time, and surface through the adapter-set loader's existing error reporting rather than through the vocabulary validator's resolution failure path.

---
