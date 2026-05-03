# Layout-Aware Code Generation

> When a page artifact carries a `layout:` block, `parlay generate-code` consumes the typed tree as the structural source of truth and infers wiring (data sources, operation calls, presentation hooks) from the combination of surface, domain model, and layout. Pages without `layout:` fall back to the existing codegen behavior unchanged. This is the Core-side counterpart to Studio's Design Loop: Studio produces structure, Core resolves bindings.

---

## Codegen Reads the Layout Block When Present

**Goal**: When a page has a `layout:` block, codegen treats it as the structural specification of the rendered output — node hierarchy, component types, layout parameters, presentation tokens — instead of inferring structure from surface and domain alone.
**Persona**: UX Designer
**Priority**: P0
**Context**: Studio writes layout into pages; the same `parlay generate-code` command must consume what Studio produced. The two products meet at the page artifact: Studio writes layout, Core reads it.
**Action**: During code generation, after loading the buildfile, the agent reads the page's `layout:` block (if present), resolves each node's `type` against the adapter's component vocabulary, and emits framework-specific component instances matching the tree shape. Layout parameters and tokens flow through to the generated code unchanged in semantic.
**Objects**: layout, page, generate-code, framework-adapter, component-instance

**Constraints**:
- Codegen reads the layout block but does not modify it — round-trip writes are Studio's job, not codegen's
- The buildfile schema does not need a separate layout section — codegen reads layout directly from the page artifact at the same time it reads the surface
- Generated code preserves the structural shape of the layout tree: a `clarity.region` with two `clarity.region` children produces a parent component containing two child components, in that order
- Token references in layout (`spacing-lg`, `spacing-xl`) are emitted as adapter-defined token references in the generated code, never as raw pixel values
- Generation is deterministic: the same layout + surface + domain + adapter input produces byte-identical output across runs and across agents
- Adapter-vocabulary mismatch — a page declares `componentVocabulary: clarity@17` but the loaded adapter only knows `clarity@16` — fails generation with a clear actionable error rather than producing partial output

**Verify**:
- A page with a layout block of three Clarity components produces three Angular component instances in the generated template, in the same order
- A page without a layout block produces the same generated output as it did before this feature shipped (regression-tested by re-generating an existing project)
- A layout block referencing a `type` unknown to the loaded adapter fails generation with the offending type and the loaded adapter version named
- Two AI agents reading the same page (with layout) plus the same buildfile and adapter produce code that passes the same testcases

---

## Infer Wiring From Surface + Domain + Layout

**Goal**: When the layout describes structure but not data flow, infer the bindings — which data goes into which node, which actions invoke which operations — from the page's surface and domain model.
**Persona**: UX Designer
**Priority**: P0
**Context**: Layout is intentionally wiring-free (per `page-layout-field`'s constraints). The page's surface declares `Shows:` and `Actions:` at a feature level; the domain declares entities and operations; layout declares the shape. Codegen has all three and needs to wire them together.
**Action**: For each layout node that consumes data (e.g., a datagrid, a list, a form field), match it against the surface's `Shows:` and the domain's entity fields based on a combination of structural hints (`contentShape`, child column types) and naming conventions. For each node that emits an action (a button, a clickable row), match it against the surface's `Actions:` and the domain's operations. The agent presents ambiguous matches as disambiguation prompts.
**Objects**: wiring, surface, domain-model, layout-node, binding, operation, presentation-hint

**Constraints**:
- Inference is allowed to use AI, deterministic rules, or both — v4 §8 explicitly leaves the implementation open. Common cases must be handled by rules; the AI is a fallback for ambiguity
- The result of inference is never written back to the layout — wiring stays in generated code, derived fresh each codegen pass
- Rule-based examples that must work without AI involvement: a `clarity.datagrid-column` with `contentShape: badge` whose surface `Shows:` field is an enum-typed domain field → render as Clarity badge; a `clarity.button` with `label: Create task` whose surface `Actions:` includes `invoke-create` → wire to the `createTask` domain operation
- Ambiguous bindings — e.g., two domain operations could match the same button — produce a disambiguation prompt during generation, not a silent guess
- A generated component's binding is traceable back to its (layout-node, surface-fragment, domain-element) source triple, so a designer can ask "why did this column render as a badge?" and get an answer
- Inference is bounded to the active feature — codegen does not pull in wiring from other features' surfaces or domains

**Verify**:
- The Tasks-screen example in the v4 appendix produces working Angular + Clarity code with all columns wired to `Task` fields, the create button wired to `createTask`, and the status column rendering as a Clarity badge using `TaskStatus`'s presentation tones
- A layout node with no matching surface fragment produces a generation-time warning naming the orphan node, not a silent miss
- A layout node with multiple matching surface fragments triggers a disambiguation prompt
- Removing a field from the domain model and re-running codegen produces an actionable error pointing at the layout nodes that referenced the removed field

**Questions**:
- What is the boundary between "rules that always run" and "AI that fills the gap"? §8 leaves the split open. Decide during dialog authoring with a starter rule set in mind.
- Should inference results be cached across runs to keep codegen deterministic? §9 explicitly forbids "any always-on cache or inference loop" inside Studio, but Core's codegen cache is a separate question.

---

## Backward-Compatible Path for Layout-Free Pages

**Goal**: Projects that have not adopted Studio yet — or features that simply do not have a layout authored — must continue to codegen exactly as they do today.
**Persona**: UX Designer (existing parlay user not running Studio)
**Priority**: P0
**Context**: Studio is opt-in. Many projects will have pages without layout blocks for some time, possibly indefinitely. Those projects must see no behavior change from this Core release.
**Action**: When a page has no `## Layout` section, the codegen path falls through to the existing logic — surface + domain + adapter produce the prototype the same way they always have. The layout-aware path activates only when a layout block is present and parses successfully.
**Objects**: page, layout, generate-code, backward-compatibility

**Constraints**:
- The presence-of-layout decision is per-page, not per-project — one feature can have layouts and another can not, in the same codegen pass
- If a page has a layout block but the block fails parse (vocabulary mismatch, schema-version mismatch), codegen fails the page rather than silently falling back to the layout-free path. Silent fallback would mask a real authoring error
- Existing testcase generation continues to work for layout-free pages
- Documentation makes the opt-in nature visible — running `parlay generate-code` on a project with no layouts produces the same output as before

**Verify**:
- A project with no layouts at all generates byte-identical output before and after this feature ships, given the same adapter version and source state
- A project with one layout-bearing page and one layout-free page generates each correctly — layout-driven output for the first, surface-only output for the second
- A page with a malformed layout block fails generation for that page; layout-free pages in the same project still generate

---
