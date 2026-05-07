# Cross-cutting target paths

> Resolve the latent contradiction between the buildfile schema and the deep validator around how cross-cutting `target-files:` and `target-pattern:` map to `plan.creates` vs `plan.modifies`. Today, a purely-introducing cross-cutting cannot use `target-files:` at all — the path is unrepresentable. `target-pattern:` silently bypasses plan checks. And a feature that needs to both create and then modify the same file (a registry plus its first entry) cannot be expressed in one cross-cutting entry. This feature pins the contract end-to-end: target-files routes to plan by entry kind, target-pattern resolves to concrete paths at validation time, one entry may carry both create- and modify-targets, and `plan.modifies` is satisfied by an on-disk file or another feature's `plan.creates` in the same pass.

---

## Cross-cutting `target-files:` map to plan by entry kind

**Goal**: Make a purely-introducing cross-cutting representable: its `target-files:` paths are matched against `plan.creates`, not `plan.modifies`. Modifies-only entries continue to match against `plan.modifies` as today. Authoring is no longer forced through `target-pattern:` to escape a validator that only looks in one half of the plan.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: The deep validator's `cross-cutting-target-not-in-plan` rule iterates `cc.TargetFiles` and only matches against plan entries where `kind == "modify"`. The schema (line 160) already says purely-introducing cross-cuttings list their paths in `plan.creates`. The two halves of the contract disagree, and the workaround in shipped buildfiles (`@studio-support/page-layout-field`) was to switch to `target-pattern:`, which sidesteps the rule entirely. The fix is on the validator side: the schema stays as written.
**Action**: Extend `validatePlanSection` in `core/internal/agent/validate.go` so that, for each cross-cutting entry, target-files routes by entry kind. An entry is "purely-introducing" when it has no on-disk file at any of its `target-files:` paths AND has no companion `target-modifies:` declaration; for such entries, every `target-files:` path must appear in `plan.creates` with a matching `cross-cutting/<id>` source. An entry is "modifies-only" otherwise; for it, every `target-files:` path must appear in `plan.modifies` (today's behavior). The classification is mechanical — a function over the entry plus the source root, no AI involvement.
**Objects**: cross-cutting-entry, target-files, plan-creates, plan-modifies, entry-kind, validator

**Constraints**:
- Entry kind is determined by a deterministic function over `(entry, source-root)`, not authored — authors do not annotate "I'm a purely-introducing entry"
- An entry whose `target-files:` list mixes existing-on-disk and not-yet-existing paths is rejected with a dedicated error code (`cross-cutting-mixed-target-kinds`) directing the author to split the entry, OR to use the both-kinds shape from intent 3 if the mix is intentional
- Error codes for the new branches are stable: `cross-cutting-target-not-in-creates` (purely-introducing entry's target missing from `plan.creates`) and `cross-cutting-target-not-in-modifies` (modifies-only entry's target missing from `plan.modifies`). The legacy `cross-cutting-target-not-in-plan` code is retained for purely-introducing entries that have no plan rows at all (entry-not-in-plan)
- Schema (`buildfile.schema.md`) is amended to make the routing rule explicit — current line 160's "purely-introducing → plan.creates" sentence stays; a parallel sentence is added stating modifies-only entries route to plan.modifies, and the new error codes are listed under the validation bullets
- No change to authoring shape — a buildfile that already worked under the old contract continues to validate; the change only widens the set of valid buildfiles

**Verify**:
- A cross-cutting entry whose every `target-files:` path is missing on disk and whose paths all appear in `plan.creates` with a matching source validates cleanly
- A cross-cutting entry whose every `target-files:` path exists on disk and whose paths appear in `plan.modifies` continues to validate cleanly (no regression)
- A purely-introducing cross-cutting whose `target-files:` path appears in `plan.modifies` (the old workaround attempt) fails with `plan-modify-target-missing`, as today
- A purely-introducing cross-cutting with no plan row at all fails with `cross-cutting-target-not-in-plan` and a fix message naming `plan.creates`
- A cross-cutting entry whose `target-files:` mixes existing and non-existing paths fails with `cross-cutting-mixed-target-kinds` and a fix message pointing to either entry-splitting or the both-kinds shape

---

## `target-pattern:` resolves to concrete paths at validation time

**Goal**: Close the silent-bypass loophole. `target-pattern:` no longer skips plan cross-checks. The validator resolves the pattern to a concrete path set at validation time and applies the same routing rules as `target-files:`.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: `target-pattern:` is currently treated as opaque by the cross-cutting plan-check loop — it iterates `cc.TargetFiles` only. Any cross-cutting that uses `target-pattern:` validates trivially regardless of plan contents. That is what `@studio-support/page-layout-field` exploited. The point of `target-pattern:` is to *abbreviate* a `target-files:` list, not to *escape* the plan contract.
**Action**: Extend pattern resolution in `validatePlanSection` so each cross-cutting entry's `target-pattern:` glob is evaluated against the union of (a) files present on disk under the source root and (b) `plan.creates` paths from every feature in the same project-wide validation pass. The resolved set is treated identically to a `target-files:` list and routed by entry kind per intent 1. Patterns that resolve to zero paths are an error (`cross-cutting-pattern-empty`), forcing the author to either fix the glob or remove the entry.
**Objects**: target-pattern, pattern-resolution, source-root, cross-pass-creates, plan-routing

**Constraints**:
- Pattern resolution explicitly includes the cross-pass `plan.creates` set — a cross-cutting whose pattern matches a file *another feature in the same generate-code pass* will create resolves to that future path. This makes `target-pattern:` symmetric with `target-files:` under the relaxation introduced in intent 4
- The matchable-set for a single buildfile validation (without a project pass) is the on-disk set only; the cross-pass extension is documented as "project-pass mode" and gated on the validator being invoked through `parlay validate --project` or its in-process equivalent
- Pattern syntax stays as today (Go `filepath.Match`-style); no glob-feature creep in this feature
- An empty resolution set is an error, not a warning — silent-no-op cross-cuttings are exactly the bug we are fixing
- Resolution is deterministic and order-independent — the same `(pattern, on-disk-set, cross-pass-creates-set)` produces the same resolved path set
- Resolution does NOT call out to the adapter or shell — it is a pure function over the inputs

**Verify**:
- A cross-cutting with `target-pattern: "src/registry/*.go"` whose pattern matches three on-disk files validates cleanly only when all three appear in `plan.modifies` with the entry as source
- A cross-cutting with `target-pattern: "src/generated/clients/*.ts"` whose pattern matches zero on-disk files but matches one path in another feature's `plan.creates` resolves to that path and validates cleanly when the path appears in this feature's `plan.creates`
- A cross-cutting whose pattern resolves to zero paths fails with `cross-cutting-pattern-empty`, citing the pattern and the search roots
- Re-validating the same buildfile twice produces identical resolved sets and identical verdicts

---

## A cross-cutting entry may declare both create-targets and modify-targets

**Goal**: Express in one entry the natural "introduce a registry, then write its first entry" pattern, where a single cross-cutting concern both creates a file and modifies it (or, more commonly, creates one file and modifies a sibling). Today this either requires splitting the concern into two entries (losing the "this is one cross-cutting concern" framing) or using `target-pattern:` to paper over both kinds and bypassing plan checks.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: Some cross-cuttings are genuinely two-kinded. A "register this feature in the global router" cross-cutting both creates `routes/<feature>.go` and modifies `routes/index.go`. The buildfile schema today forces such concerns into either two cross-cutting entries (artificial split) or one entry with `target-pattern:` (silent bypass). With intent 1 routing by entry kind, we need a way to declare an entry as legitimately both-kinded.
**Action**: Add an optional `target-creates:` field to the cross-cutting entry schema, parallel to `target-files:`. Its semantics are identical to `target-files:` for a purely-introducing entry — every listed path must appear in `plan.creates` with the entry as source. An entry that declares both `target-files:` and `target-creates:` is valid; `target-files:` routes to `plan.modifies` and `target-creates:` routes to `plan.creates`. The "mixed kinds in one list" error from intent 1 only fires when both kinds appear in `target-files:` (or in a resolved `target-pattern:`); declaring them on separate fields is the supported expression.
**Objects**: target-creates, target-files, two-kinded-entry, plan-routing

**Constraints**:
- `target-creates:` is a new optional field — buildfiles that don't use it are unaffected
- When both fields are present, `target-files:` is interpreted as modifies-only (every path must exist on disk) regardless of the entry-kind heuristic from intent 1; the heuristic is bypassed when the author declares the shape explicitly
- A `target-pattern:` on a two-kinded entry resolves once and is split by the on-disk vs cross-pass-creates classification — on-disk matches go to `plan.modifies`, future-create matches go to `plan.creates`
- Schema documents `target-creates:` alongside `target-files:` and `target-pattern:`, with examples for each common shape (purely-introducing, modifies-only, two-kinded)
- New error code `cross-cutting-target-creates-not-in-plan` for paths listed in `target-creates:` that do not appear in `plan.creates`
- An entry that lists the same path in both `target-files:` and `target-creates:` is rejected with `cross-cutting-target-double-listed`

**Verify**:
- A cross-cutting with `target-files: [routes/index.go]` and `target-creates: [routes/feature.go]` validates cleanly when `plan.modifies` cites `routes/index.go` and `plan.creates` cites `routes/feature.go`, both with this cross-cutting as source
- The same entry with `routes/feature.go` accidentally listed in `plan.modifies` fails with both `plan-modify-target-missing` and `cross-cutting-target-creates-not-in-plan`
- An entry with overlapping `target-files:` and `target-creates:` paths fails with `cross-cutting-target-double-listed`, naming the offending path
- A buildfile with no `target-creates:` anywhere validates identically to today (no regression)

---

## `plan.modifies` is satisfied by an on-disk file OR another feature's `plan.creates` in the same pass

**Goal**: Allow generate-code to plan a project-wide pass where feature A creates a file and feature B modifies it, without B's buildfile failing validation just because the file is not yet on disk at the moment B is checked in isolation.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: Today, `plan-modify-target-missing` rejects any `plan.modifies` path absent from the source root. That is correct for single-feature validation but wrong for project-pass validation, where a sibling feature in the same pass will produce the file. The relaxation must not silently mask real authoring errors when validating a single feature against its real-world source root, so the relaxed mode is gated on the validator running in project-pass mode.
**Action**: Thread a project-wide `plannedCreates` set through `validatePlanSection` (a `map[string]struct{}` of every `plan.creates` path across every feature in the pass, excluding the feature being validated). When the set is non-nil, the `plan-modify-target-missing` rule treats a path as satisfied if it exists on disk OR appears in `plannedCreates`. When the set is nil (single-feature validation, the legacy default), the rule behaves as today. Generate-code's strict-target rule is unchanged — runtime behavior still requires the file to exist when modify is attempted, and the codegen pass orders create-then-modify by walking the plan dependency graph.
**Objects**: plannedCreates, project-pass, plan-modify-target-missing, dependency-graph, generate-code

**Constraints**:
- The relaxation is opt-in via the project-pass entry point — bare `parlay validate --type buildfile --deep <path>` against one buildfile uses the on-disk-only check, so a developer running validation in a single-feature loop still gets the strictest answer
- `plannedCreates` carries source attribution alongside each path (`feature/<slug>` or similar) so error messages can name the producing feature when a modify resolves through a sibling create
- A modify-path satisfied by a sibling create still requires the feature's plan row to cite the cross-cutting / component as source — the relaxation is on existence, not provenance
- `parlay validate` gains (or extends) a project-pass invocation that walks every feature's buildfile, collects each `plan.creates` set, then validates each feature with the union-minus-self as `plannedCreates`. The exact CLI shape is decided in dialogs
- Generate-code orders create-then-modify deterministically — every path appearing in both another feature's `plan.creates` and this feature's `plan.modifies` is emitted by the producing feature first; cycles (A modifies B's create, B modifies A's create) are a hard error with code `plan-create-modify-cycle`

**Verify**:
- A two-feature project where feature A has `plan.creates: [router.go]` and feature B has `plan.modifies: [router.go]` validates cleanly under project-pass mode
- The same two-feature project, validated one feature at a time with single-feature mode, returns `plan-modify-target-missing` for feature B (legacy behavior preserved)
- A two-feature cycle (A creates X and modifies Y; B creates Y and modifies X) fails with `plan-create-modify-cycle`, naming both features and both paths
- A modify-path satisfied through a sibling create whose plan row lacks proper `sources:` attribution still fails with the existing source-attribution rule
- Generate-code emission for the two-feature project writes the create before the modify; reversing the order in the project-pass output (e.g., by feature slug alphabetization) still produces the create-first emission order

---
