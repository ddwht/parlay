package commands

// parlay-feature: initiatives
// parlay-component: EmptyInitiativeCreationResult

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var newInitiativeCmd = &cobra.Command{
	Use:   "new-initiative <name>",
	Short: "Create an empty initiative directory across the three trees",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runNewInitiative,
}

func runNewInitiative(cmd *cobra.Command, args []string) error {
	name := strings.Join(args, " ")
	slug := parser.Slugify(name)

	intentsRoot := filepath.Join(config.SpecDir, config.IntentsDir)
	initiativePath := filepath.Join(intentsRoot, slug)

	if config.HasIntentsMd(initiativePath) {
		return fmt.Errorf("`%s` exists at the top level as a feature. An initiative can't share a top-level slug with a feature. Pick a different name, or first move the existing feature into an initiative with parlay move-feature", slug)
	}

	if info, err := os.Stat(initiativePath); err == nil && info.IsDir() {
		fmt.Printf("Initiative %s already exists at %s/ — no changes made.\n", slug, initiativePath)
		return nil
	}

	for _, root := range threeTreeRoots() {
		if err := os.MkdirAll(filepath.Join(root, slug), 0755); err != nil {
			return fmt.Errorf("creating initiative directory in %s: %w", root, err)
		}
	}

	handoffPath := filepath.Join(config.SpecDir, config.HandoffDir, slug)
	buildPath := filepath.Join(config.BuildRoot(), slug)

	fmt.Printf("Initiative %s created at %s/ (with matching parallel paths under %s/ and %s/).\n", slug, initiativePath, handoffPath, buildPath)
	fmt.Println("The directory is empty and in deferred classification — it becomes a proper initiative once it contains at least one feature subdirectory. A README.md on its own does not change that — it is narrative, not a classification signal.")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  - Add features with parlay add-feature <name> --initiative %s\n", slug)
	fmt.Printf("  - Move existing features in with parlay move-feature @<feature> --to %s\n", slug)
	fmt.Printf("  - Optionally write %s/README.md with the initiative's \"why\" and scope notes\n", initiativePath)

	return nil
}
