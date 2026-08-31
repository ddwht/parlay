package embedded

import (
	"strings"
	"testing"
)

// The output-less blessing path is reachable only if the deployed workflow
// drives it. A CLI flag nobody's guidance names is a capability that does not
// exist in practice: the spec-only feature still reaches an empty manifest,
// still enters no emitted set, and its amendment still stays pending forever.
//
// These pin the workflow, not the prose. Each asserts a property that, if it
// broke, would silently restore the original deadlock.

func refineSkillBody(t *testing.T) string {
	t.Helper()
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if strings.Contains(s.Name, "refine") {
			return string(s.Content)
		}
	}
	t.Fatal("refine skill not found")
	return ""
}

func TestRefineNamesTheOutputlessPath(t *testing.T) {
	body := refineSkillBody(t)
	for _, want := range []string{"--outputless-feature", "--confirm-outputless"} {
		if !strings.Contains(body, want) {
			t.Errorf("refine guidance never names %s, so the path is unreachable and a "+
				"spec-only feature's amendment stays pending forever", want)
		}
	}
}

// The confirmation is a human answer, so the guidance must actually ask.
func TestRefineAsksBeforeConfirmingOutputless(t *testing.T) {
	body := refineSkillBody(t)
	idx := strings.Index(body, "--confirm-outputless")
	if idx < 0 {
		t.Fatal("no output-less guidance")
	}
	before := body[:idx]
	if !strings.Contains(before, "ask the user") {
		t.Error("the guidance passes --confirm-outputless without instructing the agent to ask " +
			"the user first — a confirmation nobody was asked for is not a confirmation")
	}
	if !strings.Contains(before, "owes no generated code") {
		t.Error("the guidance must state the precise question, not a paraphrase: the user is " +
			"being asked whether the feature owes generated code, and nothing else")
	}
}

// The path must never be described as applying to governance, which is the
// one confusion that would turn it back into a bypass.
func TestRefineSaysOutputlessIsNotGovernanceConfirmation(t *testing.T) {
	body := refineSkillBody(t)
	idx := strings.Index(body, "--confirm-outputless")
	if idx < 0 {
		t.Fatal("no output-less guidance")
	}
	rest := body[idx:]
	if !strings.Contains(rest, "apply-governance") {
		t.Error("the guidance must say governance still goes through apply-governance, or a " +
			"reader will take the output-less confirmation for a general one")
	}
}

// The ordinary path must stay ordinary: the plain re-baseline command must not
// carry the confirmation flag.
func TestRefineOrdinaryRebaselineDoesNotPassConfirmation(t *testing.T) {
	body := refineSkillBody(t)
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "save-build-state") {
			continue
		}
		if strings.Contains(line, "--confirm-outputless") && !strings.Contains(line, "--outputless-feature") {
			t.Errorf("a save-build-state invocation carries --confirm-outputless with no named "+
				"subject: %q", strings.TrimSpace(line))
		}
	}
}

// The two evidence cases rest on different facts and must be asked
// differently. "Its buildfile plans none" is false about a feature that has no
// buildfile, and a confirmation obtained by telling the user something untrue
// is not a confirmation. The CLI deliberately permits the unknown-inventory
// case with explicit human assertion, so the guidance must ask for exactly
// that rather than paraphrasing absence as a zero plan.
func TestRefineAsksTheRightOutputlessQuestionForEachCase(t *testing.T) {
	body := refineSkillBody(t)
	if !strings.Contains(body, "its buildfile plans none") {
		t.Error("the readable-empty-plan question is missing")
	}
	if !strings.Contains(body, "no buildfile plan") || !strings.Contains(body, "nothing mechanical") {
		t.Error("the unknown-inventory question is missing: a feature with no buildfile has no " +
			"mechanical inventory, and the user must be told that is the basis they are " +
			"asserting on")
	}
	if !strings.Contains(body, "Never ask the first question when there is no plan") {
		t.Error("the guidance must forbid asking the plan-based question when no plan exists")
	}
}

// A ceremony no workflow names is a capability that does not exist in practice.
// Same rule that caught the output-less path shipping as flags nobody invoked.
func TestRefineNamesTheEvolveCeremony(t *testing.T) {
	body := refineSkillBody(t)
	if !strings.Contains(body, "amends_intents") {
		t.Fatal("refine guidance never mentions the evolution vocabulary")
	}
	idx := strings.Index(body, "CHANGES a founding promise without ending it")
	if idx < 0 {
		t.Fatal("refine has no guidance for a transition that keeps the lineage alive")
	}
	section := body[idx:]
	for _, want := range []string{
		"apply-amendment",
		"scope_impact",
		"get an explicit yes",
		"Nothing checks either claim",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("the evolve guidance must state %q", want)
		}
	}
	if !strings.Contains(section, "Do not add it yourself") {
		t.Error("the guidance must forbid the agent inventing the author's scope claim — " +
			"manufacturing it means approving a claim nobody made")
	}
	if !strings.Contains(section, "refused for now") {
		t.Error("the guidance must say which modes have no applier yet")
	}
}
