package commands

import (
	"encoding/json"
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
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
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		slug := parser.FeatureSlug(args[0])
		output, err := collectForFeature(cfg, slug)
		if err != nil {
			return err
		}
		return printJSON(cmd, output)
	}

	// No argument: scan all features (including initiative-nested)
	featureIDs, err := cfg.AllFeatures()
	if err != nil {
		return fmt.Errorf("cannot enumerate features: %w", err)
	}

	var all allQuestionsOutput
	for _, featureID := range featureIDs {
		output, err := collectForFeature(cfg, featureID)
		if err != nil {
			continue // feature may not have intents yet
		}
		if output.Count > 0 {
			all.Features = append(all.Features, *output)
			all.Total += output.Count
		}
	}

	return printJSON(cmd, all)
}

func collectForFeature(cfg *config.Context, slug string) (*questionsOutput, error) {
	// Open questions on a promise that has been withdrawn are history, not
	// work: collecting them would ask a designer to resolve a question about
	// something the project has already decided not to do.
	res, err := resolveActiveIntents(cfg, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to read intents for %s: %w", slug, err)
	}
	intents := res.Active

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

func printJSON(cmd *cobra.Command, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
