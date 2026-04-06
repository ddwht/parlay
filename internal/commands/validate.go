package commands

import (
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

func init() {
	validateCmd.Flags().StringVar(&validateType, "type", "", "File type: surface, buildfile, yaml")
	validateCmd.MarkFlagRequired("type")
	validateCmd.Flags().BoolVar(&validateDeep, "deep", false, "Enable cross-reference validation (buildfile only)")
	validateCmd.Flags().StringVar(&validateAdapter, "adapter", "", "Path to adapter file for vocabulary validation (used with --deep)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	path := args[0]

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	var validator agent.Validator
	switch validateType {
	case "surface":
		validator = agent.ValidateSurface
	case "buildfile":
		validator = agent.ValidateBuildfile
	case "yaml":
		validator = agent.ValidateYAML
	default:
		return fmt.Errorf("unknown type %q — supported: surface, buildfile, yaml", validateType)
	}

	if err := validator(path, content); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		os.Exit(1)
	}

	// Deep validation for buildfiles
	if validateDeep && validateType == "buildfile" {
		errors := agent.ValidateBuildfileDeep(path, validateAdapter)
		if len(errors) > 0 {
			fmt.Fprintf(os.Stderr, "Deep validation found %d issue(s):\n", len(errors))
			for _, e := range errors {
				fmt.Fprintf(os.Stderr, "  - %s\n", e)
			}
			os.Exit(1)
		}
	}

	fmt.Println("OK")
	return nil
}
