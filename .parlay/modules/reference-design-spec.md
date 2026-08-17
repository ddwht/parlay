# reference-design-spec

_Extract design spec from Figma_

# Reference Design Spec

Extract visual design details from a Figma file and generate a design-spec.yaml that enriches the buildfile with per-fragment widget specifics, tokens (including motion), variants, spacing, and colors. Design-spec does not carry structural layout (component nesting, direction, gap, padding, alignment) — that's `<page>.layout.yaml`'s scope; see `design-spec.schema.md`'s "Relationship to layout.schema.md".

This is an **optional** step between surface creation and build-feature. The pipeline works without it — adapter defaults apply when no design-spec exists.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)
- `figma-link`: URL to Figma file or frame

## Prerequisites

- **Figma MCP** must be available. If not, inform the user and stop.
- **Surface** must exist at `spec/intents/{feature}/surface.md`. If not, tell the user to run `/parlay-create-artifacts @{feature}` first.

<!-- parlay:active-root-aware -->
## Active root

Every relative path below is interpreted against the **active root** — the parlay project root resolved by the CLI from cwd, the `--root` flag, or `PARLAY_ROOT`. The CLI handles resolution; this skill describes paths abstractly. Two categories matter:

- **Active-root paths** (`.parlay/build/`, `spec/intents/`, etc.) live under whichever root the CLI resolves to.
- **Repo-level-root paths** (`.parlay/schemas/`, `.parlay/adapters/`, the deployed agent surface) live only at the repo-level root. When the active root is a child, the CLI loads these from the parent automatically.

When invoking the CLI, pass `--ambiguity-as-signal` on commands that might face an ambiguous active root. If a CLI invocation exits with code 11 and emits a JSON envelope on stderr (`{"kind":"ambiguity",...}`), the root cannot be guessed — the candidates are real projects, and picking one writes into the wrong tree.

Who resolves it depends on where you are running. If you own the user interaction — the loop driver, or a skill the user invoked directly — prompt with the listed candidate roots and re-invoke with `--root <chosen>`. If you are a **phase module** running inside a subagent, you have no interactive tool: return an `ambiguity` decision request listing the candidates as options and let the driver ask.

## Steps

1. **Check Figma MCP** — Attempt to use the Figma MCP tool. If unavailable:
   - Tell the user: "Figma MCP is required for this skill. Please configure it and try again."
   - Stop. Do not proceed.

2. **Load schemas** — Read:
   - `.parlay/schemas/design-spec.schema.md`
   - `.parlay/schemas/surface.schema.md`
   - `.parlay/schemas/adapter.schema.md`

3. **Load feature surface** — Read `spec/intents/{feature}/surface.md`. Parse fragment names. If the file does not exist, stop and tell the user to create the surface first.

4. **Load adapter** — Resolve the adapter from `.parlay/adapter-set.yaml`'s presentation slot and read `.parlay/adapters/{slug}.adapter.yaml`. (`prototype-framework:` was removed in v0.3 — there is no fallback; a project without an adapter-set converts via `parlay migrate-config`.) Extract the `design-system:` section to identify which categories have `source: figma`.

5. **Read Figma via MCP** — Connect to Figma and read the file/frame at the provided link. Extract:
   - Component hierarchy and naming
   - Design tokens used (colors, typography, spacing, shadows, motion/transitions)
   - Component variants and states
   - Style references (fills, strokes, effects)
   - Auto-layout properties (direction, spacing, padding, sizes) are visible in the Figma read but are NOT written to design-spec.yaml — they're structural, and structure belongs in `<page>.layout.yaml` (see `layout.schema.md`), which this skill does not author. If the designer wants that structure captured, tell them to author a layout file separately; don't fold it into this skill's output.

6. **Map Figma components to surface fragments** — For each surface fragment, find matching Figma components. Use name similarity, structural similarity, and content similarity. Present the mapping to the user for confirmation:
   ```
   Here's my proposed mapping:
   - Task Board → Figma "TaskBoard" frame
   - Task Detail Drawer → Figma "TaskDetail" frame
   - Dashboard Metrics → (no Figma match — adapter defaults will apply)
   Does this mapping look right?
     A: Yes, proceed
     B: Let me adjust the mapping
     C: Cancel
   ```
   Handle:
   - Exact match: auto-map
   - Multiple candidates: ask user to pick
   - No match: skip (adapter defaults)
   - Figma component with no fragment: present each unmapped component to the user and ask:
     - A: Create a new fragment in surface.md for this element (provide a suggested name, Shows, Actions, Page, Region, and Order based on the Figma context — e.g., a page header becomes a fragment in the `header` region)
     - B: Skip — it's decorative or not part of this feature
     - C: Assign to an existing fragment (list candidates)
     If the user chooses A, write the new fragment to surface.md before proceeding to extraction.

7. **Extract visual details per mapped fragment** — For each mapped pair:
   - **widget**: Determine the exact framework widget variant from the Figma component structure (e.g., "Table with fixed header and bordered cells" not just "Table")
   - **tokens**: Cross-reference applied styles with the adapter's `design-system:` categories, respecting the `source` field for each category:
     - `source: figma` — record the specific Figma token values as-is (these ARE the source of truth).
     - `source: framework` — do NOT record raw Figma hex values or Figma token names. Instead, map each Figma visual property to the closest framework token or component class from the adapter's `format` and `usage` fields (e.g., map a red background to `--cds-alias-status-danger`, not `#ec221f`; map a primary-styled button to the framework's `btn btn-primary` class, not a custom `.btn-brand` class). If no close framework match exists, omit the value — the framework default will apply.
     - `source: not-defined` — record the Figma values as-is (these fill a gap the framework doesn't cover).
   - **variants**: Extract from Figma component variants/properties (loading, error, empty, hover states)
   - **spacing**: Extract padding and gap values
   - **colors**: Extract fill and stroke color references

8. **Confirm token mapping** — For `source: framework` categories, present the proposed Figma-to-framework token mapping table to the user for confirmation (same pattern as Step 6's component mapping). Group by category (colors, spacing, typography, etc.). For each mapping:
   - Clear 1:1 match → show as the proposed mapping
   - Ambiguous (multiple framework candidates) → offer lettered options (A/B/C)
   - No match → propose omitting (adapter defaults apply)
   Ask the user to confirm or adjust before proceeding.

9. **Detect shared values** — If multiple fragments use the same tokens, spacing, or colors, extract them into the `shared:` section to avoid repetition.

10. **Generate design-spec.yaml** — Write to `.parlay/build/{feature}/design-spec.yaml`:
   - If the file already exists, read it first and preserve fragments that were manually edited (compare against a stored hash or check for a `# manual` comment marker)
   - Set `feature:`, `figma-source:`, `generated:` fields
   - Write `shared:` section if common values were detected
   - Write per-fragment sections for each mapped fragment
   - Skip unmapped fragments — they use adapter defaults

11. **Report** — Tell the user:
    - How many fragments were mapped from Figma
    - How many fragments were skipped (no Figma match)
    - Which adapter design-system categories now have Figma-sourced values
    - Remind them to run `/parlay-build-feature @{feature}` next

## Error Handling

- `figma-mcp-unavailable`: Figma MCP tool is not available. Tell the user to configure Figma MCP and retry.
- `no-surface`: surface.md does not exist. Tell user to run `/parlay-create-artifacts @{feature}` first.
- `no-adapter`: adapter file not found. Tell user to run `parlay init` or `parlay register-adapter <path>`.
- `figma-read-failed`: MCP could not read the Figma file. Check the link and permissions.
- `no-fragments-mapped`: No Figma components could be mapped to surface fragments. Ask user if they want to proceed with manual annotation or cancel.

## Output Location

`.parlay/build/{feature}/design-spec.yaml` — tool-internal, same directory as buildfile.yaml and testcases.yaml. The designer never needs to see or edit this file, but it is reviewable if they choose to look.
