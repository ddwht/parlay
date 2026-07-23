# Cross-cutting target-deletes

> The buildfile `cross-cutting:` entry schema has `target-files:` (paths to modify), `target-pattern:` (a glob resolving to modify targets), and `target-creates:` (paths to introduce, parallel to `target-files:` for the two-kinded shape) — but no `target-deletes:` field analogous to `target-creates:`. A cross-cutting concern that drives a deletion has nowhere to declare it: the deletion is expressed only as a top-level `plan.deletes` entry whose `sources:` cites the cross-cutting id by convention. Nothing schema-enforces that a `plan.deletes` row traces back to a declared delete-intent on the cross-cutting entry, so a deletion's provenance is weaker than a create's or a modify's. This feature adds `target-deletes:` to close the create/modify/delete symmetry and make delete traceability schema-enforced rather than conventional.

---

## Add `target-deletes:` to the cross-cutting entry schema

**Goal**: Give a `cross-cutting:` entry a `target-deletes:` field, parallel to `target-creates:`, so a cross-cutting concern that removes files declares them explicitly, and the deep validator enforces that each declared delete has a matching `plan.deletes` row sourced from the entry — the same routing guarantee `target-creates:` and `target-files:` already get for creates and modifies.
**Persona**: Parlay tool maintainer
**Priority**: P3
**Context**: `core/internal/embedded/schemas/buildfile.schema.md` (cross-cutting section, around lines 204-268) lists `target-files:`, `target-pattern:`, and `target-creates:`, with "at least one of `target-files`, `target-pattern`, or `target-creates` must be present." The deep validator (`validatePlanSection` in `core/internal/agent/validate_project.go`) routes `target-files:` to `plan.modifies`, `target-creates:` to `plan.creates`, and resolves `target-pattern:` per-path — each with a dedicated error code (`cross-cutting-target-not-in-modifies`, `cross-cutting-target-creates-not-in-plan`, etc.). The `deepCrossCuttingEntry` struct decodes `TargetFiles`, `TargetPattern`, `TargetCreates` — but no `TargetDeletes`. `plan.deletes` rows exist (build-feature step 8 emits one per `components.removed[]` id), and cross-cutting-driven deletes can be placed there with `sources:` citing the cross-cutting id, but only by convention — there is no `cross-cutting-target-deletes-not-in-plan` rule tying a declared delete-intent to its `plan.deletes` row, so a deletion's traceability is second-class next to creates and modifies.
**Action**: Add an optional `target-deletes:` field to the cross-cutting entry (schema + `deepCrossCuttingEntry.TargetDeletes`). Route each `target-deletes:` path to `plan.deletes`, requiring a matching row sourced from the entry, with a new stable error code (`cross-cutting-target-deletes-not-in-plan`) for a declared delete missing from `plan.deletes`. Document `target-deletes:` alongside `target-files:`/`target-creates:`/`target-pattern:` with an example, and extend the "at least one target field present" rule to count `target-deletes:`. The routing is mechanical, mirroring the existing target-creates handling — no AI.
**Objects**: cross-cutting-entry, target-deletes, plan-deletes, target-creates, deep-cross-cutting-entry, plan-routing, delete-traceability

**Constraints**:
- `target-deletes:` is a new optional field — buildfiles that do not use it validate byte-identically to today; the change only widens the set of valid buildfiles.
- A `target-deletes:` path routes strictly to `plan.deletes`; a declared delete missing from `plan.deletes` fails with `cross-cutting-target-deletes-not-in-plan` naming the path and the cross-cutting id (symmetric with `cross-cutting-target-creates-not-in-plan`).
- The within-entry double-listing guard extends to deletes: the same path listed in both `target-deletes:` and `target-files:`/`target-creates:` is rejected (a path cannot be simultaneously deleted and modified/created by one entry).
- A two-kinded (or three-kinded) entry declaring `target-files:` + `target-creates:` + `target-deletes:` together is valid, each list routing to its own plan section; the on-disk heuristic that classifies a bare `target-files:`-only entry is bypassed when explicit fields are declared.
- The "at least one target field present" rule counts `target-deletes:` — an entry whose only target field is `target-deletes:` is valid.

**Verify**:
- A cross-cutting entry with `target-deletes: [legacy/old.go]` validates cleanly when `plan.deletes` cites `legacy/old.go` with the entry as `sources`.
- The same entry with the delete missing from `plan.deletes` fails with `cross-cutting-target-deletes-not-in-plan` naming `legacy/old.go` and the cross-cutting id.
- An entry listing the same path in both `target-deletes:` and `target-files:` (or `target-creates:`) is rejected with the double-listing code.
- An entry whose only target field is `target-deletes:` satisfies the "at least one target field present" rule.
- A buildfile with no `target-deletes:` anywhere validates byte-identically to today.
