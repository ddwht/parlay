package embedded

import (
	"strings"
	"testing"
)

// The deploy-time gate injection: a module declaring `gate-stage:` gets the
// uniform Step 0 gate block, parameterized by its stage; a module without the
// field is untouched. This is the property that makes the gate impossible to
// omit from any phase module, present or future.
func TestGateStepInjectedByFrontmatter(t *testing.T) {
	all, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]SkillEntry{}
	for _, s := range all {
		byName[s.Name] = s
	}

	gated := map[string]string{
		"build-feature": "build",
		"generate-code": "code",
	}
	for name, stage := range gated {
		s, ok := byName[name]
		if !ok {
			t.Fatalf("module %s not found among embedded skills", name)
		}
		if s.GateStage != stage {
			t.Errorf("%s: GateStage = %q, want %q", name, s.GateStage, stage)
		}
		body := string(s.Content)
		if !strings.Contains(body, "## Step 0 — Gate") {
			t.Errorf("%s: deployed body is missing the injected Step 0 gate block", name)
		}
		if !strings.Contains(body, "parlay internal gate @{feature} --stage "+stage) {
			t.Errorf("%s: injected gate block does not name --stage %s", name, stage)
		}
		// The marker itself must be consumed, never shipped verbatim.
		if strings.Contains(body, gateMarker) {
			t.Errorf("%s: raw gate marker leaked into the deployed body", name)
		}
	}

	// A module with no gate-stage is untouched.
	if ca, ok := byName["create-artifacts"]; ok {
		if ca.GateStage != "" {
			t.Errorf("create-artifacts must declare no gate-stage; got %q", ca.GateStage)
		}
		if strings.Contains(string(ca.Content), "## Step 0 — Gate") {
			t.Error("create-artifacts must not carry an injected gate block")
		}
	}
}

// injectGateStep prepends the block when no marker is present, so declaring the
// stage is sufficient on its own — an author cannot declare gate-stage and then
// omit the instruction by forgetting the marker.
func TestGateStepPrependedWhenMarkerAbsent(t *testing.T) {
	body := []byte("# Some Module\n\nsome content\n")
	out := string(injectGateStep(body, "build"))
	if !strings.HasPrefix(out, "## Step 0 — Gate") {
		t.Errorf("with no marker the gate block must be prepended; got prefix %q", out[:min(40, len(out))])
	}
	if !strings.Contains(out, "# Some Module") {
		t.Error("prepending must preserve the original body")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// The machine-authorization carrier must reach every gated module.
//
// The flag existed on the gate command and the loop's argument list while the
// injected Step 0 hard-coded `parlay internal gate @{feature} --stage <stage>`
// with no conditional argument — so the gates that actually run inside
// build-feature and generate-code could never exercise the waiver, and the
// guidance conditioning behaviour on the flag named a carrier those modules had
// no way to use. A promise made in an argument description is not forwarding.
func TestGateStepCarriesMachineAuthorization(t *testing.T) {
	all, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	gated := 0
	for _, s := range all {
		if s.GateStage == "" {
			continue
		}
		gated++
		body := string(s.Content)
		if !strings.Contains(body, "--authorize-criteria=machine") {
			t.Errorf("%s is gated at %q but its injected gate cannot carry an authorized run's waiver", s.Name, s.GateStage)
		}
	}
	if gated == 0 {
		t.Fatal("no gated modules found — this test would pass vacuously")
	}
}

// The expansion used to promise the gate "writes nothing". That stopped being
// true when a passing machine code boundary began recording its waiver, and a
// module telling an agent the gate is side-effect-free while it writes is worse
// than silence.
func TestGateStepDoesNotClaimItWritesNothing(t *testing.T) {
	all, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		if s.GateStage == "" {
			continue
		}
		if strings.Contains(string(s.Content), "writes nothing, so") {
			t.Errorf("%s still claims the gate writes nothing; an authorized machine run records its waiver there", s.Name)
		}
	}
}
