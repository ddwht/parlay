# Infrastructure Layer — Dialogs

---

### Define the Infrastructure Artifact

**Trigger**: A developer has a feature with behind-the-scenes changes but no user-facing surface, and hits the pipeline gap.

User: I'm working on the qualified-identifier-resolver feature. It only has behind-the-scenes changes — no UI, nothing the designer sees. When I try to run /parlay-build-feature, check-readiness blocks with "no surface." How do I get this through the pipeline?
System: Features that change internal code without affecting the user-facing surface use `infrastructure.md` instead of `surface.md`. It's a parallel artifact that describes behavioral capabilities in framework-agnostic terms — just like surface.md describes what the user sees without naming widgets.
System: Create `spec/intents/qualified-identifier-resolver/infrastructure.md` with one fragment per discrete capability. Each fragment has:
System: - **Affects**: what area of the system this touches (e.g., "feature resolution", "directory traversal")
System: - **Behavior**: what the capability does, in framework-agnostic terms
System: - **Invariants**: testable properties that must hold after implementation
System: - **Source**: `@feature/intent-slug` traceability link
System: - Optional: **Caching**, **Backward-Compatible**, **Notes**
System: A feature needs at least one of `surface.md` or `infrastructure.md` to proceed. You can have both if your feature has user-facing AND behind-the-scenes changes.
User: So I skip dialogs and surface generation entirely?
System: Yes — the pipeline path for a pure infrastructure feature is: intents → infrastructure.md (you author it directly) → /parlay-build-feature → /parlay-generate-code. No dialogs, no surface generation step.
User: What if my feature has both? Like initiatives — it has UI output AND resolver changes.
System: Then it has both files. `surface.md` describes the three UI fragments (orientation, creation result, empty initiative result). `infrastructure.md` describes the resolver and classification changes. Build-feature reads both and produces a buildfile with `components:` from the surface and `cross-cutting:` entries from the infrastructure.

#### Branch: Developer asks about framework-agnosticism

User: Why can't I just name the function I want to modify? I know it's `AllFeaturePaths` in `config.go`.
System: Because `infrastructure.md` is framework-agnostic — like `surface.md`. Surface says `data-list`, not `bulleted-list` or `<ul>`. Infrastructure says "directory traversal capability", not `AllFeaturePaths` or `os.walk`.
System: The reason: the same intents and infrastructure can target different frameworks by swapping the adapter. A Go CLI adapter resolves "directory traversal" to a Go function in `config.go`. A Python adapter resolves it to a Python class in a resolver module. If you hardcode `AllFeaturePaths`, the infrastructure is locked to Go.
System: Build-feature does the translation when it reads your infrastructure.md — it consults the adapter's file conventions, scans the source tree, and generates framework-specific `cross-cutting:` entries in the buildfile. That's where function names and file paths appear.

#### Branch: Developer asks what's allowed vs. not allowed

User: So what can I put in infrastructure.md and what can't I?
System: **Allowed** — domain vocabulary from your intents: "feature", "initiative", "intents.md", "qualified identifier", "three parallel trees". These are part of the problem, not the framework.
System: **Not allowed** — implementation vocabulary: function names (`classifyDir`), file paths (`internal/config/config.go`), type signatures (`(DirClass, error)`), language keywords (`func`, `struct`, `class`), grep patterns targeting code identifiers.
System: The validation tool warns if it detects framework-specific content in your Behavior or Affects fields. It's a lint, not a hard block — but the warnings mean your file won't be portable.

---

### Define the Infrastructure Schema

**Trigger**: The developer needs to author or validate an `infrastructure.md` file and wants to know the exact format.

User: What's the schema for infrastructure.md? I want to make sure my fragments are valid before running build-feature.
System: The schema lives at `.parlay/schemas/infrastructure.schema.md`. Here's the fragment format:
System: ```
System: ## <Fragment Name>
System:
System: **Affects**: <abstract scope — what area of the system this touches>
System: **Behavior**: <what the capability does, in framework-agnostic terms>
System: **Invariants**:
System: - <testable property that must hold>
System: **Source**: @feature/intent-slug
System: **Caching**: <on-first-access | none | per-process>
System: **Backward-Compatible**: yes | no
System:
System: **Notes**:
System: - <additional constraints, design decisions, edge cases>
System: ```
System: Required fields: Fragment Name, Affects, Behavior, Source.
System: Optional: Invariants, Caching, Backward-Compatible, Notes.
User: Can I validate it?
System (background): Runs `parlay validate --type infrastructure --json spec/intents/qualified-identifier-resolver/infrastructure.md`.
System (condition: validation passes): [OK] Infrastructure file is valid — ==N== fragments, all with required fields and valid Source references.
System (condition: validation fails): Found ==N== issues:
System: - Fragment "==name==": ==error description==
System: Fix the issues and re-validate.
System (condition: portability warnings): [WARN] Portability: Fragment "==name==" Behavior contains framework-specific content (==detail==). Consider rephrasing in domain terms.

#### Branch: Developer asks about Affects vs. the old Modifies field

User: I used to write `Modifies: AllFeaturePaths, hasIntentsMd`. What do I write in Affects instead?
System: `Affects` describes the abstract scope, not concrete code. Think about what area of the system the change touches:
System: - `Modifies: AllFeaturePaths, hasIntentsMd` → `Affects: feature resolution, directory traversal`
System: - `Modifies: resolveQualifiedPath` → `Affects: feature resolution`
System: - `Modifies: internal/commands/collect_questions.go, sync.go` → `Affects: command dispatch` (or more specific: "feature enumeration in bulk commands")
System: Build-feature reads Affects and scans the source tree to find the actual files and functions. It uses the adapter's file conventions to know where to look.

#### Branch: Developer asks about Invariants

User: What are Invariants for?
System: Invariants are testable properties that must hold after your capability is implemented. They feed testcases for cross-cutting entries — the same way intent Verify bullets feed testcases for components.
System: Example for a directory classifier:
System: - A directory with intents.md at its root is always classified as Feature
System: - A directory with child subdirectories containing intents.md is always classified as Initiative
System: - A directory matching both rules produces a hybrid error naming the path
System: Each invariant is falsifiable — a test can check it. "Classify directories correctly" is too vague. "A directory with intents.md is always Feature" is testable.

---

### Bridge Infrastructure to Framework-Specific Cross-Cutting

**Trigger**: /parlay-build-feature @==feature== on a feature that has `infrastructure.md`

User: /parlay-build-feature @qualified-identifier-resolver
System (background): Reads skill instructions. Loads schemas, adapter, blueprint.
System (background): Reads `spec/intents/qualified-identifier-resolver/intents.md`.
System (background): Checks for `surface.md` — not found. Checks for `infrastructure.md` — found.
System (background): Check-readiness passes: feature has `infrastructure.md`.
System (background): Reads `infrastructure.md`. Finds ==N== infrastructure fragments.
System (background): For each fragment, resolves abstract Affects to concrete targets:
System (background): - Reads fragment Affects ("feature resolution") and Behavior
System (background): - Consults adapter file conventions (source-root: internal/commands/, naming: snake_case)
System (background): - Scans source tree for code related to the Affects scope
System (background): - Generates framework-specific `cross-cutting:` entry with target-files, introduces, transform
System (background): Generates buildfile with `cross-cutting:` section and models. No `components:` or `routes:`.
System (background): Validates buildfile with `parlay validate --type buildfile --deep`.
System: [OK] Build specification ready. Internal artifacts saved to .parlay/build/qualified-identifier-resolver/.
System: Buildfile contains ==N== cross-cutting entries (no components — this is a pure infrastructure feature).
System: Next: run /parlay-generate-code to apply the infrastructure changes via Tier 2 intelligent merge.

#### Branch: Feature has both surface.md and infrastructure.md

User: /parlay-build-feature @initiatives
System (background): Finds both `surface.md` (3 fragments) and `infrastructure.md` (==N== fragments).
System (background): Maps surface fragments → `components:` entries (3 components with routes) via adapter widget mapping.
System (background): Resolves infrastructure fragments → `cross-cutting:` entries (==N== entries) via adapter + source tree scan.
System: [OK] Build specification ready.
System: Buildfile contains 3 components (from surface) and ==N== cross-cutting entries (from infrastructure).
System: Next: run /parlay-generate-code to generate component code and apply infrastructure changes.

#### Branch: Feature has neither surface.md nor infrastructure.md

User: /parlay-build-feature @some-incomplete-feature
System (background): Check-readiness finds neither `surface.md` nor `infrastructure.md`.
System: [ERR] Can't build — no `surface.md` or `infrastructure.md` found for @some-incomplete-feature. Create at least one:
System: - Run /parlay-create-artifacts @some-incomplete-feature to determine which artifact(s) this feature needs
System: - Or author `infrastructure.md` directly for behind-the-scenes features

#### Branch: Affects scope cannot be resolved to any file

User: /parlay-build-feature @new-capability
System (background): Reads infrastructure.md. Fragment "Cache Invalidation" has Affects: "response caching".
System (background): Scans source tree for code related to "response caching". No matching files found.
System: Fragment "Cache Invalidation" affects "response caching", but I couldn't find existing code related to this scope in the source tree.
  A: Show me the Behavior so I can point you to the right files
  B: This is a new capability — generate a new file for it
  C: Skip this fragment for now
User: A
System: The Behavior says: "Invalidate cached feature-tree scans when a feature directory is created, moved, or deleted."
User: That's in internal/config/config.go — look for the featureTreeOnce variable.
System (background): Generates cross-cutting entry with target-files: internal/config/config.go, using the user's guidance.
System: Got it — targeting internal/config/config.go for the cache invalidation change.

---

### Update Generate-Code to Process Infrastructure-Sourced Entries

**Trigger**: /parlay-generate-code when the merged buildfile contains `cross-cutting:` entries

User: /parlay-generate-code
System (background): Loads all buildfiles. Finds `cross-cutting:` entries in qualified-identifier-resolver's buildfile.
System (background): Processes components first (from other features' buildfiles, if any).
System (background): Processes brownfield mount (step 14.5) for existing files.
System (background): Processes `cross-cutting:` entries (after components and mount, before tests):
System: Applying ==N== cross-cutting changes:
System (background): Entry 1: Resolves `target-files: internal/config/config.go`. Reads the file. Applies Tier 2 intelligent merge.
System: Proposed change to internal/config/config.go:
System: ==unified diff showing modified resolver functions + new caching==
  A: Apply this change
  B: Skip — I'll integrate manually
  C: Edit the proposed change
User: A
System (background): Applies the diff.
System (background): Entry 2: Resolves `target-pattern:` by grepping source tree. Found 4 matching files.
System: Pattern matched 4 files: collect_questions.go, sync.go, extract_domain_model.go, check_coverage.go
System (background): Applies Tier 2 intelligent merge to each.
System: Proposed change to internal/commands/collect_questions.go:
System: ==unified diff==
  A: Apply this change
  B: Skip — I'll integrate manually
  C: Edit the proposed change
User: A
System (background): Repeats for remaining files.
System (condition: all applied): [OK] All cross-cutting changes applied.
System (background): Runs tests.
System (condition: tests pass): [OK] All tests pass. Build state committed.
System: Applied ==N== cross-cutting changes across ==M== files. Infrastructure is in place.

#### Branch: target-pattern matches zero files

System (background): Entry 2: Resolves `target-pattern:`. Greps source tree. Found 0 matching files.
System: [WARN] Pattern matched 0 files in the source tree. The pattern may be ahead of the codebase, or the callers may have already been updated. Skipping this entry.

#### Branch: target file doesn't exist for a modify-only entry

System (background): Entry 1: Resolves `target-files: internal/config/config.go`. File not found.
System: [ERR] Target file `internal/config/config.go` does not exist. The cross-cutting entry targets this file but it's missing. Verify the path and the project structure.

---
