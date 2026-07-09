// parlay-feature: parlay-tool
// parlay-component: validate
// parlay-extends: infrastructure-layer/InfrastructureValidationResult
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-validate-cli-type
// parlay-extends: parlay-tool/cross-cutting-target-paths/project-pass-validation-and-cli-flag

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate --type <type> <path>",
	Short: "Validate a file against its schema",
	Args:  validateArgs,
	RunE:  runValidate,
}

var validateType string
var validateDeep bool
var validateAdapter string
var validateJSON bool
var validateProject bool
var validateRoot string

func init() {
	validateCmd.Flags().StringVar(&validateType, "type", "", "File type: surface, buildfile, blueprint, yaml, infrastructure, domain-model")
	validateCmd.Flags().BoolVar(&validateDeep, "deep", false, "Enable cross-reference validation (buildfile only)")
	validateCmd.Flags().StringVar(&validateAdapter, "adapter", "", "Path to adapter file for vocabulary validation (used with --deep)")
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output structured JSON errors for agent consumption")
	// parlay-feature: parlay-tool/cross-cutting-target-paths
	// parlay-component: validate (extends project-pass-validation-and-cli-flag)
	validateCmd.Flags().BoolVar(&validateProject, "project", false, "Validate every buildfile under the resolved root in project-pass mode (no positional path; implies --type buildfile --deep)")
	validateCmd.Flags().StringVar(&validateRoot, "root", "", "Project root for --project (defaults to cwd or PARLAY_ROOT)")
}

// validateArgs reconciles the legacy positional-path requirement with the
// new --project flag: --project takes zero positional arguments, the
// legacy modes still take exactly one. Combining --project with a path is
// rejected with a structured error.
func validateArgs(cmd *cobra.Command, args []string) error {
	projectFlag, _ := cmd.Flags().GetBool("project")
	if projectFlag {
		if len(args) != 0 {
			return fmt.Errorf("validate-project-takes-no-path: --project does not take a positional path argument; got %v", args)
		}
		return nil
	}
	return cobra.ExactArgs(1)(cmd, args)
}

type validateJSONResult struct {
	Path   string                  `json:"path"`
	Type   string                  `json:"type"`
	OK     bool                    `json:"ok"`
	Errors []agent.ValidationError `json:"errors,omitempty"`
}

// projectValidateJSONResult is the JSON envelope emitted when --project is
// set. It carries one entry per feature plus an aggregate ok flag.
//
// parlay-feature: parlay-tool/cross-cutting-target-paths
// parlay-component: validate (extends project-pass-validation-and-cli-flag)
type projectValidateJSONResult struct {
	Root     string                   `json:"root"`
	OK       bool                     `json:"ok"`
	Features []agent.FeatureVerdict   `json:"features,omitempty"`
	Note     string                   `json:"note,omitempty"`
}

func runValidate(cmd *cobra.Command, args []string) error {
	if validateProject {
		return runValidateProject(cmd)
	}

	path := args[0]

	content, err := os.ReadFile(path)
	if err != nil {
		return outputValidate(cmd, path, []agent.ValidationError{{
			Code:    "file-not-readable",
			Message: fmt.Sprintf("cannot read %s: %s", path, err),
			Context: path,
			Fix:     "verify the file path is correct",
		}})
	}

	// --type is required for the non-project paths.
	if validateType == "" {
		return fmt.Errorf("--type is required (one of: surface, buildfile, blueprint, yaml, infrastructure, domain-model, adapter-set, capabilities, coverage-review)")
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
	case "infrastructure":
		validator = agent.ValidateInfrastructure
	case "domain-model":
		// parlay-feature: studio-support/domain-model-yaml-migration
		// parlay-component: validate (extends domain-model-validate-cli-type)
		validator = agent.ValidateDomainModel
	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: cli-and-deployer-registration
	case "adapter-set":
		validator = wrapOutcomeValidator(agent.ValidateAdapterSet)
	case "capabilities":
		validator = wrapOutcomeValidator(agent.ValidateCapabilities)
	case "coverage-review":
		// coverage-review validation is hash-aware and needs full inputs;
		// surface a minimal YAML-shape check here.
		validator = agent.ValidateYAML
	default:
		return fmt.Errorf("unknown type %q — supported: surface, buildfile, blueprint, yaml, infrastructure, domain-model, adapter-set, capabilities, coverage-review", validateType)
	}

	if err := validator(path, content); err != nil {
		return outputValidate(cmd, path, []agent.ValidationError{{
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
			return outputValidate(cmd, path, errors)
		}
	}

	return outputValidate(cmd, path, nil)
}

// runValidateProject implements the --project mode: walk every buildfile
// under the resolved root and validate each in project-pass mode (which
// threads cross-feature plannedCreates through validatePlanSection).
//
// parlay-feature: parlay-tool/cross-cutting-target-paths
// parlay-component: validate (extends project-pass-validation-and-cli-flag)
func runValidateProject(cmd *cobra.Command) error {
	root := validateRoot
	if root == "" {
		if env := os.Getenv("PARLAY_ROOT"); env != "" {
			root = env
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot resolve project root: %s", err)
			}
			root = cwd
		}
	}
	verdicts, err := agent.ValidateBuildfilesProjectStructured(root)
	if err != nil {
		return fmt.Errorf("project-pass walk failed: %s", err)
	}

	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: cli-and-deployer-registration
	//
	// Multi-target gate: when the project carries an adapter-set with a
	// non-presentation slot, also walk the capabilities + supports +
	// blueprint rules. Presentation-only projects short-circuit inside
	// ValidateProjectMultiTarget and contribute zero outcomes here.
	multiOutcomes := agent.ValidateProjectMultiTarget(agent.ModeBuild, root)
	if len(multiOutcomes) > 0 {
		// Attribute them to a synthetic "_project" feature so they
		// surface alongside per-feature verdicts.
		var errs []agent.ValidationError
		for _, o := range multiOutcomes {
			if o.Severity != agent.SeverityError {
				continue
			}
			errs = append(errs, agent.ValidationError{
				Code:    o.Code,
				Message: o.Message,
				Context: o.Context,
				Fix:     o.Fix,
			})
		}
		if len(errs) > 0 {
			verdicts = append(verdicts, agent.FeatureVerdict{
				Feature:       "_project",
				BuildfilePath: filepath.Join(root, ".parlay", "adapter-set.yaml"),
				Errors:        errs,
			})
		}
	}

	totalErrors := 0
	for _, v := range verdicts {
		totalErrors += len(v.Errors)
	}
	ok := totalErrors == 0

	if validateJSON {
		out := projectValidateJSONResult{
			Root:     root,
			OK:       ok,
			Features: verdicts,
		}
		if len(verdicts) == 0 {
			out.Note = fmt.Sprintf("no buildfiles under %s/.parlay/build/", root)
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		if !ok {
			return NewExitCodeError(1)
		}
		return nil
	}

	// Text output.
	if len(verdicts) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "OK (no buildfiles under %s/.parlay/build/)\n", root)
		return nil
	}
	if ok {
		fmt.Fprintf(cmd.OutOrStdout(), "OK (%d feature(s) validated)\n", len(verdicts))
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "FAIL: %d issue(s) across %d feature(s)\n", totalErrors, len(verdicts))
	for _, v := range verdicts {
		if len(v.Errors) == 0 {
			continue
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "\nfeature %s (%s):\n", v.Feature, v.BuildfilePath)
		for _, e := range v.Errors {
			fmt.Fprintf(cmd.ErrOrStderr(), "  [%s] %s\n", e.Code, e.Message)
			if e.Fix != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "    fix: %s\n", e.Fix)
			}
		}
	}
	return NewExitCodeError(1)
}

// wrapOutcomeValidator adapts an agent.ValidationOutcome-returning
// validator to the legacy agent.Validator signature. Outcomes with severity
// error are joined into a single error string; warnings are dropped silently
// (the structured JSON path uses ValidationOutcome directly when needed).
//
// parlay-feature: parlay-tool/multi-adapter
// parlay-component: cli-and-deployer-registration
func wrapOutcomeValidator(fn func(agent.ValidationMode, string, []byte) []agent.ValidationOutcome) agent.Validator {
	return func(path string, content []byte) error {
		outcomes := fn(agent.ModeAuthoring, path, content)
		var msgs []string
		for _, o := range outcomes {
			if o.Severity == agent.SeverityError {
				msgs = append(msgs, fmt.Sprintf("%s: %s", o.Code, o.Message))
			}
		}
		if len(msgs) == 0 {
			return nil
		}
		return fmt.Errorf("%s", strings.Join(msgs, "\n"))
	}
}

func outputValidate(cmd *cobra.Command, path string, errors []agent.ValidationError) error {
	if validateJSON {
		result := validateJSONResult{
			Path:   path,
			Type:   validateType,
			OK:     len(errors) == 0,
			Errors: errors,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		if len(errors) > 0 {
			return NewExitCodeError(1)
		}
		return nil
	}

	// Text output (default)
	if len(errors) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "OK")
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "FAIL: %d issue(s)\n", len(errors))
	for _, e := range errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "  [%s] %s\n", e.Code, e.Message)
		if e.Fix != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "    fix: %s\n", e.Fix)
		}
	}
	return NewExitCodeError(1)
}
