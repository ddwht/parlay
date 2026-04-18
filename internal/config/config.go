// parlay-section: cross-cutting
// parlay-extends: qualified-identifier-resolver/qualified-path-resolver
// parlay-extends: qualified-identifier-resolver/feature-enumeration-helper
// parlay-extends: initiatives/directory-classification-validation
// parlay-extends: initiatives/duplicate-slug-detection
// parlay-extends: initiatives/cross-tree-traversal-consistency

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	AIAgent            string `yaml:"ai-agent"`
	SDDFramework       string `yaml:"sdd-framework"`
	PrototypeFramework string `yaml:"prototype-framework"`
}

const (
	ParlayDir     = ".parlay"
	ConfigFile    = "config.yaml"
	BlueprintFile = "blueprint.yaml"
	SchemasDir    = "schemas"
	AdaptersDir   = "adapters"
	BuildDir      = "build"
	SpecDir       = "spec"
	IntentsDir    = "intents"
	HandoffDir    = "handoff"
	PagesDir      = "pages"
)

func ConfigPath() string {
	return filepath.Join(ParlayDir, ConfigFile)
}

func SchemasPath() string {
	return filepath.Join(ParlayDir, SchemasDir)
}

func BlueprintPath() string {
	return filepath.Join(ParlayDir, BlueprintFile)
}

func FeaturePath(identifier string) string {
	return resolveQualifiedPath(identifier, filepath.Join(SpecDir, IntentsDir))
}

func AdaptersPath() string {
	return filepath.Join(ParlayDir, AdaptersDir)
}

func PagesPath() string {
	return filepath.Join(SpecDir, PagesDir)
}

// BuildRoot is the root directory for tool-internal build artifacts.
func BuildRoot() string {
	return filepath.Join(ParlayDir, BuildDir)
}

// BuildPath is the per-feature directory for tool-internal build artifacts
// (buildfile.yaml, testcases.yaml, .baseline.yaml).
func BuildPath(identifier string) string {
	return resolveQualifiedPath(identifier, filepath.Join(ParlayDir, BuildDir))
}

// HandoffRoot is the root directory for engineering handoff artifacts.
func HandoffRoot() string {
	return filepath.Join(SpecDir, HandoffDir)
}

// HandoffPath is the per-feature directory for engineering handoff artifacts
// (specification.md and any future handoff content).
func HandoffPath(identifier string) string {
	return resolveQualifiedPath(identifier, filepath.Join(SpecDir, HandoffDir))
}

// ProjectBuildPath is the directory for project-level build state
// (merged section baseline, project code-hashes). Cross-cutting files
// that serve all features are tracked here, not per-feature.
func ProjectBuildPath() string {
	return filepath.Join(ParlayDir, BuildDir, "_project")
}

func Load() (*ProjectConfig, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(cfg *ProjectConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0644)
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

var (
	featureTreeOnce   sync.Once
	featureTreeResult []string
	featureTreeErr    error
)

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

func AllFeatures() ([]string, error) {
	return AllFeaturePaths(filepath.Join(SpecDir, IntentsDir))
}

// AllFeaturePaths walks the given tree root and returns qualified identifiers
// for all features. When treeRoot differs from spec/intents/, classification
// is performed against spec/intents/ (the authoritative source) so that
// features missing from the requested tree are still enumerated.
func AllFeaturePaths(treeRoot string) ([]string, error) {
	intentsRoot := filepath.Join(SpecDir, IntentsDir)

	if treeRoot != intentsRoot {
		featureTreeOnce.Do(func() {
			featureTreeResult, featureTreeErr = scanFeatureTree(intentsRoot)
		})
		if featureTreeErr != nil {
			return nil, featureTreeErr
		}
		return featureTreeResult, nil
	}

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
