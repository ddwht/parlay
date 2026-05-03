# Infrastructure Schema

File: `spec/intents/<feature-name>/infrastructure.md`
Contains one or more infrastructure fragments separated by `---`. Describes behind-the-scenes behavioral capabilities — classifiers, validation guards, traversal changes, caching strategies, shared resolvers — that intent constraints require but that produce no user-facing surface.

Infrastructure fragments are the behind-the-scenes counterpart to surface fragments. Surface fragments describe what the user sees; infrastructure fragments describe what behavioral capabilities the codebase needs. Both feed the buildfile: surface → `components:`, infrastructure → `cross-cutting:`. Both are **framework-agnostic** — concrete file paths, function signatures, and language keywords are resolved at build-feature time by consulting the adapter and scanning the existing source tree.

## Template

```
# <Feature Name> — Infrastructure

---

## <Fragment Name>

**Affects**: <abstract scope — domain-level labels like "feature resolution", "validation pipeline">
**Behavior**: <human-readable description of the capability, framework-agnostic>
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
| Affects | Yes | `**Affects**:` single-line description of the abstract scope of the change. Domain-level labels (e.g., `feature resolution`, `validation pipeline`), not file paths or function names. |
| Behavior | Yes | `**Behavior**:` human-readable description of what the capability does, in framework-agnostic terms. Tells the agent WHAT the code must do; the adapter and the agent decide HOW at build-feature and generate-code time. |
| Invariants | No | `**Invariants**:` followed by `- ` prefixed lines. Each bullet is one declarative, testable property (e.g., "A fragment missing Affects fails validation with an error naming the fragment"). Used by build-feature to seed testcases. |
| Source | Yes | `**Source**:` comma-separated `@feature/slug` references. Every fragment must trace back to its source intent(s). |
| Caching | No | `**Caching**:` abstract caching strategy. Values: `on-first-access`, `none`, `per-process`, or a custom description. |
| Backward-Compatible | No | `**Backward-Compatible**:` `yes` or `no`. Whether existing callers must continue working without changes. |
| Notes | No | `**Notes**:` followed by `- ` prefixed lines. Additional constraints, design decisions, edge cases. |

## Constraints

- `Affects`, `Behavior`, and `Source` are required on every fragment. Validation errors out if any are missing.
- `Affects` and `Behavior` are **framework-agnostic**. They must NOT contain: function names or signatures (e.g., `classifyDir(path string)`), file paths with language-specific extensions (e.g., `internal/config/config.go`, `app/models/user.rb`), language keywords that imply implementation (`func`, `def`, `class`, `interface`, `struct`, `impl`, `enum`, `trait`, `module`), or qualified import paths. The portability lint emits warnings (not errors) when it detects these — the file is still valid, but the warnings flag content that should be rephrased in domain terms.
- Fragments MAY reference domain concepts from the intents (e.g., "feature", "initiative", "intents.md", "qualified identifier") because these are part of the problem domain, not the framework. The line is: domain vocabulary is allowed; implementation vocabulary is not.
- Concrete `target-files:`, `target-pattern:`, and `introduces:` values do **not** belong in infrastructure.md. They are generated at build-feature time by the adapter bridge (see Buildfile mapping below).
- Fragment names must be unique within the feature's infrastructure.md.

## Buildfile mapping

Infrastructure fragments are translated into `cross-cutting:` entries by the **adapter bridge** at build-feature time. The translation is not a 1:1 field rename — it is a resolution step that consults the adapter and the existing source tree:

1. Build-feature reads `Affects:` to determine what area of the codebase the capability touches.
2. It consults the adapter's `file-conventions` and `coding-conventions` to know how that area is organized in the current framework (e.g., a Go CLI puts shared resolvers in `internal/<area>/`; a Python service puts them in `<area>/__init__.py`).
3. It scans the existing source tree to find concrete files matching the abstract scope, producing the buildfile entry's `target-files:` (explicit paths) or `target-pattern:` (a grep pattern for fan-out).
4. It reads `Behavior:` to understand the capability and emits a framework-specific `transform:` describing what the code must do.
5. It infers `introduces:` (new functions, types, constants) from `Behavior:` plus the adapter's naming and structure conventions.
6. `Source:` carries through verbatim as `source:`.
7. `Invariants:` seed the testcases generated for the cross-cutting entry.
8. `Caching:`, `Backward-Compatible:`, and `Notes:` carry through as hints embedded in `transform:` or as separate buildfile fields.

When `Affects:` cannot be resolved to any file in the source tree, build-feature pauses and asks the designer which files are affected — it never guesses.

The same infrastructure.md combined with a different adapter produces different `cross-cutting:` entries appropriate to that adapter's framework. The fragment provides the WHAT (capability + invariants); the adapter provides the HOW.

## Validation

When an infrastructure file is loaded, the tool verifies:
- Every fragment has a unique `## ` name
- Every fragment has an `Affects` field (error: `missing-affects`)
- Every fragment has a `Behavior` field (error: `missing-behavior`)
- Every fragment has a `Source` reference (error: `missing-source`)
- Source references point to existing intents (when `--deep` validation is enabled)
- If `Backward-Compatible` is present, its value is `yes` or `no`

Portability lint (warnings, non-blocking) scans `Affects` and `Behavior` for:
- Function signatures (parenthesized parameter lists with type annotations)
- File extensions (`.go`, `.py`, `.ts`, `.js`, `.rs`, `.java`, `.rb`, `.swift`, `.kt`)
- Language keywords (`func`, `def`, `class`, `interface`, `struct`, `impl`, `enum`, `trait`, `module`)
- Qualified import paths

Each warning names the fragment, quotes the offending content, and suggests rephrasing in domain terms.

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
