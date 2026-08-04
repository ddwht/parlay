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

// canonicalDecisionKinds reads the kind vocabulary out of its single
// source — the decision-protocol expansion in skills.go — rather than
// restating it here. A test with its own copy of the list is a fifth copy
// of the thing this test exists to stop people making copies of.
func canonicalDecisionKinds(t *testing.T) map[string]bool {
	t.Helper()
	const marker = "kind: phase-boundary        # "
	i := strings.Index(decisionProtocolExpansion, marker)
	if i < 0 {
		t.Fatal("could not find the kind enum in decisionProtocolExpansion — the protocol block's shape has drifted from this test")
	}
	rest := decisionProtocolExpansion[i+len(marker):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	kinds := map[string]bool{}
	for _, k := range strings.Split(rest, "|") {
		if k = strings.TrimSpace(k); k != "" {
			kinds[k] = true
		}
	}
	if len(kinds) < 2 {
		t.Fatalf("parsed %d kinds from the protocol block; the parse has drifted", len(kinds))
	}
	return kinds
}

// briefDecisionKinds extracts one agent brief's declared enum.
func briefDecisionKinds(t *testing.T, body string) map[string]bool {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "kind:") || !strings.Contains(line, "#") {
			continue
		}
		enum := line[strings.Index(line, "#")+1:]
		kinds := map[string]bool{}
		for _, k := range strings.Split(enum, "|") {
			if k = strings.TrimSpace(k); k != "" {
				kinds[k] = true
			}
		}
		return kinds
	}
	return nil
}

// The three phase-group briefs each narrow the decision-kind vocabulary to
// what their phase can actually raise, and that narrowing is deliberate — a
// designer cannot raise `overwrite` because it writes no code. What is NOT
// deliberate is a brief naming a kind the protocol does not define, which
// is what a hand-copied enum drifts into.
func TestAgentBriefKindsAreSubsetsOfTheProtocol(t *testing.T) {
	canonical := canonicalDecisionKinds(t)
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		kinds := briefDecisionKinds(t, string(a.Content))
		if len(kinds) == 0 {
			t.Errorf("%s declares no decision-kind enum", a.Name)
			continue
		}
		for k := range kinds {
			if !canonical[k] {
				t.Errorf("%s names decision kind %q, which the protocol in skills.go does not define — the hand-copied enum has drifted", a.Name, k)
			}
		}
	}
}

// impasse is the one kind every phase must be able to raise. Any of them
// can discover that the pipeline cannot express what the spec asks for,
// and a group that hits an impasse without the kind in its brief cannot
// report it — it falls back to filing the finding as prose, which is the
// dead end Part C exists to close.
func TestEveryAgentBriefCanRaiseImpasse(t *testing.T) {
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		if kinds := briefDecisionKinds(t, string(a.Content)); !kinds["impasse"] {
			t.Errorf("%s cannot raise `impasse`; every phase group must be able to", a.Name)
		}
	}
}
