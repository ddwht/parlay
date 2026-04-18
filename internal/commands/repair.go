// parlay-feature: repair-project-state
// parlay-component: RepairReport

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var repairDryRun bool
var repairYes bool

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Validate and reconcile the three parallel trees",
	RunE:  runRepair,
}

func init() {
	repairCmd.Flags().BoolVar(&repairDryRun, "dry-run", false, "Report mismatches without applying repairs or prompting")
	repairCmd.Flags().BoolVar(&repairYes, "yes", false, "Auto-confirm unambiguous repairs (still pauses on ambiguous)")
}

type mismatch struct {
	Category string // initiative-rename, feature-move, missing-directory, extra-directory, ambiguous, unrecognized
	OldPath  string
	NewPath  string
	Tree     string
	Paths    []string
	Detail   string
}

func runRepair(cmd *cobra.Command, args []string) error {
	if repairDryRun && repairYes {
		return fmt.Errorf("--dry-run and --yes are mutually exclusive")
	}

	roots := threeTreeRoots()
	intentsRoot := roots[0]

	mismatches, err := detectMismatches(intentsRoot, roots)
	if err != nil {
		return fmt.Errorf("scanning trees: %w", err)
	}

	if len(mismatches) == 0 {
		fmt.Println("Project is in lockstep. No repairs needed.")
		return nil
	}

	fmt.Printf("Detected %d inconsistencies:\n\n", len(mismatches))

	applied, failed, unresolved := 0, 0, 0

	for i, m := range mismatches {
		fmt.Printf("**Mismatch %d: %s**\n", i+1, m.Category)
		for _, p := range m.Paths {
			fmt.Printf("  %s\n", p)
		}

		if repairDryRun {
			if m.Category == "unrecognized" {
				fmt.Println("  [WOULD SKIP] Unrecognized — requires manual resolution.")
			} else {
				fmt.Printf("  [WOULD FIX] %s\n", m.Detail)
			}
			fmt.Println()
			continue
		}

		switch m.Category {
		case "missing-directory":
			fmt.Printf("  Recreate %s? [Y/n] ", m.NewPath)
			if repairYes || promptYesDefault(true) {
				if mkErr := os.MkdirAll(m.NewPath, 0755); mkErr != nil {
					fmt.Printf("  [ERR] %v. This repair was rolled back.\n", mkErr)
					failed++
				} else {
					fmt.Printf("  [OK] Created %s\n", m.NewPath)
					applied++
				}
			} else {
				unresolved++
			}

		case "extra-directory":
			count := countFiles(m.OldPath)
			fmt.Printf("  Delete %s (%d files)? [y/N] ", m.OldPath, count)
			if promptYesDefault(false) {
				if rmErr := os.RemoveAll(m.OldPath); rmErr != nil {
					fmt.Printf("  [ERR] %v. This repair was rolled back.\n", rmErr)
					failed++
				} else {
					fmt.Printf("  [OK] Deleted %s (%d files)\n", m.OldPath, count)
					applied++
				}
			} else {
				fmt.Printf("  Kept %s — no changes.\n", m.OldPath)
				unresolved++
			}

		case "initiative-rename", "feature-move":
			fmt.Printf("  %s? [Y/n] ", m.Detail)
			if repairYes || promptYesDefault(true) {
				if mvErr := os.Rename(m.OldPath, m.NewPath); mvErr != nil {
					fmt.Printf("  [ERR] %v. This repair was rolled back.\n", mvErr)
					failed++
				} else {
					fmt.Printf("  [OK] %s\n", m.Detail)
					applied++
				}
			} else {
				unresolved++
			}

		case "unrecognized":
			fmt.Println("  Unresolved — doesn't fit any repair category. Please resolve manually.")
			unresolved++

		default:
			unresolved++
		}
		fmt.Println()
	}

	if !repairDryRun {
		fmt.Printf("Repair complete. Applied %d, failed %d, unresolved %d.\n", applied, failed, unresolved)
	}

	if unresolved > 0 || failed > 0 {
		os.Exit(1)
	}
	return nil
}

func detectMismatches(intentsRoot string, roots []string) ([]mismatch, error) {
	intentsDirs, err := listFeatureDirs(intentsRoot)
	if err != nil {
		return nil, err
	}

	var mismatches []mismatch
	for _, relPath := range intentsDirs {
		for _, root := range roots[1:] {
			fullPath := filepath.Join(root, relPath)
			if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
				mismatches = append(mismatches, mismatch{
					Category: "missing-directory",
					NewPath:  fullPath,
					Paths: []string{
						fmt.Sprintf("%s/%s (exists)", intentsRoot, relPath),
						fmt.Sprintf("%s (missing)", fullPath),
					},
					Detail: fmt.Sprintf("Recreate %s", fullPath),
				})
			}
		}
	}

	for _, root := range roots[1:] {
		otherDirs, _ := listFeatureDirs(root)
		for _, relPath := range otherDirs {
			intentsPath := filepath.Join(intentsRoot, relPath)
			if _, statErr := os.Stat(intentsPath); os.IsNotExist(statErr) {
				fullPath := filepath.Join(root, relPath)
				mismatches = append(mismatches, mismatch{
					Category: "extra-directory",
					OldPath:  fullPath,
					Paths: []string{
						fmt.Sprintf("%s (%d files, no source in %s)", fullPath, countFiles(fullPath), intentsRoot),
					},
					Detail: fmt.Sprintf("Delete %s", fullPath),
				})
			}
		}
	}

	return mismatches, nil
}

func listFeatureDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, e.Name())

		subEntries, subErr := os.ReadDir(filepath.Join(root, e.Name()))
		if subErr != nil {
			continue
		}
		for _, sub := range subEntries {
			if sub.IsDir() && !strings.HasPrefix(sub.Name(), "_") {
				dirs = append(dirs, filepath.Join(e.Name(), sub.Name()))
			}
		}
	}

	sort.Strings(dirs)
	return dirs, nil
}

func countFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

func promptYesDefault(defaultYes bool) bool {
	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}
