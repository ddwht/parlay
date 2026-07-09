// parlay-feature: parlay-tool/multi-adapter
// parlay-component: domain-operations-migration-prompt
//
// Reads the project domain-model.yaml, walks each entry under operations:,
// asks the designer which feature owns it (when ambiguous), and writes a
// stub with kind: unknown into the chosen feature's capabilities.yaml.

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateDomainOperationsCmd = &cobra.Command{
	Use:   "migrate-domain-operations",
	Short: "Migrate deprecated domain-model.operations entries into per-feature capabilities.yaml stubs",
	Args:  cobra.NoArgs,
	RunE:  runMigrateDomainOperations,
}

var (
	migrateDomainOperationsFeature        string
	migrateDomainOperationsNonInteractive bool
)

func init() {
	migrateDomainOperationsCmd.Flags().StringVar(&migrateDomainOperationsFeature, "feature", "",
		"Explicit target feature (e.g. @task-list) used whenever an operation has more than one candidate feature")
	migrateDomainOperationsCmd.Flags().BoolVar(&migrateDomainOperationsNonInteractive, "non-interactive", false,
		"Force headless mode even when a TTY is attached — ambiguous targeting then hard-errors instead of prompting (mirrors build-feature's --non-interactive contract)")
}

type domainModelShape struct {
	Operations []map[string]interface{} `yaml:"operations,omitempty"`
	Entities   []domainEntity           `yaml:"entities,omitempty"`
}

type domainEntity struct {
	Name string `yaml:"name"`
}

func runMigrateDomainOperations(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	dmPath := cfg.DomainModelPath()
	content, err := os.ReadFile(dmPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(cmd.OutOrStdout(), "no domain-model.yaml found; nothing to migrate")
			return nil
		}
		return fmt.Errorf("read domain-model.yaml: %w", err)
	}

	var dm domainModelShape
	if err := yaml.Unmarshal(content, &dm); err != nil {
		return fmt.Errorf("parse domain-model.yaml: %w", err)
	}
	if len(dm.Operations) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "domain-model.operations is empty; nothing to migrate")
		return nil
	}

	intentsRoot := filepath.Join(cfg.Root.Path, "spec", "intents")
	features, _ := os.ReadDir(intentsRoot)

	// Headless doctrine (mirrors build-feature steps 7.6-7.9): the flag
	// wins over TTY detection in both directions. With --non-interactive
	// set, this run is headless even if a TTY is attached; without it, a
	// missing TTY still triggers headless behavior automatically.
	headless := migrateDomainOperationsNonInteractive || !ttyInteractive(nil)

	explicitFeature := ""
	if migrateDomainOperationsFeature != "" {
		explicitFeature = parser.FeatureSlug(migrateDomainOperationsFeature)
	}

	reader := bufio.NewReader(os.Stdin)
	migrated := 0
	for _, op := range dm.Operations {
		entity, _ := op["entity"].(string)
		title, _ := op["title"].(string)
		if title == "" {
			title, _ = op["name"].(string)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\noperation: %s (entity: %s)\n", title, entity)
		var candidates []string
		for _, f := range features {
			if f.IsDir() {
				candidates = append(candidates, f.Name())
			}
		}

		choice := candidates[0]
		switch {
		case len(candidates) <= 1:
			// Unambiguous (or no candidates at all) — keep the existing
			// single-candidate behavior unchanged.
		case explicitFeature != "":
			if !slices.Contains(candidates, explicitFeature) {
				return fmt.Errorf("--feature %q is not a candidate feature for operation %q (candidates: %s)",
					explicitFeature, title, strings.Join(candidates, ", "))
			}
			choice = explicitFeature
		case headless:
			// Never guess in headless mode — hard error instead, per the
			// migrate-domain-operations skill's headless contract.
			return fmt.Errorf("ambiguous-target: operation %q (entity: %s) matches %d candidate features (%s) — "+
				"headless/non-interactive mode does not guess; re-run with --feature <slug> naming the target",
				title, entity, len(candidates), strings.Join(candidates, ", "))
		default:
			fmt.Fprintln(cmd.OutOrStdout(), "candidate features:")
			for i, c := range candidates {
				fmt.Fprintf(cmd.OutOrStdout(), "  [%d] %s\n", i+1, c)
			}
			fmt.Fprint(cmd.OutOrStdout(), "pick (number) > ")
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line != "" {
				var idx int
				if _, err := fmt.Sscanf(line, "%d", &idx); err == nil && idx >= 1 && idx <= len(candidates) {
					choice = candidates[idx-1]
				}
			}
		}

		stub := fmt.Sprintf(`  - id: unknown.%s
    kind: unknown
    subject:
      entity: %s
    notes: |
      Migrated from domain-model.operations: %s
`, slugify(title), entity, title)

		appendStubToCapabilities(filepath.Join(intentsRoot, choice), choice, stub)
		fmt.Fprintf(cmd.OutOrStdout(), "  -> wrote stub to %s/capabilities.yaml\n", choice)
		migrated++
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nMigrated %d entries; clear domain-model.operations after review\n", migrated)
	return nil
}

func slugify(s string) string {
	out := strings.ToLower(strings.TrimSpace(s))
	out = strings.ReplaceAll(out, " ", "-")
	out = strings.ReplaceAll(out, "_", "-")
	if out == "" {
		out = "operation"
	}
	return out
}

func appendStubToCapabilities(featDir, feature, stub string) {
	capPath := filepath.Join(featDir, "capabilities.yaml")
	if _, err := os.Stat(capPath); os.IsNotExist(err) {
		header := fmt.Sprintf("schema_version: 1\nfeature: %s\n\noperations:\n", feature)
		_ = os.WriteFile(capPath, []byte(header+stub), 0o644)
		return
	}
	content, _ := os.ReadFile(capPath)
	out := append(content, []byte("\n"+stub)...)
	_ = os.WriteFile(capPath, out, 0o644)
}
