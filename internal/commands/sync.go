package commands

// Generated from buildfile component: coverage-report
// Type: report | Widget: sectioned-output | Layout: report-output

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var syncCmdImpl = &cobra.Command{
	Use:   "sync <@feature>",
	Short: "Check intent-dialog coverage",
	Args:  cobra.ExactArgs(1),
	RunE:  runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	// Data input: feature-ref from command-argument (strip @ prefix)
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)

	// Operation: read-file intents.md, parse using intent-schema
	intentsPath := filepath.Join(featurePath, "intents.md")
	intents, err := parser.ParseIntentsFile(intentsPath)
	if err != nil {
		return fmt.Errorf("failed to read intents: %w", err)
	}

	// Operation: read-file dialogs.md, parse using dialog-schema
	dialogsPath := filepath.Join(featurePath, "dialogs.md")
	dialogs, err := parser.ParseDialogsFile(dialogsPath)
	if err != nil {
		return fmt.Errorf("failed to read dialogs: %w", err)
	}

	// Computed: match intents to dialogs by title, slug, and word overlap
	type coverageMatch struct {
		IntentTitle string
		DialogTitle string
	}

	var covered []coverageMatch
	var uncoveredIntents []parser.Intent
	matchedDialogs := make(map[string]bool)

	for _, intent := range intents {
		found := false
		for _, dialog := range dialogs {
			if matchesIntent(intent, dialog) {
				covered = append(covered, coverageMatch{intent.Title, dialog.Title})
				matchedDialogs[dialog.Slug] = true
				found = true
				break
			}
		}
		if !found {
			uncoveredIntents = append(uncoveredIntents, intent)
		}
	}

	var orphanDialogs []parser.Dialog
	for _, dialog := range dialogs {
		if !matchedDialogs[dialog.Slug] {
			orphanDialogs = append(orphanDialogs, dialog)
		}
	}

	// Element: all-clear (visible-when: uncovered == 0 && orphans == 0)
	if len(uncoveredIntents) == 0 && len(orphanDialogs) == 0 {
		fmt.Println("All intents are covered. No orphan dialogs.")
		return nil
	}

	// Element: covered-header + covered-list (visible-when: covered.length > 0)
	if len(covered) > 0 {
		fmt.Println("Covered intents:")
		for _, m := range covered {
			fmt.Printf("  %s — matched by %s\n", m.IntentTitle, m.DialogTitle)
		}
		fmt.Println()
	}

	// Element: uncovered-header + uncovered-list (visible-when: uncovered.length > 0)
	if len(uncoveredIntents) > 0 {
		fmt.Println("Intents without dialogs:")
		for _, intent := range uncoveredIntents {
			fmt.Printf("  %s — no matching dialog found\n", intent.Title)
		}
		fmt.Println()
	}

	// Element: orphan-header + orphan-list (visible-when: orphan.length > 0)
	if len(orphanDialogs) > 0 {
		fmt.Println("Orphan dialogs (no matching intent):")
		for _, dialog := range orphanDialogs {
			fmt.Printf("  %s — doesn't trace to any intent\n", dialog.Title)
		}
		fmt.Println()
	}

	// Action: template-prompt (selection → lettered-prompt, visible-when: uncovered.length > 0)
	if len(uncoveredIntents) > 0 {
		fmt.Println("Generate dialog templates for uncovered intents?")
		fmt.Println("  A: Yes, generate templates for all")
		fmt.Println("  B: Let me pick which ones")
		fmt.Println("  C: No, just the report")
		fmt.Print("> ")

		reader := bufio.NewReader(os.Stdin)
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToUpper(choice))

		switch choice {
		case "A":
			// Action: generate-all (file-operation, enabled-when: selected == A)
			return appendDialogTemplates(uncoveredIntents, dialogsPath)

		case "B":
			// Action: pick-specific (selection → lettered-prompt, enabled-when: selected == B)
			selected := promptForSelection(uncoveredIntents, reader)
			if len(selected) > 0 {
				return appendDialogTemplates(selected, dialogsPath)
			}
			fmt.Println("No intents selected.")

		case "C":
			// Action: dismiss (enabled-when: selected == C) — no-op
		}
	}

	return nil
}

func promptForSelection(intents []parser.Intent, reader *bufio.Reader) []parser.Intent {
	fmt.Println("Which intents should I generate templates for?")
	for i, intent := range intents {
		letter := string(rune('A' + i))
		fmt.Printf("  %s: %s\n", letter, intent.Title)
	}
	fmt.Print("Enter letters (e.g., A,C): ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToUpper(input))

	var selected []parser.Intent
	for _, ch := range strings.Split(input, ",") {
		ch = strings.TrimSpace(ch)
		if len(ch) == 1 {
			idx := int(ch[0] - 'A')
			if idx >= 0 && idx < len(intents) {
				selected = append(selected, intents[idx])
			}
		}
	}
	return selected
}

func appendDialogTemplates(intents []parser.Intent, dialogsPath string) error {
	f, err := os.OpenFile(dialogsPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open dialogs.md: %w", err)
	}
	defer f.Close()

	for _, intent := range intents {
		tmpl := generateDialogTemplate(intent)
		if _, err := f.WriteString(tmpl); err != nil {
			return err
		}
	}

	fmt.Printf("Added %d dialog templates to dialogs.md.\n", len(intents))
	return nil
}

func matchesIntent(intent parser.Intent, dialog parser.Dialog) bool {
	if strings.EqualFold(intent.Title, dialog.Title) {
		return true
	}
	if intent.Slug == dialog.Slug {
		return true
	}

	intentWords := significantWords(intent.Title)
	dialogWords := significantWords(dialog.Title)
	overlap := wordOverlap(intentWords, dialogWords)

	if len(intentWords) > 0 && len(dialogWords) > 0 {
		minLen := len(intentWords)
		if len(dialogWords) < minLen {
			minLen = len(dialogWords)
		}
		if float64(overlap)/float64(minLen) >= 0.6 {
			return true
		}
	}

	return false
}

func significantWords(s string) []string {
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"for": true, "to": true, "in": true, "on": true, "of": true,
		"with": true, "from": true, "by": true, "is": true, "it": true,
	}
	var words []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		if !stop[w] {
			words = append(words, w)
		}
	}
	return words
}

func wordOverlap(a, b []string) int {
	set := make(map[string]bool)
	for _, w := range a {
		set[w] = true
	}
	count := 0
	for _, w := range b {
		if set[w] {
			count++
		}
	}
	return count
}
