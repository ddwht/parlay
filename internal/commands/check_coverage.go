// parlay-feature: parlay-tool/authoring
// parlay-component: CoverageReport

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
	"gopkg.in/yaml.v3"
)

var checkCoverageCmd = &cobra.Command{
	Use:   "check-coverage <@feature>",
	Short: "Check intent-dialog coverage and full-chain traceability (JSON output for agent consumption)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckCoverage,
}

type coverageMatch struct {
	Intent string `json:"intent"`
	Dialog string `json:"dialog"`
}

type chainGap struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type coverageOutput struct {
	Feature   string          `json:"feature"`
	Covered   []coverageMatch `json:"covered"`
	Uncovered []string        `json:"uncovered"`
	Orphans   []string        `json:"orphans"`
	Chain     *chainCoverage  `json:"chain,omitempty"`
	Drift     *driftOutput    `json:"drift,omitempty"`
}

type chainCoverage struct {
	IntentsWithoutSurface    []chainGap `json:"intents_without_surface,omitempty"`
	FragmentsWithoutBuildfile []chainGap `json:"fragments_without_buildfile,omitempty"`
	ComponentsWithoutTests   []chainGap `json:"components_without_tests,omitempty"`
	OrphanedReferences       []chainGap `json:"orphaned_references,omitempty"`
}

func runCheckCoverage(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)

	intents, err := parser.ParseIntentsFile(filepath.Join(featurePath, "intents.md"))
	if err != nil {
		return fmt.Errorf("failed to read intents: %w", err)
	}

	dialogs, err := parser.ParseDialogsFile(filepath.Join(featurePath, "dialogs.md"))
	if err != nil {
		return fmt.Errorf("failed to read dialogs: %w", err)
	}

	output := coverageOutput{Feature: slug}
	matchedDialogs := make(map[string]bool)

	for _, intent := range intents {
		found := false
		for _, dialog := range dialogs {
			if matchesIntent(intent, dialog) {
				output.Covered = append(output.Covered, coverageMatch{
					Intent: intent.Title,
					Dialog: dialog.Title,
				})
				matchedDialogs[dialog.Slug] = true
				found = true
				break
			}
		}
		if !found {
			output.Uncovered = append(output.Uncovered, intent.Title)
		}
	}

	for _, dialog := range dialogs {
		if !matchedDialogs[dialog.Slug] {
			output.Orphans = append(output.Orphans, dialog.Title)
		}
	}

	// Full-chain traceability: check downstream artifacts if they exist
	chain := checkChain(featurePath, slug, intents)
	if chain != nil {
		output.Chain = chain
	}

	// Drift detection: check if intents changed since last build
	drift, _ := detectDrift(slug, featurePath)
	if drift != nil && drift.HasDrift {
		output.Drift = drift
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func checkChain(featurePath, slug string, intents []parser.Intent) *chainCoverage {
	surfacePath := filepath.Join(featurePath, "surface.md")
	buildPath := config.BuildPath(slug)
	buildfilePath := filepath.Join(buildPath, "buildfile.yaml")
	testcasesPath := filepath.Join(buildPath, "testcases.yaml")

	// Check if any downstream artifact exists
	hasSurface := fileExists(surfacePath)
	hasBuildfile := fileExists(buildfilePath)
	hasTestcases := fileExists(testcasesPath)

	if !hasSurface && !hasBuildfile && !hasTestcases {
		return nil
	}

	chain := &chainCoverage{}

	// Build set of intent slugs
	intentSlugs := make(map[string]bool)
	for _, intent := range intents {
		intentSlugs[intent.Slug] = true
	}

	// 1. Check intents → surface coverage
	var surfaceSourceSlugs map[string]bool
	var fragmentNames map[string]bool

	if hasSurface {
		fragments, err := parser.ParseSurfaceFile(surfacePath)
		if err == nil {
			surfaceSourceSlugs = make(map[string]bool)
			fragmentNames = make(map[string]bool)

			for _, frag := range fragments {
				fragmentNames[parser.Slugify(frag.Name)] = true
				// Parse Source references: "@feature/intent-slug, @feature/intent-slug"
				for _, ref := range parseSourceRefs(frag.Source, slug) {
					surfaceSourceSlugs[ref] = true
				}
			}

			// Intents not referenced by any surface fragment
			for _, intent := range intents {
				if !surfaceSourceSlugs[intent.Slug] {
					chain.IntentsWithoutSurface = append(chain.IntentsWithoutSurface, chainGap{
						Name:   intent.Title,
						Reason: fmt.Sprintf("no surface fragment references @%s/%s", slug, intent.Slug),
					})
				}
			}

			// Surface fragments referencing intents that don't exist
			for ref := range surfaceSourceSlugs {
				if !intentSlugs[ref] {
					chain.OrphanedReferences = append(chain.OrphanedReferences, chainGap{
						Name:   fmt.Sprintf("@%s/%s", slug, ref),
						Reason: "surface Source references an intent that no longer exists",
					})
				}
			}
		}
	}

	// 2. Check surface → buildfile coverage
	var buildfileComponents map[string]string // component name → source slug

	if hasBuildfile {
		bf, err := parseBuildfileRefs(buildfilePath, slug)
		if err == nil {
			buildfileComponents = bf

			if fragmentNames != nil {
				// Surface fragments not represented in buildfile
				referencedFragments := make(map[string]bool)
				for _, sourceSlug := range buildfileComponents {
					referencedFragments[sourceSlug] = true
				}
				for fragSlug := range fragmentNames {
					if !referencedFragments[fragSlug] {
						chain.FragmentsWithoutBuildfile = append(chain.FragmentsWithoutBuildfile, chainGap{
							Name:   fragSlug,
							Reason: "surface fragment has no matching buildfile component",
						})
					}
				}
			}
		}
	}

	// 3. Check buildfile → testcases coverage
	if hasTestcases && buildfileComponents != nil {
		testedComponents, err := parseTestcaseRefs(testcasesPath)
		if err == nil {
			for compName := range buildfileComponents {
				if !testedComponents[compName] {
					chain.ComponentsWithoutTests = append(chain.ComponentsWithoutTests, chainGap{
						Name:   compName,
						Reason: "buildfile component has no test suite",
					})
				}
			}
		}
	}

	// Return nil if no gaps found
	if len(chain.IntentsWithoutSurface) == 0 &&
		len(chain.FragmentsWithoutBuildfile) == 0 &&
		len(chain.ComponentsWithoutTests) == 0 &&
		len(chain.OrphanedReferences) == 0 {
		return nil
	}

	return chain
}

// parseSourceRefs extracts intent slugs from a Source field value.
// Input: "@feature/intent-slug, @feature/other-slug" → ["intent-slug", "other-slug"]
// Only returns slugs matching the given feature.
func parseSourceRefs(source, feature string) []string {
	var slugs []string
	prefix := "@" + feature + "/"

	for _, ref := range strings.Split(source, ",") {
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(ref, prefix) {
			slugs = append(slugs, strings.TrimPrefix(ref, prefix))
		}
	}
	return slugs
}

// parseBuildfileRefs extracts component names and their source fragment slugs from a buildfile.
func parseBuildfileRefs(path, feature string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var bf struct {
		Components map[string]struct {
			Source string `yaml:"source"`
		} `yaml:"components"`
	}

	if err := yaml.Unmarshal(data, &bf); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	prefix := "@" + feature + "/"
	for name, comp := range bf.Components {
		sourceSlug := ""
		if strings.HasPrefix(comp.Source, prefix) {
			sourceSlug = parser.Slugify(strings.TrimPrefix(comp.Source, prefix))
		}
		result[name] = sourceSlug
	}
	return result, nil
}

// parseTestcaseRefs extracts the set of component names that have test suites.
func parseTestcaseRefs(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tc struct {
		Suites []struct {
			Component string `yaml:"component"`
		} `yaml:"suites"`
	}

	if err := yaml.Unmarshal(data, &tc); err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, suite := range tc.Suites {
		result[suite.Component] = true
	}
	return result, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
