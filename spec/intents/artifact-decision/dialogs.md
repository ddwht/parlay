# Artifact Decision — Dialogs

---

### Determine Required Artifacts from Intents and Dialogs

**Trigger**: /parlay-create-artifacts @==feature== (after dialogs are authored)

User: /parlay-create-artifacts @initiatives
System (background): Reads intents.md and dialogs.md for @initiatives.
System (background): Analyzes intents: Persona is UX Designer on all three intents. Objects reference user-facing concepts (initiative, feature, project). Dialog System turns show CLI output ([OK] messages, created paths, next-step hints). No intents reference specific code constructs like function names or file paths.
System: This feature needs **surface.md** — the intents describe user-facing command output (initiative creation, feature scaffolding, orientation).
System: Proceeding to generate surface.md.
  A: Yes, proceed
  B: Also add infrastructure.md (this feature has behind-the-scenes changes too)
  C: Switch to infrastructure.md only
User: A
System (background): Generates surface.md for @initiatives.
System: Generated 3 surface fragments in surface.md.

#### Branch: Pure infrastructure feature

User: /parlay-create-artifacts @qualified-identifier-resolver
System (background): Reads intents.md and dialogs.md for @qualified-identifier-resolver.
System (background): Analyzes intents: Persona is Parlay Developer on all three intents. Objects reference code constructs (config, resolver, validator, schema). Constraints name specific functions (FeaturePath, BuildPath, AllFeatures). Dialog System turns describe code modifications and detection patterns.
System: This feature needs **infrastructure.md** — the intents describe behind-the-scenes code changes (resolver updates, bulk-walker modifications, schema extension). No user-facing output.
System: You'll author infrastructure.md directly with one fragment per discrete change.
  A: Yes, proceed — show me the infrastructure.md format
  B: Also add surface.md (this feature has user-facing output too)
  C: Switch to surface.md only
User: A
System: Each infrastructure fragment describes a behavioral capability in framework-agnostic terms:
System: ```
System: ## Fragment Name
System: **Affects**: what area of the system this touches (e.g., "feature resolution")
System: **Behavior**: what the capability does, in domain terms
System: **Invariants**:
System: - testable property that must hold
System: **Source**: @feature/intent-slug
System: ```
System: Required fields: Fragment Name, Affects, Behavior, Source. Optional: Invariants, Caching, Backward-Compatible, Notes. See the infrastructure schema for the full field list.

#### Branch: Mixed feature

User: /parlay-create-artifacts @some-mixed-feature
System (background): Reads intents.md and dialogs.md for @some-mixed-feature.
System (background): Analyzes intents: two intents have Persona: UX Designer with dialog turns showing CLI output. One intent has Persona: Parlay Developer with constraints referencing config.FeaturePath and an os.ReadDir detection pattern.
System: This feature needs **both** — surface.md for the user-facing command output (2 intents) and infrastructure.md for the behind-the-scenes resolver change (1 intent).
System: I'll generate surface.md first, then you'll author infrastructure.md.
  A: Yes, proceed with both
  B: Surface only — I'll handle the infrastructure change manually
  C: Infrastructure only — the user-facing part is handled elsewhere
User: A
System (background): Generates surface.md for the 2 user-facing intents.
System: Generated 2 surface fragments. Now author infrastructure.md for the resolver change.

#### Branch: Ambiguous — agent can't decide

User: /parlay-create-artifacts @ambiguous-feature
System (background): Reads intents.md and dialogs.md. Intents have Persona: UX Designer but Constraints reference internal function names. Dialogs show both CLI output and code modification steps.
System: I'm not sure whether this feature needs surface.md, infrastructure.md, or both. The intents describe user-facing output but also reference internal code constructs.
  A: Surface only — the code references are just context, not changes this feature makes
  B: Infrastructure only — the CLI output is incidental, the real work is code changes
  C: Both — this feature has user-facing output AND behind-the-scenes changes
  D: Let me explain what this feature does
User: C
System: Got it — proceeding with both. Generating surface.md first, then you'll author infrastructure.md.

---
