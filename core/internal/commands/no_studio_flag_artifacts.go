// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: no-studio-flag-artifacts
//
// noStudioFlagArtifacts is the package-level boolean target for the
// --no-studio flag on `parlay create-artifacts`. Identical semantics
// to the create-domain-model variant — silences the Studio prompt for
// that invocation only.

package commands

// noStudioFlagArtifacts holds the value of --no-studio for
// `parlay create-artifacts`. Read by the command's RunE and merged
// with config.NoStudio (logical OR) before being passed through to
// dispatchStudioHook.
var noStudioFlagArtifacts bool

// resolveNoStudioForArtifacts merges the --no-studio flag value with
// the project-config no-studio key. See no_studio_flag_domain_model.go
// for the rationale; this is a per-trio mirror.
func resolveNoStudioForArtifacts(cfgNoStudio bool) bool {
	return noStudioFlagArtifacts || cfgNoStudio
}
