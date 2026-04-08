package commands

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/parser"
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
func codeHashesPath(slug string) string {
	return filepath.Join(config.BuildPath(slug), CodeHashesFile)
}

// loadCodeHashes reads the sidecar file for a feature. Returns nil (no
// error) when the file does not exist — that's the first-generation case.
func loadCodeHashes(slug string) (*CodeHashes, error) {
	data, err := os.ReadFile(codeHashesPath(slug))
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

// saveCodeHashes writes the sidecar file for a feature. Creates the
// parent directory if needed.
func saveCodeHashes(slug string, hashes *CodeHashes) error {
	path := codeHashesPath(slug)
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
// writing.
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

// marshalCodeHashes serializes a CodeHashes struct to YAML bytes for atomic
// disk writes. Symmetric with marshalBaseline.
func marshalCodeHashes(h *CodeHashes) ([]byte, error) {
	return yaml.Marshal(h)
}
