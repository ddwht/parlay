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
// was retired. "Studio" no longer names anything a user can install, and
// the thing being suppressed is the in-process editor, so the flag says
// that now. --no-studio is still registered and still works; it is hidden
// from help so it stops teaching the old name to new readers.

package commands

import "github.com/spf13/cobra"

// noEditorFlag holds the value of --no-editor, shared across all three
// trio commands' flag registrations. Read by each command's RunE and
// merged with the project config (logical OR) by resolveNoEditor.
var noEditorFlag bool

// noEditorFlagDeprecated backs the deprecated --no-studio spelling. It is
// a separate variable rather than a second binding of noEditorFlag because
// cobra writes a flag's default into its target at registration time, so
// two registrations sharing one pointer would have the second's default
// clobber the first's parsed value.
var noEditorFlagDeprecated bool

// noEditorFlagHelpText is the one-line help text shown next to
// --no-editor in `parlay <trio> --help`. Shared across all three
// registrations so the testcase "flag help text is one line" passes
// against any of them.
const noEditorFlagHelpText = "skip the open-editor prompt at the end"

// registerNoEditorFlags binds both spellings onto cmd. Callers register
// through this rather than calling Flags().BoolVar directly, so the
// deprecated alias cannot be attached to one trio command and forgotten
// on another.
func registerNoEditorFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&noEditorFlag, "no-editor", false, noEditorFlagHelpText)
	cmd.Flags().BoolVar(&noEditorFlagDeprecated, "no-studio", false, noEditorFlagHelpText)
	// Hidden, not deprecated-with-a-message: cobra prints its deprecation
	// notice to stdout, which would corrupt the JSON that several of these
	// commands emit. Hiding it keeps the flag working and keeps it out of
	// help without writing anything to a stream a caller may be parsing.
	_ = cmd.Flags().MarkHidden("no-studio")
}

// resolveNoEditor merges both flag spellings with the project-config
// opt-out. Logical OR: any source suppresses the prompt; all must be
// false for the prompt to fire.
func resolveNoEditor(cfgNoEditor bool) bool {
	return noEditorFlag || noEditorFlagDeprecated || cfgNoEditor
}
