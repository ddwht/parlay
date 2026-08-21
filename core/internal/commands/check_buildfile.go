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
	return emitCheckBuildfileOutput(cmd, computeCheckBuildfile(cfg, slug))
}

// computeCheckBuildfile validates a feature's already-emitted buildfile against
// the live source tree and returns the structured result, with no stdout I/O
// and no process exit. Shared by the cobra command and the gate aggregator so
// the plan-integrity / adapter-vocab checks are not reimplemented.
func computeCheckBuildfile(cfg *config.Context, slug string) checkBuildfileOutput {
	bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")

	output := checkBuildfileOutput{
		Feature: slug,
		Path:    bfPath,
		Issues:  []checkBuildfileIssue{},
	}

	if _, err := os.Stat(bfPath); err != nil {
		// A unit has no buildfile and never will — nothing builds it, which
		// is the point. The generic message below is worse than silence
		// here: it tells the caller to run build-feature, and build-feature
		// on a unit is precisely the thing that must not happen.
		if config.IsAuthoredUnit(cfg.FeaturePath(slug)) {
			output.Issues = append(output.Issues, checkBuildfileIssue{
				Severity: "info",
				Code:     "buildfile-not-applicable",
				Message:  fmt.Sprintf("%s is a hand-authored unit; it has no buildfile because nothing generates its code", slug),
				Context:  cfg.FeaturePath(slug),
				Fix:      "no action — declare the unit's sources in authored.yaml, which is already done",
			})
			return output
		}
		output.Issues = append(output.Issues, checkBuildfileIssue{
			Severity: "error",
			Code:     "buildfile-not-found",
			Message:  fmt.Sprintf("buildfile.yaml does not exist at %s", bfPath),
			Context:  bfPath,
			Fix:      fmt.Sprintf("run /parlay-build-feature @%s to generate it", slug),
		})
		return output
	}

	// Auto-discover the adapter so vocab validation runs without the
	// caller having to pass --adapter manually.
	adapterPath := autoDiscoverAdapter(cfg, bfPath)
	if adapterPath != "" {
		output.Adapter = adapterPath
	}

	for _, e := range agent.ValidateBuildfileDeepStructured(bfPath, adapterPath) {
		// No local severity fallback. ValidateBuildfileDeepStructured returns
		// ApplyBuildfileSeverity(...), which stamps every finding that arrives
		// without one — so a second table here could only ever disagree with
		// agent.RuleSeverity, which is exactly what it used to do.
		output.Issues = append(output.Issues, checkBuildfileIssue{
			Severity: e.Severity,
			Code:     e.Code,
			Message:  e.Message,
			Context:  e.Context,
			Fix:      e.Fix,
		})
	}

	return output
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
		Adapter    string `yaml:"adapter"`
		AdapterSet string `yaml:"adapter-set"`
		Targets    map[string]struct {
			Adapter string `yaml:"adapter"`
		} `yaml:"targets"`
	}
	var bf adapterField
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return ""
	}
	// v1: the top-level adapter:. v2: the presentation slot's adapter — the
	// only kind whose vocabulary (widgets/actions/flows) the deep validator
	// checks. Backend adapters carry no widget vocabulary, so vocabulary
	// validation stays presentation-scoped.
	name := bf.Adapter
	if name == "" && bf.AdapterSet != "" {
		name = bf.Targets["presentation"].Adapter
	}
	if name == "" {
		return ""
	}
	candidate := filepath.Join(cfg.AdaptersPath(), name+".adapter.yaml")
	if _, err := os.Stat(candidate); err != nil {
		return ""
	}
	return candidate
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
