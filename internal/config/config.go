package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	AIAgent            string `yaml:"ai-agent"`
	SDDFramework       string `yaml:"sdd-framework"`
	PrototypeFramework string `yaml:"prototype-framework"`
}

const (
	ParlayDir   = ".parlay"
	ConfigFile  = "config.yaml"
	SchemasDir  = "schemas"
	AdaptersDir = "adapters"
	BuildDir    = "build"
	SpecDir     = "spec"
	IntentsDir  = "intents"
	HandoffDir  = "handoff"
	PagesDir    = "pages"
)

func ConfigPath() string {
	return filepath.Join(ParlayDir, ConfigFile)
}

func SchemasPath() string {
	return filepath.Join(ParlayDir, SchemasDir)
}

func FeaturePath(slug string) string {
	return filepath.Join(SpecDir, IntentsDir, slug)
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
func BuildPath(slug string) string {
	return filepath.Join(ParlayDir, BuildDir, slug)
}

// HandoffRoot is the root directory for engineering handoff artifacts.
func HandoffRoot() string {
	return filepath.Join(SpecDir, HandoffDir)
}

// HandoffPath is the per-feature directory for engineering handoff artifacts
// (specification.md and any future handoff content).
func HandoffPath(slug string) string {
	return filepath.Join(SpecDir, HandoffDir, slug)
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
