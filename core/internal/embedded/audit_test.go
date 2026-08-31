package embedded

import (
	"strings"
	"testing"
)

// TestGenerateCodeStrictTargetRule guards against a regression of the
// multi-root failure mode: the agent inventing a new file path under
// the source root instead of merging into the file named by a
// cross-cutting entry's Affects/target-files clause. The rule lives in
// the skill's step 14.7 prose; this test checks the prose still
// contains the load-bearing keywords.
func TestGenerateCodeStrictTargetRule(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	var generateCode string
	for _, s := range skills {
		if s.Name == "generate-code" {
			generateCode = string(s.Content)
			break
		}
	}
	if generateCode == "" {
		t.Fatal("generate-code skill not found in embedded bundle")
	}
	// Every keyword below must appear within step 14.7's strict-target
	// rule. Removing any of them weakens the contract — fail the build.
	required := []string{
		"strict-target rule",
		"Affects",
		"target-files",
		"do NOT silently invent",
		"STOP",
	}
	for _, kw := range required {
		if !strings.Contains(generateCode, kw) {
			t.Errorf("generate-code skill missing required keyword %q in step 14.7 strict-target rule", kw)
		}
	}
}

// TestSkillSourceAudit fails the build when an embedded skill file
// contains hardcoded `.parlay/...` or `spec/...` path references but is
// missing the `<!-- parlay:active-root-aware -->` marker that signals
// the file's prose has been reviewed for multi-root awareness.
//
// The marker convention: every skill that mentions parlay-managed paths
// must include the marker AND a "## Active root" section explaining
// that paths are relative to the resolver-chosen active root. Adding a
// new skill that mentions paths without the marker fails this test.
func TestSkillSourceAudit(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	// Path tokens that imply multi-root awareness is required. We match
	// the literal directory prefix; a skill that talks abstractly about
	// "the active root's spec/intents/" still contains the substring,
	// which is fine — the marker must accompany it either way.
	pathTokens := []string{
		".parlay/",
		"spec/intents/",
		"spec/handoff/",
		"spec/pages/",
	}

	for _, skill := range skills {
		body := string(skill.Content)
		hasPath := false
		for _, tok := range pathTokens {
			if strings.Contains(body, tok) {
				hasPath = true
				break
			}
		}
		if !hasPath {
			continue
		}
		if !strings.Contains(body, "parlay:active-root-aware") {
			t.Errorf("skill %s mentions parlay-managed paths but lacks the `<!-- parlay:active-root-aware -->` marker. Add an `## Active root` section explaining that paths resolve against the active root.",
				skill.Name)
		}
		if !strings.Contains(body, "## Active root") {
			t.Errorf("skill %s has the active-root marker but no `## Active root` section. Add one.",
				skill.Name)
		}
	}
}

// TestMarkerExpansion guards the deploy-time marker-expansion mechanism
// in skills.go: every skill's raw source may drop in the compact
// `<!-- parlay:expand-active-root -->` / `<!-- parlay:expand-co-equal-artifacts -->`
// placeholders, but ReadAllSkills must always hand deployers fully
// expanded prose — a marker that leaks through unexpanded (typo in the
// constant, marker added to a skill after expandMarkers stopped being
// called, etc.) would ship literal HTML-comment placeholder text to
// every deployed agent surface.
func TestMarkerExpansion(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	sawActiveRootExpansion := false
	sawCoEqualExpansion := false
	for _, s := range skills {
		body := string(s.Content)
		if strings.Contains(body, activeRootMarker) {
			t.Errorf("skill %s still contains the unexpanded %q marker after ReadAllSkills", s.Name, activeRootMarker)
		}
		if strings.Contains(body, coEqualArtifactsMarker) {
			t.Errorf("skill %s still contains the unexpanded %q marker after ReadAllSkills", s.Name, coEqualArtifactsMarker)
		}
		if strings.Contains(body, "## Active root") {
			sawActiveRootExpansion = true
		}
		if strings.Contains(body, coEqualArtifactsExpansion) {
			sawCoEqualExpansion = true
		}
	}
	if !sawActiveRootExpansion {
		t.Error("no skill exercised the active-root marker expansion — test fixture may have drifted; expected at least one skill to use `<!-- parlay:expand-active-root -->`")
	}
	if !sawCoEqualExpansion {
		t.Error("no skill exercised the co-equal-artifacts marker expansion — expected at least one skill to use `<!-- parlay:expand-co-equal-artifacts -->`")
	}
}

// TestCreateArtifactsStep2RoutingRulePhrase guards create-artifacts
// step 2's intent-classification routing rule against silent drift.
// This mention of the co-equal-artifacts doctrine is deliberately NOT
// marker-extracted (unlike the skill's intro sentence, which uses
// `<!-- parlay:expand-co-equal-artifacts -->`): step 2's sentence is
// context-woven prose about how classification routes an intent to
// infrastructure.md vs capabilities.yaml, not a restatement of the
// four-artifact definition itself, so forcing it through the shared
// marker would read awkwardly. A phrase contract is the cheaper way to
// keep it from drifting out of sync with the doctrine.
func TestCreateArtifactsStep2RoutingRulePhrase(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	var createArtifacts string
	for _, s := range skills {
		if s.Name == "create-artifacts" {
			createArtifacts = string(s.Content)
			break
		}
	}
	if createArtifacts == "" {
		t.Fatal("create-artifacts skill not found in embedded bundle")
	}

	required := []string{
		"co-equal",
		"an architectural intent flows to `infrastructure.md` directly",
		"not via `capabilities.yaml`",
	}
	for _, kw := range required {
		if !strings.Contains(createArtifacts, kw) {
			t.Errorf("create-artifacts skill missing required phrase %q in step 2's routing rule", kw)
		}
	}
}

// TestDesignerAgentMentionsFourArtifacts guards against the designer
// agent's artifacts-phase prose drifting back to the pre-four-artifact
// world (it used to say "surface.md, infrastructure.md, or both"). The
// agent must name all four co-equal spec artifact filenames so a reader
// can tell at a glance which subset the artifacts phase might produce.
func TestDesignerAgentMentionsFourArtifacts(t *testing.T) {
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatalf("ReadAllAgents: %v", err)
	}
	var designer string
	for _, a := range agents {
		if a.Name == "designer" {
			designer = string(a.Content)
			break
		}
	}
	if designer == "" {
		t.Fatal("designer agent not found in embedded bundle")
	}

	required := []string{"surface", "capabilities.yaml", "infrastructure.md", "domain-model"}
	for _, kw := range required {
		if !strings.Contains(designer, kw) {
			t.Errorf("designer agent missing reference to artifact %q — the artifacts phase must name all four co-equal spec artifacts", kw)
		}
	}
}

// TestFeatureStructureSchemaMentionsFourArtifacts guards against
// feature-structure.schema.md regressing to a two-artifact (surface.md
// + domain-model.md) description of the project layout. The doc must
// name capabilities.yaml and infrastructure.md as first-class artifacts
// alongside surface and domain-model.
func TestFeatureStructureSchemaMentionsFourArtifacts(t *testing.T) {
	content, err := schemasFS.ReadFile("schemas/feature-structure.schema.md")
	if err != nil {
		t.Fatalf("failed to read feature-structure.schema.md: %v", err)
	}
	body := string(content)

	required := []string{"capabilities.yaml", "infrastructure.md"}
	for _, kw := range required {
		if !strings.Contains(body, kw) {
			t.Errorf("feature-structure.schema.md missing reference to %q — the doc must document all four co-equal spec artifacts", kw)
		}
	}
}

// TestBuildfileSchemaDocumentsBothShapesAndWhichOneValidates guards the
// buildfile v2 rewrite: the per-component file-I/O list must be named
// file-operations: (not operations:, which collides with the top-level
// multi-target operations: block), the buildfile must declare
// schema_version, and the old single-target shape must still exist as a
// frozen reference.
//
// This test used to be named ...IsMultiTargetPrimary... and its last
// assertion said "multi-target must be primary, not an afterthought". The
// schema no longer claims that, and retracting the claim was the fix: the
// doc had said v2 was primary and "every project is described in this shape
// from here on" while the validator accepted only v1, so following the doc
// produced a buildfile the CLI refused. An agent paid to reverse-engineer
// the validator to find out. The prose was corrected; this test kept
// enforcing the superseded claim in its name and rationale while its
// assertion — v2 content precedes the frozen appendix — happened to still
// hold for an unrelated reason.
//
// What is actually worth guarding is the honesty of the status note. That is
// the sentence whose absence costs a build cycle.
func TestBuildfileSchemaDocumentsBothShapesAndWhichOneValidates(t *testing.T) {
	content, err := schemasFS.ReadFile("schemas/buildfile.schema.md")
	if err != nil {
		t.Fatalf("failed to read buildfile.schema.md: %v", err)
	}
	body := string(content)

	required := []string{
		"schema_version",
		"file-operations:",
		"Why this was renamed",
		"Appendix: Legacy v1 buildfile shape (frozen)",
	}
	for _, kw := range required {
		if !strings.Contains(body, kw) {
			t.Errorf("buildfile.schema.md missing %q", kw)
		}
	}

	// v2 content is documented in the body, ahead of the frozen appendix —
	// that is where new multi-target material lands. This is a claim about
	// where content lives, not about which shape an author should use.
	appendixIdx := strings.Index(body, "## Appendix: Legacy v1 buildfile shape (frozen)")
	targetsIdx := strings.Index(body, "targets:")
	if appendixIdx == -1 || targetsIdx == -1 || targetsIdx > appendixIdx {
		t.Error("buildfile.schema.md must document the multi-target targets: block in the body, ahead of the frozen legacy appendix")
	}

	// And the reader must be told, before that block, which shape actually
	// validates. The reconciled truth (WP2.2): for a multi-target project the
	// v2 shape IS accepted — a buildfile carrying adapter-set: plus a
	// resolvable presentation adapter validates. The doc once claimed the
	// opposite ("not what the validator accepts today" / "not yet the accepted
	// shape"), and every build that trusted it paid for it; that phrasing now
	// survives only inside the Status note's retraction of itself, so this pin
	// no longer requires the stale sentence — it forbids its return to the v2
	// structure section and requires the honest acceptance claim there instead.
	if targetsIdx >= 0 {
		structIdx := strings.Index(body, "## Structure (v2 multi-target")
		if structIdx == -1 || structIdx > targetsIdx {
			t.Error("buildfile.schema.md must carry the v2 structure heading before the targets: block")
		} else {
			structSection := body[structIdx:targetsIdx]
			if !strings.Contains(structSection, "validates") && !strings.Contains(structSection, "accepted") {
				t.Error("the v2 structure section must state that a resolvable adapter-set buildfile validates — not leave the reader unsure which shape the validator takes")
			}
			for _, stale := range []string{"not what the validator accepts today", "not yet the accepted shape"} {
				if strings.Contains(structSection, stale) {
					t.Errorf("the v2 structure section still carries the retracted claim %q — v2 is accepted for multi-target projects (see ValidateBuildfile)", stale)
				}
			}
		}
		preamble := body[:targetsIdx]
		if !strings.Contains(preamble, "appendix-legacy-v1-buildfile-shape-frozen") {
			t.Error("buildfile.schema.md must point a single-target author at the v1 shape that validates for them")
		}
	}
}

// TestSchemaVersioningHouseRuleExists guards the Phase 5 versioning
// consolidation: the house rule doc must exist and must state the
// snake_case + migrator-or-regenerate policy explicitly, so a future
// schema author has one place to learn the convention instead of
// re-deriving it per artifact.
func TestSchemaVersioningHouseRuleExists(t *testing.T) {
	content, err := schemasFS.ReadFile("schemas/schema-versioning.schema.md")
	if err != nil {
		t.Fatalf("failed to read schema-versioning.schema.md: %v", err)
	}
	body := string(content)

	required := []string{
		"snake_case",
		"Migrator chain",
		"Regenerate",
	}
	for _, kw := range required {
		if !strings.Contains(body, kw) {
			t.Errorf("schema-versioning.schema.md missing %q", kw)
		}
	}
}

// TestLayoutSchemaVersionFieldIsSnakeCase guards the one historical
// violation the versioning house rule closes: layout.schema.md's
// version field must be schema_version (snake_case), not the old
// camelCase schemaVersion, everywhere it's declared as the field name.
func TestLayoutSchemaVersionFieldIsSnakeCase(t *testing.T) {
	content, err := schemasFS.ReadFile("schemas/layout.schema.md")
	if err != nil {
		t.Fatalf("failed to read layout.schema.md: %v", err)
	}
	body := string(content)

	if !strings.Contains(body, "schema_version") {
		t.Error("layout.schema.md must declare its version field as schema_version (snake_case)")
	}
}

// TestDesignSpecScopedAwayFromLayout guards the Phase 5 design-spec
// scoping decision: design-spec.yaml must document itself as
// non-layout enrichment only, with the per-fragment structural layout:
// field removed and a cross-reference to layout.schema.md.
//
// The reverse leg — layout.schema.md carrying a heading back to
// design-spec.schema.md — was asserted here until layout.schema.md's
// Design-Loop and Figma claims were removed by governance. layout.schema.md
// now states its own scope without naming design-spec, so only the
// design-spec side of the cross-reference is checked.
func TestDesignSpecScopedAwayFromLayout(t *testing.T) {
	designSpec, err := schemasFS.ReadFile("schemas/design-spec.schema.md")
	if err != nil {
		t.Fatalf("failed to read design-spec.schema.md: %v", err)
	}
	designSpecBody := string(designSpec)

	required := []string{
		"Scope: non-layout enrichment only",
		"Relationship to layout.schema.md",
		"motion",
	}
	for _, kw := range required {
		if !strings.Contains(designSpecBody, kw) {
			t.Errorf("design-spec.schema.md missing %q", kw)
		}
	}
}

// TestCapabilitiesInputTypeNamespaceDocumented guards the 5d addition
// defining what input.type may contain and where (nowhere, structurally)
// its field shape is declared — without this section a reader has no
// way to know input.type is an unvalidated free-form name rather than a
// closed-vocabulary reference like subject.entity.
func TestCapabilitiesInputTypeNamespaceDocumented(t *testing.T) {
	content, err := schemasFS.ReadFile("schemas/capabilities.schema.md")
	if err != nil {
		t.Fatalf("failed to read capabilities.schema.md: %v", err)
	}
	body := string(content)

	required := []string{
		"The `input.type` namespace",
		"reference into a closed vocabulary",
	}
	for _, kw := range required {
		if !strings.Contains(body, kw) {
			t.Errorf("capabilities.schema.md missing %q", kw)
		}
	}
}

// TestDomainModelV2DeferredGapsDocumented guards the Phase 4-derived
// finding that list-typed scalar/enum fields and state-machine
// transitions were silently inexpressible in domain-model.yaml. Both
// must be documented as explicit v2-deferred decisions, not left as
// undocumented gaps a migrator has to rediscover.
func TestDomainModelV2DeferredGapsDocumented(t *testing.T) {
	content, err := schemasFS.ReadFile("schemas/domain-model.schema.md")
	if err != nil {
		t.Fatalf("failed to read domain-model.schema.md: %v", err)
	}
	body := string(content)

	required := []string{
		"v2-deferred: list-typed scalar/enum fields",
		"v2-deferred: state-machine constructs",
	}
	for _, kw := range required {
		if !strings.Contains(body, kw) {
			t.Errorf("domain-model.schema.md missing %q", kw)
		}
	}
}

// TestBlueprintAndCapabilitiesPoliciesDistinguished guards the mutual
// note (added to both schemas per team-lead steering) that blueprint's
// free-form authorization.policies and capabilities' closed policies:
// enum are different vocabularies that must not be conflated.
func TestBlueprintAndCapabilitiesPoliciesDistinguished(t *testing.T) {
	blueprint, err := schemasFS.ReadFile("schemas/blueprint.schema.md")
	if err != nil {
		t.Fatalf("failed to read blueprint.schema.md: %v", err)
	}
	// This one is prose rather than a heading, so the structural anchor is
	// co-occurrence: some paragraph has to discuss capabilities' policies field
	// alongside this schema's own. The exact sentence is not the contract — the
	// fact that the distinction is drawn somewhere is.
	if !hasParagraphMentioning(string(blueprint), "capabilities", "`policies:`") {
		t.Error("blueprint.schema.md has no paragraph distinguishing its authorization.policies from capabilities' policies: enum")
	}

	capabilities, err := schemasFS.ReadFile("schemas/capabilities.schema.md")
	if err != nil {
		t.Fatalf("failed to read capabilities.schema.md: %v", err)
	}
	if !hasHeadingMentioning(string(capabilities), "blueprint") {
		t.Error("capabilities.schema.md has no heading cross-referencing blueprint's authorization.policies")
	}
}

// TestVocabularyBlockDerivationTargetDocumented is gone with the schema it
// audited. It guarded the field-name equivalence table between the adapter's
// `vocabulary:` block and componentVocabulary:/tokens: — a table that existed
// because an adapter author maintained both structured vocabularies by hand and
// they could drift. There is one structured vocabulary now, so there is no
// equivalence to document and nothing to drift.

// TestBuildfileSourceSignaturesEnforcementLayerDocumented guards the
// mutual-acknowledgment clarification in buildfile.schema.md's
// Source-signatures section: source-signatures: is a skill-mechanical
// gate (build-feature/generate-code), distinct from — but related to —
// the Go-side advisory HashedSources mechanism in .baseline.yaml. Both
// must be named so nobody later "unifies" one away against the other
// without understanding both.
func TestBuildfileSourceSignaturesEnforcementLayerDocumented(t *testing.T) {
	content, err := schemasFS.ReadFile("schemas/buildfile.schema.md")
	if err != nil {
		t.Fatalf("failed to read buildfile.schema.md: %v", err)
	}
	body := string(content)

	required := []string{
		// The gate stays skill-side even though the values it compares are
		// now computed by `parlay internal scaffold-signatures`. Splitting
		// those two is the point: hashing a file is mechanical and belongs
		// in Go, while refusing to emit is a phase decision.
		"The gate itself stays skill-mechanical, not CLI",
		"scaffold-signatures",
		"HashedSources",
		".baseline.yaml",
	}
	for _, kw := range required {
		if !strings.Contains(body, kw) {
			t.Errorf("buildfile.schema.md missing %q", kw)
		}
	}
}

// hasHeadingMentioning reports whether any markdown heading in doc mentions
// needle.
//
// The schema audit tests used to assert exact heading sentences, which made
// rewording a heading a build failure with no behavioural change — the kind of
// test that trains people to leave documentation alone. Anchoring on a stable
// identifier inside the heading (a schema filename, a field name) keeps the
// structural claim while leaving the prose editable.
func hasHeadingMentioning(doc, needle string) bool {
	for _, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

// hasParagraphMentioning reports whether any single paragraph of doc mentions
// every needle. Used where the documented fact is prose rather than a section:
// co-occurrence within one paragraph is the weakest claim that still means "these
// two things are discussed together", which is what such a note is for.
func hasParagraphMentioning(doc string, needles ...string) bool {
	for _, para := range strings.Split(doc, "\n\n") {
		all := true
		for _, n := range needles {
			if !strings.Contains(para, n) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// TestSkillDescriptionsDoNotSelfPrefix keeps the "Parlay: " prefix owned by
// exactly one layer.
//
// Every deployer templates `description: "Parlay: %s"` (claude.go:53,
// cursor.go:45), so a source description that carries the prefix itself
// deploys as "Parlay: Parlay: …". Two skills drifted into that — one of them
// shipped that way — and nothing noticed, because the doubling is only
// visible in the deployed file and only in a menu nobody diffs.
//
// The rule is one line of frontmatter, which is exactly the kind of
// convention that survives only if something checks it.
func TestSkillDescriptionsDoNotSelfPrefix(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("no skills read — the loader has drifted from this test")
	}
	for _, s := range skills {
		if strings.HasPrefix(s.Description, "Parlay: ") {
			t.Errorf("skill %s describes itself as %q — the deployers add the "+
				"\"Parlay: \" prefix, so this deploys doubled. Drop it from the frontmatter.",
				s.Name, s.Description)
		}
	}
}
