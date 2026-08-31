---
name: create-adapter
description: "Author a new framework adapter from scratch and validate it"
---

<!--
parlay-section: cross-cutting
parlay-feature: parlay-tool/adapter-authoring
-->

# Create Adapter

Author a framework adapter for a stack with no bundled adapter —
Vue + Vuetify, SwiftUI, FastAPI, TypeORM, anything. Use this when you are
starting from a framework choice rather than from an existing codebase; use
`/parlay-onboard` instead when the code already exists and the adapter should
be derived from it.

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

## Active root

Every path here — `.parlay/adapters/`, `.parlay/adapter-set.yaml`,
`.parlay/schemas/` — resolves against the **active root**, not the directory you
happen to be in. In a multi-root project, write the adapter into the root that
will use it; a child root inherits the parent's adapters, so an adapter only
belongs in a child when it overrides one.

## Arguments

- Optional: the framework (e.g. `Vue + Vuetify`) and the `kind:` it fills.
  Ask when not supplied — do not guess a kind, it changes which sections are
  required.

## The one rule that makes this work

**Never hand the user an adapter you have not validated.** Run
`parlay validate --type adapter <path>` and fix what it reports, until it is
clean. The validator checks every section of `adapter.schema.md` and reports
**all** findings at once with stable codes, so this converges in a couple of
passes rather than one round-trip per defect. An adapter that has not been
through it is a guess.

## Steps

1. **Establish the kind** — one of `presentation`, `transport`, `application`,
   `persistence`. This decides what the adapter owes:

   | kind | must declare | must NOT declare |
   |---|---|---|
   | `presentation` | `shows:` / `actions:` / `flows:` covering the whole surface vocabulary | `supports:` |
   | `transport` / `application` / `persistence` | `supports:` (operation kinds, steps, policies, errors) | `shows:` / `actions:` / `flows:` are not asked for |

   Every kind needs `name:`, `file-conventions:` (with `project-root` and `source-root`,
   `component-pattern`, `naming`, `entry-point`), and a `name:` matching the
   filename slug.

2. **Start from the closest bundled template** — do not start from an empty
   file. Run `parlay internal schema-digest` for the closed vocabularies, and
   read a bundled adapter of the same kind as a worked example:
   `react-antd`, `angular-clarity`, `angular-material`, `go-cli`
   (presentation); `openapi-rest` (transport); `nestjs-application`
   (application); `prisma-postgres` (persistence). Copy its structure, replace
   the framework specifics.

3. **Map the vocabulary (presentation only)** — every Show, Action and Flow in
   the surface vocabulary needs an entry. Two escape hatches exist and are
   correct answers, not omissions:
   - `widget: not-applicable` with a description — the framework genuinely has
     no such thing (`data-chart` in a CLI).
   - `requires: custom-implementation` — conceptually supported, no built-in
     primitive (`undo`/`redo` in most frameworks).

   An omitted term is different from either: it leaves codegen with no widget
   for a term a designer may legitimately write.

4. **Declare `supports:` (backend only)** — list only the terms **this layer**
   implements. A persistence adapter owns the data steps and the transaction
   policy; an application adapter owns `validate-input`, `authorize` and the
   `return-*` steps. Do not list a term another layer owns: coverage is checked
   as a union across the filled backend slots, so listing per-layer is what
   keeps ownership unambiguous.

5. **Write `file-conventions:`** — the part that decides where generated code
   goes:
   - **`paths:`** — one template per artifact kind, **relative to
     `project-root` + `source-root`**, using only `{feature}` `{name}` `{entity}` `{Feature}`
     `{Name}` `{Entity}`. This is what makes `plan:` derivable. Omitting it is
     allowed and means plan derivation is unavailable for this adapter —
     which is a real cost, so only do it deliberately.
   - **`packages:`** — the shared-code directories (components, hooks, utils,
     core). Distinct from `paths:`: it answers "where does reusable code live",
     which no per-artifact template expresses, and it is what `parlay simplify`
     reads to place an extracted helper.

6. **Add the optional sections that earn their place** — `compositions:`
   (state + wiring recipes), `conventions:` (team rules with `rule:` +
   `applies-to:`), `design-system:` (each category's `source:` is
   `framework` / `not-defined`), `mount-strategies:` (brownfield
   insertion; `detection:` + a `template:` with `{{placeholders}}` +
   `description:`), `patterns:` (framework taste — deliberately unvalidated,
   so use whatever values fit the framework), `componentVocabulary:` /
   `tokens:` (only if layouts will target this adapter), `toolchain:` (external
   skills / MCP servers the framework ships).

7. **Validate and iterate** — `parlay validate --type adapter <path>`. Fix
   every reported code and re-run until clean. Do not silence a finding by
   deleting the section it came from.

8. **Register it** — `parlay register-adapter <path>` copies it into
   `.parlay/adapters/` (re-validating on the way in) and prints the
   `adapter-set.yaml` stanza to pin it under its kind. For a multi-target
   project, add that stanza plus the `links:` edges the stack needs.

9. **Prove it derives** — for a presentation or backend adapter with `paths:`,
   run `parlay internal scaffold-plan @<feature>` on a real feature and confirm
   rows come out where you expect. An adapter that validates but derives
   nothing is the failure mode this skill exists to prevent.

## Error handling

- `adapter-vocabulary-incomplete` — a presentation adapter is missing surface
  terms. Add them, using `not-applicable` / `custom-implementation` where the
  framework genuinely lacks a primitive.
- `adapter-supports-shape-mismatch` — the kind and the sections disagree:
  a presentation adapter declaring `supports:`, or a backend one without it.
- `adapter-name-slug-mismatch` — rename the file or the `name:` field; adapters
  are resolved by filename.
- `adapter-path-template-invalid` — a `paths:` template uses a placeholder that
  does not exist; only the six documented ones substitute.
- Anything else: the code names the rule and the message names the fix. The
  full table is in `adapter.schema.md`'s Validation section.
