package config

import (
	"os"
	"path/filepath"
)

// ValidateChildRootForbiddenDirectories scans the child root for any
// repo-level-only directory and returns the first violation found, or
// nil if the child is clean. Single-root and parent roots should skip
// this check.
//
// surfacePaths is the list of agent-surface paths gathered from the
// deployer registry by the cobra entry layer; passing it as a parameter
// keeps this package deployer-agnostic and avoids a circular import.
// Each path is interpreted relative to the child root.
func ValidateChildRootForbiddenDirectories(child Root, surfacePaths []string) *ForbiddenDirectoryViolation {
	if child.Kind != RootKindChild {
		return nil
	}

	// Schemas always live at the parent.
	schemasDir := filepath.Join(child.Path, ParlayDir, SchemasDir)
	if dirExists(schemasDir) {
		return &ForbiddenDirectoryViolation{
			ChildRootPath: child.Path,
			Directory:     ensureTrailingSlash(schemasDir),
			Rule:          RuleSchemasParentOnly,
		}
	}

	// Agent surface lives at the parent.
	for _, sub := range surfacePaths {
		path := filepath.Join(child.Path, sub)
		if dirExists(path) || fileExists(path) {
			return &ForbiddenDirectoryViolation{
				ChildRootPath: child.Path,
				Directory:     ensureTrailingSlash(path),
				Rule:          RuleAgentSurfaceParentOnly,
			}
		}
	}
	return nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// ensureTrailingSlash adds a trailing "/" when the path looks like a
// directory (no extension, exists or not). Files keep their literal path.
func ensureTrailingSlash(p string) string {
	if p == "" {
		return p
	}
	if p[len(p)-1] == '/' {
		return p
	}
	if filepath.Ext(p) != "" {
		return p
	}
	return p + "/"
}
