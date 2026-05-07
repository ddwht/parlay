# Cross-cutting-target-paths — Infrastructure

---

## Cross-Cutting Target-Files Routed To Plan By Entry Kind

**Affects**: buildfile plan-cross-check during deep validation
**Behavior**: Deep validation of a buildfile classifies each cross-cutting entry by kind before checking the entry's target paths against the plan section. An entry is "purely-introducing" when every path it names is missing on disk and the entry declares no companion modify-target field; otherwise the entry is "modifies-only". For purely-introducing entries, every target path must appear in the plan's create-rows with a source citation referencing this cross-cutting; for modifies-only entries, every target path must appear in the plan's modify-rows with the same citation. Classification is mechanical and deterministic over the entry plus the source root — authors do not annotate kind. An entry whose target-files list mixes existing-on-disk and not-yet-existing paths is rejected without applying either routing rule, on the principle that ambiguous mixtures must be expressed via the dedicated two-kinded shape rather than inferred. The legacy entry-level error (a cross-cutting whose target appears nowhere in the plan) is preserved for compatibility, with its fix message updated to name plan-creates as the right destination for a purely-introducing entry.
**Invariants**:
- A cross-cutting entry whose every target path is missing on disk and whose paths all appear in plan-creates with a matching source citation validates successfully
- A cross-cutting entry whose every target path exists on disk and whose paths appear in plan-modifies with a matching source citation validates successfully (legacy behavior preserved)
- A purely-introducing cross-cutting entry whose target path is incorrectly placed in plan-modifies fails validation with the legacy modify-target-missing error, naming the offending path
- A purely-introducing cross-cutting entry with no plan row at all fails validation with the entry-level cross-cutting-target-not-in-plan error, naming the cross-cutting identifier and a fix message that points to plan-creates
- A cross-cutting entry whose target-files list mixes existing-on-disk and not-yet-existing paths fails validation with a dedicated mixed-target-kinds error naming both paths and their disk states, and a fix message offering entry-splitting or the two-kinded shape
- Entry-kind classification is a pure function of (entry, source-root) and reads no external state beyond the filesystem
- A buildfile that validated successfully under the legacy contract continues to validate successfully under this routing rule (the change widens the set of valid buildfiles; it does not narrow it)
**Source**: @parlay-tool/cross-cutting-target-paths/cross-cutting-target-files-map-to-plan-by-entry-kind
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The buildfile schema document is amended in lockstep so the contract is readable in one place: the existing sentence stating purely-introducing entries route to plan-creates remains, and a parallel sentence is added stating modifies-only entries route to plan-modifies. The new error codes are listed under the validation bullets.
- Stable error codes registered by this fragment: cross-cutting-target-not-in-creates, cross-cutting-target-not-in-modifies, cross-cutting-mixed-target-kinds. The legacy cross-cutting-target-not-in-plan code is retained for entry-level absence (no plan row whatsoever). The legacy plan-modify-target-missing code continues to fire when a non-existent path is placed in plan-modifies.
- The classification function is the load-bearing design choice — authors cannot misclassify their own entry because they do not annotate kind. The validator's classification and the schema's routing rule are guaranteed to agree because both follow the same deterministic procedure.

---

## Target-Pattern Resolution To Concrete Paths At Validation Time

**Affects**: buildfile plan-cross-check during deep validation
**Behavior**: A cross-cutting entry's target-pattern field is resolved to a concrete set of paths at validation time, not treated as opaque. Resolution evaluates the pattern against the union of (a) files present under the source root and (b) the cross-pass set of paths that other features in the same project pass declare as creates. The resolved set is treated identically to a target-files list and routed by entry kind per the previous fragment. Patterns that resolve to zero paths are an authoring error — silent-no-op cross-cuttings are exactly the bug being closed. Pattern syntax stays as today's match-style globbing; this fragment does not introduce new glob features. The matchable-set in single-feature validation is the on-disk set only; the cross-pass extension activates only when the validator runs in project-pass mode (see the project-pass fragment). Resolution is deterministic and order-independent — the same pattern, on-disk set, and cross-pass-creates set always produce the same resolved path set. Resolution reads only the filesystem and the in-memory cross-pass set; it makes no shell calls, no adapter calls, and no AI calls.
**Invariants**:
- A cross-cutting whose pattern matches three on-disk files validates successfully only when all three appear in plan-modifies with the entry as source citation
- A cross-cutting whose pattern matches a path declared in another feature's plan-creates set, in project-pass mode, resolves to that path and validates successfully when the path appears in this feature's plan-creates
- A cross-cutting whose pattern resolves to zero paths fails validation with a pattern-empty error citing the pattern and the search roots
- In single-feature validation (no project-pass mode), pattern resolution sees only the on-disk set; a pattern that would resolve through a sibling create in project-pass mode resolves to zero paths and fails with the pattern-empty error, with a fix-hint mentioning project-pass mode
- Re-resolving the same pattern against unchanged inputs produces a byte-identical resolved set across repeated calls
- A pattern using glob features beyond the supported syntax (such as recursive double-star) is treated as a literal-character match and resolves to zero paths, failing with the pattern-empty error
- Pattern resolution makes no shell calls, no adapter calls, and no AI calls
**Source**: @parlay-tool/cross-cutting-target-paths/target-pattern-resolves-to-concrete-paths-at-validation-time
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- Closing the silent-bypass loophole is the load-bearing design choice. The previous behavior, in which target-pattern was iterated past during plan-cross-check, allowed any cross-cutting that used target-pattern to validate trivially regardless of plan contents. The same shipped buildfile that exposed the original schema-validator drift exploited this bypass.
- Stable error code registered by this fragment: cross-cutting-pattern-empty.
- Glob-feature creep is explicitly out of scope. Recursive patterns (such as a double-star segment) remain unsupported until a separate feature adds them; this fragment closes only the routing-and-resolution loophole.

---

## Two-Kinded Cross-Cutting Entries

**Affects**: buildfile schema for cross-cutting entries; buildfile plan-cross-check during deep validation
**Behavior**: A cross-cutting entry may declare both create-targets and modify-targets in a single entry, expressing the natural pattern in which one cross-cutting concern both introduces a new file and modifies a sibling (for example, registering a feature in a global router introduces the per-feature route file and modifies the index that aggregates them). The schema gains an optional target-creates field, parallel to target-files. When both fields are present, target-files is interpreted strictly as modifies-only (every path must exist on disk and appear in plan-modifies) and target-creates is interpreted strictly as introduce-targets (every path must NOT exist on disk and appear in plan-creates). The presence of target-creates overrides the entry-kind heuristic from the routing fragment — when the author has explicitly declared the shape, the heuristic is bypassed. A target-pattern on a two-kinded entry resolves once and is split by classification: on-disk matches route to plan-modifies, future-create matches route to plan-creates. The mixed-kinds error fires only when both kinds appear in a single target-files list (or in a resolved target-pattern), never on a legitimate two-kinded entry. A path listed in both target-files and target-creates is rejected — the author must pick one, and the validator names the offending path so the choice is unambiguous.
**Invariants**:
- A cross-cutting entry with target-files naming an on-disk path AND target-creates naming a not-on-disk path validates successfully when plan-modifies cites the on-disk path and plan-creates cites the introduce-path, both with this entry as source citation
- A target-creates path that does not appear in plan-creates fails validation with a target-creates-not-in-plan error naming the path and the cross-cutting identifier
- A target-files path missing on disk in a two-kinded entry fails validation with the legacy modify-target-missing error (the heuristic is bypassed; target-files is interpreted as modifies-only)
- A path listed in both target-files and target-creates fails validation with a target-double-listed error naming the offending path and both fields, with a fix message asking the author to pick one
- A two-kinded entry with a target-pattern resolves the pattern once and routes each resolved path by classification — on-disk matches to plan-modifies, future-create matches to plan-creates — without firing the mixed-kinds error
- A buildfile with no target-creates field anywhere validates byte-identically to its legacy outcome (no regression on legacy authoring shape)
- The target-creates field is optional; buildfiles authored before this fragment shipped continue to parse and validate without modification
**Source**: @parlay-tool/cross-cutting-target-paths/a-cross-cutting-entry-may-declare-both-create-targets-and-modify-targets
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The two-kinded shape is the explicit-author-intent counterpart to the heuristic in the routing fragment: when an entry's kind is genuinely both, the author declares it directly via separate fields rather than through pattern-resolution inference. The heuristic remains for the common case (single-kind entries); the explicit shape covers the legitimate two-kind case.
- Stable error codes registered by this fragment: cross-cutting-target-creates-not-in-plan, cross-cutting-target-double-listed.
- The buildfile schema documentation is amended to describe target-creates alongside target-files and target-pattern, with worked examples for each common shape (purely-introducing, modifies-only, two-kinded).

---

## Project-Pass Validation With Cross-Pass Creates

**Affects**: deep buildfile validation across all features in a project; project-pass entry point on the validate command; emission ordering during project-wide code generation
**Behavior**: A new validation mode treats the project as a whole rather than a single feature in isolation. In project-pass mode, the validator first walks every feature's buildfile under the resolved root and collects each feature's plan-creates set into a project-wide map keyed by path with feature source attribution. It then validates each feature in turn, threading the union-minus-self as a cross-pass-creates set into the plan checks. The modify-target-missing rule treats a plan-modifies path as satisfied when the path exists on disk OR appears in the cross-pass-creates set. Single-feature validation (the legacy default) sees no cross-pass-creates set and continues to require on-disk presence; a developer running validation in a single-feature loop still gets the strictest answer. The relaxation is on existence only — provenance is unchanged: every plan row's source-citation requirement still fires independently, so a modify-path satisfied through a sibling create whose own row lacks proper attribution still fails. A directed dependency graph captures cross-feature ordering — when feature B's modify is satisfied by feature A's create, feature B depends on feature A. Cycles in this graph are rejected with a dedicated cycle error naming both features and both paths. Project-wide code generation walks the dependency graph in topological order, emitting creates before the modifies they satisfy, regardless of feature-slug alphabetization or argument order. The CLI surface is a project flag on the validate command — when set, the command takes no buildfile path argument and discovers buildfiles by walking the resolved root; when unset, behavior is unchanged from today.
**Invariants**:
- A two-feature project where feature A creates a path and feature B modifies the same path validates successfully under project-pass mode when both feature's plan rows have valid source attribution
- The same two-feature project, validated one feature at a time without project-pass mode, returns the legacy modify-target-missing error for feature B, with an additional fix-hint mentioning the project-pass mode
- A two-feature cycle (feature A creates X and modifies Y; feature B creates Y and modifies X) fails validation with a plan-create-modify-cycle error naming both features and both paths
- A modify-path satisfied through a sibling create whose own plan row's source citation does not reference a real entry in the buildfile fails validation with the existing source-attribution error — the existence relaxation does not mask provenance bugs
- Project-wide code generation emits creates before the modifies they satisfy; reversing the feature-slug alphabetization in the input still produces the create-first emission order
- Re-running project-pass validation against unchanged source produces byte-identical verdicts and a byte-identical dependency graph
- The project flag and a positional buildfile-path argument are mutually exclusive; passing both fails with a validate-project-takes-no-path error before any validation begins
- A project containing zero buildfiles produces an informational result naming the resolved root and reporting zero buildfiles, with a successful exit status (an empty project is not an error)
- The cross-pass-creates set carries source attribution per path so error messages can name the producing feature when a modify resolves through a sibling create
**Source**: @parlay-tool/cross-cutting-target-paths/plan-modifies-is-satisfied-by-an-on-disk-file-or-another-features-plan-creates-in-the-same-pass
**Caching**: per-project-pass
**Backward-Compatible**: yes

**Notes**:
- The opt-in design is the load-bearing decision: bare single-feature validation must remain strict, because a developer iterating on one feature wants the worst-case answer. Relaxation is gated on the project-pass entry point, so no existing single-feature workflow silently changes behavior.
- Stable error codes registered by this fragment: plan-create-modify-cycle, validate-project-takes-no-path.
- Code-generation's existing strict-target rule is unchanged at runtime — the file must exist when modification is attempted. The dependency graph guarantees the producing feature's create runs first; the relaxation is at validation time only.
- The dependency graph is the same data structure used by validation and by code-generation, computed once per project pass. Validation reports cycles; code-generation walks the (acyclic) graph topologically. Sharing the structure ensures the two stages cannot disagree about ordering.

---
