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

	// surface.md stopped being a runtime surface artifact in v0.3: the
	// resolver no longer falls back to it, so an .md-only feature has NO
	// surface at all, and a dual-form feature carries a stale mirror that
	// three benchmark replicates out of three measured actively misleading
	// readers. Either state is a hard error naming the migration out.
	switch {
	case hasYAML && hasMD:
		return []readinessIssue{{
			Severity: "error",
			Code:     "surface-md-unsupported",
			Message: fmt.Sprintf("%s exists beside %s; surface.md was removed as a surface artifact in v0.3 and nothing reads it",
				filepath.Base(mdPath), filepath.Base(yamlPath)),
			Fix: "run `parlay migrate-spec --retire-md` (refuses per-feature if the .md carries fragments the .yaml lacks), then `parlay internal scaffold-signatures @{feature}`",
		}}
	case hasMD:
		return []readinessIssue{{
			Severity: "error",
			Code:     "surface-md-unsupported",
			Message:  fmt.Sprintf("%s is the pre-v0.3 Markdown surface form; the runtime no longer reads it", mdPath),
			Fix:      "run `parlay migrate-spec` to emit surface.yaml (idempotent), review it, then `parlay migrate-spec --retire-md`",
		}}
	}
	return nil
}
