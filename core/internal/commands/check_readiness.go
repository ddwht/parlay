// parlay-feature: parlay-tool
// parlay-component: check-readiness
// parlay-extends: infrastructure-layer/CheckReadinessInfraSupport
// parlay-extends: parlay-tool/status-feature-phases/shared-feature-phase-helper

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
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
	checkReadinessCmd.Flags().StringVar(&readinessStage, "stage", "", "Pipeline stage to check: dialogs, create-surface, build-feature")
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
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featurePath := cfg.FeaturePath(slug)

	output := readinessOutput{
		Feature: slug,
		Stage:   readinessStage,
		Issues:  []readinessIssue{},
	}

	switch readinessStage {
	case "create-surface":
		output.Issues = checkCreateSurfaceReadiness(featurePath)
	case "build-feature":
		output.Issues = checkBuildFeatureReadiness(cfg, featurePath, slug)
	case "dialogs":
		// Readiness to author dialogs is exactly readiness to author a
		// surface minus the "you have no dialogs yet" warning, which is
		// the very thing this stage precedes. Without this stage the
		// intents->dialogs boundary had no gate at all, so nothing checked
		// that intents parse and carry Goal/Persona before dialogs were
		// generated from them.
		for _, issue := range checkCreateSurfaceReadiness(featurePath) {
			if issue.Code == "no-dialogs" {
				continue
			}
			output.Issues = append(output.Issues, issue)
		}
	default:
		return fmt.Errorf("unknown stage %q — supported: dialogs, create-surface, build-feature", readinessStage)
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
	fmt.Fprintln(cmd.OutOrStdout(), string(data))

	if !output.Ready {
		return NewExitCodeError(1)
	}
	return nil
}

// checkCreateSurfaceReadiness validates that a feature is ready to have
// surface.md authored. Phase-related file-existence is delegated to the
// shared ComputeFeaturePhase helper (via featurePathPhase); content-level
// validation (intent parsing) is performed here.
func checkCreateSurfaceReadiness(featurePath string) []readinessIssue {
	var issues []readinessIssue
	phase := featurePathPhase(featurePath)

	// Intents file must exist and have at least one valid intent.
	// Content validation requires the parser; the helper only tells us
	// the file exists.
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

	// Dialogs file is recommended but not required for surface
	// generation. Phase >= dialogs ⟹ dialogs.md exists; we delegate
	// the existence check to the shared helper rather than running
	// our own os.Stat.
	if !phaseAtLeast(phase, PhaseDialogs) {
		issues = append(issues, readinessIssue{
			Severity: "warning",
			Code:     "no-dialogs",
			Message:  "dialogs.md does not exist",
			Fix:      "run /parlay-scaffold-dialogs @{feature} to generate templates",
		})
	}

	return issues
}

// phaseAtLeast returns true when `got` is `want` or further along the
// pipeline ladder. Used by readiness checks to gate file-existence
// inferences against the shared ComputeFeaturePhase result.
func phaseAtLeast(got, want FeaturePhase) bool {
	rank := map[FeaturePhase]int{
		PhaseIntents:   0,
		PhaseDialogs:   1,
		PhaseArtifacts: 2,
		PhaseBuild:     3,
		PhaseDone:      4,
	}
	return rank[got] >= rank[want]
}

// featurePathPhase resolves the pipeline phase for a feature given only
// its on-disk feature directory (e.g. <root>/spec/intents/<slug> or
// <root>/spec/intents/<initiative>/<feat>). The companion .parlay/build/
// directory is derived by walking up to the active root and joining
// the per-feature build segment. This wrapper lets check_readiness
// route through the same computeFeaturePhaseAtPaths primitive that
// ComputeFeaturePhase uses, without forcing the existing public
// signatures (which take a raw featurePath) to grow a *config.Context
// argument.
func featurePathPhase(featurePath string) FeaturePhase {
	rootPath, segment := splitFeaturePath(featurePath)
	if rootPath == "" || segment == "" {
		// Fall back to a single-segment derivation: assume the
		// feature directory's parent is spec/intents/, and build/
		// sits at <root>/.parlay/build/<base>.
		base := filepath.Base(featurePath)
		buildPath := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(featurePath))),
			config.ParlayDir, config.BuildDir, base)
		return computeFeaturePhaseAtPaths(featurePath, buildPath)
	}
	buildPath := filepath.Join(rootPath, config.ParlayDir, config.BuildDir, segment)
	return computeFeaturePhaseAtPaths(featurePath, buildPath)
}

// splitFeaturePath splits a feature directory into the active root path
// and the slash-joined feature segment under spec/intents/. Returns
// ("", "") when the path doesn't look like a spec/intents/ subtree.
func splitFeaturePath(featurePath string) (rootPath, segment string) {
	clean := filepath.Clean(featurePath)
	marker := string(filepath.Separator) + filepath.Join(config.SpecDir, config.IntentsDir) + string(filepath.Separator)
	idx := strings.Index(clean, marker)
	if idx < 0 {
		return "", ""
	}
	return clean[:idx], clean[idx+len(marker):]
}

func checkBuildFeatureReadiness(cfg *config.Context, featurePath, slug string) []readinessIssue {
	var issues []readinessIssue
	phase := featurePathPhase(featurePath)

	// Build-feature requires everything create-surface requires
	issues = append(issues, checkCreateSurfaceReadiness(featurePath)...)

	// At least one of surface.md or infrastructure.md must exist. The
	// "at-least-one" gate is the same gate that ComputeFeaturePhase
	// applies when promoting a feature to PhaseArtifacts. We still
	// need to know which of the two is present (different validation
	// paths below), so we keep the per-file probe — but it is the
	// same low-level os.Stat primitive used by the shared helper.
	// Multi-adapter v1 prefers surface.yaml over surface.md and
	// capabilities.yaml over infrastructure.md; legacy forms still count.
	surfacePath := parser.ResolveSurfacePath(featurePath)
	infraPath := filepath.Join(featurePath, "infrastructure.md")
	hasArtifacts := phaseAtLeast(phase, PhaseArtifacts)
	hasSurface := surfacePath != ""

	// Only infrastructure.md's own existence may gate the infrastructure
	// format check. This previously read
	//
	//	hasInfra := fileExistsAt(infraPath) || fileExistsAt(capabilities.yaml)
	//
	// which conflated "has a backend artifact" with "has infrastructure.md":
	// a feature with capabilities.yaml and no infrastructure.md — that is,
	// "surface + capabilities", one of the documented valid artifact subsets
	// — went on to format-check a file it does not have. isNewSchemaFormat
	// returns false for an unreadable file, so the absent file read as
	// "legacy format" and the feature was hard-blocked from the build phase
	// by an error naming a nonexistent file, with a migration fix it could
	// not perform. The at-least-one gate is handled by hasArtifacts above.
	hasInfraFile := fileExistsAt(infraPath)

	if !hasArtifacts {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-surface-no-infrastructure",
			Message:  "neither surface.md nor infrastructure.md exists",
			Fix:      "run /parlay-create-artifacts for the decision flow, or author infrastructure.md directly for behind-the-scenes features",
		})
		return issues
	}

	if hasInfraFile && !isNewSchemaFormat(infraPath) {
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
	driftOrQuestions, _ := collectForFeature(cfg, slug)
	if driftOrQuestions != nil && driftOrQuestions.Count > 0 {
		issues = append(issues, readinessIssue{
			Severity: "warning",
			Code:     "open-questions",
			Message:  fmt.Sprintf("%d open question(s) across intents", driftOrQuestions.Count),
			Fix:      "run parlay collect-questions @{feature} for details, resolve before building",
		})
	}

	// Adapter must be configured
	pc, err := cfg.LoadProjectConfig()
	if err != nil {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-config",
			Message:  fmt.Sprintf("cannot load .parlay/config.yaml: %s", err),
			Fix:      "run parlay init to bootstrap project configuration",
		})
		return issues
	}
	// An adapter must be configured for the build stage. This is satisfied by
	// either the legacy prototype-framework field (deprecated — v0.3 removes
	// it) or, for adapter-set projects, a parseable adapter-set.yaml with at
	// least one filled slot. Studio and other multi-target projects carry no
	// prototype-framework, so requiring it alone would falsely block them.
	if pc.PrototypeFramework == "" && !hasConfiguredAdapterSet(cfg) {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     "no-adapter-configured",
			Message:  "no adapter configured (neither adapter-set.yaml nor prototype-framework)",
			Fix:      "define .parlay/adapter-set.yaml (recommended) or set prototype-framework in .parlay/config.yaml",
		})
	}

	return issues
}

// hasConfiguredAdapterSet reports whether the active root carries a parseable
// adapter-set.yaml with at least one filled target slot. This is the modern
// replacement for the deprecated prototype-framework field: an adapter-set
// with any slot filled (presentation-only included) means the project has an
// adapter configured for the build stage. A missing or unparseable file
// returns false, preserving the legacy prototype-framework requirement for
// projects that have not migrated.
func hasConfiguredAdapterSet(cfg *config.Context) bool {
	as, err := parser.ParseAdapterSet(cfg.AdapterSetPath())
	if err != nil {
		return false
	}
	return len(as.Targets) > 0
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
