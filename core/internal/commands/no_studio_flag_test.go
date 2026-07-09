// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-studio-flag-domain-model
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-sync
// parlay-artifact: test
//
// Shared tests for the collapsed --no-studio flag (one variable, one
// resolver, shared across all three trio commands). Each test resets
// the shared variable to false before exercising the resolver to keep
// the cases independent.

package commands

import "testing"

func TestNoStudioFlagHelpTextIsSingleLine(t *testing.T) {
	want := "skip the Studio open-editor prompt at the end"
	if noStudioFlagHelpText != want {
		t.Errorf("noStudioFlagHelpText = %q, want %q", noStudioFlagHelpText, want)
	}
	for i, ch := range noStudioFlagHelpText {
		if ch == '\n' {
			t.Errorf("help text contains newline at offset %d", i)
		}
	}
}

func TestResolveNoStudio_FlagOnlyTriggersDisable(t *testing.T) {
	noStudioFlag = true
	defer func() { noStudioFlag = false }()
	if !resolveNoStudio(false) {
		t.Errorf("flag=true, cfg=false → expected disable=true")
	}
}

func TestResolveNoStudio_ConfigOnlyTriggersDisable(t *testing.T) {
	noStudioFlag = false
	if !resolveNoStudio(true) {
		t.Errorf("flag=false, cfg=true → expected disable=true (logical OR)")
	}
}

func TestResolveNoStudio_BothFalseLeavesEnabled(t *testing.T) {
	noStudioFlag = false
	if resolveNoStudio(false) {
		t.Errorf("flag=false, cfg=false → expected disable=false")
	}
}

func TestResolveNoStudio_BothTrueStaysDisabled(t *testing.T) {
	noStudioFlag = true
	defer func() { noStudioFlag = false }()
	if !resolveNoStudio(true) {
		t.Errorf("flag=true, cfg=true → expected disable=true")
	}
}

// TestNoStudioFlag_SharedAcrossAllThreeCommands confirms the same
// package-level bool is genuinely bound to all three trio commands'
// --no-studio flags — the whole point of the collapse. Setting one
// command's flag must be observable through any of the three FlagSets,
// since they all wrap the same pointer.
func TestNoStudioFlag_SharedAcrossAllThreeCommands(t *testing.T) {
	if err := createDomainModelCmdImpl.Flags().Set("no-studio", "true"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		createDomainModelCmdImpl.Flags().Set("no-studio", "false")
	}()

	if !noStudioFlag {
		t.Error("expected the shared noStudioFlag to be true after setting it via create-domain-model's FlagSet")
	}
	got, err := createArtifactsCmdImpl.Flags().GetBool("no-studio")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("expected create-artifacts' FlagSet to observe the same value, since both wrap the same pointer")
	}
	got, err = syncCmdImpl.Flags().GetBool("no-studio")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("expected sync's FlagSet to observe the same value, since both wrap the same pointer")
	}
}
