package commands

import (
	"fmt"
	"path/filepath"
)

// surfaceResolutionIssues reports the two documented diagnostics about which
// surface file a feature is actually being read from.
//
// surface.schema.md's "File resolution" section has specified both since the
// surface.md → surface.yaml migration landed, and neither fired: the strings
// existed only as keys in the severity table, so nothing ever produced them.
// A designer with a stale surface.md beside a live surface.yaml got no warning
// that half their edits were inert, and a designer still on surface.md alone
// was never pointed at `parlay migrate-spec`.
//
// The check lives here rather than in ValidateSurface because it is a property
// of the feature *directory*, not of either file's contents — ValidateSurface
// is handed one path and cannot see what else is present.
func surfaceResolutionIssues(featureDir string) []readinessIssue {
	yamlPath := filepath.Join(featureDir, "surface.yaml")
	mdPath := filepath.Join(featureDir, "surface.md")
	hasYAML := fileExistsAt(yamlPath)
	hasMD := fileExistsAt(mdPath)

	switch {
	case hasYAML && hasMD:
		// Warning, not an error: the project is correct and buildable — the
		// YAML wins. What is worth saying is that the .md is now inert, since
		// editing it and seeing no effect is the confusing outcome.
		return []readinessIssue{{
			Severity: "warning",
			Code:     "surface-md-superseded",
			Message: fmt.Sprintf("%s and %s both exist; the YAML form wins and the Markdown form is inert",
				filepath.Base(yamlPath), filepath.Base(mdPath)),
			Fix: fmt.Sprintf("delete %s once you have confirmed %s carries everything it did",
				mdPath, filepath.Base(yamlPath)),
		}}
	case hasMD:
		// Informational. surface.md remains a supported input, so this must
		// not block anything — it exists to make the migration discoverable.
		return []readinessIssue{{
			Severity: "warning",
			Code:     "surface-md-legacy-format",
			Message:  fmt.Sprintf("%s is the legacy Markdown surface form", mdPath),
			Fix:      "run `parlay migrate-spec` to emit an equivalent surface.yaml; the migrator is idempotent and never deletes your file",
		}}
	}
	return nil
}
