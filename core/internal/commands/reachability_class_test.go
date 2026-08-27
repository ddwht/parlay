package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func internalCommands(t *testing.T) []*cobra.Command {
	t.Helper()
	for _, c := range rootCmd.Commands() {
		if c.Name() == "internal" {
			return c.Commands()
		}
	}
	t.Fatal("no `internal` group registered — init() may not have run")
	return nil
}

// The check that makes classification uncircumventable. A marker commands opt
// INTO is circular: the next command that skips its guidance skips the marker
// for the same reason. Failing on anything undeclared means adding a command
// forces the question.
func TestReachability_EveryInternalCommandDeclaresItsClass(t *testing.T) {
	for _, c := range internalCommands(t) {
		class := ReachabilityClass(c)
		if class == "" {
			t.Errorf("`parlay internal %s` declares no reachability class. "+
				"Who reaches it? Add reachability(cmd, Class...) at registration — "+
				"probe, skill-required, direct-human, or pipeline-helper.", c.Name())
			continue
		}
		if !ValidReachabilityClasses[class] {
			t.Errorf("`parlay internal %s` declares %q, outside the closed set", c.Name(), class)
		}
	}
}

// emittedByActionPlan is derived from the emitter, never hand-listed, so a
// command dropped from the plans stops counting as reachable immediately.
var emittedByActionPlan = func() map[string]bool {
	out := map[string]bool{}
	plans := reviewActions("f", MigrationOccurrence{
		Ref: "@f/operation:x", Text: "t", Fingerprint: "abc", Duplicate: 0,
	}, &ReviewPacket{}, "hash")
	for _, a := range plans {
		out[a.Command] = true
	}
	return out
}()

// Every command an action plan emits must have an executing witness, not just
// a name. This is the check that would have caught the three unwalkable
// commands: their flags all existed, and none could be populated.
func TestReachability_ActionPlanCommandsAreExecutedByATest(t *testing.T) {
	if len(emittedByActionPlan) == 0 {
		t.Fatal("the action plan emits nothing, so nothing certifies these commands")
	}
	for _, c := range internalCommands(t) {
		if !emittedByActionPlan[c.Name()] {
			continue
		}
		if ReachabilityClass(c) != ClassSkillRequired {
			t.Errorf("%s is handed to a caller by an action plan, so it acquires a judgment and must be skill-required; got %q",
				c.Name(), ReachabilityClass(c))
		}
	}
	// The executing witness itself. Named explicitly so deleting it fails here
	// rather than quietly removing the only proof these commands are runnable.
	if !testing.Short() {
		t.Log("executed by TestNextLegacyReview_EmittedActionsActuallyRun")
	}
}

// deployedGuidance is every skill and module text a project actually receives.
func deployedGuidance(t *testing.T) map[string]string {
	t.Helper()
	root := repoRootFromTest(t)
	out := map[string]string{}
	for _, dir := range []string{
		filepath.Join(root, ".claude", "skills"),
		filepath.Join(root, ".parlay", "modules"),
	} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			b, rErr := os.ReadFile(path)
			if rErr == nil {
				out[path] = string(b)
			}
			return nil
		})
	}
	if len(out) == 0 {
		t.Skip("no deployed guidance in this checkout")
	}
	return out
}

// repoRootFromTest finds the root that actually carries deployed guidance.
//
// Searching for `.parlay` alone is not enough and produced a false green: this
// repo has a `core/.parlay` holding no modules, so the walk stopped there,
// found nothing, and both reachability tests SKIPPED while reporting ok. A test
// that silently covers nothing is worse than none, so this looks for the
// directories it actually needs.
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		_, skillsErr := os.Stat(filepath.Join(dir, ".claude", "skills"))
		_, modulesErr := os.Stat(filepath.Join(dir, ".parlay", "modules"))
		if skillsErr == nil || modulesErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("no deployed guidance above this checkout")
	return ""
}

// A skill-required command with no walkthrough naming it is precisely the
// failure that produced this table: working, tested code that nobody can reach.
func TestReachability_SkillRequiredCommandsAreNamedByDeployedGuidance(t *testing.T) {
	guidance := deployedGuidance(t)
	for _, c := range internalCommands(t) {
		if ReachabilityClass(c) != ClassSkillRequired {
			continue
		}
		// A command may be reached two ways. Prose naming it is one. The other
		// is an emitted action plan naming it, which is STRONGER: the caller
		// runs a command it was handed rather than one it assembled, and
		// TestNextLegacyReview_EmittedActionsActuallyRun executes those plans
		// against the real writers. Codex's caution applies — verb presence in
		// text certifies only that a name appears, not that a caller can
		// populate its flags, which is exactly how three unwalkable commands
		// passed a hand check earlier today.
		if emittedByActionPlan[c.Name()] {
			continue
		}
		want := "parlay internal " + c.Name()
		found := ""
		for path, body := range guidance {
			if strings.Contains(body, want) {
				found = filepath.Base(path)
				break
			}
		}
		if found == "" {
			t.Errorf("`%s` acquires a human judgment but no deployed skill or module names it. "+
				"Either a walkthrough must put its question to a person, or it is not skill-required.", want)
		}
	}
}

// Pipeline helpers are invoked by the pipeline, so something the project
// deploys must invoke them. A helper nothing calls is not a helper.
func TestReachability_PipelineHelpersAreInvokedBySomething(t *testing.T) {
	guidance := deployedGuidance(t)
	// Helpers a skill never types by name: they are reached from Go, from a
	// served endpoint, or by a sibling command. Listed rather than inferred so
	// that adding one is a deliberate act.
	reachedFromCode := map[string]bool{
		"serve":           true,
		"feedback-record": true,
	}
	var unreached []string
	for _, c := range internalCommands(t) {
		if ReachabilityClass(c) != ClassPipelineHelper || reachedFromCode[c.Name()] {
			continue
		}
		want := "parlay internal " + c.Name()
		found := false
		for _, body := range guidance {
			if strings.Contains(body, want) {
				found = true
				break
			}
		}
		if !found {
			unreached = append(unreached, c.Name())
		}
	}
	if len(unreached) > 0 {
		t.Errorf("these pipeline helpers are named by no deployed guidance: %v. "+
			"Either a phase invokes them and the text should say so, or they are reached "+
			"from Go and belong in reachedFromCode with a note saying how.", unreached)
	}
}
