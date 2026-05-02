# Infrastructure Schema

File: `spec/intents/<feature-name>/infrastructure.md`
Contains one or more infrastructure fragments separated by `---`. Describes behind-the-scenes changes — modifications to shared code, resolvers, helpers, and internal patterns — that intent constraints require but that produce no user-facing surface.

Infrastructure fragments are the behind-the-scenes counterpart to surface fragments. Surface fragments describe what the user sees; infrastructure fragments describe what changes in the codebase. Both feed the buildfile: surface → `components:`, infrastructure → `cross-cutting:`.

## Template

```
# <Feature Name> — Infrastructure

---

## <Fragment Name>

**Modifies**: <comma-separated existing functions, types, or files being changed>
**Introduces**: <comma-separated new functions, types, or constants being added>
**Detection**: <grep pattern for finding affected files across the source tree>
**Behavior**: <human-readable description of what the change does>
**Source**: @feature/intent-slug
**Caching**: <caching strategy — tree-scan-on-first-access, none, per-process>
**Backward-Compatible**: yes | no

**Notes**:
- <Additional constraints, design decisions, edge cases>
```

## Fields

| Field | Required | Parse rule |
|---|---|---|
| Fragment Name | Yes | `## ` heading. Must be unique within feature. |
| Modifies | No | `**Modifies**:` comma-separated identifiers of existing functions, types, or file paths being changed. |
| Introduces | No | `**Introduces**:` comma-separated identifiers of new functions, types, or constants being added. Each may include an optional signature in parentheses. |
| Detection | No | `**Detection**:` a literal string or regex pattern. Used by generate-code to grep the source tree and resolve a target-pattern to a concrete file list for fan-out changes. |
| Behavior | Yes | `**Behavior**:` human-readable description of what the change does. Not executable code — tells the agent WHAT to change; the agent decides HOW via Tier 2 intelligent merge. |
| Source | Yes | `**Source**:` comma-separated `@feature/slug` references. Every fragment must trace back to its source intent(s). |
| Caching | No | `**Caching**:` caching strategy for the introduced or modified code. Values: `tree-scan-on-first-access`, `none`, `per-process`, or a custom description. |
| Backward-Compatible | No | `**Backward-Compatible**:` `yes` or `no`. Whether existing callers of modified functions must continue working without changes. |
| Notes | No | `**Notes**:` followed by `- ` prefixed lines. Additional constraints, design decisions, edge cases. |

## Constraints

- At least one of `Modifies` or `Introduces` must be present. A fragment that neither modifies existing code nor introduces new code has no purpose.
- `Behavior` is required and human-readable. It tells the agent WHAT to change; the agent decides HOW using Tier 2 intelligent merge against each target file. This preserves the determinism contract at the intent level.
- `Detection` is used only for fan-out changes — when the same transformation applies to multiple files matching a pattern (e.g., every file containing `os.ReadDir(intentsDir)`). If absent, the fragment applies only to the files explicitly named in `Modifies`.
- Fragment names must be unique within the feature's infrastructure.md.

## Buildfile mapping

Build-feature maps each infrastructure fragment to a `cross-cutting:` entry in the buildfile:

| Infrastructure field | Buildfile cross-cutting field |
|---|---|
| Modifies | `target-files:` |
| Introduces | `introduces:` |
| Detection | `target-pattern:` |
| Behavior | `transform:` |
| Source | `source:` |
| Caching | Included in `transform:` or as a hint field |
| Backward-Compatible | Included in `transform:` or as a constraint flag |
| Notes | Appended to `transform:` as additional context |

## Validation

When an infrastructure file is loaded, the tool verifies:
- Every fragment has a unique name
- Every fragment has a Behavior field
- Every fragment has a Source reference
- Every fragment has at least one of Modifies or Introduces
- Source references point to existing intents (when `--deep` validation is enabled)
- If Backward-Compatible is present, its value is `yes` or `no`

## Parsing

- Fragment boundaries: `---` separators
- Fragment name: `## ` heading
- Field extraction: `**Field**:` pattern
- Modifies parsing: comma-separated identifiers after `**Modifies**:`
- Introduces parsing: comma-separated identifiers after `**Introduces**:`
- Detection: single string or regex after `**Detection**:`
- Behavior: text after `**Behavior**:`
- Source references: comma-separated `@` prefixed values
- Caching: text after `**Caching**:`
- Backward-Compatible: `yes` or `no` after `**Backward-Compatible**:`
- Notes: `- ` prefixed lines under `**Notes**:`
