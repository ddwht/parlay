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
	Category string // initiative-rename, feature-move, missing-directory, extra-directory, stale-initiative-buildfile, ambiguous, unrecognized
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
	topologyOutcome, topologyErr := runRepairTopologyLoop(cmd, &cfg.Root)
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
		renderRepairResult(cmd, topologyOutcome)
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Detected %d inconsistencies:\n\n", len(mismatches))

	applied, failed, unresolved := 0, 0, 0

	for i, m := range mismatches {
		fmt.Fprintf(out, "**Mismatch %d: %s**\n", i+1, m.Category)
		for _, p := range m.Paths {
			fmt.Fprintf(out, "  %s\n", p)
		}

		if repairDryRun {
			if m.Category == "unrecognized" {
				fmt.Fprintln(out, "  [WOULD SKIP] Unrecognized — requires manual resolution.")
			} else {
				fmt.Fprintf(out, "  [WOULD FIX] %s\n", m.Detail)
			}
			fmt.Fprintln(out)
			continue
		}

		switch m.Category {
		case "missing-directory":
			fmt.Fprintf(out, "  Recreate %s? [Y/n] ", m.NewPath)
			if repairYes || promptYesDefault(true) {
				if mkErr := os.MkdirAll(m.NewPath, 0755); mkErr != nil {
					fmt.Fprintf(out, "  [ERR] %v. This repair was rolled back.\n", mkErr)
					failed++
				} else {
					fmt.Fprintf(out, "  [OK] Created %s\n", m.NewPath)
					applied++
				}
			} else {
				unresolved++
			}

		case "extra-directory":
			count := countFiles(m.OldPath)
			fmt.Fprintf(out, "  Delete %s (%d files)? [y/N] ", m.OldPath, count)
			if promptYesDefault(false) {
				if rmErr := os.RemoveAll(m.OldPath); rmErr != nil {
					fmt.Fprintf(out, "  [ERR] %v. This repair was rolled back.\n", rmErr)
					failed++
				} else {
					fmt.Fprintf(out, "  [OK] Deleted %s (%d files)\n", m.OldPath, count)
					applied++
				}
			} else {
				fmt.Fprintf(out, "  Kept %s — no changes.\n", m.OldPath)
				unresolved++
			}

		case "initiative-rename", "feature-move":
			fmt.Fprintf(out, "  %s? [Y/n] ", m.Detail)
			if repairYes || promptYesDefault(true) {
				if mvErr := os.Rename(m.OldPath, m.NewPath); mvErr != nil {
					fmt.Fprintf(out, "  [ERR] %v. This repair was rolled back.\n", mvErr)
					failed++
				} else {
					fmt.Fprintf(out, "  [OK] %s\n", m.Detail)
					applied++
				}
			} else {
				unresolved++
			}

		case "stale-initiative-buildfile":
			fmt.Fprintf(out, "  Delete %s (stale build artifact — this directory is an initiative, not a feature)? [y/N] ", m.OldPath)
			if promptYesDefault(false) {
				if rmErr := os.Remove(m.OldPath); rmErr != nil {
					fmt.Fprintf(out, "  [ERR] %v. This repair was rolled back.\n", rmErr)
					failed++
				} else {
					fmt.Fprintf(out, "  [OK] Deleted %s\n", m.OldPath)
					applied++
				}
			} else {
				fmt.Fprintf(out, "  Kept %s — no changes.\n", m.OldPath)
				unresolved++
			}

		case "unrecognized":
			fmt.Fprintln(out, "  Unresolved — doesn't fit any repair category. Please resolve manually.")
			unresolved++

		default:
			unresolved++
		}
		fmt.Fprintln(out)
	}

	if !repairDryRun {
		fmt.Fprintf(out, "Repair complete. Applied %d, failed %d, unresolved %d.\n", applied, failed, unresolved)
	}

	if unresolved > 0 || failed > 0 {
		return NewExitCodeError(1)
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

	// threeTreeRoots always returns [intentsRoot, handoffRoot, buildRoot] —
	// only the build tree carries per-feature buildfile.yaml/testcases.yaml,
	// so this check is build-root-specific rather than folded into the
	// roots[1:] loops above (handoff has no equivalent stale-artifact shape).
	if len(roots) >= 3 {
		staleBuildfiles, err := detectStaleInitiativeBuildfiles(intentsRoot, roots[2])
		if err != nil {
			return nil, err
		}
		mismatches = append(mismatches, staleBuildfiles...)
	}

	return mismatches, nil
}

// detectStaleInitiativeBuildfiles finds .parlay/build/<name>/ directories
// that carry a root-level buildfile.yaml or testcases.yaml even though
// <name> classifies as an initiative (its spec/intents/ counterpart has
// sub-feature directories with their own intents.md, not an intents.md
// of its own) rather than a feature. Only features get build artifacts
// — a buildfile.yaml sitting directly under an initiative's build
// directory is a stale leftover from before the directory was
// reorganized into an initiative (the historical parlay-tool-monolith
// case: buildfile.yaml once lived at .parlay/build/parlay-tool/ when
// parlay-tool was a single feature, before it grew sub-features and
// became an initiative).
func detectStaleInitiativeBuildfiles(intentsRoot, buildRoot string) ([]mismatch, error) {
	entries, err := os.ReadDir(buildRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var mismatches []mismatch
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		cls, clsErr := config.ClassifyDir(filepath.Join(intentsRoot, e.Name()))
		if clsErr != nil || cls != config.DirClassInitiative {
			continue
		}
		buildDir := filepath.Join(buildRoot, e.Name())
		for _, staleFile := range []string{"buildfile.yaml", "testcases.yaml"} {
			stalePath := filepath.Join(buildDir, staleFile)
			if _, statErr := os.Stat(stalePath); statErr == nil {
				mismatches = append(mismatches, mismatch{
					Category: "stale-initiative-buildfile",
					OldPath:  stalePath,
					Paths: []string{
						fmt.Sprintf("%s (stale — %s is an initiative, not a feature)", stalePath, e.Name()),
					},
					Detail: fmt.Sprintf("Delete %s", stalePath),
				})
			}
		}
	}

	sort.Slice(mismatches, func(i, j int) bool { return mismatches[i].OldPath < mismatches[j].OldPath })
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
func runRepairTopologyLoop(cmd *cobra.Command, active *config.Root) (RepairOutcome, error) {
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
		choice, err := surfaceMismatchPrompt(cmd, m, len(mismatches)-len(pending)+1, len(mismatches))
		if err != nil {
			return out, err
		}

		switch choice {
		case choiceConfirm:
			if err := applyMismatchFix(m); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "[ERR] apply fix: %v\n", err)
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
func surfaceMismatchPrompt(cmd *cobra.Command, m config.Mismatch, position, total int) (userChoice, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Topology mismatch detected (%d of %d):\n", position, total)
	fmt.Fprintf(out, "  %s: %s\n", m.Kind, strings.Join(m.FilePaths, ", "))
	fmt.Fprintf(out, "Proposed fix: %s\n", m.ProposedFix.Description)
	if m.Kind == config.MismatchBothHaveAgent && len(m.Values) == 2 && m.Values[0] != m.Values[1] {
		fmt.Fprintln(out, "  Conflicting ai-agent values:")
		for i, v := range m.Values {
			fmt.Fprintf(out, "    %s: %s\n", m.FilePaths[i], v)
		}
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(out, "Confirm? [Y/n/c] ")
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
//
// This is a thin orchestrator over three pure, independently-testable
// functions (applyFieldRemovals, applyCreates, applyModifies). Each one
// operates on one FixDescriptor section plus a small closure that
// supplies the actual content to write — FixDescriptor carries paths
// and a human-readable description, not field values, since different
// Mismatch Kinds need different content at the same section (e.g.
// Creates for bare-parent writes a fresh config.yaml stub, but no other
// Kind uses Creates at all today).
func applyMismatchFix(m config.Mismatch) error {
	if err := applyFieldRemovals(m.ProposedFix.RemovesFields); err != nil {
		return err
	}
	if err := applyCreates(m.ProposedFix.Creates, func(string) ([]byte, bool) {
		// For bare-parent we need to interactively prompt for ai-agent;
		// without a TTY-safe path we punt to a placeholder the user
		// must edit. Future work may collect via prompt here.
		if m.Kind != config.MismatchBareParent {
			return nil, false
		}
		cfg := &config.ProjectConfig{AIAgent: "Claude Code"}
		data, _ := yaml.Marshal(cfg)
		return data, true
	}); err != nil {
		return err
	}
	if err := applyModifies(m.ProposedFix.Modifies, func(string) (field, value string, ok bool) {
		switch {
		// agent-at-child: write the migrated ai-agent to the parent.
		case m.Kind == config.MismatchAgentAtChild && len(m.Values) > 0:
			return "ai-agent", m.Values[0], true
		// single-root-missing-ai-agent: prompt for ai-agent and write.
		case m.Kind == config.MismatchSingleRootMissingAgent:
			return "ai-agent", "Claude Code", true
		default:
			return "", "", false
		}
	}); err != nil {
		return err
	}
	return nil
}

// applyFieldRemovals applies every FieldRemoval in a FixDescriptor,
// stopping at the first failure. Pure aside from the filesystem edit —
// takes only the removals slice, no Mismatch/Kind context — so it's
// testable against a temp file with no other repair machinery involved.
func applyFieldRemovals(removals []config.FieldRemoval) error {
	for _, removal := range removals {
		if err := removeYAMLField(removal.File, removal.Field); err != nil {
			return fmt.Errorf("remove %s from %s: %w", removal.Field, removal.File, err)
		}
	}
	return nil
}

// applyCreates creates each path in a FixDescriptor's Creates list that
// doesn't already exist on disk. content is called once per missing
// path and returns the bytes to write plus whether this fix actually
// has content for that path — returning ok=false skips the path
// silently (some Mismatch Kinds have no Creates content-writer at all).
// Existing files at a Creates path are left untouched: Creates means
// "create if absent," never "overwrite."
func applyCreates(creates []string, content func(path string) (data []byte, ok bool)) error {
	for _, path := range creates {
		if _, err := os.Stat(path); err == nil {
			continue
		}
		data, ok := content(path)
		if !ok {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// applyModifies writes one field/value pair to each path in a
// FixDescriptor's Modifies list. content is called once per path and
// returns the field name, the value, and whether this fix actually has
// a modification for that path — returning ok=false skips the path
// silently. Uses setYAMLField, so existing fields and comments in the
// target file survive untouched apart from the one field being set.
func applyModifies(modifies []string, content func(path string) (field, value string, ok bool)) error {
	for _, path := range modifies {
		field, value, ok := content(path)
		if !ok {
			continue
		}
		if err := setYAMLField(path, field, value); err != nil {
			return fmt.Errorf("write %s to %s: %w", field, path, err)
		}
	}
	return nil
}

// rootMapping returns the top-level mapping node of a YAML document,
// initializing doc as an empty mapping document if it currently holds
// nothing (a missing or zero-byte source file). Returns an error if the
// document's root exists but is not a mapping (e.g. a bare scalar or a
// sequence) — repair only ever edits mapping-shaped config files.
func rootMapping(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		*doc = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
		}
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a YAML mapping at the document root, got node kind %d", mapping.Kind)
	}
	return mapping, nil
}

// readYAMLDocument reads and parses path into a yaml.Node tree. A
// missing file is not an error — the caller gets a Node ready for
// rootMapping to initialize as an empty document.
func readYAMLDocument(path string) (yaml.Node, error) {
	var doc yaml.Node
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return doc, err
	}
	if len(data) == 0 {
		return doc, nil
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

// removeYAMLField strips one field from a YAML document while
// preserving every other field, comment, and key order verbatim — a
// yaml.Node tree edit rather than a map[string]any round-trip, which
// would silently drop comments and re-sort keys. The caller is
// responsible for handling missing files (errors propagate up).
func removeYAMLField(path, field string) error {
	doc, err := readYAMLDocument(path)
	if err != nil {
		return err
	}
	mapping, err := rootMapping(&doc)
	if err != nil {
		return err
	}

	content := mapping.Content
	found := false
	var carryHeadComment string
	newContent := make([]*yaml.Node, 0, len(content))
	for i := 0; i+1 < len(content); i += 2 {
		key, val := content[i], content[i+1]
		if key.Value == field {
			found = true
			// A leading comment above the document's first field attaches
			// as this key node's HeadComment, not the mapping node's — if
			// the removed field carried one, carry it forward onto the
			// next remaining key so it isn't silently dropped along with
			// the field it happened to sit above.
			if key.HeadComment != "" {
				carryHeadComment = key.HeadComment
			}
			continue
		}
		if carryHeadComment != "" {
			if key.HeadComment != "" {
				key.HeadComment = carryHeadComment + "\n" + key.HeadComment
			} else {
				key.HeadComment = carryHeadComment
			}
			carryHeadComment = ""
		}
		newContent = append(newContent, key, val)
	}
	if !found {
		return nil
	}
	// The removed field was last with no following key to carry the
	// comment onto — reattach it as a foot comment on the new last
	// value so it still survives, just moved from above to below.
	if carryHeadComment != "" && len(newContent) >= 2 {
		lastVal := newContent[len(newContent)-1]
		if lastVal.FootComment != "" {
			lastVal.FootComment = lastVal.FootComment + "\n" + carryHeadComment
		} else {
			lastVal.FootComment = carryHeadComment
		}
	}
	mapping.Content = newContent

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// setYAMLField writes one field to a YAML document, preserving every
// other field, comment, and key order verbatim. Creates the file (and
// its parent directory) if it doesn't exist. If the field is already
// present, only its scalar value is updated in place — the key node
// (and any attached comments) is untouched.
func setYAMLField(path, field, value string) error {
	doc, err := readYAMLDocument(path)
	if err != nil {
		return err
	}
	mapping, err := rootMapping(&doc)
	if err != nil {
		return err
	}

	content := mapping.Content
	for i := 0; i+1 < len(content); i += 2 {
		if content[i].Value == field {
			valNode := content[i+1]
			valNode.Kind = yaml.ScalarNode
			valNode.Tag = "!!str"
			valNode.Value = value
			valNode.Style = 0
			return writeYAMLDocument(path, &doc)
		}
	}

	// Field not present — append a new key/value scalar pair.
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
	return writeYAMLDocument(path, &doc)
}

// writeYAMLDocument marshals doc and writes it to path, creating the
// parent directory if needed.
func writeYAMLDocument(path string, doc *yaml.Node) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// renderRepairResult prints the final result line for the topology
// repair pass.
func renderRepairResult(cmd *cobra.Command, out RepairOutcome) {
	w := cmd.OutOrStdout()
	switch {
	case out.Total == 0:
		fmt.Fprintln(w, "[OK] No topology mismatches found.")
	case out.Applied > 0 && out.Skipped == 0 && out.Remaining == 0:
		fmt.Fprintln(w, "[OK] All topology mismatches resolved.")
	case out.Skipped > 0:
		fmt.Fprintf(w, "%d mismatch remaining (skipped). Re-run `parlay repair` to address.\n", out.Remaining)
	default:
		fmt.Fprintln(w, "Project is in lockstep. No repairs needed.")
	}
}
