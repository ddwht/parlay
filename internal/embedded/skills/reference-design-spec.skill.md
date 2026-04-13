# Reference Design Spec

Extract visual design details from a Figma file and generate a design-spec.yaml that enriches the buildfile with per-fragment widget specifics, layout, tokens, variants, spacing, and colors.

This is an **optional** step between surface creation and build-feature. The pipeline works without it — adapter defaults apply when no design-spec exists.

## Arguments

- `feature`: The feature slug (e.g., `upgrade-plan-creation`)
- `figma-link`: URL to Figma file or frame

## Prerequisites

- **Figma MCP** must be available. If not, inform the user and stop.
- **Surface** must exist at `spec/intents/{feature}/surface.md`. If not, tell the user to run `/parlay-create-surface @{feature}` first.

## Steps

1. **Check Figma MCP** — Attempt to use the Figma MCP tool. If unavailable:
   - Tell the user: "Figma MCP is required for this skill. Please configure it and try again."
   - Stop. Do not proceed.

2. **Load schemas** — Read:
   - `.parlay/schemas/design-spec.schema.md`
   - `.parlay/schemas/surface.schema.md`
   - `.parlay/schemas/adapter.schema.md`

3. **Load feature surface** — Read `spec/intents/{feature}/surface.md`. Parse fragment names. If the file does not exist, stop and tell the user to create the surface first.

4. **Load adapter** — Read `.parlay/config.yaml` to determine the prototype framework. Read `.parlay/adapters/{framework}.adapter.yaml`. Extract the `design-system:` section to identify which categories have `source: figma`.

5. **Read Figma via MCP** — Connect to Figma and read the file/frame at the provided link. Extract:
   - Component hierarchy and naming
   - Design tokens used (colors, typography, spacing, shadows)
   - Layout properties (auto-layout direction, spacing, padding, sizes)
   - Component variants and states
   - Style references (fills, strokes, effects)

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
   - Figma component with no fragment: skip (note in report)

7. **Extract visual details per mapped fragment** — For each mapped pair:
   - **widget**: Determine the exact framework widget variant from the Figma component structure (e.g., "Table with fixed header and bordered cells" not just "Table")
   - **layout**: Extract from auto-layout properties (direction, spacing, padding, alignment, item sizes)
   - **tokens**: Cross-reference applied styles with the adapter's `design-system:` categories. For categories with `source: figma`, record the specific values.
   - **variants**: Extract from Figma component variants/properties (loading, error, empty, hover states)
   - **spacing**: Extract padding and gap values
   - **colors**: Extract fill and stroke color references

8. **Detect shared values** — If multiple fragments use the same tokens, spacing, or colors, extract them into the `shared:` section to avoid repetition.

9. **Generate design-spec.yaml** — Write to `.parlay/build/{feature}/design-spec.yaml`:
   - If the file already exists, read it first and preserve fragments that were manually edited (compare against a stored hash or check for a `# manual` comment marker)
   - Set `feature:`, `figma-source:`, `generated:` fields
   - Write `shared:` section if common values were detected
   - Write per-fragment sections for each mapped fragment
   - Skip unmapped fragments — they use adapter defaults

10. **Report** — Tell the user:
    - How many fragments were mapped from Figma
    - How many fragments were skipped (no Figma match)
    - Which adapter design-system categories now have Figma-sourced values
    - Remind them to run `/parlay-build-feature @{feature}` next

## Error Handling

- `figma-mcp-unavailable`: Figma MCP tool is not available. Tell the user to configure Figma MCP and retry.
- `no-surface`: surface.md does not exist. Tell user to run `/parlay-create-surface @{feature}` first.
- `no-adapter`: adapter file not found. Tell user to run `parlay init` or `/parlay-register-adapter`.
- `figma-read-failed`: MCP could not read the Figma file. Check the link and permissions.
- `no-fragments-mapped`: No Figma components could be mapped to surface fragments. Ask user if they want to proceed with manual annotation or cancel.

## Output Location

`.parlay/build/{feature}/design-spec.yaml` — tool-internal, same directory as buildfile.yaml and testcases.yaml. The designer never needs to see or edit this file, but it is reviewable if they choose to look.
