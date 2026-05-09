// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: version-mismatch-warning
//
// FormatStudioVersionWarning renders the one-line stderr warning
// emitted at the first successful Studio detection in the process
// when the detected parlay-studio version falls outside Core's
// expected range. The actual once-guarded emission lives in
// config.EmitStudioVersionWarningOnce — this helper exists so the
// component file owns the formatted text the testcases assert.

package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
)

// ExpectedStudioVersionRangeForDisplay is the human-readable form of
// the expected version range surfaced in the warning. Mirrors the
// internal constant in package config so the warning text in tests
// can be asserted by importing this package without dragging in
// internal config helpers.
const ExpectedStudioVersionRangeForDisplay = ">=1.0.0"

// FormatStudioVersionWarning returns the warning line a
// version-mismatched Studio should produce. Empty string when the
// detection is silent (not detected, or version unknown, or version
// inside the expected range) — callers print only when the returned
// string is non-empty.
func FormatStudioVersionWarning(d config.StudioDetection) string {
	if !d.Detected || d.Version == "" {
		return ""
	}
	if !studioVersionMismatchesForWarning(d.Version) {
		return ""
	}
	return fmt.Sprintf(
		"warning: parlay-studio version %s is older than expected (need %s); some hooks may not work.",
		d.Version, ExpectedStudioVersionRangeForDisplay,
	)
}

// studioVersionMismatchesForWarning is the local view of "is this
// version outside the expected range?" — keeps the version-rule
// logic next to the warning text. The semantics match
// config.versionMismatch but the helper is intentionally duplicated
// (one line, no shared mutable state) so the package boundary
// doesn't force exporting an internal predicate.
func studioVersionMismatchesForWarning(v string) bool {
	if v == "" {
		return false
	}
	major := v
	for i := 0; i < len(v); i++ {
		if v[i] == '.' || v[i] == '-' || v[i] == '+' {
			major = v[:i]
			break
		}
	}
	if major == "" {
		return true
	}
	if major[0] < '0' || major[0] > '9' {
		return true
	}
	return major < "1"
}
