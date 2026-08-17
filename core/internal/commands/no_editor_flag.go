// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-editor-flag-domain-model
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-sync
//
// The three trio commands (create-domain-model, create-artifacts, sync)
// each register --no-editor to silence the open-editor offer for that one
// invocation. The flag has no inverse --editor form — config-level disables
// (parlay.no_editor in .parlay/config.yaml) can only be reverted by changing
// config.
//
// This used to be three near-identical files (no_studio_flag_artifacts.go,
// no_studio_flag_domain_model.go, no_studio_flag_sync.go), each declaring
// its own package-level bool and a resolver function that did the exact
// same "flag || config" logical OR — precisely the kind of triplicated
// helper `parlay simplify` is built to flag. Collapsed into one shared
// flag variable and one shared resolver: binding the same *bool pointer
// into three different cobra Commands' FlagSets is legal (each FlagSet
// just wraps the pointer independently), and safe in production since
// exactly one subcommand's flags are ever parsed per process invocation.
//
// The flag was called --no-studio until the separate parlay-studio binary
// was retired; the hidden alias survived one deprecation window and was
// removed in v0.3. Only --no-editor is registered now.

package commands

import "github.com/spf13/cobra"

// noEditorFlag holds the value of --no-editor, shared across all three
// trio commands' flag registrations. Read by each command's RunE and
// merged with the project config (logical OR) by resolveNoEditor.
var noEditorFlag bool

// noEditorFlagHelpText is the one-line help text shown next to
// --no-editor in `parlay <trio> --help`. Shared across all three
// registrations so the testcase "flag help text is one line" passes
// against any of them.
const noEditorFlagHelpText = "skip the open-editor prompt at the end"

// registerNoEditorFlags binds the flag onto cmd. Callers register through
// this rather than calling Flags().BoolVar directly, so every trio command
// registers identically.
func registerNoEditorFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&noEditorFlag, "no-editor", false, noEditorFlagHelpText)
}

// resolveNoEditor merges the flag with the project-config opt-out.
// Logical OR: either source suppresses the prompt; both must be false for
// the prompt to fire.
func resolveNoEditor(cfgNoEditor bool) bool {
	return noEditorFlag || cfgNoEditor
}
