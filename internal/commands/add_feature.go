package commands

// parlay-feature: parlay-tool
// parlay-component: feature-scaffold-confirmation
// parlay-extends: initiatives/FeatureCreationResult

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var initiativeFlag string

var addFeatureCmd = &cobra.Command{
	Use:   "add-feature <name>",
	Short: "Create a new feature folder with intents.md and dialogs.md",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runAddFeature,
}

func init() {
	addFeatureCmd.Flags().StringVar(&initiativeFlag, "initiative", "", "Create the feature inside this initiative (auto-creates the initiative if needed)")
}

func runAddFeature(cmd *cobra.Command, args []string) error {
	name := strings.Join(args, " ")
	slug := parser.Slugify(name)

	if initiativeFlag != "" {
		return runAddFeatureWithInitiative(name, slug, initiativeFlag)
	}

	featurePath := config.FeaturePath(slug)

	if _, err := os.Stat(featurePath); err == nil {
		return fmt.Errorf("feature %q already exists at %s", slug, featurePath)
	}

	displayName := toTitleCase(name)

	if err := os.MkdirAll(featurePath, 0755); err != nil {
		return fmt.Errorf("creating feature directory: %w", err)
	}

	intentsContent := fmt.Sprintf("# %s\n\n> \n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "intents.md"), []byte(intentsContent), 0644); err != nil {
		return fmt.Errorf("creating intents.md: %w", err)
	}

	dialogsContent := fmt.Sprintf("# %s — Dialogs\n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "dialogs.md"), []byte(dialogsContent), 0644); err != nil {
		return fmt.Errorf("creating dialogs.md: %w", err)
	}

	fmt.Printf("Created feature at %s/\n", featurePath)
	fmt.Println("  intents.md")
	fmt.Println("  dialogs.md")
	fmt.Println()
	fmt.Printf("Start with intents.md. When ready, run: parlay create-dialogs @%s\n", slug)

	return nil
}

// parlay-feature: initiatives
// parlay-component: FeatureCreationResult
func runAddFeatureWithInitiative(name, featureSlug, initiativeName string) error {
	initiativeSlug := parser.Slugify(initiativeName)

	intentsRoot := filepath.Join(config.SpecDir, config.IntentsDir)
	initiativePath := filepath.Join(intentsRoot, initiativeSlug)

	if config.HasIntentsMd(initiativePath) {
		return fmt.Errorf("`%s` exists at the top level as a feature, not an initiative. A feature and an initiative can't share a top-level slug. Either pick a different initiative name, or first move the existing `%s` feature into an initiative with parlay move-feature", initiativeSlug, initiativeSlug)
	}

	featurePath := filepath.Join(initiativePath, featureSlug)
	if _, err := os.Stat(featurePath); err == nil {
		return fmt.Errorf("feature `%s` already exists inside initiative `%s` at %s/. Pick a different feature name, or move the existing feature somewhere else first", featureSlug, initiativeSlug, featurePath)
	}

	initiativeCreated := false
	if _, err := os.Stat(initiativePath); os.IsNotExist(err) {
		for _, root := range threeTreeRoots() {
			if mkErr := os.MkdirAll(filepath.Join(root, initiativeSlug), 0755); mkErr != nil {
				return fmt.Errorf("creating initiative directory in %s: %w", root, mkErr)
			}
		}
		initiativeCreated = true
	}

	for _, root := range threeTreeRoots() {
		if mkErr := os.MkdirAll(filepath.Join(root, initiativeSlug, featureSlug), 0755); mkErr != nil {
			if initiativeCreated {
				fmt.Printf("[WARN] Created initiative %s (in deferred classification — no features yet), but couldn't create feature %s inside it: %v. Re-run the same command after fixing the issue — it's idempotent.\n", initiativeSlug, featureSlug, mkErr)
				return nil
			}
			return fmt.Errorf("creating feature directory in %s: %w", root, mkErr)
		}
	}

	displayName := toTitleCase(name)
	intentsContent := fmt.Sprintf("# %s\n\n> \n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "intents.md"), []byte(intentsContent), 0644); err != nil {
		return fmt.Errorf("creating intents.md: %w", err)
	}
	dialogsContent := fmt.Sprintf("# %s — Dialogs\n\n---\n\n", displayName)
	if err := os.WriteFile(filepath.Join(featurePath, "dialogs.md"), []byte(dialogsContent), 0644); err != nil {
		return fmt.Errorf("creating dialogs.md: %w", err)
	}

	if initiativeCreated {
		fmt.Printf("Initiative %s created.\n", initiativeSlug)
	}
	fmt.Printf("Feature %s added to initiative %s at %s/.\n", featureSlug, initiativeSlug, featurePath)
	fmt.Println()
	fmt.Printf("Start with intents.md. When ready, run: parlay create-dialogs @%s/%s\n", initiativeSlug, featureSlug)

	return nil
}

func threeTreeRoots() []string {
	return []string{
		filepath.Join(config.SpecDir, config.IntentsDir),
		filepath.Join(config.SpecDir, config.HandoffDir),
		config.BuildRoot(),
	}
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
