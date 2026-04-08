package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var saveBuildStateCmd = &cobra.Command{
	Use:   "save-build-state <@feature>",
	Short: "Atomically commit the build state for a feature (baseline + code hashes)",
	Long: `Commit a successful end-to-end generation by atomically writing both
.parlay/build/<feature>/.baseline.yaml (source state for incremental rebuilds
and drift detection) and .parlay/build/<feature>/.code-hashes.yaml (generated
file content hashes for hand-edit detection).

Both files represent the same point in time: the source state and the code
state at the end of a successful build → generate-code → tests-pass cycle.
They have a consistency invariant — neither is meaningful without the other,
and they must be updated together.

This command MUST be invoked only as the final step of the
/parlay-generate-code skill, after tests pass. It MUST NOT be invoked from
/parlay-build-feature alone (the baseline would commit source state without
corresponding code state, breaking the consistency invariant and stranding
the feature in a state where parlay diff reports everything stable but no
code exists).

Both files are written using the write-then-rename pattern: each is written
to a temp file in the same directory and then renamed atomically over the
destination, so a partial failure leaves the previous state intact.`,
	Args: cobra.ExactArgs(1),
	RunE: runSaveBuildState,
}

var saveBuildStateSourceRoot string

func init() {
	saveBuildStateCmd.Flags().StringVar(&saveBuildStateSourceRoot, "source-root", "",
		"Path to the source root containing generated files (matches the adapter's file-conventions.source-root)")
	saveBuildStateCmd.MarkFlagRequired("source-root")
}

func runSaveBuildState(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")

	result, err := saveBuildState(slug, saveBuildStateSourceRoot)
	if err != nil {
		return err
	}

	fmt.Printf("Build state committed for %s:\n", slug)
	fmt.Printf("  baseline:    %s (%d intents, %d dialogs, %d fragments)\n",
		baselinePath(slug),
		result.IntentCount, result.DialogCount, result.FragmentCount)
	fmt.Printf("  code-hashes: %s (%d files",
		codeHashesPath(slug), result.FileCount)
	if result.SkippedFiles > 0 {
		fmt.Printf(", %d skipped — different feature", result.SkippedFiles)
	}
	fmt.Println(")")
	return nil
}

// saveBuildStateResult is a small summary returned by saveBuildState for the
// CLI's user-facing report.
type saveBuildStateResult struct {
	IntentCount   int
	DialogCount   int
	FragmentCount int
	FileCount     int
	SkippedFiles  int
}

// saveBuildState atomically commits both the source baseline and the code
// hashes for a feature. This is the only sanctioned write path for either
// file: nothing else in the codebase should write .baseline.yaml or
// .code-hashes.yaml independently.
//
// Both files represent the state at the end of a successful end-to-end
// generation. The two writes use writeFileAtomic (write-then-rename) so
// either both files are updated or both are left at their previous state.
//
// Sequencing:
//  1. Compute both file contents in memory (no disk writes yet)
//  2. Ensure the destination directory exists
//  3. Write baseline atomically
//  4. Write code-hashes atomically
//
// If step 3 succeeds and step 4 fails, the baseline IS updated but the
// code-hashes is not. This is a known partial-failure window — see the
// comment on writeFileAtomic for why true two-file atomicity isn't
// achievable on POSIX without a journal.
func saveBuildState(slug, sourceRoot string) (*saveBuildStateResult, error) {
	// --- Stage 1: compute both file contents ---

	baseline, err := buildBaseline(slug)
	if err != nil {
		return nil, fmt.Errorf("compute baseline: %w", err)
	}
	baselineBytes, err := marshalBaseline(baseline)
	if err != nil {
		return nil, fmt.Errorf("marshal baseline: %w", err)
	}

	hashes, skipped, err := buildCodeHashes(slug, sourceRoot)
	if err != nil {
		return nil, fmt.Errorf("compute code hashes: %w", err)
	}
	hashesBytes, err := marshalCodeHashes(hashes)
	if err != nil {
		return nil, fmt.Errorf("marshal code hashes: %w", err)
	}

	// --- Stage 2: ensure destination directory exists ---

	blPath := baselinePath(slug)
	chPath := codeHashesPath(slug)
	if err := os.MkdirAll(filepath.Dir(blPath), 0755); err != nil {
		return nil, fmt.Errorf("create build dir: %w", err)
	}

	// --- Stage 3: atomic writes ---

	if err := writeFileAtomic(blPath, baselineBytes); err != nil {
		return nil, fmt.Errorf("write baseline: %w", err)
	}
	if err := writeFileAtomic(chPath, hashesBytes); err != nil {
		// Best effort: surface that the build state is now inconsistent.
		// True atomicity across two files would need a journal; we accept
		// this narrow window in exchange for the simplicity of plain files.
		return nil, fmt.Errorf("write code hashes (baseline updated, code hashes failed — re-run save-build-state to recover): %w", err)
	}

	dialogCount := 0
	fragmentCount := 0
	if baseline.Sources != nil {
		dialogCount = len(baseline.Sources.Dialogs)
		fragmentCount = len(baseline.Sources.SurfaceFragments)
	}
	return &saveBuildStateResult{
		IntentCount:   len(baseline.Intents),
		DialogCount:   dialogCount,
		FragmentCount: fragmentCount,
		FileCount:     len(hashes.Files),
		SkippedFiles:  skipped,
	}, nil
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
