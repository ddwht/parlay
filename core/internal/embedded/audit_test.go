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
