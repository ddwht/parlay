// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate-failure
//
// Coverage-review gate. Reads .parlay/build/<feature>/coverage-review.yaml,
// computes canonical-form hashes of buildfile.yaml + testcases.yaml,
// compares against the recorded hashes, and walks required-suite
// approvals + required-coverage-term exemptions.
//
// The generate-code skill invokes this BEFORE any other read.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var checkReviewGateCmd = &cobra.Command{
	Use:   "check-review-gate <@feature>",
	Short: "Verify the coverage-review.yaml gate before generate-code (JSON output for skill consumption)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckReviewGate,
}

type reviewGateIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type reviewGateOutput struct {
	Feature string            `json:"feature"`
	Ready   bool              `json:"ready"`
	Issues  []reviewGateIssue `json:"issues"`
}

func runCheckReviewGate(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	out := reviewGateOutput{Feature: slug, Issues: []reviewGateIssue{}}

	// Adapter-set gate: presentation-only projects skip coverage-review.
	asPath := cfg.AdapterSetPath()
	asContent, err := os.ReadFile(asPath)
	if err != nil {
		out.Ready = true
		return emitReviewGateJSON(cmd, out)
	}
	adapterSet, err := parser.ParseAdapterSetBytes(asPath, asContent)
	if err != nil || !adapterSet.IsMultiTarget() {
		out.Ready = true
		return emitReviewGateJSON(cmd, out)
	}

	buildDir := cfg.BuildPath(slug)
	bfPath := filepath.Join(buildDir, "buildfile.yaml")
	tcPath := filepath.Join(buildDir, "testcases.yaml")
	reviewPath := filepath.Join(buildDir, "coverage-review.yaml")

	bfContent, err := os.ReadFile(bfPath)
	if err != nil {
		out.Issues = append(out.Issues, reviewGateIssue{
			Severity: "error",
			Code:     "buildfile-not-found",
			Message:  fmt.Sprintf("read %s: %v", bfPath, err),
		})
		return emitReviewGateJSON(cmd, out)
	}
	tcContent, err := os.ReadFile(tcPath)
	if err != nil {
		out.Issues = append(out.Issues, reviewGateIssue{
			Severity: "error",
			Code:     "testcases-not-found",
			Message:  fmt.Sprintf("read %s: %v", tcPath, err),
		})
		return emitReviewGateJSON(cmd, out)
	}

	bfHash, err := agent.CanonicalFormHash(bfContent)
	if err != nil {
		out.Issues = append(out.Issues, reviewGateIssue{
			Severity: "error",
			Code:     "canonical-form-failed",
			Message:  fmt.Sprintf("hash buildfile: %v", err),
		})
		return emitReviewGateJSON(cmd, out)
	}
	tcHash, err := agent.CanonicalFormHash(tcContent)
	if err != nil {
		out.Issues = append(out.Issues, reviewGateIssue{
			Severity: "error",
			Code:     "canonical-form-failed",
			Message:  fmt.Sprintf("hash testcases: %v", err),
		})
		return emitReviewGateJSON(cmd, out)
	}

	// Discover the required suites from the on-disk testcases. Coverage
	// of canonical operations + declared errors is the architecture's
	// minimum bar.
	required := requiredSuiteIDs(tcContent)

	in := agent.CoverageReviewInputs{
		ReviewPath:       reviewPath,
		Feature:          slug,
		BuildfileHashNow: bfHash,
		TestcasesHashNow: tcHash,
		RequiredSuites:   required,
	}
	for _, o := range agent.ValidateCoverageReview(agent.ModeBuild, in) {
		if o.Severity == agent.SeverityError {
			out.Issues = append(out.Issues, reviewGateIssue{
				Severity: "error",
				Code:     o.Code,
				Message:  o.Message,
			})
		}
	}

	out.Ready = len(out.Issues) == 0
	return emitReviewGateJSON(cmd, out)
}

// requiredSuiteIDs extracts every suite name declared in testcases.yaml.
// The architecture says every canonical operation must be covered;
// concretely, every suite present in testcases.yaml must be approved (or
// exempted) for the gate to pass.
func requiredSuiteIDs(tcContent []byte) []string {
	var shape struct {
		Suites []struct {
			Name string `yaml:"name"`
			ID   string `yaml:"id,omitempty"`
		} `yaml:"suites"`
	}
	if err := yaml.Unmarshal(tcContent, &shape); err != nil {
		return nil
	}
	out := make([]string, 0, len(shape.Suites))
	for _, s := range shape.Suites {
		if s.ID != "" {
			out = append(out, s.ID)
		} else if s.Name != "" {
			out = append(out, s.Name)
		}
	}
	return out
}

func emitReviewGateJSON(cmd *cobra.Command, out reviewGateOutput) error {
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if !out.Ready {
		return NewExitCodeError(1)
	}
	return nil
}
