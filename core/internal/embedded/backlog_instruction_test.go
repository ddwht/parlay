// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package embedded

import (
	"regexp"
	"strings"
	"testing"
)

// THE INSTRUCTION HALF.
//
// A CLI nobody is told to call does not get called. `parlay note` exists
// so a discovery mid-build survives the session, and it survives nothing
// unless the phases that make discoveries are told to run it. This is the
// mechanical check that the two halves ship together — the proposal turns
// on it, and it is exactly the kind of thing that rots silently because
// the code keeps compiling without it.
func TestSkills_PhasesThatDiscoverAreToldToCapture(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, s := range skills {
		byName[s.Name] = string(s.Content)
	}

	// The phases where discovery actually happens: generating code, and
	// authoring the artifacts that generation reads.
	for _, name := range []string{
		"generate-code", "build-feature", "create-artifacts",
		"scaffold-dialogs", "create-intents",
	} {
		body, ok := byName[name]
		if !ok {
			t.Errorf("skill %q not found", name)
			continue
		}
		if !strings.Contains(body, "parlay note") {
			t.Errorf("%s does not tell the agent to capture what it notices", name)
		}
		// The two rules that keep capture honest must travel with it.
		if !strings.Contains(body, "never a priority") && !strings.Contains(body, "Never guess a priority") {
			t.Errorf("%s omits the never-guess-a-priority rule", name)
		}
		if !strings.Contains(body, "must never fail") {
			t.Errorf("%s omits the non-blocking rule — a failed capture must not fail the phase", name)
		}
	}
}

// The driver has to surface what the phases captured, or the ids are
// written and never read.
func TestLoopSkill_SurfacesCapturedNotes(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	var loop string
	for _, s := range skills {
		if s.Name == "loop" {
			loop = string(s.Content)
		}
	}
	if loop == "" {
		t.Fatal("loop skill not found")
	}
	if !strings.Contains(loop, "notes:") {
		t.Error("the parlay-decision block has no notes: field, so a phase cannot report what it captured")
	}
	if !strings.Contains(loop, "already on screen") && !strings.Contains(loop, "phase boundary") {
		t.Error("the loop does not say the driver surfaces notes at the boundary the user already reads")
	}
}

// A subagent cannot ask anyone anything, so it must be told to file
// rather than mention.
func TestAgents_SubagentsAreToldToCapture(t *testing.T) {
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) == 0 {
		t.Fatal("no subagent definitions found")
	}
	for _, a := range agents {
		if !strings.Contains(string(a.Content), "parlay note") {
			t.Errorf("subagent %q is not told to capture discoveries", a.Name)
		}
	}
}

// A PUBLISHED EXAMPLE MUST BE RUNNABLE.
//
// The instructions shipped `--kind defect|gap|debt|idea`, which a shell
// parses as a pipeline: it runs `parlay note --kind defect`, pipes into
// `gap`, and fails. The "contains parlay note" test above approved it,
// which is the failure mode of a presence check — it proved a caller was
// named, not that the caller works.
func TestInstructions_PublishedNoteExamplesAreRunnable(t *testing.T) {
	check := func(label, body string) {
		for _, line := range strings.Split(body, "\n") {
			if !strings.Contains(line, "parlay note") {
				continue
			}
			// A pipe inside a command line is a shell pipeline, not a
			// choice list. Choices belong in the prose beside it.
			if strings.Contains(line, "|") {
				t.Errorf("%s publishes an unrunnable example (shell would pipe): %s", label, strings.TrimSpace(line))
			}
		}
	}
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		check("skill "+s.Name, string(s.Content))
	}
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		check("agent "+a.Name, string(a.Content))
	}
}

// The scoped read must PHYSICALLY precede the procedure, not merely say
// it does.
//
// All three sections were appended at the bottom of their files and then
// given a sentence claiming they appeared above the procedure. An agent
// following document order would read the backlog after it had already
// written — and the whole contract is that the USER decides whether a
// hit enters scope, which a read performed after the writing cannot
// offer. A claim about position is not a position.
func TestSkills_ScopedReadPrecedesTheProcedure(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	skillsByName := map[string]string{}
	for _, sk := range skills {
		skillsByName[sk.Name] = string(sk.Content)
	}
	procedure := regexp.MustCompile(`(?m)^## (Steps|Working|Presentation|Procedure|What)\b`)
	const readHeading = "## Read the backlog for this feature"

	for _, name := range []string{"create-intents", "scaffold-dialogs", "create-artifacts"} {
		t.Run(name, func(t *testing.T) {
			body, ok := skillsByName[name]
			if !ok {
				t.Fatalf("skill %q not found", name)
			}

			if n := strings.Count(body, readHeading); n != 1 {
				t.Fatalf("want exactly one scoped-read section, got %d — moving it must not leave a copy behind", n)
			}
			read := strings.Index(body, readHeading)
			loc := procedure.FindStringIndex(body)
			if loc == nil {
				t.Fatalf("no procedural heading found; this test cannot tell position without one")
			}
			if read > loc[0] {
				t.Errorf("the scoped read is at %d, after the procedure at %d — an agent following document order reads the backlog once it has already written",
					read, loc[0])
			}
			// And it must still exclude itself from a chained run, or
			// the designer group and the phase both perform it.
			if !strings.Contains(body, "invoked standalone") && !strings.Contains(body, "invoked on your own") {
				t.Error("the section no longer excludes itself from a chained designer run; the read would happen twice")
			}
		})
	}

	// The designer agent owns the one read for the chained group.
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatal(err)
	}
	var designer string
	for _, a := range agents {
		if strings.Contains(a.Name, "designer") {
			designer = string(a.Content)
		}
	}
	if designer == "" {
		t.Fatal("designer agent not found")
	}
	if !strings.Contains(designer, "backlog list --related") {
		t.Error("the designer agent no longer owns the group's scoped read")
	}
	if strings.Count(designer, "backlog list --related") != 1 {
		t.Error("the designer agent performs the scoped read more than once")
	}
}
