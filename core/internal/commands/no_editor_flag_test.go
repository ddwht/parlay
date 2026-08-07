// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-editor-flag-domain-model
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-sync
// parlay-artifact: test
//
// Shared tests for the collapsed --no-editor flag (one variable, one
// resolver, shared across all three trio commands). Each test resets
// the shared variables to false before exercising the resolver to keep
// the cases independent.
//
// The deprecated --no-studio spelling gets its own cases: it is the whole
// reason a second package-level bool exists, and a rename that silently
// stopped honouring the old flag would be indistinguishable from one that
// honoured it, since both leave the prompt unshown in the common case.

package commands

import (
	"testing"

	"github.com/spf13/pflag"
)

func resetNoEditorFlags() {
	noEditorFlag = false
	noEditorFlagDeprecated = false
}

func TestNoEditorFlagHelpTextIsSingleLine(t *testing.T) {
	want := "skip the open-editor prompt at the end"
	if noEditorFlagHelpText != want {
		t.Errorf("noEditorFlagHelpText = %q, want %q", noEditorFlagHelpText, want)
	}
	for i, ch := range noEditorFlagHelpText {
		if ch == '\n' {
			t.Errorf("help text contains newline at offset %d", i)
		}
	}
}

func TestResolveNoEditor_FlagOnlyTriggersDisable(t *testing.T) {
	resetNoEditorFlags()
	defer resetNoEditorFlags()
	noEditorFlag = true
	if !resolveNoEditor(false) {
		t.Errorf("flag=true, cfg=false → expected disable=true")
	}
}

func TestResolveNoEditor_ConfigOnlyTriggersDisable(t *testing.T) {
	resetNoEditorFlags()
	defer resetNoEditorFlags()
	if !resolveNoEditor(true) {
		t.Errorf("flag=false, cfg=true → expected disable=true (logical OR)")
	}
}

func TestResolveNoEditor_BothFalseLeavesEnabled(t *testing.T) {
	resetNoEditorFlags()
	defer resetNoEditorFlags()
	if resolveNoEditor(false) {
		t.Errorf("flag=false, cfg=false → expected disable=false")
	}
}

func TestResolveNoEditor_BothTrueStaysDisabled(t *testing.T) {
	resetNoEditorFlags()
	defer resetNoEditorFlags()
	noEditorFlag = true
	if !resolveNoEditor(true) {
		t.Errorf("flag=true, cfg=true → expected disable=true")
	}
}

// TestResolveNoEditor_DeprecatedFlagStillHonoured is the regression guard
// for the rename: a script or muscle-memory invocation still passing
// --no-studio must suppress the prompt exactly as --no-editor does.
func TestResolveNoEditor_DeprecatedFlagStillHonoured(t *testing.T) {
	resetNoEditorFlags()
	defer resetNoEditorFlags()
	noEditorFlagDeprecated = true
	if !resolveNoEditor(false) {
		t.Errorf("--no-studio=true → expected disable=true")
	}
}

// TestNoStudioFlagIsHiddenNotRemoved pins both halves of the deprecation:
// the flag still parses (removing it would break callers) and it no longer
// appears in help (leaving it visible would keep teaching the old name).
func TestNoStudioFlagIsHiddenNotRemoved(t *testing.T) {
	for name, fs := range map[string]*pflag.FlagSet{
		"create-domain-model": createDomainModelCmdImpl.Flags(),
		"create-artifacts":    createArtifactsCmdImpl.Flags(),
		"sync":                syncCmdImpl.Flags(),
	} {
		f := fs.Lookup("no-studio")
		if f == nil {
			t.Errorf("%s: --no-studio was removed; it must stay registered for callers that still pass it", name)
			continue
		}
		if !f.Hidden {
			t.Errorf("%s: --no-studio is still visible in help; it should be hidden", name)
		}
		if fs.Lookup("no-editor") == nil {
			t.Errorf("%s: --no-editor is not registered", name)
		}
	}
}

// TestNoEditorFlag_SharedAcrossAllThreeCommands confirms the same
// package-level bool is genuinely bound to all three trio commands'
// --no-editor flags — the whole point of the collapse. Setting one
// command's flag must be observable through any of the three FlagSets,
// since they all wrap the same pointer.
func TestNoEditorFlag_SharedAcrossAllThreeCommands(t *testing.T) {
	resetNoEditorFlags()
	defer func() {
		createDomainModelCmdImpl.Flags().Set("no-editor", "false")
		resetNoEditorFlags()
	}()

	if err := createDomainModelCmdImpl.Flags().Set("no-editor", "true"); err != nil {
		t.Fatal(err)
	}
	if !noEditorFlag {
		t.Error("expected the shared noEditorFlag to be true after setting it via create-domain-model's FlagSet")
	}
	got, err := createArtifactsCmdImpl.Flags().GetBool("no-editor")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("expected create-artifacts' FlagSet to observe the same value, since both wrap the same pointer")
	}
	got, err = syncCmdImpl.Flags().GetBool("no-editor")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("expected sync's FlagSet to observe the same value, since both wrap the same pointer")
	}
}
