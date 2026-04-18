# Artifact Generation

> Determining and creating the right design artifacts (surface.md, infrastructure.md, or both), enriching surfaces with Figma design specs, and managing page layout composition.

---

## Determine Required Artifacts from Intents and Dialogs

**Goal**: Automatically decide whether a feature needs `surface.md` (visible output), `infrastructure.md` (internal code changes), or both, so the designer doesn't have to declare artifact types manually and the pipeline always produces the right artifacts for the feature's shape.
**Persona**: Parlay Developer
**Priority**: P1
**Context**: After dialogs are done, the pipeline needs to produce either `surface.md`, `infrastructure.md`, or both before build-feature can run. Today the designer must know which artifact to create — and understanding the distinction between surface and infrastructure shouldn't be the designer's problem. The agent already has enough signal from the intents and dialogs to make this call: intents that describe command output, prompts, and status messages imply a surface; intents that describe behavioral capabilities affecting existing code imply infrastructure; features with both kinds of intents need both artifacts.
**Action**: Add a decision step to the pipeline between dialogs and artifact creation. The agent analyzes the feature's intents and dialogs, optionally scans the codebase for brownfield context, and determines the artifact set. Then it proceeds: generating `surface.md` for user-facing fragments, and guiding `infrastructure.md` authoring for behind-the-scenes fragments. For features that need both, it produces both in sequence.
**Objects**: pipeline, artifact-decision, surface, infrastructure, intent, dialog

**Constraints**:
- The decision is based on signals already in the intents and dialogs — no new metadata or declarations required from the designer
- The agent's decision is presented to the designer for confirmation before proceeding — not silently applied. The designer can override.
- If the agent can't determine the artifact type (ambiguous intents), it asks the designer
- `/parlay-create-artifacts @feature` is the single entry point for artifact creation. It handles surface generation, infrastructure authoring guidance, or both — the designer never needs to know the distinction up front.

**Verify**:
- For a feature with intents describing visible output → agent decides "surface"
- For a feature with intents describing internal code changes → agent decides "infrastructure"
- For a mixed feature → agent decides "both"
- The agent presents its decision and the designer can override
- After the decision, the pipeline proceeds to create the appropriate artifacts

---

## Reference Design Spec from Figma

**Goal**: Enrich an existing surface with per-fragment visual design details extracted from a Figma file, producing a design-spec.yaml that captures widget specifics, layout, tokens, variants, spacing, and colors that the surface deliberately omits.
**Persona**: UX Designer
**Priority**: P2
**Context**: The surface is authored and reviewed, a Figma design exists for the feature, and the team wants higher visual fidelity in the generated prototype than adapter defaults provide.
**Action**: AI agent connects to Figma via MCP, maps Figma components to existing surface fragments, and generates a design-spec.yaml with per-fragment visual annotations.
**Objects**: design-spec, surface, fragment, figma-design, design-token, feature

**Constraints**:
- The surface must already exist — this skill enriches, it does not create the surface
- Fragment names in design-spec.yaml must match fragment names in surface.md exactly
- The design-spec is optional — the pipeline must work without it
- Requires Figma MCP — if unavailable, the skill must inform the user and stop
- The design-spec references the adapter's design-system categories — token category names must match
- The design-spec is a tool internal at `.parlay/build/<feature>/design-spec.yaml`
- Build-feature reads the design-spec IF it exists; if not, adapter defaults apply unchanged

**Verify**:
- design-spec.yaml is generated at `.parlay/build/<feature>/design-spec.yaml`
- Every fragment key in design-spec.yaml matches a fragment name in surface.md
- Token category references match categories declared in the adapter's design-system section
- build-feature produces a richer buildfile when design-spec.yaml exists
- The pipeline works identically when design-spec.yaml does not exist

---

## View Assembled Page

**Goal**: See the full layout of a page by assembling all feature fragments that target it, so the designer can review the cross-feature experience.
**Persona**: UX Designer
**Priority**: P1
**Context**: Multiple features target the same page — the designer wants to see what the assembled screen looks like before locking or prototyping.
**Action**: Tool collects all fragments targeting the page from all feature surfaces, groups by region, sorts by order, and presents the assembled view.
**Objects**: page, fragment, surface, region

**Constraints**:
- Must show fragments from all features, not just the current one
- Must flag conflicts — fragments targeting the same region with the same order
- The assembled view is read-only — changes are made in individual feature surfaces

**Verify**:
- Fragments from multiple features targeting the same page are assembled together
- Fragments are grouped by region and sorted by order within each region
- Conflicting fragments (same region + same order) are flagged with a warning
- The output is read-only — no modifications to source surfaces

---

## Lock Page Layout

**Goal**: Create a page manifest that freezes the arrangement of fragments on a page, giving the layout an explicit owner and a reviewable document.
**Persona**: UX Designer
**Priority**: P2
**Context**: The assembled page view looks right, or the team needs to agree on a layout before handoff — the designer wants to lock it down.
**Action**: Tool generates a page manifest from the current assembled view, the designer reviews and adjusts, then sets the status.
**Objects**: page, page-manifest, fragment, region

**Constraints**:
- The manifest is generated from the current assembled state — not written from scratch
- The designer must review before the manifest is considered active
- A locked manifest must warn if features add or remove fragments targeting that page
- Must not block features from being prototyped in isolation

**Verify**:
- Page manifest file is created at `spec/pages/{page-name}.page.md`
- Manifest lists all fragments in their current region and order
- Warnings are emitted when fragments are added or removed after locking
- Features can still be prototyped independently even when the page is locked

---
