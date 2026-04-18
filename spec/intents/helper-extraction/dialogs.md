# Helper Extraction — Dialogs

---

### Extract Duplicated Helpers into Shared Packages

**Trigger**: /parlay-simplify (or equivalent post-generation pass)

User: /parlay-simplify
System (background): Scans all generated files (identified by parlay-component:, parlay-extends:, or parlay-section: markers). Skips user-owned files.
System (background): Compares function bodies across files for identical or near-identical matches (>80% structural similarity).
System (background): Groups duplicates by signature and body structure.
System: Found ==N== duplicated helper(s) across generated files:
System:   1. `threeTreeRoots()` — identical in add_feature.go and new_initiative.go
System:   2. `isOrphanFeatureDir()` — near-identical in add_feature.go and move_feature.go (differs only in error message string)
System: Proposed extractions:
System (background): For each group, determines the target shared package from the adapter's file-conventions and existing project structure.
System: **1. `threeTreeRoots()`**
System:   From: internal/commands/add_feature.go, internal/commands/new_initiative.go
System:   To: internal/config/helpers.go
System:   ==unified diff showing: function removed from both files, added to helpers.go, imports updated==
  A: Apply this extraction
  B: Skip — I'll keep the duplicates
  C: Change the target package
User: A
System (background): Applies the diff. Commits: "Extract threeTreeRoots into internal/config/helpers.go"
System: [OK] Extracted `threeTreeRoots` → internal/config/helpers.go. Updated 2 files.
System: **2. `isOrphanFeatureDir()`**
System:   From: internal/commands/add_feature.go, internal/commands/move_feature.go
System:   To: internal/config/helpers.go
System:   Note: near-identical — functions differ in error message string. Using the version from add_feature.go.
System:   ==unified diff==
  A: Apply this extraction
  B: Skip — I'll keep the duplicates
  C: Use the other version instead
  D: Change the target package
User: A
System (background): Applies the diff. Commits separately.
System: [OK] Extracted `isOrphanFeatureDir` → internal/config/helpers.go. Updated 2 files.
System: Done. Extracted 2 helpers into shared packages. Each extraction is a separate commit for git traceability.

#### Branch: No duplicates found

User: /parlay-simplify
System (background): Scans all generated files. Compares function bodies.
System: No duplicated helpers found across generated files. Nothing to extract.

#### Branch: Designer declines an extraction

User: /parlay-simplify
System: Found 1 duplicated helper:
System:   1. `slugifyDirName()` — identical in config.go and move_feature.go
System:   ==proposed diff==
  A: Apply this extraction
  B: Skip — I'll keep the duplicates
User: B
System: Skipped. The duplicate stays in both files. You can re-run /parlay-simplify later if you change your mind.

#### Branch: Near-identical functions with structural differences

User: /parlay-simplify
System: Found 1 near-identical helper:
System:   1. `createDirsAcrossTrees()` — 85% similar across add_feature.go and new_initiative.go
System:     Differences:
System:       - add_feature.go creates both initiative and feature subdirectories
System:       - new_initiative.go creates only the initiative directory
System:     These are structurally similar but serve different purposes. Extract?
  A: Extract a shared version (I'll parameterize the difference)
  B: Skip — they should stay separate, they'll diverge further
User: B
System: Skipped. The functions are similar but will evolve independently — keeping them separate avoids coupling.

#### Branch: Designer overrides the target package

User: /parlay-simplify
System: Found 1 duplicated helper:
System:   1. `threeTreeRoots()` — identical in add_feature.go and new_initiative.go
System:   Proposed target: internal/config/helpers.go
  A: Apply this extraction
  B: Skip
  C: Change the target package
User: C
System: Where should this function live?
User: internal/initiative/paths.go
System (background): Generates extraction diff targeting internal/initiative/paths.go instead.
System:   ==revised unified diff==
  A: Apply this extraction
  B: Skip
User: A
System (background): Creates internal/initiative/ package. Writes paths.go. Updates imports in both source files. Commits.
System: [OK] Extracted `threeTreeRoots` → internal/initiative/paths.go. Updated 2 files.

#### Branch: Running after extraction — idempotent

User: /parlay-simplify
System (background): Scans all generated files. The previously duplicated functions now exist only in their shared packages. No new duplicates.
System: No duplicated helpers found across generated files. Nothing to extract.

---
