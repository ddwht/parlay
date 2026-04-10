package commands

// Surface generation.
// Agent-enhanced: use /parlay-create-surface skill
// CLI fallback: basic heuristic generation (one fragment per intent)

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var createSurfaceCmdImpl = &cobra.Command{
	Use:   "create-surface <@feature>",
	Short: "Generate surface from intents and dialogs (basic mode — use /parlay-create-surface skill for AI-enhanced generation)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreateSurface,
}

func runCreateSurface(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)
	intentsPath := filepath.Join(featurePath, "intents.md")
	dialogsPath := filepath.Join(featurePath, "dialogs.md")
	surfacePath := filepath.Join(featurePath, "surface.md")

	intents, err := parser.ParseIntentsFile(intentsPath)
	if err != nil {
		return fmt.Errorf("failed to read intents: %w", err)
	}
	dialogs, _ := parser.ParseDialogsFile(dialogsPath)

	if len(intents) == 0 {
		return fmt.Errorf("no intents found in %s — write some intents first", intentsPath)
	}

	existingFragments := make(map[string]bool)
	if data, err := os.ReadFile(surfacePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "## ") {
				existingFragments[strings.TrimSpace(strings.TrimPrefix(line, "## "))] = true
			}
		}
	}

	dialogBySlug := make(map[string]parser.Dialog)
	for _, d := range dialogs {
		dialogBySlug[d.Slug] = d
	}

	type fragment struct{ Name, Shows, Actions, Source string }
	var newFragments []fragment
	skipped := 0

	for _, intent := range intents {
		if existingFragments[intent.Title] {
			skipped++
			continue
		}
		frag := fragment{
			Name:   intent.Title,
			Shows:  intent.Goal,
			Source: fmt.Sprintf("@%s/%s", slug, intent.Slug),
		}
		if d, ok := dialogBySlug[intent.Slug]; ok {
			frag.Actions = deriveActionsFromDialog(d)
		} else if intent.Action != "" {
			frag.Actions = intent.Action
		}
		newFragments = append(newFragments, frag)
	}

	if len(newFragments) == 0 {
		fmt.Println("All intents already have surface fragments. Nothing to generate.")
		return nil
	}

	displayName := toTitleCase(strings.ReplaceAll(slug, "-", " "))
	if len(existingFragments) == 0 {
		os.WriteFile(surfacePath, []byte(fmt.Sprintf("# %s — Surface\n\n", displayName)), 0644)
	}

	f, err := os.OpenFile(surfacePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open surface.md: %w", err)
	}
	defer f.Close()

	for _, frag := range newFragments {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("---\n\n## %s\n\n**Shows**: %s\n", frag.Name, frag.Shows))
		if frag.Actions != "" {
			b.WriteString(fmt.Sprintf("**Actions**: %s\n", frag.Actions))
		}
		b.WriteString(fmt.Sprintf("**Source**: %s\n\n", frag.Source))
		f.WriteString(b.String())
	}

	fmt.Printf("Generated %d fragments in surface.md (basic mode):\n", len(newFragments))
	for _, frag := range newFragments {
		fmt.Printf("  %s — %s\n", frag.Name, truncate(frag.Shows, 60))
	}
	if skipped > 0 {
		fmt.Printf("Skipped %d intents that already have fragments.\n", skipped)
	}
	fmt.Println()
	fmt.Println("For AI-enhanced generation, use the /parlay-create-surface skill.")

	return nil
}

func deriveActionsFromDialog(dialog parser.Dialog) string {
	var actions []string
	seen := make(map[string]bool)
	for _, turn := range dialog.Turns {
		for _, opt := range turn.Options {
			if !opt.IsFreeform {
				desc := strings.TrimSpace(strings.ReplaceAll(opt.Desc, "==", ""))
				if desc != "" && !seen[desc] {
					actions = append(actions, desc)
					seen[desc] = true
				}
			}
		}
	}
	return strings.Join(actions, ", ")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
