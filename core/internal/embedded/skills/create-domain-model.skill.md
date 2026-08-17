---
name: create-domain-model
description: "Create domain model from features"
surface: module
---

# Create Domain Model

<!-- parlay-feature: studio-support/domain-model-yaml-migration -->
<!-- parlay-component: extract-domain-model-output -->
<!-- parlay-extends: studio-support/domain-model-yaml-migration/extract-domain-model-yaml-emission -->
<!-- parlay-extends: parlay-tool/create-domain-model/embedded-skill-rename-source-file -->
<!-- parlay-extends: parlay-tool/create-domain-model/skill-greenfield-branch -->
<!-- parlay-extends: parlay-tool/create-domain-model/skill-existing-file-preservation -->

Analyze all features in the active root and write a single
`domain-model.yaml` at the project level.

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

## Steps

1. **Load schemas** — Read `.parlay/schemas/intent.schema.md`,
   `.parlay/schemas/dialog.schema.md`, `.parlay/schemas/surface.schema.md`,
   and `.parlay/schemas/domain-model.schema.md`.

2. **Scan all features** — Read `spec/intents/*/intents.md`, `dialogs.md`,
   and `surface.yaml`.

3. **Extract entities, enums, relationships, operations** — From intent
   Objects fields and implicit references in dialogs and surfaces:

   - For each entity, derive typed fields from how it is described and
     used. Field types are drawn from the closed set declared in
     `.parlay/schemas/domain-model.schema.md`
     (`{uuid, string, int, float, bool, datetime, ref, <named-enum>}`).
     Inline object literals are not allowed — lift nested shapes into a
     separate entity joined by a `ref`-typed field.
   - Identify relationships (`belongs-to`, `has-many`, references) and
     pick a cardinality from the closed set
     `{one-to-one, one-to-many, many-to-one, many-to-many}`.
   - Identify enums from lists of states declared by intents or
     dialogs. For each enum value, pick a tone from the closed set
     `{neutral, info, warning, danger, success}`. Tones are optional;
     omit when uncertain.
   - Aggregate operations across all features. Each operation declares
     `input:` as a list of `Entity.field` references and `effects:` as
     a list of free-text declarative statements.

4. **Compute the any-signal verdict (brownfield vs greenfield)** —
   After step 3 finishes the recognition pass, compute a single
   boolean `any-signal` verdict:

   - `any-signal = true` if any of (entities, relationships,
     operations) is non-empty across the feature tree.
   - `any-signal = false` otherwise.

   This is the only branch decision — there is no flag, env var, or
   separate detector. Enums alone do not flip the verdict; the
   threshold is the trio of entities/relationships/operations.

   Dispatch on the verdict:

   - **`any-signal == true` → brownfield path.** Continue to step 5
     (extract-and-write against `<activeRoot>/domain-model.yaml`).
   - **`any-signal == false` → greenfield path.** Skip ahead to
     step 6 (existing-file check, then schema-valid empty stub).

5. **Brownfield: extract and write the populated model** — When
   `any-signal == true`, write the aggregated artifact to
   `<activeRoot>/domain-model.yaml` — the canonical domain-model path
   under the active root. The file uses the shape declared
   by `.parlay/schemas/domain-model.schema.md`:

   ```yaml
   schema_version: 1
   enums: []
   entities: []
   relationships: []
   operations: []
   ```

   - **Per-feature `domain-model.md`** is **never** emitted — not as
     primary output, not as fallback, not as per-feature debug artifact.
     Existing per-feature `.md` files (from the pre-migration world)
     are not touched.
   - **Existing-file reconciliation**: when
     `<activeRoot>/domain-model.yaml` already exists, merge the
     extracted content with the file on disk using the standard
     reconciliation rules (preserve hand-authored entries that the
     extraction pass cannot reproduce; replace extractor-owned spans
     with the freshly-recognized content).
   - **Write failure**: if the YAML write fails (permissions, disk
     full), exit non-zero without writing a fallback `.md`.

   After a successful write, validate (step 7), then print the
   one-line brownfield report (step 8) and exit 0.

6. **Greenfield: existing-file preservation, then empty stub** — When
   `any-signal == false`, the greenfield branch fires. Apply these
   sub-rules in order:

   1. **Check for an existing file first.** If
      `<activeRoot>/domain-model.yaml` already exists, do **not**
      clobber it. Apply the same existing-file reconciliation path
      brownfield uses (step 5) — read the file, merge with the empty
      extracted result (a no-op against existing content), write the
      existing content back unchanged so the file is byte-equivalent
      to its prior state. Print the distinct one-line message and
      exit 0:

          domain-model.yaml present and no extractable signals — leaving existing model untouched.

      The reconciliation path used here is the SAME path brownfield
      uses for an existing file with extracted content — there is
      no separate "no-signals + existing-file" branch in the
      reconciliation logic.

   2. **No existing file: write a schema-valid empty stub.** Emit
      the canonical scaffolding at `<activeRoot>/domain-model.yaml`:

      ```yaml
      schema_version: 1
      enums: []
      entities: []
      relationships: []
      operations: []
      ```

      The stub MUST validate cleanly via
      `parlay validate --type domain-model <activeRoot>/domain-model.yaml`
      (see step 7) without modification. After a successful write,
      print exactly the stable one-liner and exit 0:

          Created empty domain-model.yaml stub at <activeRoot>/domain-model.yaml — ready to author.

      The wording "Created empty domain-model.yaml stub at {path} —
      ready to author." is **stable** — the studio-cli-hooks feature
      pattern-matches on this single line to chain its "Open
      Studio's Domain Model Editor?" prompt. This is a deliberate,
      narrow exception to the general "CI must not pattern-match
      stdout" rule stated in `build-feature` and `generate-code` —
      those skills' own output stays unstable-by-design; only this one
      greenfield-stub line is pinned. Don't generalize this exception
      to other output, and don't "fix" it to match the general rule.

   3. **Safety invariant.** A designer who hand-authored a domain
      model in Studio and then runs `parlay create-domain-model` must
      never have their work clobbered by the greenfield branch. The
      existing-file check in 6.1 holds that invariant.

   4. **TTY-agnostic.** The greenfield path produces the same output
      and exit code in CI / non-TTY runs as in interactive ones —
      this skill never inspects TTY state. Only the Studio hook (a
      separate feature, downstream of this skill) cares about TTY.

7. **Validate** — invoke:

   ```bash
   parlay validate --type domain-model --json <activeRoot>/domain-model.yaml
   ```

   If validation fails, surface the structured errors to the user and
   stop. Do not commit the YAML.

   The command exits non-zero when it found blocking findings, and 0 when
   the array is empty or holds warnings only — so the exit code and the
   array agree, and either one is a valid way to read the result. Warnings
   alone (`domain-operations-deprecated`, chiefly) are not a failure: they
   are reported and the model still commits.

8. **Report** — Print the absolute YAML path on stdout, plus the
   appropriate one-line summary on the next line:

   - **Brownfield (any-signal true)**:

         <activeRoot>/domain-model.yaml
         Wrote domain-model.yaml — N entities, M relationships, K operations.

   - **Greenfield, fresh stub (any-signal false, no existing file)**:

         <activeRoot>/domain-model.yaml
         Created empty domain-model.yaml stub at <activeRoot>/domain-model.yaml — ready to author.

   - **Greenfield, existing file preserved (any-signal false, file present)**:

         <activeRoot>/domain-model.yaml
         domain-model.yaml present and no extractable signals — leaving existing model untouched.

   No human-readable preamble.
