// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-studio-flag-domain-model
//
// noStudioFlagDomainModel is the package-level boolean target for the
// --no-studio flag on `parlay create-domain-model`. The flag has no
// inverse --studio form — config-level disables can only be reverted
// by changing config. Help text is exactly one line, asserted by
// testcase "flag help text is one line".

package commands

// noStudioFlagDomainModel holds the value of --no-studio for
// `parlay create-domain-model`. Read by the command's RunE and
// merged with config.NoStudio (logical OR) before being passed
// through to dispatchStudioHook.
var noStudioFlagDomainModel bool

// noStudioFlagHelpText is the one-line help text shown next to
// --no-studio in `parlay <trio> --help`. The same string is shared
// by every trio command's flag definition so the testcase
// "flag help text is one line" passes against any of them.
const noStudioFlagHelpText = "skip the Studio open-editor prompt at the end"

// resolveNoStudioForDomainModel merges the --no-studio flag value
// with the project-config no-studio key. Logical OR: either source
// suppresses the prompt; both must be false for the prompt to fire.
func resolveNoStudioForDomainModel(cfgNoStudio bool) bool {
	return noStudioFlagDomainModel || cfgNoStudio
}
