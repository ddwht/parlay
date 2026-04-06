package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var collectQuestionsCmd = &cobra.Command{
	Use:   "collect-questions [@feature]",
	Short: "Collect open questions from intents (JSON output for agent consumption)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCollectQuestions,
}

type questionItem struct {
	Intent   string `json:"intent"`
	Priority string `json:"priority"`
	Question string `json:"question"`
}

type questionsOutput struct {
	Feature   string         `json:"feature"`
	Questions []questionItem `json:"questions"`
	Count     int            `json:"count"`
}

type allQuestionsOutput struct {
	Features []questionsOutput `json:"features"`
	Total    int               `json:"total"`
}

func runCollectQuestions(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		slug := strings.TrimPrefix(args[0], "@")
		output, err := collectForFeature(slug)
		if err != nil {
			return err
		}
		return printJSON(output)
	}

	// No argument: scan all features
	intentsDir := filepath.Join(config.SpecDir, config.IntentsDir)
	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		return fmt.Errorf("cannot read intents directory: %w", err)
	}

	var all allQuestionsOutput
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		output, err := collectForFeature(entry.Name())
		if err != nil {
			continue // feature may not have intents yet
		}
		if output.Count > 0 {
			all.Features = append(all.Features, *output)
			all.Total += output.Count
		}
	}

	return printJSON(all)
}

func collectForFeature(slug string) (*questionsOutput, error) {
	featurePath := config.FeaturePath(slug)
	intentsPath := filepath.Join(featurePath, "intents.md")

	intents, err := parser.ParseIntentsFile(intentsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read intents for %s: %w", slug, err)
	}

	output := &questionsOutput{Feature: slug}

	for _, intent := range intents {
		priority := intent.Priority
		if priority == "" {
			priority = "P1"
		}
		for _, q := range intent.Questions {
			output.Questions = append(output.Questions, questionItem{
				Intent:   intent.Title,
				Priority: priority,
				Question: q,
			})
		}
	}
	output.Count = len(output.Questions)

	return output, nil
}

func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
