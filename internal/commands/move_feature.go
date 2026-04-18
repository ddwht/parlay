// parlay-feature: move-feature
// parlay-component: MoveResult

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var moveToFlag string
var moveOutFlag bool

var moveFeatureCmd = &cobra.Command{
	Use:   "move-feature <@feature>",
	Short: "Move a feature between initiatives or in/out of orphan state",
	Args:  cobra.ExactArgs(1),
	RunE:  runMoveFeature,
}

func init() {
	moveFeatureCmd.Flags().StringVar(&moveToFlag, "to", "", "Target initiative to move the feature into")
	moveFeatureCmd.Flags().BoolVar(&moveOutFlag, "out", false, "Move the feature to top-level orphan state")
}

func runMoveFeature(cmd *cobra.Command, args []string) error {
	if moveToFlag != "" && moveOutFlag {
		return fmt.Errorf("`--to` and `--out` are mutually exclusive. Use `--to <initiative>` to move into an initiative, or `--out` to move to the top level — not both")
	}
	if moveToFlag == "" && !moveOutFlag {
		return fmt.Errorf("missing destination. Use `--to <initiative>` to move into an initiative, or `--out` to move to the top level")
	}

	identifier := strings.TrimPrefix(args[0], "@")

	intentsRoot := filepath.Join(config.SpecDir, config.IntentsDir)
	sourcePath := config.FeaturePath(identifier)

	info, err := os.Stat(sourcePath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("feature %q not found. No directory exists at %s/", identifier, sourcePath)
	}

	cls, clsErr := config.ClassifyDir(sourcePath)
	if clsErr != nil {
		return clsErr
	}
	if cls == config.DirClassInitiative {
		slug := filepath.Base(sourcePath)
		return fmt.Errorf("`%s` is an initiative, not a feature. Only features can be moved with parlay move-feature. To rename an initiative, use parlay rename-initiative", slug)
	}

	featureSlug := filepath.Base(sourcePath)
	var destIdentifier string

	if moveOutFlag {
		destIdentifier = featureSlug
	} else {
		initSlug := parser.Slugify(moveToFlag)

		initPath := filepath.Join(intentsRoot, initSlug)
		if config.HasIntentsMd(initPath) {
			return fmt.Errorf("`%s` exists at the top level as a feature, not an initiative. A feature and an initiative can't share a top-level slug. Either pick a different initiative name, or first move the existing `%s` feature into an initiative", initSlug, initSlug)
		}

		destIdentifier = initSlug + "/" + featureSlug
	}

	if identifier == destIdentifier {
		fmt.Printf("Feature @%s is already at the target location — no change.\n", identifier)
		return nil
	}

	destPath := config.FeaturePath(destIdentifier)
	if _, statErr := os.Stat(destPath); statErr == nil {
		return fmt.Errorf("feature `%s` already exists at %s/. Rename one of the features before retrying the move", featureSlug, destPath)
	}

	initiativeCreated := false
	if moveToFlag != "" {
		initSlug := parser.Slugify(moveToFlag)
		initPath := filepath.Join(intentsRoot, initSlug)
		if _, statErr := os.Stat(initPath); os.IsNotExist(statErr) {
			for _, root := range threeTreeRoots() {
				if mkErr := os.MkdirAll(filepath.Join(root, initSlug), 0755); mkErr != nil {
					return fmt.Errorf("creating initiative directory in %s: %w", root, mkErr)
				}
			}
			initiativeCreated = true
		}
	}

	roots := threeTreeRoots()
	sourceRelPath := strings.TrimPrefix(sourcePath, intentsRoot+"/")
	destRelPath := strings.TrimPrefix(destPath, intentsRoot+"/")

	var completed []int
	for i, root := range roots {
		src := filepath.Join(root, sourceRelPath)
		dst := filepath.Join(root, destRelPath)

		if _, statErr := os.Stat(src); os.IsNotExist(statErr) {
			completed = append(completed, i)
			continue
		}

		if mvErr := gitMvOrRename(src, dst); mvErr != nil {
			for _, j := range completed {
				rollbackSrc := filepath.Join(roots[j], destRelPath)
				rollbackDst := filepath.Join(roots[j], sourceRelPath)
				os.Rename(rollbackSrc, rollbackDst)
			}
			return fmt.Errorf("move failed on %s tree: %w. Rolled back all trees — feature remains at @%s", root, mvErr, identifier)
		}
		completed = append(completed, i)
	}

	if initiativeCreated {
		fmt.Printf("Initiative %s created.\n", parser.Slugify(moveToFlag))
	}
	fmt.Println("Feature moved:")
	fmt.Printf("  Before: @%s (%s/)\n", identifier, sourcePath)
	fmt.Printf("  After:  @%s (%s/)\n", destIdentifier, destPath)
	fmt.Println("All three trees updated in lockstep. Git history preserved via `git mv`.")

	return nil
}

func gitMvOrRename(src, dst string) error {
	dstParent := filepath.Dir(dst)
	if err := os.MkdirAll(dstParent, 0755); err != nil {
		return err
	}

	if isGitRepo() {
		cmd := exec.Command("git", "mv", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git mv: %s — %w", strings.TrimSpace(string(out)), err)
		}
		return nil
	}
	return os.Rename(src, dst)
}

func isGitRepo() bool {
	_, err := os.Stat(".git")
	return err == nil
}
