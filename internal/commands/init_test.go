package commands

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/anthropics/parlay/internal/config"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInit_CreatesProjectStructure(t *testing.T) {
	dir := setupTestDir(t)

	cfg := &config.ProjectConfig{
		AIAgent:            "Claude Code",
		SDDFramework:       "GitHub SpecKit",
		PrototypeFramework: "Angular + Clarity",
	}

	os.MkdirAll(config.ParlayDir, 0755)
	os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755)
	config.Save(cfg)

	// Verify .parlay/config.yaml
	if _, err := os.Stat(filepath.Join(dir, ".parlay", "config.yaml")); os.IsNotExist(err) {
		t.Error("config.yaml not created")
	}

	// Verify config content
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if loaded.AIAgent != "Claude Code" {
		t.Errorf("AIAgent = %q, want %q", loaded.AIAgent, "Claude Code")
	}
	if loaded.SDDFramework != "GitHub SpecKit" {
		t.Errorf("SDDFramework = %q, want %q", loaded.SDDFramework, "GitHub SpecKit")
	}
	if loaded.PrototypeFramework != "Angular + Clarity" {
		t.Errorf("PrototypeFramework = %q, want %q", loaded.PrototypeFramework, "Angular + Clarity")
	}

	// Verify spec/intents/ exists
	if _, err := os.Stat(filepath.Join(dir, "spec", "intents")); os.IsNotExist(err) {
		t.Error("spec/intents/ not created")
	}
}

func TestInit_ConfigYAMLRoundtrip(t *testing.T) {
	setupTestDir(t)

	cfg := &config.ProjectConfig{
		AIAgent:            "Claude Code",
		SDDFramework:       "GitHub SpecKit",
		PrototypeFramework: "Angular + Clarity",
	}

	os.MkdirAll(config.ParlayDir, 0755)
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(config.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]string
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("config.yaml is not valid YAML: %v", err)
	}

	if parsed["ai-agent"] != "Claude Code" {
		t.Errorf("ai-agent = %q, want %q", parsed["ai-agent"], "Claude Code")
	}
}
