package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
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

var (
	saveBuildStateSourceRoot string
	saveBuildStateEmitted    string
	saveBuildStateStrict     bool
	saveBuildStatePartial    bool
)

// DefaultEmittedManifest is where codegen declares what it wrote.
//
// A manifest rather than a per-file stamp or a stdin list, and both
// alternatives were rejected for reasons worth keeping:
//
//   - A per-file stamp is self-defeating: the stamp changes the hash it is
//     stamping, and it survives a human edit, so it certifies the wrong thing.
//   - A stdin list means a 124-line heredoc with quoting risk and leaves no
//     artifact to inspect afterwards.
//
// The decisive property is that a manifest can be appended one line per
// emission, as the work happens. "Now list everything you wrote", asked at
// the end of a long generation run, is exactly the recall an agent gets
// wrong.
const DefaultEmittedManifest = ".emitted"

func init() {
	saveBuildStateCmd.Flags().StringVar(&saveBuildStateSourceRoot, "source-root", "",
		"Path to the source root containing generated files (matches the adapter's file-conventions.source-root)")
	saveBuildStateCmd.MarkFlagRequired("source-root")
	saveBuildStateCmd.Flags().StringVar(&saveBuildStateEmitted, "emitted", "",
		"Path to the newline-delimited manifest of files this run wrote (default .parlay/build/_project/.emitted when present)")
	saveBuildStateCmd.Flags().BoolVar(&saveBuildStateStrict, "strict", false,
		"Fail instead of recording when a generated file was changed outside codegen")
	saveBuildStateCmd.Flags().BoolVar(&saveBuildStatePartial, "partial", false,
		"This run regenerated only part of the project (e.g. `parlay refine`); makes --emitted mandatory")
}

// requireEmittedForPartial refuses a partial save with no emission manifest.
//
// A whole-project run without a manifest degrades: every entry becomes
// unknown, verify-generated can say nothing, and the warning above says so
// loudly. A PARTIAL run without one does something worse than degrade — it
// is affirmatively wrong. The manifest is what tells the classifier which
// files this run wrote; with it, the files a partial run did not touch keep
// the verdict they already had, because an unchanged file with no emission
// carries its previous entry forward. Without it, a run that rewrote three
// files marks every tracked file in the project as unknown, and `--strict`
// then fails on all of them.
//
// So the flag does not add a capability, it removes a way to be silently
// wrong — which is the only thing a partial caller could want here.
func requireEmittedForPartial(cfg *config.Context, emitted *emissionDeclaration) error {
	if !saveBuildStatePartial || emitted != nil {
		return nil
	}
	return fmt.Errorf("--partial requires --emitted: a partial regeneration with no manifest would mark every tracked file in the project as unknown on the strength of a run that touched a handful, and --strict would then fail on all of them. Have the run append each file it wrote to %s",
		filepath.Join(cfg.ProjectBuildPath(), DefaultEmittedManifest))
}

func runSaveBuildState(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	result, err := saveProjectBuildState(cmd, cfg, saveBuildStateSourceRoot)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Build state committed (project-level):\n")
	for _, fr := range result.Features {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %d intents, %d dialogs, %d fragments\n",
			fr.Slug, fr.IntentCount, fr.DialogCount, fr.FragmentCount)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  project baseline: %s\n", projectBaselinePath(cfg))
	fmt.Fprintf(cmd.OutOrStdout(), "  code-hashes:      %s (%d files)\n",
		projectCodeHashesPath(cfg), result.FileCount)
	return nil
}

// projectSaveResult is the summary returned by saveProjectBuildState.
type projectSaveResult struct {
	Features  []featureSaveResult
	FileCount int
	// Adopted names files that changed outside codegen this run. Recorded so
	// the caller can report them; the save itself still succeeds.
	Adopted []string
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
func saveBuildStateForFeature(cfg *config.Context, slug, sourceRoot string) error {
	baseline, err := buildBaseline(cfg, slug)
	if err != nil {
		return fmt.Errorf("compute baseline: %w", err)
	}
	bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	if sectionHashes, err := hashBuildfileSections(bfPath); err == nil && sectionHashes != nil {
		baseline.BuildfileSections = sectionHashes
	}
	baselineBytes, err := marshalBaseline(baseline)
	if err != nil {
		return err
	}
	blPath := baselinePath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(blPath), 0755); err != nil {
		return err
	}
	if err := writeFileAtomic(blPath, baselineBytes); err != nil {
		return err
	}

	hashes, _, err := buildCodeHashes(cfg, slug, sourceRoot)
	if err != nil {
		return err
	}
	hashesBytes, err := marshalCodeHashes(hashes)
	if err != nil {
		return err
	}
	chPath := codeHashesPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(chPath), 0755); err != nil {
		return err
	}
	return writeFileAtomic(chPath, hashesBytes)
}

// projectCodeHashesPath returns the project-level code-hashes sidecar path.
func projectCodeHashesPath(cfg *config.Context) string {
	return filepath.Join(cfg.ProjectBuildPath(), CodeHashesFile)
}

// saveProjectBuildState atomically commits the full project build state:
//   - Per-feature baselines for every feature (source hashes for parlay diff @feature)
//   - Project-level baseline (merged section hashes for parlay diff)
//   - Project-level code-hashes (all generated files for parlay verify-generated)
//
// This is the only sanctioned write path for these files. It MUST be
// invoked only as the final step of /parlay-generate-code, after tests pass.
func saveProjectBuildState(cmd *cobra.Command, cfg *config.Context, sourceRoot string) (*projectSaveResult, error) {
	features, err := discoverFeatures(cfg)
	if err != nil {
		return nil, fmt.Errorf("discover features: %w", err)
	}

	result := &projectSaveResult{}

	// --- Stage 1: Per-feature baselines ---
	for _, slug := range features {
		baseline, err := buildBaseline(cfg, slug)
		if err != nil {
			// Feature may not have intents yet — skip silently.
			continue
		}

		// Include per-feature buildfile section hashes (still useful for
		// per-feature diff @feature in the build-feature skill).
		bfPath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
		if sectionHashes, err := hashBuildfileSections(bfPath); err == nil && sectionHashes != nil {
			baseline.BuildfileSections = sectionHashes
		}

		baselineBytes, err := marshalBaseline(baseline)
		if err != nil {
			return nil, fmt.Errorf("marshal baseline for %s: %w", slug, err)
		}

		blPath := baselinePath(cfg, slug)
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
	mergedSections := hashMergedBuildfileSections(cfg, features)
	projectBL := &ProjectBaseline{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		MergedSections: mergedSections,
	}
	projectBLBytes, err := yaml.Marshal(projectBL)
	if err != nil {
		return nil, fmt.Errorf("marshal project baseline: %w", err)
	}
	if err := os.MkdirAll(cfg.ProjectBuildPath(), 0755); err != nil {
		return nil, fmt.Errorf("create project build dir: %w", err)
	}
	if err := writeFileAtomic(projectBaselinePath(cfg), projectBLBytes); err != nil {
		return nil, fmt.Errorf("write project baseline: %w", err)
	}

	// --- Stage 3: Project-level code-hashes (all generated files) ---
	// Scan the source root for ALL marker-tagged files, regardless of
	// feature. This includes feature-scoped files (parlay-component:) and
	// project-scoped files (parlay-scope: project + parlay-section:).
	// What codegen declared it wrote this run, and what the last run left
	// behind. Both are needed before classification: provenance is a fact
	// about who wrote a file, and neither the file nor its hash carries it.
	emitted, emittedPath, err := loadEmittedManifest(cfg, saveBuildStateEmitted)
	if err != nil {
		return nil, err
	}
	if err := requireEmittedForPartial(cfg, emitted); err != nil {
		return nil, err
	}
	previous, _ := loadProjectCodeHashes(cfg)

	// The unit declarations, resolved to concrete files. Read from
	// spec/intents/ here — which the CLI may do and codegen may not — and
	// projected into .parlay/ below so codegen can honour them without
	// reading the spec tree.
	authored, projection, err := resolveAuthoredUnits(cfg)
	if err != nil {
		return nil, err
	}

	hashes, _, err := buildCodeHashesWithProvenance(cfg, "", sourceRoot, emitted, previous, authored)
	if err != nil {
		return nil, fmt.Errorf("compute project code hashes: %w", err)
	}

	if err := writeAuthoredProjection(cfg, projection); err != nil {
		return nil, err
	}

	if emitted == nil {
		// Loud, once, on stderr. Without a declaration every entry is
		// unknown, verify-generated can say nothing about hand-edits, and a
		// silent degradation here would look exactly like the feature
		// working.
		//
		// Hand-authored files are excluded from the count: their provenance
		// comes from a declaration, not from the emission manifest, so they
		// are not among the files this warning is about. Counting them made
		// a project whose only tracked file was a unit's read "provenance
		// is unknown for all 1 tracked file(s)" about the one file whose
		// provenance was certain.
		unknownCount := 0
		for _, entry := range hashes.Files {
			if entry.Provenance != ProvenanceHandAuthored {
				unknownCount++
			}
		}
		if unknownCount > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"[WARN] no --emitted manifest: provenance is unknown for %d tracked file(s), so verify-generated cannot distinguish a regeneration from a hand-edit. Have generate-code append each file it writes to %s.\n",
				unknownCount, filepath.Join(cfg.ProjectBuildPath(), DefaultEmittedManifest))
		}
	}

	// Adoption is recorded and surfaced, not prevented. save-build-state runs
	// after tests pass at the end of a successful generation; refusing turns
	// a successful run into a failed one over a file the user may well have
	// edited deliberately — and a refused save leaves BOTH the baseline and
	// the code-hashes unwritten, breaking the consistency invariant in the
	// worse direction. The requirement is "detected and surfaced before any
	// overwrite", not "prevented". --strict exists for CI.
	var adopted []string
	for path, entry := range hashes.Files {
		if entry.Provenance == ProvenanceHandAuthored {
			// Never adopted. Adoption means "codegen's output changed and
			// codegen did not do it" — a unit file has no codegen output to
			// diverge from, so reporting one here would warn the author
			// that editing their own declared code was an irregularity.
			continue
		}
		if entry.Provenance == ProvenanceAdopted {
			adopted = append(adopted, path)
		}
	}
	sort.Strings(adopted)
	if len(adopted) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"[WARN] %d generated file(s) changed outside codegen and are recorded as adopted:\n", len(adopted))
		for _, p := range adopted {
			fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", p)
		}
		if saveBuildStateStrict {
			return nil, fmt.Errorf("--strict: %d generated file(s) were changed outside codegen", len(adopted))
		}
	}
	result.Adopted = adopted

	// Guard against a narrower --source-root silently shrinking what
	// verify-generated can ever check: in a multi-adapter project, a
	// caller might accidentally pass one adapter's own (narrower) source
	// root instead of the true project-wide root that spans every
	// adapter. Compare against the previous run's tracked file set —
	// files that vanished from tracking but still exist on disk are a
	// narrowing signal, not a legitimate deletion, so warn loudly rather
	// than silently committing a smaller CodeHashes than before.
	if previous, loadErr := loadProjectCodeHashes(cfg); loadErr == nil {
		if dropped := filesDroppedBySourceRootNarrowing(previous, hashes); len(dropped) > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"[WARN] --source-root %q no longer covers %d previously-tracked file(s) that still exist on disk — verify-generated will stop checking them. If this is unintended, pass the project-wide source root instead of a narrower one:\n",
				sourceRoot, len(dropped))
			for _, path := range dropped {
				fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", path)
			}
		}
	}

	hashesBytes, err := marshalCodeHashes(hashes)
	if err != nil {
		return nil, fmt.Errorf("marshal project code hashes: %w", err)
	}
	if err := writeFileAtomic(projectCodeHashesPath(cfg), hashesBytes); err != nil {
		return nil, fmt.Errorf("write project code hashes: %w", err)
	}
	result.FileCount = len(hashes.Files)

	// Consume the manifest. A stale .emitted left on disk would bless a later
	// run's files as generated on the strength of what a previous run wrote —
	// the exact silent blessing this whole mechanism exists to remove.
	if emittedPath != "" {
		_ = os.Remove(emittedPath)
	}

	return result, nil
}

// loadEmittedManifest reads the newline-delimited list of files codegen
// declared it wrote.
//
// Returns (nil, "", nil) only when no --emitted was passed AND no manifest
// exists at the default location. That is "the run did not say", which is a
// different state from "the run wrote nothing" and is classified differently:
// the former makes every file unknown, the latter makes every changed file
// adopted.
//
// Passing --emitted IS the declaration. So an explicitly passed path that
// does not exist reads as an EMPTY declaration — this run is
// provenance-tracked and emitted nothing — rather than as an error or as
// silence. Erroring would fail the save of a legitimate no-op regeneration;
// treating it as silence would quietly downgrade a tracked run, which looks
// identical to the feature working.
func loadEmittedManifest(cfg *config.Context, flagPath string) (*emissionDeclaration, string, error) {
	path := flagPath
	explicit := flagPath != ""
	if path == "" {
		candidate := filepath.Join(cfg.ProjectBuildPath(), DefaultEmittedManifest)
		if _, err := os.Stat(candidate); err != nil {
			return nil, "", nil
		}
		path = candidate
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if explicit && os.IsNotExist(err) {
			return &emissionDeclaration{Paths: map[string]bool{}}, "", nil
		}
		return nil, "", fmt.Errorf("read emitted manifest %s: %w", path, err)
	}
	decl := &emissionDeclaration{Paths: map[string]bool{}}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Normalized the same way check-write-set normalizes its paths, so a
		// manifest written with ./ prefixes or redundant separators matches
		// the scanner's view of the same file.
		decl.Paths[normalizeWriteSetPath(line)] = true
	}
	return decl, path, nil
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
