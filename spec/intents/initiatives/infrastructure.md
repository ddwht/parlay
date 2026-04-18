# Initiatives — Infrastructure

---

## Directory Classification Validation

**Modifies**: AllFeaturePaths, hasIntentsMd
**Introduces**: classifyDir(path string) (DirClass, error), DirClass enum (Feature, Initiative, Deferred)
**Detection**: hasIntentsMd
**Behavior**: Replace the inline classification logic in AllFeaturePaths with a dedicated classifyDir function that returns a typed classification (Feature, Initiative, or Deferred) or an error for invalid states. A directory is a Feature when it contains intents.md directly. A directory is an Initiative when it contains direct-child subdirectories that themselves contain intents.md. A directory matching neither rule is Deferred. Two invalid states produce errors: (1) Hybrid — directory contains both intents.md and child subdirectories with intents.md — error must name the path and explain the mutual exclusion. (2) Sub-initiative — a directory at depth 2+ that classifies as an Initiative — error must name the path and cite the flat-hierarchy rule. AllFeaturePaths calls classifyDir for each entry and propagates errors instead of silently skipping invalid structures.
**Source**: @initiatives/group-features-under-an-initiative
**Backward-Compatible**: yes

**Notes**:
- The existing single-level nesting rejection in AllFeaturePaths already covers sub-initiatives partially; this formalizes it as a classifyDir concern with the error messaging the intent requires
- Deferred directories (empty, or containing only README.md / arbitrary files) must be silently skipped in enumeration — they are valid but invisible to listing commands
- classifyDir checks only direct children, never recurses — a subdirectory whose intents.md is more than one level deep does not qualify

---

## Duplicate-Slug Detection

**Modifies**: resolveQualifiedPath, AllFeaturePaths
**Introduces**: checkSlugUniqueness(parentDir string) error
**Behavior**: Add a defensive check during path resolution and feature enumeration that detects when two or more directories under the same parent slugify to the same identifier. Different directory names can produce identical slugs after slugification (e.g., `password-reset/` and `password_reset/` both become `password-reset`). When a duplicate is detected, the resolver must fail with an error that lists both (or all) conflicting paths rather than silently picking one. This state cannot occur through parlay commands (which enforce per-scope uniqueness at creation time) but can result from external filesystem operations. The check runs during slug lookup in resolveQualifiedPath and during the directory scan in AllFeaturePaths.
**Source**: @initiatives/group-features-under-an-initiative
**Backward-Compatible**: yes

**Notes**:
- This is a defensive guard, not a routine validation — it protects against external corruption (manual cp, IDE drag-and-drop, botched restore)
- The check slugifies each sibling directory name and looks for collisions in the resulting set; it does not need to scan the full tree, only the parent scope being resolved
- Performance: the check is per-parent-directory, bounded by the number of siblings, and only runs during resolution — not on every file operation

---

## Cross-Tree Traversal Consistency

**Modifies**: AllFeaturePaths
**Behavior**: Ensure that AllFeaturePaths, when called with different tree roots (spec/intents/, spec/handoff/, .parlay/build/), produces consistent qualified identifiers for the same logical features. The function already accepts a treeRoot parameter; this change ensures that classification (via classifyDir) is always performed against the spec/intents/ tree — the authoritative source — regardless of which tree root was requested for path enumeration. A feature that exists in spec/intents/ but is missing from spec/handoff/ should still appear in the enumeration (flagged as out-of-sync), rather than being silently omitted because the handoff tree lacks the directory.
**Source**: @initiatives/group-features-under-an-initiative
**Caching**: tree-scan-on-first-access
**Backward-Compatible**: yes

**Notes**:
- Today, scanning spec/handoff/ independently could produce different results than scanning spec/intents/ if the trees are out of sync — this fragment makes spec/intents/ the single source of truth for what features exist
- Out-of-sync detection is informational during enumeration; the repair-project-state feature handles reconciliation
- Caching the scan result per-process avoids repeated filesystem walks when multiple commands enumerate features in a single invocation

---
