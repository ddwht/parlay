# Infrastructure Layer — Infrastructure

---

## Framework-Agnostic Schema Definition

**Affects**: schema definition, validation pipeline
**Behavior**: Replace the current infrastructure schema fields that contain framework-specific content (Modifies, Introduces, Detection) with framework-agnostic equivalents. The new schema defines: Affects (abstract scope of the change — domain-level labels like "feature resolution", not file paths), Behavior (what the capability does, framework-agnostic), Invariants (testable properties that must hold after implementation), Source (traceability), Caching (abstract strategy: on-first-access, none, per-process), Backward-Compatible (yes/no), and Notes. The schema file must be the single source of truth for what infrastructure.md can contain, and must be deployable via the standard schema deployment mechanism.
**Invariants**:
- A fragment with Affects, Behavior, and Source passes validation
- A fragment missing Affects fails validation with an error naming the fragment
- A fragment missing Behavior fails validation with an error naming the fragment
- A fragment missing Source fails validation with an error naming the fragment
- A fragment with all fields present and valid Source references passes deep validation
- The schema file is deployable and loadable by skills at runtime
**Source**: @infrastructure-layer/define-the-infrastructure-schema
**Backward-Compatible**: no

**Notes**:
- This is a breaking change to the infrastructure schema — existing infrastructure.md files authored with Modifies/Introduces/Detection will need migration to the new Affects/Behavior/Invariants format
- The old fields are not "deprecated" — they are removed from infrastructure.md entirely and exist only in the buildfile's cross-cutting section, where they belong

---

## Portability Lint

**Affects**: validation pipeline
**Behavior**: Add a portability check to infrastructure validation that detects framework-specific content in Behavior and Affects fields. The lint should warn (not error) when it finds: function signatures with type annotations (parenthesized parameter lists with types), file paths with language-specific extensions (.go, .py, .ts, .rs), language keywords that indicate implementation rather than behavior (func, def, class, interface, struct), and qualified import paths. The warning message should name the fragment, quote the offending content, and suggest rephrasing in domain terms. Portability warnings are distinct from validation errors — a file can pass validation but have portability warnings.
**Invariants**:
- Behavior text containing "func classifyDir(path string)" triggers a portability warning
- Behavior text containing "internal/config/config.go" triggers a portability warning
- Behavior text containing "classify each directory as feature, initiative, or deferred" does NOT trigger a warning
- Affects text containing "feature resolution, directory traversal" does NOT trigger a warning
- Affects text containing "internal/config/config.go" triggers a portability warning
- Portability warnings do not cause validation to fail — the file is still valid
**Source**: @infrastructure-layer/define-the-infrastructure-schema
**Backward-Compatible**: yes

---

## Adapter Bridge for Infrastructure Resolution

**Affects**: build pipeline, adapter resolution
**Behavior**: Extend the build-feature pipeline to translate framework-agnostic infrastructure fragments into framework-specific buildfile cross-cutting entries. For each infrastructure fragment, the pipeline reads the Affects field to determine what area of the codebase to scan, reads the Behavior field to understand the capability, consults the adapter's file conventions and coding conventions to determine framework-specific patterns, scans the existing source tree to find concrete files matching the abstract scope, and generates a cross-cutting entry with framework-specific target-files, target-pattern, introduces, and transform fields. When Affects cannot be resolved to any file in the source tree, the pipeline asks the designer which files are affected rather than guessing. The translation is adapter-aware: different adapters produce different cross-cutting entries from the same infrastructure fragment. Invariants from the fragment are used to generate testcases for the cross-cutting entry.
**Invariants**:
- An infrastructure fragment with Affects "feature resolution" produces a cross-cutting entry with concrete target-files pointing to the framework's path-resolution code
- The same infrastructure fragment with a different adapter produces different target-files appropriate to that adapter's framework
- A fragment whose Affects scope matches zero files in the source tree triggers an interactive question asking the designer for guidance
- The generated cross-cutting entry contains framework-specific function names and file paths even though the infrastructure fragment contained none
- Generated cross-cutting entries pass deep buildfile validation
**Source**: @infrastructure-layer/bridge-infrastructure-to-framework-specific-cross-cutting
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The resolution is less mechanical than surface-to-widget mapping — infrastructure behaviors are more varied than UI primitives, so the agent's judgment plays a larger role
- The adapter's conventions section guides naming, error handling style, and code organization for the generated cross-cutting entries
- Build-feature must never read back to infrastructure.md during cross-cutting entry generation for information that should come from the adapter — the fragment provides the WHAT, the adapter provides the HOW

---

## Feature Readiness with Infrastructure-Only Support

**Affects**: readiness validation
**Behavior**: Update the readiness check at the build-feature stage to accept a feature that has infrastructure.md but no surface.md. The rule changes from "surface.md must exist" to "at least one of surface.md or infrastructure.md must exist." A feature with neither still fails. A feature with both passes. The readiness check must also validate that the infrastructure.md file conforms to the current schema (framework-agnostic fields, not the old framework-specific fields) before accepting it.
**Invariants**:
- A feature with only infrastructure.md passes readiness at the build-feature stage
- A feature with only surface.md passes readiness (unchanged behavior)
- A feature with both surface.md and infrastructure.md passes readiness
- A feature with neither fails readiness with an error naming both files
- A feature with infrastructure.md using old-format fields (Modifies, Introduces) fails readiness with a migration error
**Source**: @infrastructure-layer/define-the-infrastructure-artifact
**Backward-Compatible**: yes

---
