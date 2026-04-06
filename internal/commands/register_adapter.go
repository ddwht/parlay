package commands

// Generated from buildfile component: adapter-registration
// Type: command-output | Widget: cobra-command | Layout: file-generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/parlay/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var registerAdapterCmd = &cobra.Command{
	Use:   "register-adapter <path>",
	Short: "Register a framework adapter",
	Args:  cobra.ExactArgs(1),
	RunE:  runRegisterAdapter,
}

type adapterFile struct {
	Name           string                 `yaml:"name"`
	Framework      string                 `yaml:"framework"`
	Version        string                 `yaml:"version"`
	ComponentTypes map[string]interface{} `yaml:"component-types"`
	ElementTypes   map[string]interface{} `yaml:"element-types"`
	ActionTypes    map[string]interface{} `yaml:"action-types"`
	LayoutPatterns map[string]interface{} `yaml:"layout-patterns"`
	FileConventions map[string]interface{} `yaml:"file-conventions"`
}

func runRegisterAdapter(cmd *cobra.Command, args []string) error {
	// Data input: adapter-path from command-argument
	adapterPath := args[0]

	// Operation: read-file, parse using adapter-schema, validate
	data, err := os.ReadFile(adapterPath)
	if err != nil {
		return fmt.Errorf("failed to read adapter file: %w", err)
	}

	var adapter adapterFile
	if err := yaml.Unmarshal(data, &adapter); err != nil {
		return fmt.Errorf("failed to parse adapter: %w", err)
	}

	if adapter.Name == "" {
		return fmt.Errorf("adapter file missing 'name' field")
	}

	// Element: adapter-name (text-output → fmt.Println)
	fmt.Printf("Registered framework adapter %q:\n", adapter.Name)

	// Element: component-count (text-output → fmt.Println)
	fmt.Printf("  Component types: %d\n", len(adapter.ComponentTypes))

	// Element: pattern-count (text-output → fmt.Println)
	fmt.Printf("  Layout patterns: %d\n", len(adapter.LayoutPatterns))

	// Element: conventions (text-output → fmt.Println)
	if sr, ok := adapter.FileConventions["source-root"]; ok {
		fmt.Printf("  File conventions: %s\n", sr)
	}

	// Operation: create-directory ".parlay/adapters/"
	adaptersDir := config.AdaptersPath()
	if err := os.MkdirAll(adaptersDir, 0755); err != nil {
		return fmt.Errorf("failed to create adapters directory: %w", err)
	}

	// Operation: copy-file to .parlay/adapters/{name}.adapter.yaml
	dstPath := filepath.Join(adaptersDir, adapter.Name+".adapter.yaml")
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("failed to copy adapter: %w", err)
	}

	fmt.Println()
	fmt.Printf("Adapter saved to %s\n", dstPath)
	fmt.Println("Set it as the prototype framework in .parlay/config.yaml to use it with build-feature.")

	return nil
}
