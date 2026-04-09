package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var diffCmd = &cobra.Command{
	Use:   "diff <@feature>",
	Short: "Show what changed since the last build (JSON output for agent consumption)",
	Long: `Compare current sources (intents/dialogs/surface) against the last
saved baseline, and report which buildfile components are stable / dirty
/ removed. Used by build-feature and generate-code skills to do
incremental rebuilds at component granularity.`,
	Args: cobra.ExactArgs(1),
	RunE: runDiff,
}

// sourceLevelDiff reports per-element changes for one source category
// (intents, dialogs, or surface fragments).
type sourceLevelDiff struct {
	Changed []string `json:"changed,omitempty"`
	New     []string `json:"new,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

// componentImpact describes a single dirty component and which of its
// upstream sources changed.
type componentImpact struct {
	Name           string   `json:"name"`
	ChangedSources []string `json:"changed_sources,omitempty"`
}

// componentDiff is the per-component impact analysis.
type componentDiff struct {
	Stable  []string          `json:"stable,omitempty"`
	Dirty   []componentImpact `json:"dirty,omitempty"`
	Removed []string          `json:"removed,omitempty"`
}

// diffOutput is the top-level JSON shape for `parlay diff @<feature>`.
type diffOutput struct {
	Feature      string            `json:"feature"`
	FirstBuild   bool              `json:"first_build"`
	HasBuildfile bool              `json:"has_buildfile"`
	Intents      sourceLevelDiff   `json:"intents"`
	Dialogs      sourceLevelDiff   `json:"dialogs"`
	Fragments    sourceLevelDiff   `json:"surface_fragments"`
	Components   componentDiff     `json:"components"`
	Sections     map[string]string `json:"sections,omitempty"`
}

func runDiff(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)

	output := diffOutput{Feature: slug}

	// Load existing baseline (or treat as first build).
	var storedBaseline Baseline
	blData, err := os.ReadFile(baselinePath(slug))
	if err != nil {
		output.FirstBuild = true
	} else {
		if err := yaml.Unmarshal(blData, &storedBaseline); err != nil {
			return fmt.Errorf("invalid baseline: %w", err)
		}
		// Older baselines without Sources are treated as first build for diff
		// purposes — there's nothing to compare against.
		if storedBaseline.Sources == nil {
			output.FirstBuild = true
		}
	}
	stored := storedBaseline.Sources
	if stored == nil {
		stored = &HashedSources{}
	}

	// Parse current sources. Missing files are not errors — they yield
	// empty maps and the relevant section reports "all stored entries
	// removed."
	currentIntents, _ := parser.ParseIntentsFile(filepath.Join(featurePath, "intents.md"))
	currentDialogs, _ := parser.ParseDialogsFile(filepath.Join(featurePath, "dialogs.md"))
	currentFragments, _ := parser.ParseSurfaceFile(filepath.Join(featurePath, "surface.md"))

	currentIntentHashes := make(map[string]string, len(currentIntents))
	for _, intent := range currentIntents {
		currentIntentHashes[intent.Slug] = hashIntentContent(intent)
	}
	currentDialogHashes := make(map[string]string, len(currentDialogs))
	for _, dialog := range currentDialogs {
		currentDialogHashes[dialog.Slug] = hashDialogContent(dialog)
	}
	currentFragmentHashes := make(map[string]string, len(currentFragments))
	fragmentBySlug := make(map[string]*parser.Fragment, len(currentFragments))
	for i := range currentFragments {
		fragSlug := parser.Slugify(currentFragments[i].Name)
		currentFragmentHashes[fragSlug] = hashFragmentContent(currentFragments[i])
		fragmentBySlug[fragSlug] = &currentFragments[i]
	}

	output.Intents = diffStringMap(stored.Intents, currentIntentHashes)
	output.Dialogs = diffStringMap(stored.Dialogs, currentDialogHashes)
	output.Fragments = diffStringMap(stored.SurfaceFragments, currentFragmentHashes)

	// Component-level impact analysis is only meaningful when there's a
	// committed baseline AND a buildfile to walk. On first_build (no
	// baseline yet) we leave Components empty: there's nothing to compare
	// against, so classifying components as "stable" would be misleading.
	// The agent uses parlay verify-generated's has_hashes signal to confirm
	// "no committed code state" and treats every component as new.
	buildfilePath := filepath.Join(config.BuildPath(slug), "buildfile.yaml")
	if fileExists(buildfilePath) {
		output.HasBuildfile = true
		if !output.FirstBuild {
			output.Components = computeComponentImpact(
				buildfilePath, slug,
				currentIntents, currentDialogs, fragmentBySlug,
				output.Intents, output.Dialogs, output.Fragments,
			)

			// Section-level diff for cross-cutting files (models → store.go,
			// routes → main.go, etc.). Compares current buildfile section
			// hashes to stored ones in the baseline.
			output.Sections = computeSectionDiff(buildfilePath, storedBaseline.BuildfileSections)
		}
	}

	return emitDiffJSON(&output)
}

// computeSectionDiff hashes the current buildfile's major sections and
// compares to the stored section hashes from the baseline. Returns a map
// of section name → "changed" | "stable" | "new" | "removed". Used by
// the generate-code skill to determine which cross-cutting files need
// regeneration.
func computeSectionDiff(buildfilePath string, storedSections map[string]string) map[string]string {
	currentSections, err := hashBuildfileSections(buildfilePath)
	if err != nil || currentSections == nil {
		return nil
	}
	if storedSections == nil {
		storedSections = make(map[string]string)
	}

	result := make(map[string]string)
	for name, currentHash := range currentSections {
		storedHash, exists := storedSections[name]
		if !exists {
			result[name] = "new"
		} else if currentHash != storedHash {
			result[name] = "changed"
		} else {
			result[name] = "stable"
		}
	}
	for name := range storedSections {
		if _, exists := currentSections[name]; !exists {
			result[name] = "removed"
		}
	}
	return result
}

// computeComponentImpact walks each component in the buildfile, traces
// its dependency chain (component → fragment → intents → dialogs), and
// classifies it as stable / dirty / removed based on which upstream
// sources changed.
func computeComponentImpact(
	buildfilePath, feature string,
	currentIntents []parser.Intent,
	currentDialogs []parser.Dialog,
	fragmentBySlug map[string]*parser.Fragment,
	intentsDiff, dialogsDiff, fragmentsDiff sourceLevelDiff,
) componentDiff {
	var result componentDiff

	bfRefs, err := parseBuildfileRefs(buildfilePath, feature)
	if err != nil {
		// Buildfile is malformed — treat all components as dirty so the
		// agent regenerates from scratch.
		return result
	}

	changedIntents := makeSet(intentsDiff.Changed)
	changedDialogs := makeSet(dialogsDiff.Changed)
	changedFragments := makeSet(fragmentsDiff.Changed)
	removedFragments := makeSet(fragmentsDiff.Removed)

	intentBySlug := make(map[string]*parser.Intent, len(currentIntents))
	for i := range currentIntents {
		intentBySlug[currentIntents[i].Slug] = &currentIntents[i]
	}

	// Sort component names for deterministic output.
	compNames := make([]string, 0, len(bfRefs))
	for name := range bfRefs {
		compNames = append(compNames, name)
	}
	sort.Strings(compNames)

	for _, compName := range compNames {
		fragSlug := bfRefs[compName]
		if fragSlug == "" {
			// Component has no fragment ref — can't trace, treat as dirty.
			result.Dirty = append(result.Dirty, componentImpact{
				Name:           compName,
				ChangedSources: []string{"untraceable:no-source-ref"},
			})
			continue
		}

		// If the fragment was removed entirely, the component is removed.
		if removedFragments[fragSlug] {
			result.Removed = append(result.Removed, compName)
			continue
		}

		frag, ok := fragmentBySlug[fragSlug]
		if !ok {
			// Fragment doesn't exist in current surface and isn't in
			// removed (which would mean it was in baseline). Either the
			// buildfile references a fragment that was never in baseline
			// (unusual) or surface.md is missing. Treat as removed.
			result.Removed = append(result.Removed, compName)
			continue
		}

		var changedSources []string
		if changedFragments[fragSlug] {
			changedSources = append(changedSources, "fragment:"+fragSlug)
		}

		// Walk fragment.Source for intent refs, then find dialogs covering them.
		intentSlugs := parseSourceRefs(frag.Source, feature)
		seenDialogs := make(map[string]bool)
		for _, intentSlug := range intentSlugs {
			if changedIntents[intentSlug] {
				changedSources = append(changedSources, "intent:"+intentSlug)
			}
			intent, ok := intentBySlug[intentSlug]
			if !ok {
				continue
			}
			for _, dialog := range currentDialogs {
				if seenDialogs[dialog.Slug] {
					continue
				}
				if matchesIntent(*intent, dialog) && changedDialogs[dialog.Slug] {
					changedSources = append(changedSources, "dialog:"+dialog.Slug)
					seenDialogs[dialog.Slug] = true
				}
			}
		}

		if len(changedSources) > 0 {
			result.Dirty = append(result.Dirty, componentImpact{
				Name:           compName,
				ChangedSources: changedSources,
			})
		} else {
			result.Stable = append(result.Stable, compName)
		}
	}

	return result
}

// diffStringMap compares stored and current hash maps and returns the
// changed / new / removed slugs (sorted for deterministic output).
func diffStringMap(stored, current map[string]string) sourceLevelDiff {
	var d sourceLevelDiff
	for slug, currentHash := range current {
		storedHash, exists := stored[slug]
		if !exists {
			d.New = append(d.New, slug)
		} else if storedHash != currentHash {
			d.Changed = append(d.Changed, slug)
		}
	}
	for slug := range stored {
		if _, exists := current[slug]; !exists {
			d.Removed = append(d.Removed, slug)
		}
	}
	sort.Strings(d.Changed)
	sort.Strings(d.New)
	sort.Strings(d.Removed)
	return d
}

func makeSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

func emitDiffJSON(output *diffOutput) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
