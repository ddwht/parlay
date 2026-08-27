<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/hand-authored-units
-->

# Authored Unit Schema

File: `spec/intents/<unit>/authored.yaml`. Declares a **hand-authored unit** — code parlay must never write, named so parlay can still see it.

Every other piece of parlay's state is downstream of emission: `.code-hashes.yaml` is keyed by the generation marker, drift is measured against what a buildfile produced, coverage is measured against suites the build phase invented. Code the tool did not write is therefore not merely untracked, it is *unrepresentable* — and a project's most load-bearing code is often exactly the part a person wrote by hand. This file is the declaration that makes such code addressable: it says where the sources are, where their tests are, and which invariants they already satisfy, without ever inviting the tool to regenerate any of it.

## Relationship to the four spec artifacts

`authored.yaml` is **not** a fifth co-equal spec artifact. Surface, capabilities, infrastructure and domain-model each describe *what the tool should produce*. This file describes *what the tool must leave alone*. It sits in the same directory because a unit is addressed by the same qualified identifier a feature is, not because it participates in the same generation pipeline.

## Structure

```yaml
schema_version: 1
unit: geometry-engine
summary: "image → 3D relief mesh; six-stage pure transform"
sources:
  - "App/Sources/BlockPrintingCore/**"
  - "App/Sources/BPGeometry/**"
tests:
  - "App/Tests/BlockPrintingCoreTests/**"
satisfies:
  - "@relief-workspace/invariant:deterministic-output"
```

| Field | Required | Description |
|---|---|---|
| `schema_version` | Yes | Integer, currently `1`. See the Versioning section. |
| `unit` | Yes | The unit's slug. Must equal the containing directory's name — the same directory-matches-identifier rule every per-feature build artifact follows for `feature:`. |
| `summary` | Yes | One line stating what the unit is. Read by humans and by the phases that must explain why they refused to generate into it. |
| `sources` | Yes | Non-empty list of root-relative globs naming the hand-authored sources. |
| `tests` | No | Root-relative globs naming the unit's own test sources. A unit with no `tests:` can satisfy no invariant by test, only by inspection. |
| `satisfies` | No | Qualified invariant references the unit's declared tests already cover. Consuming features cite these instead of generating a suite that would re-test them. |

## Globs

Globs are **root-relative**, resolved against the active root, and use the project's ordinary `**` convention. They may not be absolute and may not contain a `..` segment: a unit declares ownership of code inside the project, and a glob reaching outside the root would put files parlay cannot reason about into a manifest that claims it can.

A unit's directory itself is not a source. `sources:` names implementation files elsewhere in the tree; `spec/intents/<unit>/` holds the declaration and the unit's `intents.md`.

## Directory shape

A unit occupies `spec/intents/<unit>/` and `.parlay/build/<unit>/`, and — unlike a feature — **has no `spec/handoff/<unit>/`**. The handoff tree carries engineering specifications for code that is about to be written; a unit's code is already written, by a person, and there is nothing to hand off. This is a deliberate exception to the three-directories-together rule in `feature-structure.schema.md`, and the commands that enforce that rule (`repair`, `status`) special-case the authored class rather than "fixing" the absent twin.

`authored.yaml`'s presence is the **sole** classification signal. A unit carries `intents.md` exactly as a feature does — that is what lets it state what it is for — so keying the classification on `intents.md` would make units and features indistinguishable.

## Versioning

Policy: **migrator chain** (see `schema-versioning.schema.md`). This file is hand-authored and long-lived; nothing can regenerate it, because the thing it describes is by definition the thing the tool does not produce. A future v2 must therefore reach v1 files through a registered migrator rather than by asking the author to rewrite them.

## Validation pass

`parlay validate --type authored spec/intents/<unit>/authored.yaml`.

| Code | When it fires |
|---|---|
| `authored-invalid-yaml` | The file does not parse as YAML, or a field's value has the wrong type — `sources:` given as a bare string rather than a list, say. |
| `authored-field-missing` | A required field is absent, or present but empty — including `sources:` declared with no entries, which would claim a unit owning nothing. |
| `authored-schema-version-unsupported` | `schema_version:` is absent, not an integer, or a version this binary has no migrator chain for. |
| `authored-unit-slug-mismatch` | `unit:` does not match the containing directory's name. The identifier resolvers key on the directory, so a mismatch means every command addresses the unit by a name the file disagrees with. |
| `authored-glob-escapes-root` | A `sources:` or `tests:` glob is absolute or contains a `..` segment. |

## Resolution pass

Run by `parlay save-build-state`, which expands the declared globs against the filesystem and records the result. These fire against a project rather than a file, so `validate --type authored` cannot raise them.

| Code | When it fires |
|---|---|
| `authored-glob-empty` | A declared glob matches no file. Not a harmless no-op: the unit reads as owning files while tracking none, so it looks declared and behaves undeclared. |
| `authored-glob-overlaps-generated` | A path is claimed by two units, or is claimed by a unit and also declared emitted by codegen in the same run. Both statements are authoritative and contradictory, and nothing on disk can decide between them, so the run refuses rather than picking a winner. |

## How a unit gets declared

Two ways, and the second is the one that matters.

Directly: `parlay add-feature "<name>" --authored --sources "<glob>" --summary "<one line>"`. Writes `intents.md` and `authored.yaml`, creates the build directory, creates no handoff directory.

By offer: a phase that cannot express what the spec asks for raises a `kind: impasse` decision proposing the unit, pre-filled from what it already knows — the intents no artifact set expresses, the operations no adapter supports, the invariants it would have generated suites for, the paths it would have written. All three phase groups can raise it, and each has a signal that used to dead-end:

| Phase | Signal | Was |
|---|---|---|
| designer | no artifact subset fits an intent | pick the closest artifact and carry the fiction forward |
| build | `adapter-supports-missing-*`, `capabilities-unknown-term` | stop with a code and no recognized way forward |
| code | the buildfile asks for something unwritable | "report what is missing" — correct, and a dead end |

**`impasse`, not `ambiguity`.** An ambiguity has two readings you cannot choose between; an impasse has none. Their resolutions differ in kind: an ambiguity is settled by the user picking a reading, an impasse by the user agreeing this part will be written by hand and never generated. Offering an impasse as an ambiguity presents a choice between readings that all fail.

**Always asked, never defaulted.** Accepting a unit is a permanent scope reduction — that code will not be generated, by design — so the decision carries no `default:` and `--non-interactive` aborts (exit 11) rather than accepting on the user's behalf. The automation is in the *detection*: noticing the impasse and pre-filling the offer is mechanical, and taking it is not.

The failure mode to watch is the opposite one: a unit offered as an escape from a hard artifact decision trades a solvable problem for a permanent one. The offer is for work that genuinely resists expression, not for work that is merely awkward.

## How the per-feature commands answer for a unit

Several commands ask a question that has no answer for a unit. Each now says so rather than reporting a failure the author cannot act on — a permanent hard error on a correctly-declared unit is how a check stops being run at all.

| Code | When it fires |
|---|---|
| `hand-authored-unit` | `check-readiness` was asked whether a unit is ready for a pipeline stage. Informational, `ready: true` — a unit's code is written by a person, so there is no phase to be ready for. |
| `buildfile-not-applicable` | `check-buildfile` found no buildfile for a unit. Informational: the generic `buildfile-not-found` fix says to run `build-feature`, which on a unit is exactly what must not happen. |
| `unit-not-a-feature` | A command whose whole operation is a pipeline step was pointed at a unit — `sync`, `create-dialogs`, `move-feature` and their kin. An error, not a note: `create-dialogs` would author a `dialogs.md` **inside** the unit directory, leaving the unit looking like the half-built feature it was declared to stop being. |

The tree-walking migrations (`migrate-spec`, `migrate-capabilities`) skip a unit directory whole rather than filtering file by file, so no rewrite pass can reach anything inside one. `simplify` additionally filters its marker scan against the declared units: a feature converted into a unit leaves its old generation markers on disk, and a stale marker must not make a unit's code extractable.

`check-coverage` reports a unit's `satisfies:` and `tests:` rather than intent-to-dialog matching, and does not require the `dialogs.md` a unit never has. `check-composition` does not examine units at all: a unit produces no buildfile and therefore no fixture records, so there is no coherence question to ask, and reporting one as unbuilt would be a by-design fact filed forever as a coverage gap.

## Tracking

Resolved unit files are recorded in `.parlay/build/_project/.code-hashes.yaml` with `provenance: hand-authored`, alongside generated files. They arrive by a **second ingestion path**: the first is `parser.ScanGenerated`, whose admission gate is the generation marker — precisely the property hand-authored code does not have and must never acquire. The two cannot be unified; one is marker-keyed, the other declaration-keyed, and the declaration is the point.

`parlay verify-generated` reports these files in their own `hand_authored` bucket, each with a `changed` flag, and `--strict` does not fail on them. `--strict` asks whether every recorded file is safe to overwrite; a unit file is one parlay must never overwrite, so the question has no answer that could gate anything.

The resolved set is projected to `.parlay/build/_project/authored-files.yaml`, which is what codegen reads — `spec/intents/**` stays off-limits to it, with no filename carve-out.

## The write fence, and which layer enforces it

Codegen loads the projection as a **denylist** before it locks the plan allowlist, and refuses any write to a path in it with `unit-write-refused`. The denylist outranks both the plan and the two exempt classes: a path is refused even when a plan row names it, even when the file carries a `parlay-section:` marker, and even when it is a test file.

**This is enforced by the `generate-code` skill, not by the CLI** — the same enforcement layer, and for the same reason, as `codegen-spec-read-forbidden`. Parlay performs none of codegen's file access; the agent writes with its own tools, so no Go code path can intercept a write, and `unit-write-refused` therefore appears in no error-code table in these schemas. `parlay internal check-write-set` audits the result afterwards and exempts hand-authored files from `codegen-wrote-outside-plan`, but an audit is not the fence: it runs after the bytes are on disk.

Two properties make pre-write the only workable placement:

- **The exempt classes are marker-bearing, not path-bounded.** A test file goes "where the framework expects" and a section file "where file-conventions dictate". For a unit's source, where the framework expects tests *is* the unit's own test directory. The audit exempts both categories by design, so a write that leaked through would come back reported as authorized.
- **Brownfield mount targets exactly what a unit looks like.** Its premise is "an existing source file that is not Parlay-generated", which describes every file in a unit — they carry no marker, by definition. Its intelligent-merge tier then reads such a file and rewrites it in place.

A refusal is reported, never silently skipped. A codegen run that wanted to write into a unit has found a genuine disagreement between a buildfile and a declaration, and which one is wrong is a person's call.
