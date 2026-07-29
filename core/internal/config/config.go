// parlay-section: cross-cutting
// parlay-extends: qualified-identifier-resolver/qualified-path-resolver
// parlay-extends: qualified-identifier-resolver/feature-enumeration-helper
// parlay-extends: initiatives/directory-classification-validation
// parlay-extends: initiatives/duplicate-slug-detection
// parlay-extends: initiatives/cross-tree-traversal-consistency
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	AIAgent            string `yaml:"ai-agent"`
	SDDFramework       string `yaml:"sdd-framework"`
	PrototypeFramework string `yaml:"prototype-framework"`

	// NoStudio mirrors the parlay.no_studio key in
	// .parlay/config.yaml: when true, the trio-command Studio prompt
	// is suppressed for every invocation in this project, regardless
	// of the --no-studio flag. The merge with the per-invocation
	// flag is logical OR — either source suppresses the prompt.
	//
	// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands
	NoStudio bool `yaml:"no_studio,omitempty"`
}

const (
	ParlayDir     = ".parlay"
	ConfigFile    = "config.yaml"
	BlueprintFile = "blueprint.yaml"
	SchemasDir    = "schemas"
	ModulesDir    = "modules"
	AdaptersDir   = "adapters"
	BuildDir      = "build"
	SpecDir       = "spec"
	IntentsDir    = "intents"
	HandoffDir    = "handoff"
	PagesDir      = "pages"
)

// loadProjectConfigAt reads a ProjectConfig from a specific filesystem
// path. Used by the Context-aware (*Context).LoadProjectConfig and by
// the legacy single-root bootstrap path in `parlay init` (which writes
// the very first .parlay/config.yaml before any active root exists).
func loadProjectConfigAt(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveProjectConfigAt(path string, cfg *ProjectConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// --- Qualified identifier resolver (cross-cutting: qualified-path-resolver) ---

func resolveQualifiedPath(identifier, treeRoot string) string {
	if strings.Contains(identifier, "/") {
		parts := strings.SplitN(identifier, "/", 2)
		return filepath.Join(treeRoot, parts[0], parts[1])
	}
	return filepath.Join(treeRoot, identifier)
}

// --- Feature enumeration helper (cross-cutting: feature-enumeration-helper) ---
// --- Extended by: initiatives/directory-classification-validation ---
// --- Extended by: initiatives/duplicate-slug-detection ---
// --- Extended by: initiatives/cross-tree-traversal-consistency ---

// DirClass represents the classification of a directory under spec/intents/.
type DirClass int

const (
	DirClassFeature    DirClass = 1
	DirClassInitiative DirClass = 2
	DirClassDeferred   DirClass = 3
)

// ClassifyDir examines a directory and returns its classification.
// Feature: contains intents.md directly. Initiative: contains direct-child
// subdirectories with intents.md (checks only direct children, never recurses).
// Deferred: matches neither rule. Returns an error for hybrid directories
// (both intents.md and child dirs with intents.md).
func ClassifyDir(path string) (DirClass, error) {
	isFeature := hasIntentsMd(path)

	children, err := os.ReadDir(path)
	if err != nil {
		if isFeature {
			return DirClassFeature, nil
		}
		return DirClassDeferred, nil
	}

	hasChildFeatures := false
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		if hasIntentsMd(filepath.Join(path, child.Name())) {
			hasChildFeatures = true
			break
		}
	}

	if isFeature && hasChildFeatures {
		return 0, fmt.Errorf("hybrid directory at %s: contains intents.md (feature) and subdirectories with intents.md (initiative) — a directory cannot be both", path)
	}
	if isFeature {
		return DirClassFeature, nil
	}
	if hasChildFeatures {
		return DirClassInitiative, nil
	}
	return DirClassDeferred, nil
}

// CheckSlugUniqueness verifies that no two sibling directories under parentDir
// slugify to the same identifier. Returns an error listing conflicting paths
// when duplicates are found. This guards against external filesystem corruption.
func CheckSlugUniqueness(parentDir string) error {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return nil
	}

	slugMap := make(map[string][]string)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := slugifyDirName(entry.Name())
		slugMap[slug] = append(slugMap[slug], filepath.Join(parentDir, entry.Name()))
	}

	for slug, paths := range slugMap {
		if len(paths) > 1 {
			return fmt.Errorf("duplicate slug %q under %s: directories %s resolve to the same identifier — remove or rename one, then run parlay repair", slug, parentDir, strings.Join(paths, " and "))
		}
	}
	return nil
}

func slugifyDirName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

// ScanFeatureTree walks the given tree root and returns qualified
// feature identifiers. This is the sole public entry point for feature
// enumeration — every caller, single-root or multi-root, should resolve
// its own tree root (typically via (*Context).IntentsRoot() and
// (*Context).AllFeatures(), the Context-bound wrapper below) and call
// this directly.
//
// A previous package-level AllFeatures()/AllFeaturePaths() pair existed
// here with a sync.Once-cached result keyed to a single cwd-relative
// spec/intents/ path. That cache was unsound for anything but a single
// cwd-anchored invocation per process — every multi-root caller, and
// every test that constructs more than one project tree in the same
// test binary, would have silently read another root's stale cached
// result. Nothing in this codebase depended on the free functions by
// the time this was noticed (every real caller already went through
// (*Context).AllFeatures()), so they were removed rather than fixed.
func ScanFeatureTree(treeRoot string) ([]string, error) {
	return scanFeatureTree(treeRoot)
}

func scanFeatureTree(treeRoot string) ([]string, error) {
	entries, err := os.ReadDir(treeRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", treeRoot, err)
	}

	if err := CheckSlugUniqueness(treeRoot); err != nil {
		return nil, err
	}

	var result []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		topSlug := entry.Name()
		topPath := filepath.Join(treeRoot, topSlug)

		cls, err := ClassifyDir(topPath)
		if err != nil {
			return nil, err
		}

		switch cls {
		case DirClassFeature:
			result = append(result, topSlug)
		case DirClassInitiative:
			if err := CheckSlugUniqueness(topPath); err != nil {
				return nil, err
			}
			children, childErr := os.ReadDir(topPath)
			if childErr != nil {
				continue
			}
			for _, child := range children {
				if !child.IsDir() {
					continue
				}
				childPath := filepath.Join(topPath, child.Name())
				childCls, childClsErr := ClassifyDir(childPath)
				if childClsErr != nil {
					return nil, childClsErr
				}
				if childCls == DirClassFeature {
					result = append(result, topSlug+"/"+child.Name())
				}
				if childCls == DirClassInitiative {
					return nil, fmt.Errorf("sub-initiative at %s: contains subdirectories with intents.md at depth 2, violating the flat-hierarchy rule — initiatives can only be direct children of %s", childPath, treeRoot)
				}
			}
		case DirClassDeferred:
			// valid, silently skipped in enumeration
		}
	}

	return result, nil
}

func hasIntentsMd(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "intents.md"))
	if err != nil {
		_, err = os.Stat(filepath.Join(dir, "Intents.md"))
	}
	return err == nil
}

// HasIntentsMd is the exported form for use by commands that need
// to check directory classification without the full traversal.
func HasIntentsMd(dir string) bool {
	return hasIntentsMd(dir)
}
