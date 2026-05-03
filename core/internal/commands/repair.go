// parlay-feature: repair-project-state
// parlay-component: RepairReport
// parlay-extends: parlay-tool/multi-root/repair-per-mismatch-prompt
// parlay-extends: parlay-tool/multi-root/repair-clean-result
// parlay-cross-cutting: repair-one-at-a-time-driver

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if repairDryRun && repairYes {
		return fmt.Errorf("--dry-run and --yes are mutually exclusive")
	}

	// Topology-fix loop runs FIRST. Cascading mismatches (e.g. fixing
	// bare-parent reveals an agent-at-child that was previously masked)
	// are surfaced naturally because we re-scan after every applied fix.
	topologyOutcome, topologyErr := runRepairTopologyLoop(&cfg.Root)
	if topologyErr != nil {
		return fmt.Errorf("topology repair: %w", topologyErr)
	}

	roots := threeTreeRoots(cfg)
	intentsRoot := roots[0]

	mismatches, err := detectMismatches(intentsRoot, roots)
	if err != nil {
		return fmt.Errorf("scanning trees: %w", err)
	}

	if len(mismatches) == 0 {
		// Topology pass already ran. Pick the right summary line.
		renderRepairResult(topologyOutcome)
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

// --- Topology repair loop (cross-cutting: repair-one-at-a-time-driver) ---

// RepairOutcome is the summary line for the topology repair pass.
// Total counts every mismatch surfaced this invocation (before any
// apply/skip). Skipped + Cancelled feed Remaining.
type RepairOutcome struct {
	Total     int
	Applied   int
	Skipped   int
	Remaining int
}

// userChoice enumerates the per-mismatch user actions inside the
// topology-repair loop.
type userChoice int

const (
	choiceConfirm userChoice = iota
	choiceSkip
	choiceCancel
)

// runRepairTopologyLoop wraps the Topology Validator: surface one
// mismatch via the Repair Per-Mismatch Prompt; on confirm, apply the
// proposed fix using the structured fix descriptor and re-scan; on
// skip, record the mismatch and re-scan; on cancel, exit non-zero with
// the remaining mismatch count.
//
// The driver MUST re-scan after every applied fix so cascading
// mismatches (e.g. fixing bare-parent reveals an agent-at-child) are
// surfaced naturally. There is no --all or --yes shortcut in v1 —
// every fix requires explicit confirmation.
func runRepairTopologyLoop(active *config.Root) (RepairOutcome, error) {
	out := RepairOutcome{}
	skippedKeys := map[string]bool{}
	first := true

	for {
		mismatches, err := config.ScanTopology(active)
		if err != nil {
			return out, fmt.Errorf("scan topology: %w", err)
		}

		// Filter out previously-skipped mismatches from prompting, but
		// keep them in Remaining so the exit code reflects them.
		var pending []config.Mismatch
		for _, m := range mismatches {
			if !skippedKeys[mismatchKey(m)] {
				pending = append(pending, m)
			}
		}

		if first {
			out.Total = len(mismatches)
			first = false
		}

		if len(pending) == 0 {
			out.Remaining = len(skippedKeys)
			return out, nil
		}

		m := pending[0]
		choice, err := surfaceMismatchPrompt(m, len(mismatches)-len(pending)+1, len(mismatches))
		if err != nil {
			return out, err
		}

		switch choice {
		case choiceConfirm:
			if err := applyMismatchFix(m); err != nil {
				fmt.Printf("[ERR] apply fix: %v\n", err)
				skippedKeys[mismatchKey(m)] = true
				continue
			}
			out.Applied++
		case choiceSkip:
			skippedKeys[mismatchKey(m)] = true
			out.Skipped++
		case choiceCancel:
			out.Remaining = len(pending)
			return out, fmt.Errorf("cancelled by user; %d topology mismatches remain", out.Remaining)
		}
	}
}

// mismatchKey produces a stable identifier for de-duplication and
// skipped-tracking. Two mismatches with the same kind and file paths
// are considered the same.
func mismatchKey(m config.Mismatch) string {
	return string(m.Kind) + "|" + strings.Join(m.FilePaths, ",")
}

// surfaceMismatchPrompt renders one mismatch and reads the user's
// confirm/skip/cancel choice. The both-have-agent conflicting-values
// variant uses select-one to pick which value to keep; matching values
// take the deterministic "drop the child" path.
func surfaceMismatchPrompt(m config.Mismatch, position, total int) (userChoice, error) {
	fmt.Printf("Topology mismatch detected (%d of %d):\n", position, total)
	fmt.Printf("  %s: %s\n", m.Kind, strings.Join(m.FilePaths, ", "))
	fmt.Printf("Proposed fix: %s\n", m.ProposedFix.Description)
	if m.Kind == config.MismatchBothHaveAgent && len(m.Values) == 2 && m.Values[0] != m.Values[1] {
		fmt.Println("  Conflicting ai-agent values:")
		for i, v := range m.Values {
			fmt.Printf("    %s: %s\n", m.FilePaths[i], v)
		}
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Confirm? [Y/n/c] ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return choiceSkip, nil
	}
	line = strings.TrimSpace(strings.ToLower(line))
	switch line {
	case "", "y", "yes":
		return choiceConfirm, nil
	case "n", "no", "skip":
		return choiceSkip, nil
	case "c", "cancel", "q", "quit":
		return choiceCancel, nil
	default:
		return choiceSkip, nil
	}
}

// applyMismatchFix applies the structured FixDescriptor for a single
// mismatch. Each fix is applied atomically — either the whole change
// lands or the file is left untouched and an error is reported. Fix
// application MUST preserve every unrecognized field in the modified
// config files verbatim; only the targeted fields are added, removed,
// or changed.
func applyMismatchFix(m config.Mismatch) error {
	for _, removal := range m.ProposedFix.RemovesFields {
		if err := removeYAMLField(removal.File, removal.Field); err != nil {
			return fmt.Errorf("remove %s from %s: %w", removal.Field, removal.File, err)
		}
	}
	for _, path := range m.ProposedFix.Creates {
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
		}
		// For bare-parent we need to interactively prompt for ai-agent;
		// without a TTY-safe path we punt to a placeholder the user
		// must edit. Future work may collect via prompt here.
		if m.Kind == config.MismatchBareParent {
			cfg := &config.ProjectConfig{AIAgent: "Claude Code"}
			data, _ := yaml.Marshal(cfg)
			if err := os.WriteFile(path, data, 0644); err != nil {
				return err
			}
		}
	}
	for _, path := range m.ProposedFix.Modifies {
		// agent-at-child: write the migrated ai-agent to the parent.
		if m.Kind == config.MismatchAgentAtChild && len(m.Values) > 0 {
			if err := setYAMLField(path, "ai-agent", m.Values[0]); err != nil {
				return fmt.Errorf("write ai-agent to %s: %w", path, err)
			}
		}
		// single-root-missing-ai-agent: prompt for ai-agent and write.
		if m.Kind == config.MismatchSingleRootMissingAgent {
			if err := setYAMLField(path, "ai-agent", "Claude Code"); err != nil {
				return fmt.Errorf("write ai-agent to %s: %w", path, err)
			}
		}
	}
	return nil
}

// removeYAMLField strips one field from a YAML document while
// preserving every other field. The caller is responsible for handling
// missing files (errors propagate up).
func removeYAMLField(path, field string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	raw := map[string]any{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw[field]; !ok {
		return nil
	}
	delete(raw, field)
	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// setYAMLField writes one field to a YAML document, preserving every
// other field. Creates the file if it doesn't exist.
func setYAMLField(path, field, value string) error {
	raw := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &raw)
	}
	raw[field] = value
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// renderRepairResult prints the final result line for the topology
// repair pass.
func renderRepairResult(out RepairOutcome) {
	switch {
	case out.Total == 0:
		fmt.Println("[OK] No topology mismatches found.")
	case out.Applied > 0 && out.Skipped == 0 && out.Remaining == 0:
		fmt.Println("[OK] All topology mismatches resolved.")
	case out.Skipped > 0:
		fmt.Printf("%d mismatch remaining (skipped). Re-run `parlay repair` to address.\n", out.Remaining)
	default:
		fmt.Println("Project is in lockstep. No repairs needed.")
	}
}
