# Testcases infrastructure suite kind

> The v2 `testcases.yaml` schema closes the suite `kind:` discriminator to exactly `{presentation, operation}`, and the validator rejects anything else with `testcases-suite-kind-unknown`. Neither value fits a cross-cutting infrastructure suite — the tests that cover an `infrastructure.md`-derived `cross-cutting:` entry (a boundary, an allowlist, a dependency pin, a probe) that produces no surface fragment and no capability operation. A feature whose only build output is cross-cutting has nowhere to put its testcases under schema_version 2; the workaround was to emit v1 testcases (no `schema_version:`), which loses v2's coverage accounting and trips the `testcases-source-refs-missing-legacy` warning. This feature adds a first-class infrastructure suite kind so a cross-cutting-only feature can express its coverage in the v2 shape.

---

## Add an `infrastructure` suite kind to the v2 testcases schema

**Goal**: Let a v2 `testcases.yaml` suite declare `kind: infrastructure` (or an equivalently-named cross-cutting kind) that references a `cross-cutting:` entry as its source of truth, so an infrastructure-only feature has a valid, non-legacy home for its testcases. Presentation and operation suites are unaffected.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: `core/internal/embedded/schemas/testcases.schema.md` (around line 117) states: "Multi-target projects bump `testcases.yaml` to `schema_version: 2` with a `kind:` discriminator over the closed set `{presentation, operation}`." The validator emits `testcases-suite-kind-unknown` for any suite whose `kind:` falls outside that set. A `kind: presentation` suite is required to cite a surface fragment via `source_refs:`, and a `kind: operation` suite must reference a `@<feature>/operation:<id>` from `capabilities.yaml`; the coverage walker fires `testcases-operation-uncovered` per uncovered canonical operation. A cross-cutting infrastructure suite has neither a surface fragment nor a canonical operation to cite — its subject is a `cross-cutting:` entry in the buildfile (sourced from an `infrastructure.md` fragment). With no matching kind, the only shapes that validate force the author to either mislabel the suite (`presentation`/`operation`, then fail the source_refs/coverage rules) or drop to v1 (no `schema_version:`), which loads as `kind: presentation` with an auto-populated approximate `source_refs[0]` and raises `testcases-source-refs-missing-legacy` as a warning.
**Action**: Extend the closed `kind:` set in `testcases.schema.md` to include an infrastructure/cross-cutting kind. Define its `source_refs:` shape to cite a `cross-cutting:` id (the normalized reference to the buildfile cross-cutting entry, itself traceable to an `infrastructure.md` fragment). Update the coverage rules so an infrastructure suite satisfies coverage for its referenced cross-cutting entry, parallel to how an operation suite covers a canonical operation. Update `testcases-suite-kind-unknown`'s allowed-set message. The classification and coverage checks stay mechanical — no AI.
**Objects**: testcases-schema, suite-kind, infrastructure-suite, cross-cutting-entry, source-refs, coverage-walker, schema-version-2

**Constraints**:
- The `kind:` set stays closed — this feature widens it by exactly one documented member, it does not open the discriminator to free-form values. `testcases-suite-kind-unknown` still fires for anything outside the widened set.
- An infrastructure suite's `source_refs:` cites a real `cross-cutting:` entry that exists in the feature's buildfile; a dangling reference fails with a dedicated code (parallel to `testcases-source-refs-missing` / `testcases-operation-uncovered`).
- Presentation and operation suites are byte-for-byte unaffected; a testcases file that validates today continues to validate identically.
- The coverage accounting is symmetric with the operation path: every `cross-cutting:` entry in the buildfile that warrants coverage has at least one covering infrastructure suite, or an explicit exemption; a missing one fires a `testcases-*-uncovered`-family code naming the cross-cutting id.
- The schema documents the new kind alongside `presentation` and `operation`, with an example suite citing a cross-cutting entry and asserting an `infrastructure.md` `Invariants:` bullet.

**Verify**:
- A v2 `testcases.yaml` with a `kind: infrastructure` suite citing a real `cross-cutting:` id validates cleanly (exit 0), where today the same suite fails with `testcases-suite-kind-unknown`.
- An infrastructure-only feature (buildfile has `cross-cutting:` entries, no `components:` and no `operations:`) can express complete coverage under `schema_version: 2` with no v1 fallback and no `testcases-source-refs-missing-legacy` warning.
- An infrastructure suite citing a non-existent cross-cutting id fails with a dangling-reference code naming the id.
- A cross-cutting entry that warrants coverage but has no covering infrastructure suite (and no exemption) fires the uncovered-family code naming the cross-cutting id.
- A pre-existing v2 file containing only `presentation` and `operation` suites validates byte-identically to today.
