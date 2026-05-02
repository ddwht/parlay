# Qualified Identifier Resolver — Infrastructure

---

## Qualified Path Resolver

**Modifies**: config.FeaturePath, config.BuildPath, config.HandoffPath
**Behavior**: Accept either a bare slug (`password-reset`) or a qualified identifier (`auth-overhaul/password-reset`). If the identifier contains `/`, split into initiative and feature components and construct the path as `<tree-root>/<initiative>/<feature>/`. If no `/`, resolve as today (orphan at top level). If a bare slug is ambiguous (exists both as orphan and inside one or more initiatives), return an error listing all matching paths. Factor the parsing logic once — the three helpers differ only in tree root.
**Source**: @qualified-identifier-resolver/support-qualified-identifiers-in-path-resolvers
**Caching**: tree-scan-on-first-access
**Backward-Compatible**: yes

**Notes**:
- Cache the tree structure in a package-level variable populated on first call via sync.Once or equivalent. Subsequent calls do pure path construction with no filesystem I/O.
- The ambiguity check only fires for bare slugs. Qualified identifiers are unambiguous by definition — they name the initiative explicitly.
- The resolver does NOT validate that the resolved path exists on disk. It constructs the path deterministically. Existence checks are the caller's responsibility.

---

## Feature Enumeration Helper

**Introduces**: config.AllFeatures() []string, config.AllFeaturePaths(treeRoot string) []string
**Detection**: os.ReadDir(filepath.Join(config.SpecDir, config.IntentsDir))
**Behavior**: Walk the given tree root at depth 1 and 2. Classify each directory: a directory with `intents.md` at its root is a feature; a depth-1 directory whose direct children contain `intents.md` is an initiative (its children are nested features); a directory matching neither rule is deferred and skipped. Return qualified identifiers — bare slugs for orphans, `<initiative>/<feature>` for nested. Error on depth-2+ initiative-like structures (flat-hierarchy rule violation). AllFeatures() is a convenience wrapper that calls AllFeaturePaths with `spec/intents/` as the tree root.
**Source**: @qualified-identifier-resolver/centralize-feature-enumeration-for-bulk-walker-commands
**Backward-Compatible**: yes

**Notes**:
- The Detection pattern identifies every bulk-walker caller that currently does an inline os.ReadDir scan. Generate-code greps the source tree for this pattern and applies Tier 2 intelligent merge to each matching file, replacing the inline scan with a config.AllFeatures() call.
- The helper returns results in filesystem order (alphabetical within each level). Orphan features appear first (top-level), then nested features grouped by initiative.
- The helper shares the same cached tree structure as the Qualified Path Resolver — both read the tree once and reuse it.

---

## Cross-Cutting Buildfile Schema

**Modifies**: internal/embedded/schemas/buildfile.schema.md, internal/agent/validate.go
**Introduces**: ValidateBuildfileCrossCutting(entries []CrossCuttingEntry) []ValidationError
**Behavior**: Add a `cross-cutting:` section definition to buildfile.schema.md documenting the fields (id, source, target-files, target-pattern, transform, introduces, caching, backward-compatible), validation rules, and the relationship to infrastructure.md fragments. Update the deep validator in validate.go to parse and check cross-cutting entries: id uniqueness, source reference validity, at least one of target-files or target-pattern present, transform required.
**Source**: @qualified-identifier-resolver/add-cross-cutting-section-to-the-buildfile-schema

**Notes**:
- The schema documentation follows the same structure as models:, fixtures:, routes:, and components: — template, fields table, validation rules, parsing rules.
- The section is optional in the buildfile. Deep validation skips cross-cutting checks when the section is absent. Existing buildfiles without it continue to pass.
- After updating the schema source, deploy via `make sync-skills`.

---
