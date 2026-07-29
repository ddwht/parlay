package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// checkBuildfileCmd validates a feature's already-emitted buildfile.yaml
// against the live source tree, with a JSON shape suited for skill
// consumption. It's a feature-ref-aware wrapper around `parlay validate
// --type buildfile --deep` that auto-resolves the buildfile path and
// the project's adapter, and lifts plan-section integrity errors into
// the same structured output the build-feature skill already parses.
//
// Lives alongside check-readiness in the agent-facing utility set.
// Build-feature invokes it after emitting buildfile.yaml; failures
// block the buildfile from being committed without user review.
var checkBuildfileCmd = &cobra.Command{
	Use:   "check-buildfile <@feature>",
	Short: "Validate a feature's buildfile against the live source tree (JSON output for agent consumption)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckBuildfile,
}

type checkBuildfileIssue struct {
	Severity string `json:"severity"` // "error" or "warning"
	Code     string `json:"code"`
	Message  string `json:"message"`
	Context  string `json:"context,omitempty"`
	Fix      string `json:"fix"`
}

type checkBuildfileOutput struct {
	Feature string                `json:"feature"`
	Path    string                `json:"path"`
	Adapter string                `json:"adapter,omitempty"`
	Ready   bool                  `json:"ready"`
	Issues  []checkBuildfileIssue `json:"issues"`
}

func runCheckBuildfile(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")

	output := checkBuildfileOutput{
		Feature: slug,
		Path:    bfPath,
		Issues:  []checkBuildfileIssue{},
	}

	if _, err := os.Stat(bfPath); err != nil {
		output.Issues = append(output.Issues, checkBuildfileIssue{
			Severity: "error",
			Code:     "buildfile-not-found",
			Message:  fmt.Sprintf("buildfile.yaml does not exist at %s", bfPath),
			Context:  bfPath,
			Fix:      fmt.Sprintf("run /parlay-build-feature @%s to generate it", slug),
		})
		return emitCheckBuildfileOutput(cmd, output)
	}

	// Auto-discover the adapter so vocab validation runs without the
	// caller having to pass --adapter manually.
	adapterPath := autoDiscoverAdapter(cfg, bfPath)
	if adapterPath != "" {
		output.Adapter = adapterPath
	}

	for _, e := range agent.ValidateBuildfileDeepStructured(bfPath, adapterPath) {
		severity := e.Severity
		if severity == "" {
			severity = severityForCode(e.Code)
		}
		output.Issues = append(output.Issues, checkBuildfileIssue{
			Severity: severity,
			Code:     e.Code,
			Message:  e.Message,
			Context:  e.Context,
			Fix:      e.Fix,
		})
	}

	return emitCheckBuildfileOutput(cmd, output)
}

// autoDiscoverAdapter reads the buildfile's `adapter:` field and resolves
// the adapter file path under the active root's adapters directory.
// Returns "" when discovery fails — the caller treats absent as "skip
// vocabulary validation".
func autoDiscoverAdapter(cfg *config.Context, buildfilePath string) string {
	data, err := os.ReadFile(buildfilePath)
	if err != nil {
		return ""
	}
	type adapterField struct {
		Adapter string `yaml:"adapter"`
	}
	var bf adapterField
	if err := yaml.Unmarshal(data, &bf); err != nil || bf.Adapter == "" {
		return ""
	}
	candidate := filepath.Join(cfg.AdaptersPath(), bf.Adapter+".adapter.yaml")
	if _, err := os.Stat(candidate); err != nil {
		return ""
	}
	return candidate
}

// severityForCode maps known agent.ValidationError codes to severity
// levels. Most cross-reference and plan-integrity errors are blocking;
// adapter-vocab mismatches and plan-create-collision are warnings (the
// agent might still want to proceed after asking the user).
//
// Deprecated: severity now lives in one place — agent.RuleSeverity, backed
// by ruleSeverityTable — and is stamped onto each finding by
// agent.ApplyBuildfileSeverity. Keeping a private second table here is what
// made check-buildfile and validate --deep disagree about the same file.
// Retained only as a fallback for findings that arrive without a severity.
func severityForCode(code string) string {
	switch code {
	case "plan-create-collision",
		"unknown-component-widget",
		"unknown-component-action",
		"unknown-flow":
		return "warning"
	}
	return "error"
}

func emitCheckBuildfileOutput(cmd *cobra.Command, output checkBuildfileOutput) error {
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
	fmt.Fprintln(cmd.OutOrStdout(), string(data))

	if !output.Ready {
		return NewExitCodeError(1)
	}
	return nil
}
