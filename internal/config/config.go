// parlay-section: cross-cutting
// parlay-extends: qualified-identifier-resolver/qualified-path-resolver
// parlay-extends: qualified-identifier-resolver/feature-enumeration-helper

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

var (
	featureTreeOnce sync.Once
	featureTreeErr  error
	featureTreeMap  []featureEntry
)

type featureEntry struct {
	QualifiedID    string
	Classification string // "feature", "initiative", "deferred"
}

func AllFeatures() ([]string, error) {
	return AllFeaturePaths(filepath.Join(SpecDir, IntentsDir))
}

func AllFeaturePaths(treeRoot string) ([]string, error) {
	entries, err := os.ReadDir(treeRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", treeRoot, err)
	}

	var result []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		topSlug := entry.Name()
		topPath := filepath.Join(treeRoot, topSlug)

		if hasIntentsMd(topPath) {
			result = append(result, topSlug)
			continue
		}

		children, err := os.ReadDir(topPath)
		if err != nil {
			continue
		}

		isInitiative := false
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			childPath := filepath.Join(topPath, child.Name())
			if hasIntentsMd(childPath) {
				isInitiative = true
				result = append(result, topSlug+"/"+child.Name())

				grandchildren, _ := os.ReadDir(childPath)
				for _, gc := range grandchildren {
					if gc.IsDir() {
						gcPath := filepath.Join(childPath, gc.Name())
						if hasIntentsMd(gcPath) {
							return nil, fmt.Errorf("depth-2+ initiative-like structure at %s violates flat-hierarchy rule", gcPath)
						}
					}
				}
			}
		}
		if !isInitiative {
			// deferred — skip
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
