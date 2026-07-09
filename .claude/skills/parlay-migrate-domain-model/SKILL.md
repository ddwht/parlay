---
name: parlay-migrate-domain-model
description: "Parlay: Convert domain-model.md to domain-model.yaml"
---

# Migrate Domain Model

Convert a project's legacy `domain-model.md` to `domain-model.yaml` at
the active root, prepend a one-line deprecation header to the original
markdown, and exit cleanly. One-shot port; on subsequent runs, the
command refuses to overwrite without `--force`.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** —
the parlay project root resolved by the CLI from cwd, the `--root` flag,
or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths
abstractly.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that
might face an ambiguous active root. If a CLI invocation exits with code
11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`),
re-prompt the user via AskUserQuestion with the listed candidate roots,
then re-invoke with `--root <chosen>`.

## Arguments

None. Flags:

- `--dry-run` — print the planned YAML and a unified diff against any
  existing artifact; touch nothing on disk.
- `--force` — re-parse the markdown and overwrite an existing
  `domain-model.yaml`. Prints a single warning line to stderr before
  overwriting. Non-interactive (no Y/N prompt — parlay commands are
  non-interactive).
- `--root <name>` — multi-root targeting (inherited).
- `--ambiguity-as-signal` — emit a structured ambiguity envelope on
  stderr when the active root is ambiguous (inherited).

## Inputs (and the strict isolation rule)

This skill reads ONLY from these locations:

- `<activeRoot>/domain-model.md` — the legacy source.
- `<activeRoot>/domain-model.yaml` — the idempotency guard target.
- `.parlay/schemas/domain-model.schema.md` — for the YAML shape.
- `.parlay/config.yaml` — for the active root.

The skill does **not** read `spec/intents/**`. The markdown form is the
source of truth for this one-shot migration; feature intents are
handled by `parlay create-domain-model`.

## Steps

1. **Resolve the active root** — invoke the CLI:

   ```bash
   parlay status --root <name> --ambiguity-as-signal
   ```

   Capture the active root path. On exit code 11, surface the ambiguity
   envelope to the user and re-prompt for `--root`.

2. **Detect the layout** — list the active root:

   - If neither `domain-model.md` nor `domain-model.yaml` exists, print
     `nothing to migrate` on stdout, exit 0. This is a no-op safe to
     run as part of an unconditional upgrade script.
   - If `domain-model.yaml` exists and `--force` is **not** set, print
     `[ERR] already migrated: <yaml-path>` on stderr, exit non-zero.
     Do not modify the YAML, do not re-parse the .md.
   - If `domain-model.yaml` exists and `--force` **is** set, print
     `[WARN] --force will overwrite existing domain-model.yaml; any
     hand edits to the YAML will be discarded` on stderr, then proceed.
   - Otherwise (only `.md` exists), proceed.

3. **Parse the markdown** — read `domain-model.md` and translate its
   sections into the YAML shape declared in `domain-model.schema.md`:

   - Headings name entities. Bulleted property lists become `fields:`.
   - Cross-references (`Order has many Items`) become relationships.
   - Lists of states become enums.
   - Operations described in dialogs become `operations:` entries.
   - Set `schema_version: 1` at the top.

   **Ambiguity handling**: if the markdown declares a field without a
   declared type, a relationship without a clear cardinality, or an
   enum value without a tone, emit the field with an annotation marker
   (`<unresolved: original prose said 'the date'>`) and continue.
   Designer must resolve every annotation before downstream commands
   accept the model.

4. **Validate the planned YAML** — invoke:

   ```bash
   parlay validate --type domain-model --json /tmp/planned-domain-model.yaml
   ```

   If validation fails (other than the deliberate annotation markers),
   surface the errors to the user and stop.

5. **`--dry-run` branch** — if `--dry-run` is set:

   - Print the planned YAML on stdout.
   - Print a unified diff against the existing `domain-model.yaml` (or
     an empty file if none exists).
   - Exit code mirrors what a real run would do (zero on a clean
     migration; non-zero on ambiguity).
   - Do **not** write the YAML, do **not** prepend the deprecation
     header to the .md, do **not** create temp files.

6. **Default / `--force` branch** — write the YAML:

   - Write `<activeRoot>/domain-model.yaml` with the parsed content.
   - Prepend the deprecation header to `domain-model.md`. The header is:

     ```markdown
     > **Deprecated** — see [`./domain-model.yaml`](./domain-model.yaml).
     > Edits to this markdown have no effect post-migration; the YAML is the live source.
     ```

     Prepending the header is idempotent — a second `--force` run that
     already sees the header leaves it in place.

7. **Report** —

   - On success: print the absolute YAML path on stdout, exit 0.
   - On ambiguous markdown: print `[ERR] ambiguous markdown — see
     annotation markers in <yaml-path>` on stderr, exit non-zero.
   - On already-migrated (without `--force`): exit non-zero with the
     `[ERR] already migrated: <yaml-path>` message.
   - On greenfield (no `.md`, no `.yaml`): print `nothing to migrate`,
     exit 0.

## Stdout / stderr discipline

Success messages and the dry-run diff go to **stdout**. Errors,
warnings, and the ambiguity envelope go to **stderr**. Never mixed.

## Error handling

- `markdown-input-refused` — only `parlay load-domain-model` emits this;
  `migrate-domain-model` is the markdown-accepting path.
- `already-migrated` — the YAML already exists and `--force` was not
  passed.
- `ambiguous-markdown` — the markdown had at least one unresolved
  field/relationship/enum value.
- `multi-root-ambiguous` — exits with code 11 plus the standard
  ambiguity JSON envelope on stderr; no partial migration is performed.
