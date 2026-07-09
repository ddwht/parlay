// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-studio-flag-domain-model
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-sync
//
// The three trio commands (create-domain-model, create-artifacts, sync)
// each register their own --no-studio flag to silence the Studio
// open-editor prompt for that one invocation. The flag has no inverse
// --studio form — config-level disables (parlay.no_studio in
// .parlay/config.yaml) can only be reverted by changing config.
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

package commands

// noStudioFlag holds the value of --no-studio, shared across all three
// trio commands' flag registrations. Read by each command's RunE and
// merged with config.NoStudio (logical OR) before being passed through
// to dispatchStudioHook.
var noStudioFlag bool

// noStudioFlagHelpText is the one-line help text shown next to
// --no-studio in `parlay <trio> --help`. Shared across all three
// registrations so the testcase "flag help text is one line" passes
// against any of them.
const noStudioFlagHelpText = "skip the Studio open-editor prompt at the end"

// resolveNoStudio merges the --no-studio flag value with the
// project-config no-studio key. Logical OR: either source suppresses
// the prompt; both must be false for the prompt to fire.
func resolveNoStudio(cfgNoStudio bool) bool {
	return noStudioFlag || cfgNoStudio
}
