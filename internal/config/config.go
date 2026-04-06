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
	SpecDir     = "spec"
	IntentsDir  = "intents"
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
