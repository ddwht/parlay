# Artifact Generation — Dialogs

---

### Determine Required Artifacts

**Trigger**: /parlay-create-artifacts @==feature== (after dialogs are authored)

User: /parlay-create-artifacts @initiatives
System (background): Reads intents.md and dialogs.md for @initiatives.
System (background): Analyzes intents for surface and infrastructure signals.
System: This feature needs **surface.md** — the intents describe visible command output.
System: Proceeding to generate surface.md.
  A: Yes, proceed
  B: Also add infrastructure.md
  C: Switch to infrastructure.md only
User: A
System (background): Generates surface.md for @initiatives.
System: Generated 3 surface fragments in surface.md.

#### Branch: Pure infrastructure feature

User: /parlay-create-artifacts @qualified-identifier-resolver
System (background): Analyzes intents — all describe internal code changes with no visible output.
System: This feature needs **infrastructure.md** — the intents describe behind-the-scenes code changes. No visible output.
System: You'll author infrastructure.md directly with one fragment per discrete capability.
  A: Yes, proceed — show me the infrastructure.md format
  B: Also add surface.md
  C: Switch to surface.md only
User: A
System: Each infrastructure fragment describes a behavioral capability in framework-agnostic terms.

#### Branch: Mixed feature

User: /parlay-create-artifacts @some-mixed-feature
System (background): Analyzes intents — some describe visible output, some describe internal changes.
System: This feature needs **both** — surface.md for the visible output and infrastructure.md for the internal changes.
  A: Yes, proceed with both
  B: Surface only
  C: Infrastructure only
User: A
System (background): Generates surface.md first, then guides infrastructure.md authoring.

---

### Reference Design Spec from Figma

**Trigger**: /parlay-reference-design-spec @feature-name ==figma-link==

User: /parlay-reference-design-spec @upgrade-plan-creation ==link to Figma file==
System (background): Checks for Figma MCP availability.
System (condition: no surface): I can't create a design spec yet — surface.md doesn't exist for @upgrade-plan-creation. Run /parlay-create-artifacts @upgrade-plan-creation first.
System (background): Connects to Figma via MCP, reads the design file.
System: I found ==number== components in the Figma file. Let me map them to your ==number== surface fragments.
System: Here's my proposed mapping:
System: - ==Fragment A== → ==Figma Component X==
System: - ==Fragment B== → ==Figma Component Y==
System: Does this mapping look right?
  A: Yes, proceed
  B: Let me adjust the mapping
  C: Cancel
User: Selects A
System (background): Extracts visual details and generates design-spec.yaml.
System: Design spec generated. Run /parlay-build-feature to produce a richer buildfile.

---

### View Assembled Page

**Trigger**: /parlay-view-page ==page-name==

User: /parlay-view-page dashboard
System (background): Collects all fragments targeting "dashboard" from all feature surfaces.
System: Here's the assembled dashboard view:
System: **header**:
System: 1. @fleet-overview/health-summary
System: **main**:
System: 1. @fleet-overview/cluster-table
System: 2. @upgrade-plan/readiness-card
System (condition: conflicts exist): I found ==number== conflicts:
System: - ==Fragment A== and ==Fragment B== both target ==region== with the same order.
System (condition: unplaced fragments exist): ==number== fragments have no page target yet.

---

### Lock Page Layout

**Trigger**: /parlay-lock-page ==page-name==

User: /parlay-lock-page dashboard
System (background): Assembles the current page view for "dashboard".
System: Here's the layout I'll lock. Who should own this page?
User: Platform UX Team
System (background): Creates page manifest with Owner and Status: draft.
System: The page manifest is ready at spec/pages/dashboard.page.md.

---
