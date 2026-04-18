# Helper Extraction

> A post-generation refactoring pass that detects shared helper functions duplicated across generated files and proposes extracting them into a shared package. Runs after `/parlay-generate-code` completes, not during generation. Prevents generated code from accumulating copy-paste duplication as new features reuse the same patterns.

---

## Extract Duplicated Helpers into Shared Packages

**Goal**: After code generation produces files across multiple features, detect helper functions that appear identically (or near-identically) in more than one generated file, and propose extracting them into a shared package — so that generated code stays DRY without requiring the buildfile to predict which helpers will be reused.
**Persona**: UX Designer
**Priority**: P2
**Context**: When generate-code runs for the `initiatives` feature, it produces `isOrphanFeatureDir()` and `createInitiativeDirs()` inside `add_feature.go` (via intelligent merge). When `move-feature` and `features-and-initiatives-renaming` are later generated, they need the same functions. Without extraction, each file gets its own copy — functionally correct but a maintenance burden and a source of subtle divergence over time. The buildfile can't predict this: each component is generated independently, and the duplication only becomes visible at the project level after multiple features have been built.
**Action**: Provide a `/parlay-simplify` skill (or equivalent post-generation pass) that scans all generated files for duplicated function bodies, groups them by signature+body similarity, and proposes extractions. For each proposed extraction: name the shared package (using the adapter's file-conventions for package/module organization), show which files currently contain the duplicate, generate a diff that moves the function into the shared package and replaces each copy with an import, and present the diff for the designer's approval.
**Objects**: helper, shared-package, duplication, generated-file, refactoring

**Constraints**:
- Extraction is always a proposal, never automatic. The designer may decline any extraction (e.g., if the functions will diverge in future iterations). The skill presents each as a diff and waits for approval.
- Only functions in parlay-generated files (identified by `parlay-component:`, `parlay-extends:`, or `parlay-section:` markers) are candidates for extraction. User-owned files are never scanned for duplication and never modified.
- Near-identical matching must handle minor differences: different variable names, different error message strings, different format strings — the function body's structure should match even if literals differ. A threshold (e.g., >80% AST similarity for Go, >80% token similarity for other languages) determines "near-identical."
- The target shared package should be determined from the adapter's file-conventions and the project's existing package structure. For Go with `source-root: internal/commands/`, helpers that serve multiple commands would naturally move to `internal/config/` or a new `internal/initiative/` package. The skill proposes the location; the designer can override.
- The extraction must update all import paths in all files that previously contained the duplicate. For Go, this means adding the import and removing the local function definition. The skill handles both sides of the refactoring.
- In a git repository, the extraction should use individual commits (one per extraction) so that `git log --follow` can trace each function to its new location.
- The skill must be idempotent: running it a second time on a project where all duplicates have already been extracted should report "no duplicates found" and exit cleanly.

**Verify**:
- After generating code for `initiatives` (producing `isOrphanFeatureDir` in `add_feature.go`) and `move-feature` (producing the same function in `move_feature.go`), running `/parlay-simplify` detects the duplication and proposes extracting `isOrphanFeatureDir` into `internal/config/` (or the designer-approved location)
- The proposed diff removes the function from both generated files, adds it to the shared package, and updates the import statements in both files
- Declining the extraction leaves both files unchanged
- Running `/parlay-simplify` a second time after the extraction reports no duplicates
- User-owned files (no parlay markers) are never flagged for extraction, even if they contain functions with similar signatures
- Near-identical functions (same body structure, different error message strings) are detected and grouped together; the extraction uses the first-seen variant and the designer can pick which version to keep
- Each extraction is committed separately so git history tracks the move

---
