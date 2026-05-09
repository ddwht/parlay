// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-studio-flag-domain-model
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-sync
// parlay-artifact: test
//
// Shared tests for the three --no-studio flag variants. The flag
// targets are package-level booleans, so each test resets the
// relevant variable to false before exercising the resolver to keep
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
	noStudioFlagDomainModel = true
	defer func() { noStudioFlagDomainModel = false }()
	if !resolveNoStudioForDomainModel(false) {
		t.Errorf("flag=true, cfg=false → expected disable=true")
	}
}

func TestResolveNoStudio_ConfigOnlyTriggersDisable(t *testing.T) {
	noStudioFlagArtifacts = false
	if !resolveNoStudioForArtifacts(true) {
		t.Errorf("flag=false, cfg=true → expected disable=true (logical OR)")
	}
}

func TestResolveNoStudio_BothFalseLeavesEnabled(t *testing.T) {
	noStudioFlagSync = false
	if resolveNoStudioForSync(false) {
		t.Errorf("flag=false, cfg=false → expected disable=false")
	}
}

func TestResolveNoStudio_BothTrueStaysDisabled(t *testing.T) {
	noStudioFlagDomainModel = true
	defer func() { noStudioFlagDomainModel = false }()
	if !resolveNoStudioForDomainModel(true) {
		t.Errorf("flag=true, cfg=true → expected disable=true")
	}
}
