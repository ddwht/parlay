package commands

// Generated from buildfile component: feature-scaffold-confirmation
// Type: command-output | Widget: cobra-command | Layout: file-generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var addFeatureCmd = &cobra.Command{
	Use:   "add-feature <name>",
	Short: "Create a new feature folder with intents.md and dialogs.md",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runAddFeature,
}

func runAddFeature(cmd *cobra.Command, args []string) error {
	// Data input: feature-name from command-argument
	name := strings.Join(args, " ")

	// Computed: slug from Slugify(feature-name)
	slug := parser.Slugify(name)

	// Computed: path from spec/intents/{slug}/
	featurePath := config.FeaturePath(slug)

	if _, err := os.Stat(featurePath); err == nil {
		return fmt.Errorf("feature %q already exists at %s", slug, featurePath)
	}

	displayName := toTitleCase(name)

	// Operation: create-directory "spec/intents/{slug}/"
	if err := os.MkdirAll(featurePath, 0755); err != nil {
		return fmt.Errorf("failed to create feature directory: %w", err)
	}

	// Operation: create-file intents.md with template
	intentsContent := fmt.Sprintf("# %s\n\n> \n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "intents.md"), []byte(intentsContent), 0644); err != nil {
		return fmt.Errorf("failed to create intents.md: %w", err)
	}

	// Operation: create-file dialogs.md with template
	dialogsContent := fmt.Sprintf("# %s — Dialogs\n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "dialogs.md"), []byte(dialogsContent), 0644); err != nil {
		return fmt.Errorf("failed to create dialogs.md: %w", err)
	}

	// Element: created-path (path-reference → path-line)
	fmt.Printf("Created feature at %s/\n", featurePath)

	// Element: files-list (data-list → bulleted-list)
	fmt.Println("  intents.md")
	fmt.Println("  dialogs.md")

	// Element: next-step (text-output → fmt.Println)
	fmt.Println()
	fmt.Printf("Start with intents.md. When ready, run: parlay create-dialogs @%s\n", slug)

	return nil
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
