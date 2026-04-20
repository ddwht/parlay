# Qualified Identifier Resolver — Dialogs

---

### Support Qualified Identifiers in Path Resolvers

**Trigger**: The developer is implementing the initiatives feature and needs the path resolvers to handle the new two-level hierarchy.

User: I've defined the initiatives structural model — features can live at spec/intents/<feature>/ or spec/intents/<initiative>/<feature>/. But config.FeaturePath still does a plain filepath.Join with a single slug. How do I make it work with the qualified identifiers?
System: You need to update config.FeaturePath, config.BuildPath, and config.HandoffPath to accept either a bare slug or an initiative/feature qualified identifier. This is a cross-cutting infrastructure change — author it in infrastructure.md for this feature.
System: The resolver should handle three cases:
System: 1. Bare slug (`password-reset`) → resolves to the orphan at `spec/intents/password-reset/`
System: 2. Qualified identifier (`auth-overhaul/password-reset`) → splits on `/`, resolves to `spec/intents/auth-overhaul/password-reset/`
System: 3. Ambiguous bare slug (exists both as orphan and nested) → errors listing all matching paths
System: The change is backward-compatible — every existing caller passes a bare slug today and must continue working.
User: All three helpers need the same logic?
System: Yes — the qualified-identifier parsing is identical across FeaturePath, BuildPath, and HandoffPath. Only the tree root differs (spec/intents/, spec/handoff/, .parlay/build/). Factor the logic once and vary the root.
User: What about performance? Some commands call FeaturePath in tight loops.
System: The resolver should cache the tree structure on first access — a single os.ReadDir scan, stored in a package-level variable. No filesystem I/O on subsequent calls. Capture that in the Caching field of the infrastructure.md fragment.

#### Branch: Ambiguity in practice

User: When would a bare slug actually be ambiguous?
System: When the designer creates an orphan feature `password-reset` at the top level AND there's a nested `auth-overhaul/password-reset` inside an initiative. Both exist, both have the slug `password-reset`. A bare `@password-reset` command would be ambiguous — which one?
System: Per the initiatives spec, a bare slug always means the orphan at the top level; it never reaches into initiatives. But the resolver should still detect the situation defensively and warn, because the designer might not realize the collision exists.

---

### Centralize Feature Enumeration for Bulk-Walker Commands

**Trigger**: The developer notices that parlay collect-questions (or sync, extract-domain-model, check-coverage) misses features that were moved into an initiative.

User: I moved password-reset into the auth-overhaul initiative. Now `parlay collect-questions` doesn't see it anymore — it only reports questions from orphan features. What's wrong?
System: The bulk-walker commands scan spec/intents/ one level deep via os.ReadDir. With initiatives, nested features live one level deeper — the scan misses them. You need a shared helper that walks both levels.
System: Author a `config.AllFeatures()` infrastructure fragment in this feature's infrastructure.md. The helper should:
System: - Walk spec/intents/ at depth 1 and 2
System: - Classify: feature (has intents.md), initiative (has direct-child features), deferred (skip)
System: - Return qualified identifiers: bare slugs for orphans, initiative/feature for nested
System: - Error on depth-2+ initiative-like structures (flat-hierarchy rule)
System: Then every bulk-walker needs updating — use a Detection pattern in infrastructure.md to find all callers.
User: What pattern identifies the callers?
System: Grep for `os.ReadDir(filepath.Join(config.SpecDir, config.IntentsDir))` — that's the inline scan each command currently uses. Put that in the Detection field. Generate-code will grep the source tree, find every matching file, and apply Tier 2 intelligent merge to replace the inline scan with a config.AllFeatures() call.
User: Does AllFeatures work for handoff and build trees too?
System: Yes — the helper should accept a tree root parameter so commands can enumerate features under spec/handoff/ or .parlay/build/ as needed. Same classification logic, different base path.

#### Branch: Backward compatibility with no initiatives

User: What if the project has no initiatives yet? Does AllFeatures still work?
System: Yes — with no initiatives, every top-level directory is either an orphan feature or deferred. AllFeatures returns bare slugs only, in the same order as the old os.ReadDir scan. Existing behavior is preserved.

---

### Add Cross-Cutting Section to the Buildfile Schema

**Trigger**: The developer runs /parlay-build-feature on a feature with infrastructure.md and the generated buildfile has a cross-cutting: section, but parlay validate --type buildfile --deep doesn't know how to validate it.

User: I ran /parlay-build-feature @qualified-identifier-resolver. The buildfile has a cross-cutting: section with two entries. But when I run parlay validate --type buildfile --deep, it ignores the cross-cutting entries — no validation on the source references or target paths. How do I get that checked?
System: The cross-cutting: section isn't formally defined in buildfile.schema.md yet. The pipeline accepts it (build-feature writes it, generate-code processes it), but the deep validator doesn't know the field rules. You need to add the schema definition.
System: Update `internal/embedded/schemas/buildfile.schema.md` to document cross-cutting: alongside models:, fixtures:, routes:, and components:. The schema should define:
System: - `id:` — unique identifier (required)
System: - `source:` — intent traceability reference (required)
System: - `target-files:` — explicit file paths (optional, but at least one of target-files or target-pattern required)
System: - `target-pattern:` — grep pattern for fan-out (optional)
System: - `transform:` — human-readable change description (required)
System: - `introduces:` — new functions/types being added (optional)
System: Then update the deep validator in internal/agent/validate.go to check cross-cutting entries: source exists, targets resolve, no duplicate ids.
User: What about the diff lifecycle?
System: parlay diff already classifies components as stable/dirty/removed. Cross-cutting entries follow the same lifecycle — the diff command needs to include them in its output. Document that in the schema too.
System (background): After updating the schema source, run `make sync-skills` to deploy.
System: Once deployed, `parlay validate --type buildfile --deep` will catch missing source references, missing transforms, and entries that name no targets.

#### Branch: Buildfile without cross-cutting

User: What about existing buildfiles that don't have a cross-cutting section?
System: The section is optional. Buildfiles without it continue to pass validation — the deep validator simply skips cross-cutting checks when the section is absent. No migration needed.

---
