# Infrastructure Schema — authoring digest

Derived from `infrastructure.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

The two artifacts answer different questions about the feature; the choice is not enforced by a validator and the schema does not auto-classify. Use prose judgment, guided by the question the fragment is trying to answer.

- **`capabilities.yaml`** answers "what command or query does the backend expose?" — a closed-vocabulary operation against a domain entity, with input, steps, output shape, and allowed errors. If the fragment can be expressed as `kind: command | query` plus a subject entity, it belongs in `capabilities.yaml`.
- **`infrastructure.md`** answers "what shape must the codebase hold for those operations to work safely?" — a constraint on imports, a check that must run at startup, a bounded vocabulary of external calls, a pinned library version, a feature-stable error code outside the closed errors vocabulary, a build-time invariant. If the fragment describes a property of the source tree, the build pipeline, or the runtime environment rather than an operation a caller triggers, it belongs in `infrastructure.md`.

Many features have both. A feature that introduces a new operation typically also introduces architectural constraints around it (which package the operation lives in, which external services it may call, which library versions are required) — the operation lands in `capabilities.yaml`, the constraints in `infrastructure.md`, and the buildfile composes both.

Four representative architectural categories are worked through in the examples below: **boundary**, **probe**, **allowlist**, and **dependency pin**. These categories are illustrative, not exhaustive — other architectural concerns also belong in `infrastructure.md`. Authors of new fragments may extend the list freely; the schema is advisory and does not enforce a closed taxonomy.

---

```
# <Feature Name> — Infrastructure

---

## <Fragment Name>

**Affects**: <abstract scope — domain-level labels like "package import boundary", "startup probe">
**Behavior**: <human-readable description of the constraint, framework-agnostic>
**Invariants**:
- <testable property that must hold after implementation>
- <another invariant>
**Source**: @feature/intent-slug
**Caching**: <abstract strategy — on-first-access, none, per-process>
**Backward-Compatible**: yes | no

**Notes**:
- <Additional constraints, design decisions, edge cases>
```

---

| Field | Required | Parse rule |
|---|---|---|
| Fragment Name | Yes | `## ` heading. Must be unique within feature. |
| Affects | Yes | `**Affects**:` single-line description of the abstract scope of the constraint. Domain-level labels (e.g., `package import boundary`, `startup probe`), not file paths or function names. |
| Behavior | Yes | `**Behavior**:` human-readable description of what the constraint requires, in framework-agnostic terms. Tells the agent WHAT the codebase shape must guarantee; the adapter and the agent decide HOW at build-feature and generate-code time. |
| Invariants | No | `**Invariants**:` followed by `- ` prefixed lines. Each bullet is one declarative, testable property (e.g., "A package outside internal/sdk that imports the upstream SDK fails the build with a named lint"). Used by build-feature to seed testcases. |
| Source | Yes | `**Source**:` comma-separated `@feature/slug` references. Every fragment must trace back to its source intent(s). |
| Caching | No | `**Caching**:` abstract caching strategy. Values: `on-first-access`, `none`, `per-process`, or a custom description. |
| Backward-Compatible | No | `**Backward-Compatible**:` `yes` or `no`. Whether existing callers must continue working without changes. |
| Notes | No | `**Notes**:` followed by `- ` prefixed lines. Additional constraints, design decisions, edge cases. |
| Deliberately-Specific | No | `**Deliberately-Specific**:` followed by one line of justification. Suppresses the portability lint for this fragment. See "Promoted fragments and deliberate specificity" below. |

---

- `Affects`, `Behavior`, and `Source` are required on every fragment. Validation errors out if any are missing.
- `Affects` and `Behavior` are **framework-agnostic**. They must NOT contain: function names or signatures (e.g., `classifyDir(path string)`), file paths with language-specific extensions (e.g., `internal/config/config.go`, `app/models/user.rb`), language keywords that imply implementation (`func`, `def`, `class`, `interface`, `struct`, `impl`, `enum`, `trait`, `module`), or qualified import paths. The portability lint emits warnings (not errors) when it detects these — the file is still valid, but the warnings flag content that should be rephrased in domain terms.
- Fragments MAY reference domain concepts from the intents (e.g., "feature", "initiative", "intents.md", "qualified identifier") because these are part of the problem domain, not the framework. The line is: domain vocabulary is allowed; implementation vocabulary is not.
- Concrete `target-files:`, `target-pattern:`, and `introduces:` values do **not** belong in infrastructure.md. They are generated at build-feature time by the adapter bridge (see Buildfile mapping below).
- Fragment names must be unique within the feature's infrastructure.md.

---

`infrastructure.md` is the promotion target for implementation-shaped refinements — a change stated in a person's words that is real, is architectural, and belongs in the spec rather than only in the code.

The alternative is what happens without it: someone prompts an agent directly, the change lands in code, and the spec never learns about it. Every subsequent drift check compares generated output against a specification that no longer describes what the system does, and the divergence is invisible because nothing recorded that it happened. Promotion is how a change becomes part of the design rather than an undocumented local edit.

A promoted fragment is an ordinary fragment. It carries the same required fields, and in particular it carries a **resolvable `Source:`** — but the source of a promoted fragment is a person's request, not a pre-existing intent. Cite the intent the refinement modifies when there is one; when the refinement introduces a concern no intent covers, the promotion has surfaced a gap in the intents, and the honest move is to say so rather than to invent a citation that resolves.

### Promoted fragments and deliberate specificity

The portability lint warns when `Affects` or `Behavior` contains implementation vocabulary — function names, file paths with language extensions, language keywords. It **warns rather than forbids**, which is deliberate: specificity is allowed here, because some architectural constraints are genuinely about a named thing. "No package outside `internal/sdk` may import the upstream SDK" is not improvable by being made vaguer.

Promotion makes that case common rather than rare, and a warning that fires on every promoted fragment forever is worse than no warning: people learn to scroll past the category, and the *accidental* specificity the lint exists to catch scrolls past with it.

So a fragment may declare `**Deliberately-Specific**:` with a one-line justification, which suppresses the lint for that fragment. The justification is the point — it is not a mute switch, it is a claim on the record that this fragment names a specific thing because the constraint is about that thing. A reviewer can disagree with it; nobody can disagree with a warning that was never read.

Do not add it to silence a fragment that could have been phrased in domain terms. That trades a warning you would have fixed for a permanent one you have promised not to.

---

Infrastructure fragments are translated into `cross-cutting:` entries by the **adapter bridge** at build-feature time. The translation is not a 1:1 field rename — it is a resolution step that consults the adapter and the existing source tree:

1. Build-feature reads `Affects:` to determine what area of the codebase the constraint touches.
2. It consults the adapter's `file-conventions` and `coding-conventions` to know how that area is organized in the current framework (e.g., a Go CLI puts shared resolvers in `internal/<area>/`; a Python service puts them in `<area>/__init__.py`).
3. It scans the existing source tree to find concrete files matching the abstract scope, producing the buildfile entry's `target-files:` (explicit paths) or `target-pattern:` (a grep pattern for fan-out).
4. It reads `Behavior:` to understand the constraint and emits a framework-specific `transform:` describing what the code must do.
5. It infers `introduces:` (new functions, types, constants, lints) from `Behavior:` plus the adapter's naming and structure conventions.
6. `Source:` carries through verbatim as `source:`.
7. `Invariants:` seed the testcases generated for the cross-cutting entry.
8. `Caching:`, `Backward-Compatible:`, and `Notes:` carry through as hints embedded in `transform:` or as separate buildfile fields.

When `Affects:` cannot be resolved to any file in the source tree, build-feature pauses and asks the designer which files are affected — it never guesses.

The same infrastructure.md combined with a different adapter produces different `cross-cutting:` entries appropriate to that adapter's framework. The fragment provides the WHAT (constraint + invariants); the adapter provides the HOW.
