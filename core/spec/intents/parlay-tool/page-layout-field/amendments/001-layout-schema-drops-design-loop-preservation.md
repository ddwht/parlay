---
amendment: layout-schema-drops-design-loop-preservation
date: 2026-08-31
trigger: "user authorization (2026-08-31) to remove every Design-Loop and Figma claim from layout.schema.md, which this feature's build contract required to stay byte-equivalent"
affects:
  - "@parlay-tool/page-layout-field/infrastructure:layout-tree-schema"
---

## Change

This feature no longer requires `layout.schema.md` to preserve the Design-Loop
and Figma content it inherited. Two assertions and the build constraint behind
them are retired:

- the testcase `layout-schema-doc-preserves-design-loop-figma-block-marker`,
  which asserted the file still contains the HTML comment
  `parlay-feature: design-loop/design-loop` and the heading
  `## Optional figma: block (Design Loop)`;
- the buildfile constraint requiring "the entire `## Optional `figma:` block
  (Design Loop)` section (including its inner header comment naming
  parlay-feature: design-loop/design-loop)" to stay byte-equivalent through
  this feature's edit.

The sibling assertion `layout-schema-doc-preserves-adapter-vocabulary-extension-marker`
is untouched and still binding: the universal-container-fields section and its
`parlay-feature: studio-support/adapter-vocabulary-extension` marker remain
owned by that feature, and this feature still may not take them over. The
preservation rule was never about the figma block specifically; it was about
this feature editing only the section it owns. That rule survives. Only the
list of sections it names shrinks, because one of them no longer exists.

The Layout Tree Schema fragment is also recast where it names the writer.
Its behavior is unchanged — the typed-tree shape, the universal node and
container fields, the vocabulary-delegated per-component fields, the recursive
walk, the pinned schema-version, and every error naming node identifier and
field all stand exactly as specified. What changes is that the schema's writers
are no longer "Studio's design-loop": the tree shape is a core contract for
`*.page.md` layouts and the buildfile's layout regions, and its readers —
codegen, validation tooling, the layout precheck — are the whole of its
audience.

## Why

The schema and the assertions disagreed about the world, and something had to
give. The user authorized removing every Design-Loop and Figma claim from
`layout.schema.md` on 2026-08-31; those claims include a `figma:` block whose
only documented consumer is a `parlay-design-loop` skill that has never
existed in any deployed skill set. The schema was shipping a section that
pointed at `.claude/skills/parlay-design-loop/SKILL.md`, a file no `parlay
upgrade` has ever written. So the removal did not delete a working contract —
it deleted a claim that was already untrue, and had been for as long as the
section existed.

That left the assertions guarding it, which is why this record exists rather
than a quiet schema edit. These are not incidental prose in a retired root:
`studio-support/page-layout-field` is a core-root, already-built feature, and
a testcase plus a buildfile constraint are its contract. Editing the schema
and leaving them standing would produce a feature whose own tests assert the
opposite of what its source says — the failure mode where a green suite means
nobody ran it. Deleting them without a record would be worse: quietly dropping
an assertion because it became inconvenient is exactly the move the ledger
exists to prevent, and the next reader would find a test that vanished with no
decision attached.

So the assertions are retired here, named, with the authorization that
retires them and the reason it was granted. The layout schema keeps doing the
one job it was always doing — defining the universal container fields and the
tree shape every layout author uses — and stops advertising a round-trip that
was never wired up.

## Acceptance

- `core/internal/embedded/schemas/layout.schema.md` contains no
  `parlay-feature: design-loop/design-loop` marker, no
  `## Optional figma: block (Design Loop)` heading, and no `figma:` block
  section, and this feature's suite is green with it absent.
- `core/internal/embedded/schemas/layout.schema.md` still contains the
  `parlay-feature: studio-support/adapter-vocabulary-extension` marker and its
  universal-container-fields section, byte-equivalent.
- The layout schema still declares the universal container fields, the
  top-level structure, and the layout precheck error codes, and a layout tree
  using only universal fields still validates against it.
