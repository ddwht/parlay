// parlay-feature: parlay-tool
// parlay-component: check-readiness
// parlay-extends: infrastructure-layer/CheckReadinessInfraSupport

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var checkReadinessCmd = &cobra.Command{
	Use:   "check-readiness <@feature>",
	Short: "Check feature readiness for a given pipeline stage (JSON output for agent consumption)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckReadiness,
}

var readinessStage string

func init() {
	checkReadinessCmd.Flags().StringVar(&readinessStage, "stage", "", "Pipeline stage to check: create-surface, build-feature")
	checkReadinessCmd.MarkFlagRequired("stage")
}

type readinessIssue struct {
	Severity string `json:"severity"` // "error" or "warning"
	Code     string `json:"code"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
}

type readinessOutput struct {
	Feature string           `json:"feature"`
	Stage   string           `json:"stage"`
	Ready   bool             `json:"ready"`
	Issues  []readinessIssue `json:"issues"`
}

func runCheckReadiness(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)

	output := readinessOutput{
		Feature: slug,
		Stage:   readinessStage,
		Issues:  []readinessIssue{},
	}

	switch readinessStage {
	case "create-surface":
		output.Issues = checkCreateSurfaceReadiness(featurePath)
	case "build-feature":
		output.Issues = checkBuildFeatureReadiness(featurePath, slug)
	default:
		return fmt.Errorf("unknown stage %q — supported: create-surface, build-feature", readinessStage)
	}

	// Ready if no errors (warnings don't block)
	output.Ready = true
	for _, issue := range output.Issues {
		if issue.Severity == "error" {
			output.Ready = false
			break
		}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	if !output.Ready {
		os.Exit(1)
	}
	return nil
}

func checkCreateSurfaceReadiness(featurePath string) []readinessIssue {
	var issues []readinessIssue

	// Intents file must exist and have at least one valid intent
	intentsPath := filepath.Join(featurePath, "intents.md")
	intents, err := parser.ParseIntentsFile(intentsPath)
	if err != nil {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "intents-not-readable",
			Message:  fmt.Sprintf("cannot read intents.md: %s", err),
			Fix:      "ensure spec/intents/{feature}/intents.md exists and is valid",
		})
		return issues
	}
	if len(intents) == 0 {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-intents",
			Message:  "intents.md has no intent blocks",
			Fix:      "add at least one intent (## Title with Goal and Persona)",
		})
		return issues
	}

	for _, intent := range intents {
		if intent.Goal == "" {
			issues = append(issues, readinessIssue{
				Severity: "error",
				Code:     "missing-goal",
				Message:  fmt.Sprintf("intent %q has no Goal", intent.Title),
				Fix:      "add **Goal**: line to the intent",
			})
		}
		if intent.Persona == "" {
			issues = append(issues, readinessIssue{
				Severity: "error",
				Code:     "missing-persona",
				Message:  fmt.Sprintf("intent %q has no Persona", intent.Title),
				Fix:      "add **Persona**: line to the intent",
			})
		}
	}

	// Dialogs file is recommended but not required for surface generation
	dialogsPath := filepath.Join(featurePath, "dialogs.md")
	if !fileExists(dialogsPath) {
		issues = append(issues, readinessIssue{
			Severity: "warning",
			Code:     "no-dialogs",
			Message:  "dialogs.md does not exist",
			Fix:      "run /parlay-scaffold-dialogs @{feature} to generate templates",
		})
	}

	return issues
}

func checkBuildFeatureReadiness(featurePath, slug string) []readinessIssue {
	var issues []readinessIssue

	// Build-feature requires everything create-surface requires
	issues = append(issues, checkCreateSurfaceReadiness(featurePath)...)

	// At least one of surface.md or infrastructure.md must exist.
	surfacePath := filepath.Join(featurePath, "surface.md")
	infraPath := filepath.Join(featurePath, "infrastructure.md")
	hasSurface := fileExists(surfacePath)
	hasInfra := fileExists(infraPath)

	if !hasSurface && !hasInfra {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-surface-no-infrastructure",
			Message:  "neither surface.md nor infrastructure.md exists",
			Fix:      "run /parlay-create-artifacts for the decision flow, or author infrastructure.md directly for behind-the-scenes features",
		})
		return issues
	}

	if hasInfra && !isNewSchemaFormat(infraPath) {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "old-infrastructure-schema",
			Message:  "infrastructure.md uses old-format fields (Modifies/Introduces/Detection)",
			Fix:      "migrate to the framework-agnostic format: replace Modifies with **Affects**: (abstract scope), remove Introduces and Detection (these are now generated at build time), and add **Invariants**: for testable properties",
		})
	}

	if !hasSurface {
		// Pure infrastructure feature — skip surface validation, proceed.
		return issues
	}

	fragments, err := parser.ParseSurfaceFile(surfacePath)
	if err != nil {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "surface-not-readable",
			Message:  fmt.Sprintf("cannot parse surface.md: %s", err),
			Fix:      "check surface.md for syntax errors",
		})
		return issues
	}
	if len(fragments) == 0 {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-fragments",
			Message:  "surface.md has no fragments",
			Fix:      "add at least one ## fragment with **Shows** and **Source**",
		})
	}

	for _, frag := range fragments {
		if frag.Source == "" {
			issues = append(issues, readinessIssue{
				Severity: "error",
				Code:     "fragment-missing-source",
				Message:  fmt.Sprintf("fragment %q has no Source reference", frag.Name),
				Fix:      "add **Source**: @{feature}/{intent-slug} to trace back to source intent",
			})
		}
		if frag.Page == "" {
			issues = append(issues, readinessIssue{
				Severity: "error",
				Code:     "fragment-missing-page",
				Message:  fmt.Sprintf("fragment %q has no Page target", frag.Name),
				Fix:      "add **Page**: <page-name> to place the fragment",
			})
		}
		if frag.Region == "" {
			issues = append(issues, readinessIssue{
				Severity: "warning",
				Code:     "fragment-missing-region",
				Message:  fmt.Sprintf("fragment %q has no Region target", frag.Name),
				Fix:      "add **Region**: <region> to position within the page",
			})
		}
	}

	// Open questions are warnings, not errors — agent decides whether to block
	driftOrQuestions, _ := collectForFeature(slug)
	if driftOrQuestions != nil && driftOrQuestions.Count > 0 {
		issues = append(issues, readinessIssue{
			Severity: "warning",
			Code:     "open-questions",
			Message:  fmt.Sprintf("%d open question(s) across intents", driftOrQuestions.Count),
			Fix:      "run parlay collect-questions @{feature} for details, resolve before building",
		})
	}

	// Adapter must be configured
	cfg, err := config.Load()
	if err != nil {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-config",
			Message:  fmt.Sprintf("cannot load .parlay/config.yaml: %s", err),
			Fix:      "run parlay init to bootstrap project configuration",
		})
		return issues
	}
	if cfg.PrototypeFramework == "" {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-prototype-framework",
			Message:  "no prototype-framework configured",
			Fix:      "set prototype-framework in .parlay/config.yaml",
		})
	}

	return issues
}

// parlay-feature: infrastructure-layer
// parlay-component: readiness-new-schema-validation
func isNewSchemaFormat(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(data)
	hasOldModifies := strings.Contains(content, "**Modifies**:")
	hasOldIntroduces := strings.Contains(content, "**Introduces**:")
	if hasOldModifies || hasOldIntroduces {
		return false
	}
	return strings.Contains(content, "**Affects**:")
}
