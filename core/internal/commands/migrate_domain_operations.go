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
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateDomainOperationsCmd = &cobra.Command{
	Use:   "migrate-domain-operations",
	Short: "Migrate deprecated domain-model.operations entries into per-feature capabilities.yaml stubs",
	Args:  cobra.NoArgs,
	RunE:  runMigrateDomainOperations,
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
		if len(candidates) > 1 {
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
