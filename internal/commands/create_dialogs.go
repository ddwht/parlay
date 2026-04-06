package commands

// Generated from buildfile component: dialog-template-report
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

var createDialogsCmd = &cobra.Command{
	Use:   "create-dialogs <@feature>",
	Short: "Scaffold dialog templates from intents",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreateDialogs,
}

func runCreateDialogs(cmd *cobra.Command, args []string) error {
	// Data input: feature-ref from command-argument (strip @ prefix)
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)

	// Operation: read-file intents.md, parse using intent-schema
	intentsPath := filepath.Join(featurePath, "intents.md")
	intents, err := parser.ParseIntentsFile(intentsPath)
	if err != nil {
		return fmt.Errorf("failed to read intents: %w", err)
	}

	if len(intents) == 0 {
		return fmt.Errorf("no intents found in %s — write some intents first", intentsPath)
	}

	// Element: intent-count (text-output → fmt.Println)
	fmt.Printf("Found %d intents.\n", len(intents))

	// Computed: existing-dialogs from read dialogs.md (raw content for duplicate detection)
	dialogsPath := filepath.Join(featurePath, "dialogs.md")
	existing, _ := os.ReadFile(dialogsPath)
	existingContent := string(existing)

	// Computed: new-intents = intents where title not in existing-dialogs
	var newIntents []parser.Intent
	skipped := 0
	for _, intent := range intents {
		if strings.Contains(existingContent, "### "+intent.Title) {
			skipped++
			continue
		}
		newIntents = append(newIntents, intent)
	}

	// Element: all-covered (visible-when: new-intents.length == 0)
	if len(newIntents) == 0 {
		fmt.Println("All intents already have dialog templates. Nothing to generate.")
		return nil
	}

	// Operation: for-each new-intents, generate-template
	var templates []string
	for _, intent := range newIntents {
		templates = append(templates, generateDialogTemplate(intent))
	}

	// Operation: append-file dialogs.md
	f, err := os.OpenFile(dialogsPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open dialogs.md: %w", err)
	}
	defer f.Close()

	for _, tmpl := range templates {
		if _, err := f.WriteString(tmpl); err != nil {
			return err
		}
	}

	// Element: template-count (text-output → fmt.Println, visible-when: new-intents.length > 0)
	fmt.Printf("Added %d dialog templates to dialogs.md.\n", len(newIntents))

	// Element: skip-count (text-output → fmt.Println, visible-when: skipped > 0)
	if skipped > 0 {
		fmt.Printf("Skipped %d intents that already have dialogs.\n", skipped)
	}

	// Element: next-step (text-output → fmt.Println, visible-when: new-intents.length > 0)
	fmt.Println()
	fmt.Println("Review and rewrite them to capture the real conversation.")

	return nil
}

// generateDialogTemplate produces a template following the buildfile specification:
// title from intent.title, trigger from intent.action (if present),
// placeholder turns: "User: ==describe what the user does==" and "System: ==respond to help user: {goal}=="
func generateDialogTemplate(intent parser.Intent) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("### %s\n\n", intent.Title))

	if intent.Action != "" {
		b.WriteString(fmt.Sprintf("**Trigger**: %s\n\n", intent.Action))
	}

	b.WriteString("User: ==describe what the user does==\n")
	if intent.Goal != "" {
		b.WriteString(fmt.Sprintf("System: ==respond to help user: %s==\n", intent.Goal))
	} else {
		b.WriteString("System: ==system response==\n")
	}

	b.WriteString("\n---\n\n")
	return b.String()
}
