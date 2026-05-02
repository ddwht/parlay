# Qualified Identifier Resolver

> Cross-cutting infrastructure changes required by the `initiatives` feature: path resolvers must accept qualified identifiers (`<initiative>/<feature>`), and bulk-walker commands must traverse nested feature directories. Implementation specifics live in `infrastructure.md`; the buildfile's `cross-cutting:` section schema is formalized here. Assumes the structural model defined in the `initiatives` feature and the infrastructure pipeline defined in `infrastructure-layer`.

---

## Support Qualified Identifiers in Path Resolvers

**Goal**: Ensure path resolution helpers accept qualified identifiers so that every command works seamlessly for both orphan and initiative-nested features without per-command branching.
**Persona**: Parlay Developer
**Priority**: P1
**Context**: The `initiatives` feature introduces a two-level hierarchy where features can live either at `spec/intents/<feature>/` (orphan) or at `spec/intents/<initiative>/<feature>/` (nested). Today, `config.FeaturePath(slug)` does a simple `filepath.Join` — it doesn't know about initiatives. Centralizing initiative-awareness in the resolver is the only scalable approach; duplicating the logic per-command would be fragile.
**Action**: Modify the path resolution helpers to accept either a bare slug or a qualified identifier containing `/`, resolve to the correct path on the requested tree, and fail defensively when a bare slug is ambiguous. Implementation details (function signatures, caching strategy, backward-compatibility rules) are captured in this feature's `infrastructure.md`.
**Objects**: config, resolver, qualified-identifier, feature, initiative

**Constraints**:
- The resolver change must be backward-compatible — every existing caller passes a bare slug today and must continue to work without modification
- Ambiguous bare slugs (same slug exists as orphan and inside one or more initiatives) must error, not guess
- The change applies to three helpers in lockstep (`FeaturePath`, `BuildPath`, `HandoffPath`) — identical logic, different tree roots
- This is a cross-cutting change that flows through `infrastructure.md` → buildfile `cross-cutting:` → generate-code Tier 2 intelligent merge

**Verify**:
- `config.FeaturePath("password-reset")` resolves to `spec/intents/password-reset/` for an orphan
- `config.FeaturePath("auth-overhaul/password-reset")` resolves to `spec/intents/auth-overhaul/password-reset/`
- `config.BuildPath("auth-overhaul/password-reset")` resolves to `.parlay/build/auth-overhaul/password-reset/`
- Passing an ambiguous bare slug returns an error naming all matching paths
- Every existing caller continues to compile and return the same path as before

---

## Centralize Feature Enumeration for Bulk-Walker Commands

**Goal**: Ensure that every command enumerating features across the project sees both orphan and initiative-nested features, so that moving a feature into an initiative never silently hides it from a project-wide scan.
**Persona**: Parlay Developer
**Priority**: P1
**Context**: Several existing commands enumerate all features by scanning `spec/intents/` one level deep via `os.ReadDir`. With initiatives, features live one level deeper. These commands will silently miss nested features unless updated to use a shared traversal helper. Implementation details (helper signature, classification logic, detection pattern for finding callers) are captured in this feature's `infrastructure.md`.
**Action**: Provide a shared enumeration helper that walks both levels, classifies directories per the initiatives structural model, and returns qualified identifiers. Update every bulk-walker command to use it instead of inline scanning.
**Objects**: config, bulk-walker, feature-enumeration, qualified-identifier

**Constraints**:
- The helper must return qualified identifiers consistent with the resolver — commands can pass them directly to `config.FeaturePath()` etc.
- Classification must follow the initiatives rules: feature (has intents.md), initiative (depth-1 with direct-child features), deferred (skipped)
- Depth-2+ initiative detection is invalid and must error
- The helper must work across all three trees (`spec/intents/`, `spec/handoff/`, `.parlay/build/`)
- Backward-compatible — with no initiatives present, the helper returns the same bare slugs as the old scan

**Verify**:
- Returns both orphan slugs and initiative-qualified identifiers in one list
- With no initiatives, returns the same set as the old `os.ReadDir` scan
- `parlay collect-questions` (no `@feature`) returns questions from both orphan and nested features
- Empty initiatives (deferred) produce no entries
- Depth-2+ structures trigger an error
- After moving a feature into an initiative, `parlay collect-questions` still finds it without additional action

---

## Add Cross-Cutting Section to the Buildfile Schema

**Goal**: Formalize the `cross-cutting:` section in the buildfile schema so that infrastructure changes have the same schema rigor as components, and `parlay validate --type buildfile --deep` can validate them.
**Persona**: Parlay Developer
**Priority**: P1
**Context**: The `infrastructure-layer` feature already implemented the pipeline for infrastructure.md → `cross-cutting:` entries: build-feature reads infrastructure.md and maps fragments to `cross-cutting:` entries, generate-code processes them via step 14.7. What's missing is the formal schema definition in `buildfile.schema.md` — the `cross-cutting:` section is accepted by the pipeline but not yet documented in the schema alongside `models:`, `fixtures:`, `routes:`, and `components:`. Without the schema definition, deep validation can't check cross-cutting entries for structural correctness.
**Action**: Update `buildfile.schema.md` (source at `internal/embedded/schemas/buildfile.schema.md`) to document the `cross-cutting:` section: field definitions (`id`, `source`, `target-files`, `target-pattern`, `transform`, `introduces`, `caching`, `backward-compatible`), validation rules, and the relationship to infrastructure.md fragments. Update the deep validator to check cross-cutting entries. Deploy via `make sync-skills`.
**Objects**: buildfile-schema, cross-cutting, validation

**Constraints**:
- Each `cross-cutting:` entry must have an `id` (unique within the section) and a `source` reference tracing to an intent
- Either `target-files:` or `target-pattern:` (or both) must be present — the entry must name what it targets
- `transform:` is required — the human-readable change description
- `introduces:` is optional — lists new functions/types being added
- The section is optional — buildfiles without it remain valid
- Deep validation checks: `source` references point to real intents, `target-files` paths exist (or `target-pattern` matches at least one file), `introduces` entries have valid locations
- The `cross-cutting:` section follows the same diff lifecycle as `components:` — `parlay diff` classifies entries as stable/dirty/removed
- Schema deployed via `make sync-skills` (source at `internal/embedded/schemas/buildfile.schema.md`)

**Verify**:
- `parlay validate --type buildfile --deep` accepts a buildfile with a `cross-cutting:` section
- A buildfile without a `cross-cutting:` section still passes validation
- Deep validation catches: missing `source`, missing `transform`, missing both `target-files` and `target-pattern`
- `parlay diff` includes `cross-cutting:` entries in its stable/dirty/removed output
- The `cross-cutting:` section is documented in `buildfile.schema.md` with the same structure as `models:`, `fixtures:`, `routes:`, and `components:`

---
