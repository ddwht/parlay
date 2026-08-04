# Infrastructure Schema

File: `spec/intents/<feature-name>/infrastructure.md`
Contains one or more infrastructure fragments separated by `---`. Each fragment is **architectural prose for concerns that do not reduce to operations**: boundaries the codebase must respect, probes it must run, allowlists it must enforce, dependency pins it must hold, and other shape constraints on the source tree, the build pipeline, or the runtime environment.

Architectural prose is the co-equal counterpart to the operation-shaped content that lives in `capabilities.yaml`. The two artifacts cover orthogonal concerns and neither is a stand-in for the other: `capabilities.yaml` declares what the backend *does* (commands and queries against domain entities), and `infrastructure.md` declares what shape the codebase *holds* (architectural prose that constrains how those operations can be implemented). See `capabilities.schema.md` for the operation-shaped artifact.

Infrastructure fragments are also the behind-the-scenes counterpart to surface fragments. Surface fragments describe what the user sees; infrastructure fragments describe what shape the codebase needs. Both feed the buildfile: surface → `components:`, infrastructure → `cross-cutting:`. All four spec artifacts (surface, capabilities, infrastructure, domain-model) are **framework-agnostic** — concrete file paths, function signatures, and language keywords are resolved at build-feature time by consulting the adapter and scanning the existing source tree.

## When to use infrastructure.md vs capabilities.yaml

The two artifacts answer different questions about the feature; the choice is not enforced by a validator and the schema does not auto-classify. Use prose judgment, guided by the question the fragment is trying to answer.

- **`capabilities.yaml`** answers "what command or query does the backend expose?" — a closed-vocabulary operation against a domain entity, with input, steps, output shape, and allowed errors. If the fragment can be expressed as `kind: command | query` plus a subject entity, it belongs in `capabilities.yaml`.
- **`infrastructure.md`** answers "what shape must the codebase hold for those operations to work safely?" — a constraint on imports, a check that must run at startup, a bounded vocabulary of external calls, a pinned library version, a feature-stable error code outside the closed errors vocabulary, a build-time invariant. If the fragment describes a property of the source tree, the build pipeline, or the runtime environment rather than an operation a caller triggers, it belongs in `infrastructure.md`.

Many features have both. A feature that introduces a new operation typically also introduces architectural constraints around it (which package the operation lives in, which external services it may call, which library versions are required) — the operation lands in `capabilities.yaml`, the constraints in `infrastructure.md`, and the buildfile composes both.

Four representative architectural categories are worked through in the examples below: **boundary**, **probe**, **allowlist**, and **dependency pin**. These categories are illustrative, not exhaustive — other architectural concerns also belong in `infrastructure.md`. Authors of new fragments may extend the list freely; the schema is advisory and does not enforce a closed taxonomy.

## Template

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

## Fields

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

## Constraints

- `Affects`, `Behavior`, and `Source` are required on every fragment. Validation errors out if any are missing.
- `Affects` and `Behavior` are **framework-agnostic**. They must NOT contain: function names or signatures (e.g., `classifyDir(path string)`), file paths with language-specific extensions (e.g., `internal/config/config.go`, `app/models/user.rb`), language keywords that imply implementation (`func`, `def`, `class`, `interface`, `struct`, `impl`, `enum`, `trait`, `module`), or qualified import paths. The portability lint emits warnings (not errors) when it detects these — the file is still valid, but the warnings flag content that should be rephrased in domain terms.
- Fragments MAY reference domain concepts from the intents (e.g., "feature", "initiative", "intents.md", "qualified identifier") because these are part of the problem domain, not the framework. The line is: domain vocabulary is allowed; implementation vocabulary is not.
- Concrete `target-files:`, `target-pattern:`, and `introduces:` values do **not** belong in infrastructure.md. They are generated at build-feature time by the adapter bridge (see Buildfile mapping below).
- Fragment names must be unique within the feature's infrastructure.md.

## Promotion: where a refinement lands

`infrastructure.md` is the promotion target for implementation-shaped refinements — a change stated in a person's words that is real, is architectural, and belongs in the spec rather than only in the code.

The alternative is what happens without it: someone prompts an agent directly, the change lands in code, and the spec never learns about it. Every subsequent drift check compares generated output against a specification that no longer describes what the system does, and the divergence is invisible because nothing recorded that it happened. Promotion is how a change becomes part of the design rather than an undocumented local edit.

A promoted fragment is an ordinary fragment. It carries the same required fields, and in particular it carries a **resolvable `Source:`** — but the source of a promoted fragment is a person's request, not a pre-existing intent. Cite the intent the refinement modifies when there is one; when the refinement introduces a concern no intent covers, the promotion has surfaced a gap in the intents, and the honest move is to say so rather than to invent a citation that resolves.

### Promoted fragments and deliberate specificity

The portability lint warns when `Affects` or `Behavior` contains implementation vocabulary — function names, file paths with language extensions, language keywords. It **warns rather than forbids**, which is deliberate: specificity is allowed here, because some architectural constraints are genuinely about a named thing. "No package outside `internal/sdk` may import the upstream SDK" is not improvable by being made vaguer.

Promotion makes that case common rather than rare, and a warning that fires on every promoted fragment forever is worse than no warning: people learn to scroll past the category, and the *accidental* specificity the lint exists to catch scrolls past with it.

So a fragment may declare `**Deliberately-Specific**:` with a one-line justification, which suppresses the lint for that fragment. The justification is the point — it is not a mute switch, it is a claim on the record that this fragment names a specific thing because the constraint is about that thing. A reviewer can disagree with it; nobody can disagree with a warning that was never read.

Do not add it to silence a fragment that could have been phrased in domain terms. That trades a warning you would have fixed for a permanent one you have promised not to.

## Worked examples

Four fragments drawn from real architectural categories. Each populates the field set above without changing the field semantics; together they illustrate the breadth of content that belongs in `infrastructure.md` rather than `capabilities.yaml`. The set is representative, not exhaustive — other architectural concerns also belong here. Parlay's own historical infrastructure.md fragments (skill deployment, registry traversal, validation pipeline) are excluded from this example set by design so that the schema documentation stays project-agnostic.

```
# Example feature — Infrastructure

---

## SDK import boundary

**Affects**: package import boundary
**Behavior**: Only the dedicated SDK wrapper package may import the upstream SDK directly. Every other package goes through the wrapper, so swapping the underlying SDK is a single-file change.
**Invariants**:
- A package outside the wrapper that imports the upstream SDK fails the build with a named lint that points at the offending import line.
- The wrapper exports a stable surface that does not leak SDK-specific types into callers.
**Source**: @example/sdk-boundary
**Backward-Compatible**: yes

---

## External-system startup probe

**Affects**: startup probe
**Behavior**: At process startup, the application probes every required external system once and either records a healthy result or aborts startup with a structured error that names the failed system and the probe's URL.
**Invariants**:
- A failed probe aborts startup before any request handler is registered.
- The probe result is recorded in a process-wide status surface visible to readiness handlers.
- Startup logs name the probed system, the URL, and the outcome on every run.
**Source**: @example/startup-probe
**Caching**: per-process

---

## Wrapper API allowlist

**Affects**: bounded vocabulary of wrapper calls
**Behavior**: The SDK wrapper exposes a closed allowlist of operations to callers. Any wrapper method not on the allowlist is unreachable; adding a method requires a deliberate allowlist edit.
**Invariants**:
- Calling a wrapper method outside the allowlist produces a build error naming the method and the allowlist file.
- The allowlist is the single source of truth — no per-caller exception flags exist.
- The allowlist is reviewable as one file, not scattered across call sites.
**Source**: @example/wrapper-allowlist
**Backward-Compatible**: yes

---

## Library version pin

**Affects**: dependency version baseline
**Behavior**: The project pins the upstream library to a minimum version compatible with the feature's API surface; older versions fail the build at dependency resolution time rather than at runtime.
**Invariants**:
- Lowering the pinned version below the documented floor fails the build with a named error that points at the manifest line.
- The pin is documented adjacent to the manifest entry so a reader sees both the version and the rationale together.
**Source**: @example/version-pin
**Backward-Compatible**: yes
```

## Buildfile mapping

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

## Validation

When an infrastructure file is loaded, the tool verifies that every fragment has a unique `## ` name, that the three required fields are present, that `Source` references point to existing intents (when `--deep` validation is enabled), and that `Backward-Compatible`, if present, is `yes` or `no`.

| Code | When it fires |
|---|---|
| `missing-affects` | A fragment has no `**Affects**:` field. |
| `missing-behavior` | A fragment has no `**Behavior**:` field. |
| `missing-source` | A fragment has no `**Source**:` reference, so nothing records which intent it came from and a change described in prose cannot be routed to it. |

These three were documented as prose bullets rather than as a table for as long as this schema existed, which made them invisible to the conformance check that asserts every documented code is actually emitted — the check reads tables only. All three were emitted the whole time; the documentation simply could not be verified against the implementation. A code in a bullet list is a promise nothing holds you to.

The schema is **advisory** with respect to operation-shaped content: no validator rule rejects a fragment in `infrastructure.md` simply because it looks operation-shaped. The `migrate-capabilities` command is the only enforcement path and is opt-in; running it moves operation-shaped fragments into `capabilities.yaml` while leaving architectural prose in place. Authors who keep operation-shaped fragments in `infrastructure.md` accept that the migrator will move them on the next opt-in run.

Portability lint (warnings, non-blocking) scans `Affects` and `Behavior` for:
- Function signatures (parenthesized parameter lists with type annotations)
- File extensions (`.go`, `.py`, `.ts`, `.js`, `.rs`, `.java`, `.rb`, `.swift`, `.kt`)
- Language keywords (`func`, `def`, `class`, `interface`, `struct`, `impl`, `enum`, `trait`, `module`)
- Qualified import paths

Each warning names the fragment, quotes the offending content, and suggests rephrasing in domain terms. Portability lint is unchanged by the architectural-scope clarification — fragments that already pass the lint continue to pass.

## Parsing

- Fragment boundaries: `---` separators
- Fragment name: `## ` heading
- Field extraction: `**Field**:` pattern
- Affects: text after `**Affects**:`
- Behavior: text after `**Behavior**:`
- Invariants: `- ` prefixed lines under `**Invariants**:`
- Source: comma-separated `@` prefixed values after `**Source**:`
- Caching: text after `**Caching**:`
- Backward-Compatible: `yes` or `no` after `**Backward-Compatible**:`
- Notes: `- ` prefixed lines under `**Notes**:`
