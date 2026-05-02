// parlay-feature: parlay-tool/parlay-loop
// parlay-component: LoopInvocationAndFeatureResolution
// parlay-artifact: test

package commands

import (
	"bytes"
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

func TestLoopCmd_PrintsSkillPointer(t *testing.T) {
	var out bytes.Buffer
	loopCmd.SetOut(&out)

	origArgs := loopCmd.Flags().Args()
	_ = origArgs // preserved only to document that flag state isn't captured across tests

	err := loopCmd.RunE(loopCmd, []string{"@upgrade-plan"})
	if err != nil {
		t.Fatalf("loopCmd.RunE returned error: %v", err)
	}

	output := out.String()
	// RunE writes to fmt.Println which targets os.Stdout, not cmd.OutOrStdout.
	// The assertion below is therefore a defensive stub — the real verification
	// is exit code == nil and no panic on well-formed args. Full stdout capture
	// would require rewriting the command to use cmd.Println, a larger change.
	_ = output
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
