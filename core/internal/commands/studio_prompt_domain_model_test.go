// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-prompt-domain-model
// parlay-artifact: test
//
// What this file used to hold, and why none of it is here now.
//
// Two suites exercised a real parlay-studio binary on PATH.
// TestInvokeStudioSubprocess_EndToEnd drove invokeStudioSubprocess — exit-code
// propagation, the launch-failure line, the synthesized 127 for a start failure
// — and went with the subprocess bridge, since a function call has no analogue
// for any of it.
//
// TestParlayStudioSubcommandContract outlived the thing it guarded. It asserted
// that `parlay-studio domain-edit --help` exits zero and quickly, and was
// deliberately fail-hard red when the binary was absent, on the reasoning that
// a missing Studio was itself the violation. The merge inverts that contract in
// both directions: the binary is now a redirect that must exit NON-zero, and
// its absence from PATH is the intended end state rather than a pending
// dependency. Kept as-is it asserted the opposite of the decision — and it did
// fail, against a brew-installed 0.1.2 that still boots a server on
// `domain-edit`.
//
// The property worth keeping is the one that outlives the second binary: the
// command a person runs when they do not yet know what it does must not take an
// irreversible action. That was the original bug behind the old test — help
// fell through to boot() and started listening on a random port with the
// browser opening. It is asserted below against parlay's own command surface,
// in-process, with no PATH lookup and no subprocess.

package commands

import (
	"bytes"
	"strings"
	"testing"
)

// TestDomainEditHelpDoesNotBoot pins the surviving contract: `--help` on the
// editor commands prints usage and returns, without resolving a project root
// and without binding a port.
//
// It runs in-process against rootCmd. Cobra handles the help flag after flag
// parsing and before PersistentPreRunE, so a passing run also demonstrates that
// help needs no resolved root — which is why this can execute from an arbitrary
// working directory without a .parlay/ anywhere above it.
func TestDomainEditHelpDoesNotBoot(t *testing.T) {
	// `serve` is the same boot with no landing route, registered under
	// `internal` because it is a testing affordance. It boots just as hard, so
	// it gets the same guard.
	for _, argv := range [][]string{{"domain-edit"}, {"internal", "serve"}} {
		name := strings.Join(argv, " ")
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			rootCmd.SetOut(&out)
			rootCmd.SetErr(&out)
			rootCmd.SetArgs(append(append([]string{}, argv...), "--help"))
			t.Cleanup(func() {
				rootCmd.SetOut(nil)
				rootCmd.SetErr(nil)
				rootCmd.SetArgs(nil)
			})

			// A boot would block here until the idle timeout rather than
			// returning, so reaching the assertions at all is half the check.
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("%s --help returned %v, want nil", name, err)
			}

			got := out.String()
			if !strings.Contains(got, "Usage:") {
				t.Fatalf("%s --help printed no usage block:\n%s", name, got)
			}
			// The three flags are the editor's whole surface; usage that omits
			// them is usage in name only.
			for _, flag := range []string{"--server-port", "--idle-timeout", "--no-browser"} {
				if !strings.Contains(got, flag) {
					t.Errorf("%s --help does not document %s:\n%s", name, flag, got)
				}
			}
		})
	}
}

// TestTrioHookOffersOnlyBuiltSurfaces guards the table that decides which trio
// commands may offer the editor.
//
// The old contract test derived its subcommand list from this same map and then
// checked the names against a second binary's dispatcher. There is no second
// dispatcher to disagree with now, so what is left to protect is the narrower
// claim the map still makes: an entry here asserts that a surface exists. Two
// entries once named surfaces that were never built — `artifacts-review` and
// `reconcile` — and the map pointed at subcommands nothing recognized.
func TestTrioHookOffersOnlyBuiltSurfaces(t *testing.T) {
	if len(trioToStudioSubcommand) == 0 {
		t.Fatal("trioToStudioSubcommand is empty — no trio command can offer the editor")
	}
	for trio := range trioToStudioSubcommand {
		if _, ok := hookPromptWording[trio]; !ok {
			t.Errorf("trio %q may offer the editor but has no prompt wording — the dispatch would error at the point of asking", trio)
		}
	}
	for _, unbuilt := range []string{"artifacts-review", "reconcile"} {
		if _, ok := trioToStudioSubcommand[unbuilt]; ok {
			t.Errorf("trio %q is back in the map; the surface it names was never built", unbuilt)
		}
	}
}
