---
amendment: design-spec-surface-retired
date: 2026-08-31
trigger: "user authorization (2026-08-31) to retire the design-spec/Figma authoring surface as part of parlay becoming backend-only; the pipeline was delivered but has never been used"
supersedes_intents:
  - reference-design-spec-from-figma
---

## Change

This feature no longer promises design-spec enrichment. The founding intent
"Reference Design Spec from Figma" is retired whole: there is no
`/parlay-reference-design-spec`, no `design-spec.yaml`, and no path by which a
Figma file reaches a buildfile.

What goes with it is the entire surface that intent's constraints named, and
nothing beyond it:

- The skill that produced the artifact — `reference-design-spec.skill.md` —
  and the deployed module it became.
- The artifact's schema, `design-spec.schema.md`, and the authoring digest
  derived from it. The intent required the design-spec to reference "the
  adapter's design-system categories"; that cross-reference dies with both
  halves, the surviving half being the adapter schema below.
- The `figma` value in an adapter's `design-system.<category>.source:`
  vocabulary, which existed for exactly one reason: to tell the build phase to
  read values out of a design-spec. The vocabulary is now
  `{framework, not-defined}`, in the schema and in the validator that enforces
  it, which had its own hardcoded copy of the set.
- The design-spec reading instructions in `build-feature.skill.md`. The intent
  promised "build-feature reads the design-spec IF it exists"; it no longer
  does, and the surrounding prose is rewritten to stand on its own rather than
  to describe an absence.
- The `design-spec-fragments` and `design-spec-shared` baseline fields, the
  `hashDesignSpecFragments` helper that populated them, the `design_spec`
  source-level diff, and the `design-spec:<fragment>` values that diff could
  emit in `changed_sources`. The intent's fourth verify criterion — "build-feature
  produces a richer buildfile when design-spec.yaml exists" — was the only
  thing these ever served.
- The registry rows that advertised the surface: the module table in
  `loop.skill.md`, the design-spec rows in `feature-structure.schema.md`, the
  `create-surface-by-figma` entry point in `surface.schema.md`, and the
  design-spec registration in the generated schema `DIGEST.md`.

The intent's fifth verify criterion — "the pipeline works identically when
design-spec.yaml does not exist" — is the one that survives, promoted from a
conditional to the whole truth. That was always the supported path; it is now
the only one.

Two things this record does not touch. `parlay-tool/multi-root`'s frozen
buildfile names `internal/embedded/skills/reference-design-spec.skill.md` in
its sources and file list. Those rows are now historical: they are evidence of
what the multi-root wrapper edit covered when it ran, not a claim that the file
is there today, and they are preserved byte-unchanged rather than repaired.
This is the same disposition `studio-support/page-layout-field`'s amendments
001 and 003 settled — a retired obligation and its preserved evidence are
different things, and rewriting frozen build artifacts to make a grep come back
clean destroys the record the ledger exists to keep. Separately,
`parlay-tool/domain-model`'s domain model still lists a `generateSurfaceFromFigma`
operation and a `create-surface-by-figma` transition. That is another feature's
contract artifact, and one feature may not retire another's; it stands until a
decision in that feature's own ledger names it.

## Why

Parlay is becoming a backend-only product. A design-tool integration is not
part of what it is for anymore, and the question is only whether to carry the
one that exists.

It has never been used. There is no `design-spec.yaml` anywhere in this
repository or any project built from it, and no adapter — shipped, embedded, or
project-authored — declares `source: figma` for any design-system category.
Every mechanism enumerated above ran, when it ran at all, against a file that
was never there: `hashDesignSpecFragments` returned empty on every invocation,
the `design_spec` diff was empty on every build, and no baseline on disk carries
either field. This is not a working feature being cut for scope. It is
scaffolding for a workflow nobody ever entered, and its cost has been entirely
in the reading.

That cost is the argument. The surface is small in lines and wide in reach: it
touches two schemas' vocabularies plus a third's registry, four skills, the
baseline format, the diff format, and a Go validator with its own copy of the
adapter vocabulary. Every one of those is a place where an author or an agent
is told that a Figma round-trip is available. Left alone, the instructions do
not merely sit there — `build-feature.skill.md` spends a block of its
buildfile-authoring step explaining how to fold design-spec tokens and variants
into components, conditioned on a file that cannot exist. An agent reading it
must work out for itself that the branch is dead. Documentation for a path with
no entrance is worse than no documentation, because it reads as current.

Retiring it in the ledger rather than deleting it quietly is the point of this
record. "Reference Design Spec from Figma" is a founding intent of a built
feature, with a persona, constraints, and five verify criteria that the
baseline still hashes. Deleting the skill and the schema while that promise
stood in `intents.md` would leave the feature promising an artifact its own
pipeline cannot produce, and the next reader would find the gap with no
decision attached to it. The founding document is frozen and stays exactly as
written; this amendment is what makes it history rather than a broken promise.

If parlay ever grows a design-tool integration again, it should define its own
contract rather than resurrect this one. This surface was shaped by
assumptions that have since been overtaken — most concretely, it predates
`<page>.layout.yaml`, and had already been narrowed once when structural layout
moved out of design-spec's scope into the layout tree, leaving it holding
tokens, variants, spacing and colors. A future integration would land in a
world where layout, page assembly, and the domain model are all settled
elsewhere, and it would want a contract drawn against that world. Preserving
this one against that day preserves the wrong shape and calls it a head start.

## Acceptance

- No embedded skill, schema, or generated digest describes a design-spec,
  a `/parlay-reference-design-spec` command, or a `create-surface-by-figma`
  entry point, and `parlay upgrade` prunes the previously-deployed module,
  schema, and digest from a project rather than orphaning them on disk.
- An adapter declaring `source: figma` for a `design-system:` category is
  rejected with `adapter-design-system-source-unknown`, and the schema and the
  validator state the same vocabulary, `{framework, not-defined}`.
- `parlay internal diff @feature` emits no `design_spec` section and no
  `changed_sources` value prefixed `design-spec:`, and a newly written
  `.baseline.yaml` carries no `design-spec-fragments` or `design-spec-shared`
  key.
- An existing `.baseline.yaml` written before this amendment still loads
  without error, any design-spec keys in it being ignored rather than rejected.
- `build-feature` reads a feature's founding documents and contract artifacts
  and produces a buildfile with no step conditioned on a design-spec, and its
  instructions read as complete rather than as describing a missing file.
- `parlay-tool/multi-root`'s buildfile still names
  `internal/embedded/skills/reference-design-spec.skill.md` in its sources and
  file list, byte-unchanged, as evidence of an edit that ran.
