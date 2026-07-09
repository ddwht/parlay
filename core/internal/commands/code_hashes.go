package commands

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

// CodeHashesFile is the sidecar filename inside .parlay/build/<feature>/.
const CodeHashesFile = ".code-hashes.yaml"

// CodeHashes is the on-disk schema for tracking generated-file content
// hashes. Used by parlay verify-generated to detect user edits to files
// that the tool considers "stable" (i.e., would otherwise be skipped
// during incremental code generation).
//
// The map key is the file path relative to the project root, exactly as
// recorded by parlay save-code-hashes when the file was last generated.
type CodeHashes struct {
	GeneratedAt string                   `yaml:"generated-at"`
	Files       map[string]CodeHashEntry `yaml:"files"`
}

// CodeHashEntry pairs a generated file's owning component with the
// content hash captured at generation time.
type CodeHashEntry struct {
	Component string `yaml:"component"`
	Hash      string `yaml:"hash"`
}

// codeHashesPath returns the canonical sidecar location for a feature.
func codeHashesPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), CodeHashesFile)
}

// loadCodeHashes reads the sidecar file for a feature. Returns nil (no
// error) when the file does not exist — that's the first-generation case.
func loadCodeHashes(cfg *config.Context, slug string) (*CodeHashes, error) {
	data, err := os.ReadFile(codeHashesPath(cfg, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hashes CodeHashes
	if err := yaml.Unmarshal(data, &hashes); err != nil {
		return nil, fmt.Errorf("invalid code-hashes file: %w", err)
	}
	return &hashes, nil
}

// loadProjectCodeHashes reads the project-level code-hashes sidecar
// (.parlay/build/_project/.code-hashes.yaml). Returns nil (no error)
// when the file does not exist — the first-generation case, or a
// project that has never run save-build-state.
func loadProjectCodeHashes(cfg *config.Context) (*CodeHashes, error) {
	data, err := os.ReadFile(projectCodeHashesPath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hashes CodeHashes
	if err := yaml.Unmarshal(data, &hashes); err != nil {
		return nil, fmt.Errorf("invalid project code-hashes file: %w", err)
	}
	return &hashes, nil
}

// filesDroppedBySourceRootNarrowing compares a previous project-level
// CodeHashes snapshot against a freshly computed one and returns every
// file the old snapshot tracked that the new one does not — restricted
// to files that still exist on disk. A file legitimately deleted since
// the last run is not a narrowing signal (it's supposed to disappear
// from tracking); a file that still exists on disk but fell out of
// tracking means the --source-root passed this run doesn't cover
// ground the previous run's source-root did, which silently shrinks
// what verify-generated can ever check again unless caught here.
func filesDroppedBySourceRootNarrowing(previous, current *CodeHashes) []string {
	if previous == nil {
		return nil
	}
	var dropped []string
	for path := range previous.Files {
		if _, stillTracked := current.Files[path]; stillTracked {
			continue
		}
		if _, statErr := os.Stat(path); statErr == nil {
			dropped = append(dropped, path)
		}
	}
	sort.Strings(dropped)
	return dropped
}

// saveCodeHashes writes the sidecar file for a feature. Creates the
// parent directory if needed.
func saveCodeHashes(cfg *config.Context, slug string, hashes *CodeHashes) error {
	path := codeHashesPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(hashes)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// hashFileContent returns a 16-char hex sha256 prefix of the file at path.
// Matches the granularity used by baseline.go's sha256Hex helper so that
// hash strings are visually consistent across the tool.
func hashFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8]), nil
}

// buildCodeHashes scans a source root for parlay markers, hashes each file,
// and returns a CodeHashes struct ready for serialization. Markers belonging
// to a different feature are skipped (returned as the second value). Does
// not touch disk; callers (typically saveBuildState) are responsible for
// writing. cfg is currently unused inside this helper but accepted to keep
// the signature consistent with the surrounding migration.
func buildCodeHashes(cfg *config.Context, slug, sourceRoot string) (*CodeHashes, int, error) {
	_ = cfg
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
		// When slug is non-empty (per-feature mode): only record files
		// belonging to this feature or feature-less files. When slug is
		// empty (project-level mode): accept all files regardless of feature.
		if slug != "" && marker.Feature != "" && marker.Feature != slug {
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

// marshalCodeHashes serializes a CodeHashes struct to YAML bytes for atomic
// disk writes. Symmetric with marshalBaseline.
func marshalCodeHashes(h *CodeHashes) ([]byte, error) {
	return yaml.Marshal(h)
}
