// parlay-feature: parlay-tool/parlay-loop
// parlay-component: LoopInvocationAndFeatureResolution
// parlay-artifact: test

package commands

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLoopCmd_IsRegistered(t *testing.T) {
	var found bool
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "loop" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("loopCmd is not registered under rootCmd")
	}
}

// TestLoopCmd_PrintsSkillPointer asserts the CLI command points at the skill.
//
// This test used to assert nothing — it captured the output, explained that
// capture could not work because "RunE writes to fmt.Println which targets
// os.Stdout, not cmd.OutOrStdout", and discarded the buffer with `_ = output`.
// That was true when it was written. RunE uses fmt.Fprintln(cmd.OutOrStdout())
// now, so the output has been capturable for some time and nothing rechecked the
// premise. A test that documents why it cannot assert is a test that stops being
// read once the reason expires.
//
// What it must hold: this command exists only to redirect. `parlay loop` cannot
// run the pipeline — the loop needs an agent that can spawn subagents — so the
// one job of the CLI entry point is to name the skill that can. Printing nothing,
// or printing without naming it, leaves the user at a command that succeeded and
// did nothing.
func TestLoopCmd_PrintsSkillPointer(t *testing.T) {
	var out bytes.Buffer
	loopCmd.SetErr(&out)
	t.Cleanup(func() { loopCmd.SetErr(nil) })

	// The refusal is now a non-zero exit, not a silent success. A command
	// that declines to do the work and returns nil is indistinguishable from
	// one that did it, so any wrapper checking $? concluded the pipeline had
	// run. The notice goes to stderr for the same reason: it is a refusal,
	// not output.
	err := loopCmd.RunE(loopCmd, []string{"@upgrade-plan"})
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want an ExitCodeError so a script can see the refusal, got %v", err)
	}
	if exitErr.Code == 0 {
		t.Error("an agent-only refusal must not exit 0")
	}

	output := out.String()
	if strings.TrimSpace(output) == "" {
		t.Fatal("loop printed nothing; the command's only job is to redirect to the skill")
	}
	// The skill name is the actionable part. Anchored on the name itself rather
	// than on the sentence around it, so rewording the message stays free.
	if !strings.Contains(output, "/parlay-loop") {
		t.Errorf("output does not name the /parlay-loop skill, which is the only thing it has to say:\n%s", output)
	}
	// And it has to say why, or the redirect reads as an arbitrary refusal.
	if !strings.Contains(strings.ToLower(output), "agent") {
		t.Errorf("output does not explain that an AI agent is required:\n%s", output)
	}
}

func TestLoopCmd_HasFromFlag(t *testing.T) {
	flag := loopCmd.Flags().Lookup("from")
	if flag == nil {
		t.Fatal("loop command missing --from flag")
	}
	if flag.DefValue != "intents" {
		t.Errorf("--from default = %q, want %q", flag.DefValue, "intents")
	}
	usage := flag.Usage
	for _, phase := range []string{"intents", "dialogs", "artifacts", "build", "code"} {
		if !strings.Contains(usage, phase) {
			t.Errorf("--from usage missing phase %q: %q", phase, usage)
		}
	}
}
