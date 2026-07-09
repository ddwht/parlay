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

// TestBuildfileSchemaIsMultiTargetPrimaryWithFileOperations guards the
// Phase 5 buildfile v2 rewrite: multi-target must be the primary
// documented shape (not a bolted-on "Section: Multi-target" appendix),
// the per-component file-I/O list must be named file-operations: (not
// operations:, which now collides with the top-level multi-target
// operations: block), the buildfile must declare schema_version, and
// the old single-target shape must still exist somewhere as a frozen
// historical reference.
func TestBuildfileSchemaIsMultiTargetPrimaryWithFileOperations(t *testing.T) {
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

	// The primary Structure block (before the Appendix) must lead with
	// the multi-target shape, not bury it in a bolted-on section deep in
	// the file. Assert the multi-target `targets:` block appears before
	// the frozen appendix begins.
	appendixIdx := strings.Index(body, "## Appendix: Legacy v1 buildfile shape (frozen)")
	targetsIdx := strings.Index(body, "targets:")
	if appendixIdx == -1 || targetsIdx == -1 || targetsIdx > appendixIdx {
		t.Error("buildfile.schema.md's primary Structure block must declare targets: before the frozen legacy appendix — multi-target must be primary, not an afterthought")
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
// field removed and cross-references to layout.schema.md in both
// directions.
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

	layout, err := schemasFS.ReadFile("schemas/layout.schema.md")
	if err != nil {
		t.Fatalf("failed to read layout.schema.md: %v", err)
	}
	if !strings.Contains(string(layout), "Relationship to design-spec.schema.md") {
		t.Error("layout.schema.md missing the reciprocal cross-reference to design-spec.schema.md")
	}
}
