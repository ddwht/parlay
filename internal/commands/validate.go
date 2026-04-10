package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/parlay/internal/agent"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate --type <type> <path>",
	Short: "Validate a file against its schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runValidate,
}

var validateType string
var validateDeep bool
var validateAdapter string
var validateJSON bool

func init() {
	validateCmd.Flags().StringVar(&validateType, "type", "", "File type: surface, buildfile, blueprint, yaml")
	validateCmd.MarkFlagRequired("type")
	validateCmd.Flags().BoolVar(&validateDeep, "deep", false, "Enable cross-reference validation (buildfile only)")
	validateCmd.Flags().StringVar(&validateAdapter, "adapter", "", "Path to adapter file for vocabulary validation (used with --deep)")
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output structured JSON errors for agent consumption")
}

type validateJSONResult struct {
	Path   string                  `json:"path"`
	Type   string                  `json:"type"`
	OK     bool                    `json:"ok"`
	Errors []agent.ValidationError `json:"errors,omitempty"`
}

func runValidate(cmd *cobra.Command, args []string) error {
	path := args[0]

	content, err := os.ReadFile(path)
	if err != nil {
		return outputValidate(path, []agent.ValidationError{{
			Code:    "file-not-readable",
			Message: fmt.Sprintf("cannot read %s: %s", path, err),
			Context: path,
			Fix:     "verify the file path is correct",
		}})
	}

	var validator agent.Validator
	switch validateType {
	case "surface":
		validator = agent.ValidateSurface
	case "buildfile":
		validator = agent.ValidateBuildfile
	case "blueprint":
		validator = agent.ValidateBlueprint
	case "yaml":
		validator = agent.ValidateYAML
	default:
		return fmt.Errorf("unknown type %q — supported: surface, buildfile, blueprint, yaml", validateType)
	}

	if err := validator(path, content); err != nil {
		return outputValidate(path, []agent.ValidationError{{
			Code:    "schema-validation-failed",
			Message: err.Error(),
			Context: path,
			Fix:     "fix the structural issues reported above",
		}})
	}

	// Deep validation for buildfiles
	if validateDeep && validateType == "buildfile" {
		errors := agent.ValidateBuildfileDeepStructured(path, validateAdapter)
		if len(errors) > 0 {
			return outputValidate(path, errors)
		}
	}

	return outputValidate(path, nil)
}

func outputValidate(path string, errors []agent.ValidationError) error {
	if validateJSON {
		result := validateJSONResult{
			Path:   path,
			Type:   validateType,
			OK:     len(errors) == 0,
			Errors: errors,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		if len(errors) > 0 {
			os.Exit(1)
		}
		return nil
	}

	// Text output (default)
	if len(errors) == 0 {
		fmt.Println("OK")
		return nil
	}
	fmt.Fprintf(os.Stderr, "FAIL: %d issue(s)\n", len(errors))
	for _, e := range errors {
		fmt.Fprintf(os.Stderr, "  [%s] %s\n", e.Code, e.Message)
		if e.Fix != "" {
			fmt.Fprintf(os.Stderr, "    fix: %s\n", e.Fix)
		}
	}
	os.Exit(1)
	return nil
}
