// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-status-line
//
// FormatStudioStatusLine renders the one-line "Studio detected"
// summary that `parlay status` adds to its output when
// StudioDetection.detected is true. When detection is false the
// helper returns an empty string in normal mode; --verbose surfaces
// a short diagnostic for the not-executable case so designers can
// notice a misconfigured PATH without reading source.

package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
)

// FormatStudioStatusLine returns the line `parlay status` should
// print between the topology line and the feature listing. Empty
// string means "render nothing for this Detection record" — the
// caller then skips the print entirely so default `parlay status`
// stays Studio-silent for the no-Studio case.
//
// Verbose=true surfaces the not-executable diagnostic naming the
// offending path so misconfigured PATHs can be spotted without
// reading source.
func FormatStudioStatusLine(d config.StudioDetection, verbose bool) string {
	if d.Detected {
		return fmt.Sprintf("Studio detected: %s (version %s)", d.BinaryPath, d.Version)
	}
	if verbose && d.Reason == config.StudioReasonNotExecutable && d.BinaryPath != "" {
		return fmt.Sprintf("studio: not detected (found at %s but not executable)", d.BinaryPath)
	}
	return ""
}
