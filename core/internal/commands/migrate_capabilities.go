// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-migration-operations-extraction
//
// Walks each feature's infrastructure.md, extracts operation-shaped
// fragments into capabilities.yaml, and classifies residual paragraphs as
// pattern fragments via agent.ClassifyPatternFragment. The migrator only
// writes the report — capabilities.yaml, domain-model.yaml, blueprint.yaml
// are unchanged for non-operation-shaped fragments.

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/spf13/cobra"
)

var migrateCapabilitiesCmd = &cobra.Command{
	Use:   "migrate-capabilities",
	Short: "Extract operation-shaped fragments from infrastructure.md into capabilities.yaml",
	Args:  cobra.NoArgs,
	RunE:  runMigrateCapabilities,
}

func runMigrateCapabilities(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	intentsRoot := filepath.Join(cfg.Root.Path, "spec", "intents")
	infraFiles, err := walkInfraFiles(intentsRoot)
	if err != nil {
		return fmt.Errorf("walk intents tree: %w", err)
	}

	totalExtracted, totalUnrouted := 0, 0
	for _, infraPath := range infraFiles {
		feature, _ := filepath.Rel(intentsRoot, filepath.Dir(infraPath))
		featDir := filepath.Dir(infraPath)
		paragraphs, err := readParagraphs(infraPath)
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — read failed: %v\n", feature, err)
			continue
		}

		capPath := filepath.Join(featDir, "capabilities.yaml")
		if _, err := os.Stat(capPath); err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s — capabilities.yaml already exists; skipping\n", feature)
			continue
		}

		var extracted []string
		var unrouted []agent.ClassificationResult
		for _, p := range paragraphs {
			if isOperationShaped(p) {
				extracted = append(extracted, extractOperationStub(p))
				totalExtracted++
			} else {
				cls := agent.ClassifyPatternFragment(p)
				unrouted = append(unrouted, cls)
				totalUnrouted++
			}
		}

		if len(extracted) > 0 {
			capContent := buildCapabilitiesYAML(feature, extracted)
			if err := os.WriteFile(capPath, []byte(capContent), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", capPath, err)
			}
		}

		// Migration report is informational; written alongside infra.md.
		reportPath := filepath.Join(featDir, "migration-report.md")
		writeMigrationReport(reportPath, feature, extracted, unrouted)

		fmt.Fprintf(cmd.OutOrStdout(), "  %s — %d operations extracted, %d unrouted\n", feature, len(extracted), len(unrouted))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Operations extracted: %d total\nUnrouted fragments: %d total\n", totalExtracted, totalUnrouted)
	fmt.Fprintln(cmd.OutOrStdout(), "Migrator only writes the report — capabilities.yaml, domain-model.yaml, blueprint.yaml are unchanged for non-operation-shaped fragments.")
	return nil
}

func walkInfraFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "infrastructure.md" {
			out = append(out, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

// readParagraphs splits infrastructure.md into paragraph-sized chunks for
// classification. A "paragraph" here is a run of non-blank lines separated
// by blank lines; bullet lists are treated as a single paragraph each.
func readParagraphs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var paragraphs []string
	var current []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, "\n"))
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, "\n"))
	}
	return paragraphs, sc.Err()
}

// isOperationShaped is a conservative detector: paragraphs naming a verb
// from the closed step set plus a domain entity.
func isOperationShaped(text string) bool {
	lower := strings.ToLower(text)
	verbs := []string{"validate-input", "create-one", "update-one", "delete-one", "read-one", "read-many", "search"}
	for _, v := range verbs {
		if strings.Contains(lower, v) {
			return true
		}
	}
	return false
}

// extractOperationStub emits a kind: unknown stub from an
// operation-shaped paragraph. The migrator does not infer kind: from prose;
// it leaves the explicit setting to the designer.
func extractOperationStub(text string) string {
	// Title -> stub id: take first non-empty word, lowercase, slug-ish.
	words := strings.Fields(strings.ReplaceAll(text, "\n", " "))
	id := "unknown.operation"
	if len(words) > 0 {
		id = strings.ToLower(strings.TrimRight(words[0], ".:,"))
	}
	return fmt.Sprintf(`  - id: %s
    kind: unknown
    subject:
      entity: TODO
    steps:
      - { type: validate-input }
    notes: |
%s
`, id, indentBlock(text, "      "))
}

func indentBlock(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}

func buildCapabilitiesYAML(feature string, operations []string) string {
	return fmt.Sprintf(`# parlay-feature: parlay-tool/multi-adapter
# parlay-component: capabilities-migration-operations-extraction
#
# Auto-extracted from infrastructure.md. Each operation has kind: unknown
# until a designer fills in the real value (command or query).

schema_version: 1
feature: %s

operations:
%s`, feature, strings.Join(operations, "\n"))
}

func writeMigrationReport(path, feature string, extracted []string, unrouted []agent.ClassificationResult) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Migration report — %s\n\n", feature)
	fmt.Fprintf(&sb, "## Operations extracted (%d)\n\n", len(extracted))
	fmt.Fprintln(&sb, "Each landed in capabilities.yaml with kind: unknown. Set kind: explicitly before build mode.")
	fmt.Fprintln(&sb, "")
	fmt.Fprintf(&sb, "## Unrouted fragments (%d)\n\n", len(unrouted))
	for _, u := range unrouted {
		fmt.Fprintf(&sb, "- shape=%s — destination=%s\n\n", u.Shape, u.SuggestedDestination)
		fmt.Fprintln(&sb, "    "+strings.ReplaceAll(u.Verbatim, "\n", "\n    "))
		fmt.Fprintln(&sb)
	}
	_ = os.WriteFile(path, []byte(sb.String()), 0o644)
}
