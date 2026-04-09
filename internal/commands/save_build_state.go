package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/parlay/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var saveBuildStateCmd = &cobra.Command{
	Use:   "save-build-state",
	Short: "Atomically commit the project-level build state (all features + project baseline + code hashes)",
	Long: `Commit a successful end-to-end generation at the project level by
atomically writing:

  1. Per-feature baselines for ALL features (.parlay/build/<feature>/.baseline.yaml)
  2. Project-level baseline (.parlay/build/_project/.baseline.yaml) with
     merged section hashes across all features
  3. Project-level code hashes (.parlay/build/_project/.code-hashes.yaml)
     tracking ALL generated files

This command MUST be invoked only as the final step of /parlay-generate-code
(project-level), after tests pass. All files are written using the
write-then-rename pattern for atomicity.`,
	Args: cobra.NoArgs,
	RunE: runSaveBuildState,
}

var saveBuildStateSourceRoot string

func init() {
	saveBuildStateCmd.Flags().StringVar(&saveBuildStateSourceRoot, "source-root", "",
		"Path to the source root containing generated files (matches the adapter's file-conventions.source-root)")
	saveBuildStateCmd.MarkFlagRequired("source-root")
}

func runSaveBuildState(cmd *cobra.Command, args []string) error {
	result, err := saveProjectBuildState(saveBuildStateSourceRoot)
	if err != nil {
		return err
	}

	fmt.Printf("Build state committed (project-level):\n")
	for _, fr := range result.Features {
		fmt.Printf("  %s: %d intents, %d dialogs, %d fragments\n",
			fr.Slug, fr.IntentCount, fr.DialogCount, fr.FragmentCount)
	}
	fmt.Printf("  project baseline: %s\n", projectBaselinePath())
	fmt.Printf("  code-hashes:      %s (%d files)\n",
		projectCodeHashesPath(), result.FileCount)
	return nil
}

// projectSaveResult is the summary returned by saveProjectBuildState.
type projectSaveResult struct {
	Features []featureSaveResult
	FileCount int
}

type featureSaveResult struct {
	Slug          string
	IntentCount   int
	DialogCount   int
	FragmentCount int
}

// saveBuildStateForFeature is a per-feature save helper used by tests.
// The CLI command (save-build-state) is project-level; this function
// provides backward-compatible per-feature saves for unit tests that
// operate on a single feature in isolation.
func saveBuildStateForFeature(slug, sourceRoot string) error {
	baseline, err := buildBaseline(slug)
	if err != nil {
		return fmt.Errorf("compute baseline: %w", err)
	}
	bfPath := filepath.Join(config.BuildPath(slug), "buildfile.yaml")
	if sectionHashes, err := hashBuildfileSections(bfPath); err == nil && sectionHashes != nil {
		baseline.BuildfileSections = sectionHashes
	}
	baselineBytes, err := marshalBaseline(baseline)
	if err != nil {
		return err
	}
	blPath := baselinePath(slug)
	if err := os.MkdirAll(filepath.Dir(blPath), 0755); err != nil {
		return err
	}
	if err := writeFileAtomic(blPath, baselineBytes); err != nil {
		return err
	}

	hashes, _, err := buildCodeHashes(slug, sourceRoot)
	if err != nil {
		return err
	}
	hashesBytes, err := marshalCodeHashes(hashes)
	if err != nil {
		return err
	}
	chPath := codeHashesPath(slug)
	if err := os.MkdirAll(filepath.Dir(chPath), 0755); err != nil {
		return err
	}
	return writeFileAtomic(chPath, hashesBytes)
}

// projectCodeHashesPath returns the project-level code-hashes sidecar path.
func projectCodeHashesPath() string {
	return filepath.Join(config.ProjectBuildPath(), CodeHashesFile)
}

// saveProjectBuildState atomically commits the full project build state:
//   - Per-feature baselines for every feature (source hashes for parlay diff @feature)
//   - Project-level baseline (merged section hashes for parlay diff)
//   - Project-level code-hashes (all generated files for parlay verify-generated)
//
// This is the only sanctioned write path for these files. It MUST be
// invoked only as the final step of /parlay-generate-code, after tests pass.
func saveProjectBuildState(sourceRoot string) (*projectSaveResult, error) {
	features, err := discoverFeatures()
	if err != nil {
		return nil, fmt.Errorf("discover features: %w", err)
	}

	result := &projectSaveResult{}

	// --- Stage 1: Per-feature baselines ---
	for _, slug := range features {
		baseline, err := buildBaseline(slug)
		if err != nil {
			// Feature may not have intents yet — skip silently.
			continue
		}

		// Include per-feature buildfile section hashes (still useful for
		// per-feature diff @feature in the build-feature skill).
		bfPath := filepath.Join(config.BuildPath(slug), "buildfile.yaml")
		if sectionHashes, err := hashBuildfileSections(bfPath); err == nil && sectionHashes != nil {
			baseline.BuildfileSections = sectionHashes
		}

		baselineBytes, err := marshalBaseline(baseline)
		if err != nil {
			return nil, fmt.Errorf("marshal baseline for %s: %w", slug, err)
		}

		blPath := baselinePath(slug)
		if err := os.MkdirAll(filepath.Dir(blPath), 0755); err != nil {
			return nil, fmt.Errorf("create build dir for %s: %w", slug, err)
		}
		if err := writeFileAtomic(blPath, baselineBytes); err != nil {
			return nil, fmt.Errorf("write baseline for %s: %w", slug, err)
		}

		fr := featureSaveResult{Slug: slug, IntentCount: len(baseline.Intents)}
		if baseline.Sources != nil {
			fr.DialogCount = len(baseline.Sources.Dialogs)
			fr.FragmentCount = len(baseline.Sources.SurfaceFragments)
		}
		result.Features = append(result.Features, fr)
	}

	// --- Stage 2: Project-level baseline (merged section hashes) ---
	mergedSections := hashMergedBuildfileSections(features)
	projectBL := &ProjectBaseline{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		MergedSections: mergedSections,
	}
	projectBLBytes, err := yaml.Marshal(projectBL)
	if err != nil {
		return nil, fmt.Errorf("marshal project baseline: %w", err)
	}
	if err := os.MkdirAll(config.ProjectBuildPath(), 0755); err != nil {
		return nil, fmt.Errorf("create project build dir: %w", err)
	}
	if err := writeFileAtomic(projectBaselinePath(), projectBLBytes); err != nil {
		return nil, fmt.Errorf("write project baseline: %w", err)
	}

	// --- Stage 3: Project-level code-hashes (all generated files) ---
	// Scan the source root for ALL marker-tagged files, regardless of
	// feature. This includes feature-scoped files (parlay-component:) and
	// project-scoped files (parlay-scope: project + parlay-section:).
	hashes, _, err := buildCodeHashes("", sourceRoot) // empty slug = accept all features
	if err != nil {
		return nil, fmt.Errorf("compute project code hashes: %w", err)
	}
	hashesBytes, err := marshalCodeHashes(hashes)
	if err != nil {
		return nil, fmt.Errorf("marshal project code hashes: %w", err)
	}
	if err := writeFileAtomic(projectCodeHashesPath(), hashesBytes); err != nil {
		return nil, fmt.Errorf("write project code hashes: %w", err)
	}
	result.FileCount = len(hashes.Files)

	return result, nil
}

// writeFileAtomic writes data to path using the write-then-rename pattern:
// data is first written to a temp file in the same directory, then the
// temp file is fsync'd and atomically renamed over the destination.
//
// On POSIX, rename within a single filesystem is atomic, so the destination
// always contains either the previous content or the new content — never a
// partially-written intermediate state. The temp file is created in the same
// directory as the destination to keep the rename on the same filesystem
// (a cross-filesystem rename would silently fall back to copy+delete and
// lose atomicity).
//
// On any error, the temp file is removed before returning.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	// Defer cleanup runs on every error path. On success, the rename has
	// already replaced tmpPath with the destination, so Stat returns
	// not-exist and Remove is a no-op.
	defer func() {
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	// Sync to disk before rename. Without this, a crash between rename and
	// fsync could leave a renamed-but-empty file. Sync is cheap for the
	// small files this is used for.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// CreateTemp creates files with mode 0600. Restore the standard
	// 0644 mode that os.WriteFile would have produced.
	if err := os.Chmod(tmpPath, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}
