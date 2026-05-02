// parlay-feature: parlay-tool/parlay-loop
// parlay-component: SubagentDefinitionBundle
// parlay-artifact: test
package embedded

import (
	"strings"
	"testing"
)

func TestReadAllAgents_ReturnsThreePhaseGroupAgents(t *testing.T) {
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatalf("ReadAllAgents failed: %v", err)
	}

	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}

	wantNames := map[string]bool{"designer": false, "build": false, "code": false}
	for _, a := range agents {
		if _, ok := wantNames[a.Name]; !ok {
			t.Errorf("unexpected agent name: %q", a.Name)
			continue
		}
		wantNames[a.Name] = true

		if len(a.Content) == 0 {
			t.Errorf("agent %q has empty content", a.Name)
		}
		// Each agent must declare its own name in frontmatter.
		if !strings.Contains(string(a.Content), "name: parlay-"+a.Name) {
			t.Errorf("agent %q missing expected frontmatter name: parlay-%s", a.Name, a.Name)
		}
	}

	for name, seen := range wantNames {
		if !seen {
			t.Errorf("expected agent %q was not returned", name)
		}
	}
}

func TestReadAllSkills_IncludesLoopSkill(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills failed: %v", err)
	}

	var loopSkill *SkillEntry
	for i := range skills {
		if skills[i].Name == "loop" {
			loopSkill = &skills[i]
			break
		}
	}
	if loopSkill == nil {
		t.Fatal("loop skill not found in embedded skills bundle")
	}

	body := string(loopSkill.Content)
	wantRefs := []string{"parlay-designer", "parlay-build", "parlay-code"}
	for _, ref := range wantRefs {
		if !strings.Contains(body, ref) {
			t.Errorf("loop.skill.md missing reference to %q", ref)
		}
	}
}
