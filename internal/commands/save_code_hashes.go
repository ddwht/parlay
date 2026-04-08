package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var saveCodeHashesCmd = &cobra.Command{
	Use:   "save-code-hashes <@feature>",
	Short: "Scan a source root for parlay markers and write the .code-hashes.yaml sidecar",
	Long: `Walk the source root, find every file with a parlay marker, hash its
content, and write .parlay/build/<feature>/.code-hashes.yaml. Called by
/parlay-generate-code after writing generated files so that subsequent
parlay verify-generated invocations can detect user edits.`,
	Args: cobra.ExactArgs(1),
	RunE: runSaveCodeHashes,
}

var saveCodeHashesSourceRoot string

func init() {
	saveCodeHashesCmd.Flags().StringVar(&saveCodeHashesSourceRoot, "source-root", "",
		"Path to the source root to scan (typically the adapter's file-conventions.source-root for the feature)")
	saveCodeHashesCmd.MarkFlagRequired("source-root")
}

func runSaveCodeHashes(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")

	hashes, skipped, err := buildCodeHashes(slug, saveCodeHashesSourceRoot)
	if err != nil {
		return err
	}

	if err := saveCodeHashes(slug, hashes); err != nil {
		return fmt.Errorf("save failed: %w", err)
	}

	fmt.Printf("Code hashes saved: %s (%d files",
		codeHashesPath(slug), len(hashes.Files))
	if skipped > 0 {
		fmt.Printf(", %d skipped — different feature", skipped)
	}
	fmt.Println(")")
	return nil
}

// buildCodeHashes scans a source root for parlay markers, hashes each
// file, and returns a CodeHashes struct ready to be saved. Markers
// belonging to a different feature are skipped (returned as the second
// value). Exposed for tests.
func buildCodeHashes(slug, sourceRoot string) (*CodeHashes, int, error) {
	markers, err := parser.ScanGenerated(sourceRoot)
	if err != nil {
		return nil, 0, fmt.Errorf("scan failed: %w", err)
	}

	hashes := &CodeHashes{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Files:       make(map[string]CodeHashEntry, len(markers)),
	}

	skipped := 0
	for _, marker := range markers {
		// Only record files that belong to THIS feature. A source root
		// shared across features would otherwise pollute the sidecar.
		// If the marker has no feature field, accept it (legacy markers).
		if marker.Feature != "" && marker.Feature != slug {
			skipped++
			continue
		}
		hash, err := hashFileContent(marker.Path)
		if err != nil {
			return nil, 0, fmt.Errorf("hash failed for %s: %w", marker.Path, err)
		}
		hashes.Files[marker.Path] = CodeHashEntry{
			Component: marker.Component,
			Hash:      hash,
		}
	}

	return hashes, skipped, nil
}
