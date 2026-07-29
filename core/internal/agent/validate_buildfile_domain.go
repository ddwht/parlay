package agent

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// domainEntityNames returns the entity names declared in the project's
// canonical domain-model.yaml, resolved relative to a buildfile path.
//
// Why this exists: buildfile.schema.md and the build-feature skill both
// state that a non-empty top-level models: is deprecated and that entity
// declarations belong in domain-model.yaml. The deep validator, however,
// resolved model references *only* against bf.Models — so a buildfile
// written the documented way failed with missing-model-reference, whose
// own fix text told the author to do the opposite of the docs. Every
// buildfile in the wild therefore carries a duplicated copy of the
// entities domain-model.yaml already declares, which is precisely the
// duplication domain-model.yaml exists to remove.
//
// Resolution is best-effort: a missing or unparseable domain model yields
// nil, and validation falls back to models:-only behaviour rather than
// failing. Deprecating the field is a separate step from being able to
// live without it; this makes the latter true first.
func domainEntityNames(buildfilePath string) map[string]bool {
	root := projectRootFromBuildfilePath(buildfilePath)
	if root == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(root, "domain-model.yaml"))
	if err != nil {
		return nil
	}
	var doc struct {
		Entities []struct {
			Name string `yaml:"name"`
		} `yaml:"entities"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, e := range doc.Entities {
		if e.Name != "" {
			out[e.Name] = true
		}
	}
	return out
}

// projectRootFromBuildfilePath derives the active root from a buildfile
// path by locating the ".parlay" path component and taking its parent.
// Works for both flat (.parlay/build/<feature>/) and initiative-nested
// (.parlay/build/<initiative>/<feature>/) layouts, since it keys off
// .parlay rather than counting directory levels.
func projectRootFromBuildfilePath(buildfilePath string) string {
	abs, err := filepath.Abs(buildfilePath)
	if err != nil {
		abs = buildfilePath
	}
	parts := strings.Split(filepath.ToSlash(abs), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == ".parlay" {
			return filepath.FromSlash(strings.Join(parts[:i], "/"))
		}
	}
	return ""
}
