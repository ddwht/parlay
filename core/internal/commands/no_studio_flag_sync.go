// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-studio-flag-sync
//
// noStudioFlagSync is the package-level boolean target for the
// --no-studio flag on `parlay sync`. Same semantics as the other
// trio-command variants — silences the Studio prompt for that
// invocation only.

package commands

// noStudioFlagSync holds the value of --no-studio for `parlay sync`.
// Read by the command's RunE and merged with config.NoStudio
// (logical OR) before being passed through to dispatchStudioHook.
var noStudioFlagSync bool

// resolveNoStudioForSync merges the --no-studio flag value with the
// project-config no-studio key. See no_studio_flag_domain_model.go
// for the rationale.
func resolveNoStudioForSync(cfgNoStudio bool) bool {
	return noStudioFlagSync || cfgNoStudio
}
