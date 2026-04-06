package commands

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var checkCoverageCmd = &cobra.Command{
	Use:   "check-coverage <@feature>",
	Short: "Check intent-dialog coverage (JSON output for agent consumption)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckCoverage,
}

type coverageOutput struct {
	Feature   string          `json:"feature"`
	Covered   []coverageMatch `json:"covered"`
	Uncovered []string        `json:"uncovered"`
	Orphans   []string        `json:"orphans"`
}

type coverageMatch struct {
	Intent string `json:"intent"`
	Dialog string `json:"dialog"`
}

func runCheckCoverage(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)

	intents, err := parser.ParseIntentsFile(filepath.Join(featurePath, "intents.md"))
	if err != nil {
		return fmt.Errorf("failed to read intents: %w", err)
	}

	dialogs, err := parser.ParseDialogsFile(filepath.Join(featurePath, "dialogs.md"))
	if err != nil {
		return fmt.Errorf("failed to read dialogs: %w", err)
	}

	output := coverageOutput{Feature: slug}
	matchedDialogs := make(map[string]bool)

	for _, intent := range intents {
		found := false
		for _, dialog := range dialogs {
			if matchesIntent(intent, dialog) {
				output.Covered = append(output.Covered, coverageMatch{
					Intent: intent.Title,
					Dialog: dialog.Title,
				})
				matchedDialogs[dialog.Slug] = true
				found = true
				break
			}
		}
		if !found {
			output.Uncovered = append(output.Uncovered, intent.Title)
		}
	}

	for _, dialog := range dialogs {
		if !matchedDialogs[dialog.Slug] {
			output.Orphans = append(output.Orphans, dialog.Title)
		}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
