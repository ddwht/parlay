# Buildfile plan-shape validation

> Two gaps in how the deep buildfile validator reads the `plan:` section. First, the validator is blind to the multi-target `plan.targets.<kind>.creates|modifies|deletes` shape — it decodes only the flat top-level `plan.creates|modifies|deletes`. A multi-target buildfile that carries per-target plan rows must redundantly duplicate the flat aggregate to satisfy the validator, and any per-target rows that disagree with the aggregate go unchecked. Second, plan paths are resolved by joining against a single root derived from the buildfile's own location, so a child-root buildfile must express paths relative to the resolved child root (`internal/x.go`, not `studio/internal/x.go`); a wrong-prefixed `plan.modifies` entry fails late with `plan-modify-target-missing`, while a wrong-prefixed `plan.creates` entry passes silently because creates are only checked for non-existence. This feature closes both: the validator understands the per-target plan shape, and buildfile path prefixes are validated (or normalized) so a wrong prefix is caught early.

---

## Deep validator reads and reconciles `plan.targets.<kind>` rows

**Goal**: Make the deep buildfile validator aware of the multi-target `plan.targets.<kind>.creates|modifies|deletes` shape so a multi-target buildfile is not forced to duplicate the flat top-level plan to satisfy validation, and so per-target plan rows are actually validated (path presence, sources attribution) and reconciled against the top-level aggregate instead of silently ignored.
**Persona**: Parlay tool maintainer
**Priority**: P2
**Context**: In `core/internal/agent/validate.go` the `deepPlan` struct (around line 151) decodes only `Modifies`, `Creates`, `Deletes` from the top-level `plan:` — there is no `Targets` field. `validatePlanSection` in `core/internal/agent/validate_project.go` therefore reads only the flat top-level plan when it cross-checks components, cross-cuttings, and on-disk shape. Meanwhile `core/internal/embedded/schemas/buildfile.schema.md` (around line 336) says multi-target buildfiles "additionally nest per-target plan rows under `plan.targets.<kind>:`" and the top-level rows are "what the per-target rows aggregate from," and `core/internal/embedded/skills/build-feature.skill.md` (around line 300) normalizes legacy `plan.creates`/`plan.modifies` INTO `plan.targets.<kind>.creates`/`plan.targets.<kind>.modifies`. The net effect: the validator only ever inspects the flat aggregate, so a build agent that emits per-target rows must ALSO emit the flat shape or the validator reports `missing-plan` / `component-not-in-plan`; and a buildfile whose per-target rows disagree with the aggregate (a path present under `plan.targets.transport.creates` but missing from top-level `plan.creates`) validates clean, then codegen may touch a path the validator never checked.
**Action**: Add a `Targets` field to `deepPlan` (a map keyed by target kind, each with `creates`/`modifies`/`deletes` entry lists) and teach `validatePlanSection` to fold the per-target rows into the effective plan it validates. Either (a) treat the union of top-level + per-target rows as the plan under check, or (b) require the top-level to be the exact aggregate of the per-target rows and flag any divergence — decide which in dialogs. Every per-target entry is subject to the same per-entry rules the flat entries already get (non-empty `path`, non-empty `sources`, on-disk create/modify checks). The reconciliation is deterministic — a pure function over the decoded plan, no adapter, no AI.
**Objects**: deep-plan, plan-targets, target-kind, flat-plan-aggregate, validate-plan-section, multi-target-buildfile, plan-entry

**Constraints**:
- A presentation-only buildfile (no `plan.targets:`) validates byte-identically to today — the new decoding and reconciliation are inert when `plan.targets:` is absent.
- Every per-target plan entry carries the same integrity requirements as a flat entry: a missing `path` or missing `sources` fails with the existing `plan-entry-missing-path` / `plan-entry-missing-sources` codes, now reachable through the per-target rows.
- A divergence between the top-level aggregate and the per-target rows (a path in one but not the other) is surfaced with a dedicated, stable error code naming the offending path and the target kind — it is not silently tolerated.
- The chosen contract (union vs strict-aggregate) is written into `buildfile.schema.md` so build-feature emits exactly what the validator expects — the schema, the normalization step, and the validator must agree, ending the "emit both shapes to be safe" workaround.
- No change to the flat-only authoring shape a single-target project uses; the change only adds awareness of the per-target rows a multi-target project already emits.

**Verify**:
- A multi-target buildfile that carries only `plan.targets.<kind>` rows (no duplicated flat aggregate, or an aggregate consistent with the chosen contract) validates cleanly, where today it reports `missing-plan` or `component-not-in-plan`.
- A per-target entry with an empty `path` or empty `sources` fails with `plan-entry-missing-path` / `plan-entry-missing-sources` (the per-target rows are actually inspected).
- A buildfile whose `plan.targets.transport.creates` lists a path absent from the top-level `plan.creates` fails with the divergence code naming the path and `transport`.
- A presentation-only buildfile with no `plan.targets:` validates byte-identically to today.

---

## Child-root buildfile path prefixes are validated against the resolved root

**Goal**: Catch a buildfile plan path that carries the child-root directory prefix (e.g. `studio/internal/x.go`) when the resolved root is already the child, so the mistake surfaces uniformly across `plan.creates`, `plan.modifies`, and `plan.deletes` — not only when a `plan.modifies` entry happens to fail its existence check. One documented prefix convention is canonical, and a deviation is reported (or normalized) deterministically.
**Persona**: Parlay tool maintainer
**Priority**: P2
**Context**: `planRootDirFromBuildfilePath` in `core/internal/agent/validate_project.go` (around line 690) derives the source root by walking up from the buildfile to `<root>/.parlay/build/` and returning `<root>`. For a child-root feature the buildfile lives at `<childRoot>/.parlay/build/<feature>/buildfile.yaml`, so the resolved root is `<childRoot>` and every plan path is joined against it. A path written with the child prefix — `studio/internal/x.go` instead of `internal/x.go` — resolves to `<childRoot>/studio/internal/x.go`, which does not exist. The disk-shape loop in `validatePlanSection` reports `plan-modify-target-missing` for such a `plan.modifies` entry, but a `plan.creates` entry is only checked for a collision (it errors only if the joined path DOES exist), so a wrong-prefixed create passes silently. The inconsistency means a prefix mistake lurks until the first `plan.modifies` entry trips over it, rather than being caught at once for the whole plan.
**Action**: Add a prefix-consistency check to `validatePlanSection` (or a `parlay repair` / migrate step) that, given the resolved root and its registered-root identity, flags any plan path whose leading segment restates the resolved root's own directory name — applied uniformly to creates, modifies, and deletes. Pin one convention as canonical in `buildfile.schema.md`: plan paths are relative to the resolved root (no child prefix). Offer either a dedicated validation error (`plan-path-root-prefixed` or similar) directing the author to drop the prefix, and/or a normalization pass that strips the redundant prefix. The check is mechanical — a string comparison against the resolved root's base name, no AI.
**Objects**: plan-path, resolved-root, child-root-prefix, path-normalization, validate-plan-section, repair, canonical-prefix-convention

**Constraints**:
- The canonical convention (root-relative, no child prefix) is documented once in `buildfile.schema.md`; the validator and any normalization step enforce exactly that convention, so creates and modifies agree.
- The new check fires identically for `plan.creates`, `plan.modifies`, and `plan.deletes` — a wrong-prefixed create is caught at validation time, not only after a sibling modify fails.
- A correctly-prefixed buildfile (paths already root-relative) validates byte-identically to today — the check adds errors only for genuinely root-prefixed paths.
- The check does not fire for a parent-root buildfile whose paths are legitimately root-relative and happen to share a leading segment with an unrelated directory — the trigger is specifically "leading segment equals the resolved root's own directory name," not any coincidental match. (Cross-root references belong to a separate feature; this one only normalizes the redundant self-prefix.)
- If normalization is offered, it is opt-in via an explicit `parlay repair`/migrate verb, never a silent rewrite during ordinary validation — consistent with the project's "external commands use explicit repair, never silent auto-detection" rule.

**Verify**:
- A child-root buildfile with `plan.creates: [studio/internal/x.go]` where the resolved root is `studio` fails with the root-prefixed code (today it passes silently).
- A child-root buildfile with `plan.modifies: [studio/internal/x.go]` fails with the root-prefixed code and a fix message naming the root-relative path `internal/x.go`, in addition to or in place of the late `plan-modify-target-missing`.
- A child-root buildfile whose paths are already root-relative (`internal/x.go`) validates byte-identically to today.
- A parent-root buildfile with a legitimate path whose first segment coincidentally matches an unrelated directory does not trigger the root-prefixed code.
