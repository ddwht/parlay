// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-launch-failure-line
//
// formatStudioLaunchFailure renders the "[ERR] parlay-studio … exited
// with code N — see Studio's output above. Trio-command artifact
// completed before Studio launched and is on disk." line printed to
// stderr when an accepted Studio invocation crashes or exits non-zero.
// The trio command itself then exits non-zero so callers see the
// failure.

package commands

import "fmt"

// formatStudioLaunchFailure returns the exact failure line the
// shared dispatch prints on a non-zero Studio exit. Lower-case
// because the dispatcher is the only call site; the launch-failure
// text is part of the dispatch's observable contract, asserted by
// the studio-launch-failure-line testcases.
func formatStudioLaunchFailure(subcommand string, exitCode int) string {
	return fmt.Sprintf(
		"[ERR] parlay-studio %s exited with code %d — see Studio's output above. Trio-command artifact completed before Studio launched and is on disk.",
		subcommand, exitCode,
	)
}

// FormatStudioLaunchFailure is the exported wrapper for callers
// outside this package that want the same launch-failure text (e.g.
// downstream tooling rendering a similar error from a different
// surface). Internal call sites use the lower-case form.
func FormatStudioLaunchFailure(subcommand string, exitCode int) string {
	return formatStudioLaunchFailure(subcommand, exitCode)
}
