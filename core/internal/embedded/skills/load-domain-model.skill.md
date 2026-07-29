---
name: load-domain-model
description: "Load and integrate external domain model"
surface: module
---

# Load Domain Model

<!-- parlay-feature: studio-support/domain-model-yaml-migration -->
<!-- parlay-component: load-domain-model-conflict-prompt -->
<!-- parlay-extends: studio-support/domain-model-yaml-migration/load-domain-model-version-notice -->
<!-- parlay-extends: studio-support/domain-model-yaml-migration/load-domain-model-yaml-only-and-url -->

Load an external domain model and integrate it with the active root's
domain model.

<!-- parlay:active-root-aware -->
<!-- parlay:expand-active-root -->

## Arguments

- `path-or-url` (required) — A local file path **or** an HTTP(S) URL.
  Markdown sources (path with `.md` extension or URL whose body is
  markdown) are refused with the actionable error
  `load accepts YAML only; run \`parlay migrate-domain-model\` in the source project first`.

## Steps

1. **Validate the input form** — If `path-or-url` ends with `.md`,
   refuse with `markdown-input-refused`. The migration path goes
   through `parlay migrate-domain-model`, which is a separate command.

2. **Fetch / read the source**:

   - **Local path**: read the file directly from disk. If the file is
     missing, surface the standard not-found error and exit non-zero.
   - **HTTP(S) URL**: fetch via the agent's WebFetch / Bash tooling.
     HTTPS certificate verification is honored by default; bypass flags
     are out of scope for this feature. On non-200 responses or non-YAML
     bodies, exit non-zero with the offending response surfaced —
     `URL fetch failed: <status> from <url>`.

3. **Validate as YAML** — invoke:

   ```bash
   parlay validate --type domain-model --json /tmp/incoming-domain-model.yaml
   ```

   On failure, surface the structured errors and stop.

4. **Schema-version dispatch** — read the incoming YAML's
   `schema_version` and route through the migrator chain:

   - **Equal** to the running Core's expected version: proceed
     directly.
   - **Older**: route the in-memory model through the per-version
     migrator chain (e.g., `v1→v2`). Print a single line on stderr:

     ```
     migrating loaded model from v<source-version> to v<target-version>
     ```

     The on-disk source is **not** modified — only the in-memory model
     is migrated.
   - **Newer**: refuse with
     `[ERR] schema_version <source-version> is newer than this Core release supports (<target-version>); run parlay upgrade`,
     exit non-zero.

5. **Read the local model** — load the active root's `domain-model.yaml`
   via the CLI's standard domain-model loader. If the local model is absent,
   start with an empty-but-valid model (`schema_version: 1`, empty
   lists) and proceed to merge.

6. **Compare entities** — For each entity in the incoming model:

   - **Not present locally** → add silently.
   - **Present and structurally identical** → merge silently.
   - **Present but fields differ** → conflict; pause and present the
     designer with a side-by-side and four options:

     ```
     Conflict on entity <name>:

     Incoming                    Local
     --------                    -----
     <incoming fields>           <local fields>

     A: Keep local
     B: Take incoming
     C: Merge field-by-field — walk each differing field with the same option set
     D: Rename one — the user types a new name for the incoming entity
     ```

   Use AskUserQuestion to collect the choice for each conflict.

7. **Field-by-field merge (option C)** — for each differing field on
   the conflicting entity, present the same A/B/C/D scoped to that
   field. Walk the differences sequentially.

8. **Validate the merged model** — before writing, run the deep
   validator on the merged in-memory artifact:

   ```bash
   parlay validate --type domain-model --json /tmp/merged-domain-model.yaml
   ```

   A merge that would leave the local YAML in an invalid state (broken
   references introduced by partial merge) is **rejected as a whole** —
   partial writes are not committed.

9. **Write merged model** — atomically write the merged YAML to
   `<activeRoot>/domain-model.yaml`.

10. **Report** — Confirm integration and summarize what changed:

    ```
    merged into <activeRoot>/domain-model.yaml
    +N entities, +M enums, +K relationships, +J operations
    ```

## Stdout / stderr discipline

Success messages and the merged YAML path go to **stdout**. The
schema-version migration notice, conflict prompts, and errors go to
**stderr**. Never mixed. `migrate-domain-model` follows the same
stdout/stderr split — see its own Stdout / stderr discipline section.
