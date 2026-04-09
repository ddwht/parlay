package parser

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Marker is the parlay metadata embedded at the top of a generated file.
// It identifies ownership and provenance so the tool can track, verify,
// and incrementally regenerate generated files. Files without a marker
// are user-owned and must not be modified or deleted by parlay tooling.
//
// Three marker variants:
//
//	Component file:   parlay-component: X
//	Component test:   parlay-component: X + parlay-artifact: test
//	Section-derived:  parlay-section: models (or routes, fixtures, etc.)
//
// A marker is valid if it has at least one of Component or Section.
type Marker struct {
	Feature   string `json:"feature" yaml:"feature"`
	Component string `json:"component,omitempty" yaml:"component,omitempty"`
	Section   string `json:"section,omitempty" yaml:"section,omitempty"`
	Artifact  string `json:"artifact,omitempty" yaml:"artifact,omitempty"`
	Path      string `json:"path" yaml:"path"`
}

// markerScanLimit is the number of leading lines a file is scanned for a
// parlay marker. Markers must appear at the top of the file.
const markerScanLimit = 20

// commentPrefixes are the comment leaders this parser recognizes when
// looking for parlay-* fields. Extending to other styles (HTML <!-- -->,
// CSS /* */) is straightforward — add a stripper here.
var commentPrefixes = []string{"//", "#"}

// ParseMarker reads the first markerScanLimit lines of the file at path
// and returns the parlay marker found there, or nil if no marker exists.
// Returns an error only if the file cannot be opened or read; an absent
// marker is not an error.
func ParseMarker(path string) (*Marker, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseMarkerFromReader(f, path)
}

func parseMarkerFromReader(r io.Reader, path string) (*Marker, error) {
	scanner := bufio.NewScanner(r)
	var marker *Marker
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		if lineCount > markerScanLimit {
			break
		}
		stripped := stripCommentPrefix(strings.TrimSpace(scanner.Text()))
		if feature, ok := matchField(stripped, "parlay-feature:"); ok {
			if marker == nil {
				marker = &Marker{Path: path}
			}
			marker.Feature = feature
		}
		if component, ok := matchField(stripped, "parlay-component:"); ok {
			if marker == nil {
				marker = &Marker{Path: path}
			}
			marker.Component = component
		}
		if section, ok := matchField(stripped, "parlay-section:"); ok {
			if marker == nil {
				marker = &Marker{Path: path}
			}
			marker.Section = section
		}
		if artifact, ok := matchField(stripped, "parlay-artifact:"); ok {
			if marker == nil {
				marker = &Marker{Path: path}
			}
			marker.Artifact = artifact
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	// A marker is valid if it identifies at least one of: a component
	// (implementation or test file) or a section (cross-cutting file).
	if marker == nil || (marker.Component == "" && marker.Section == "") {
		return nil, nil
	}
	return marker, nil
}

func stripCommentPrefix(line string) string {
	for _, prefix := range commentPrefixes {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return line
}

func matchField(line, prefix string) (string, bool) {
	if strings.HasPrefix(line, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
	}
	return "", false
}

// ScanGenerated walks rootDir recursively and returns every file containing
// a parlay marker. Hidden directories (names starting with '.') and common
// non-source dirs (node_modules, vendor, dist, build) are skipped. Files
// that fail to open or parse are silently skipped — they cannot have
// markers if we can't read them.
func ScanGenerated(rootDir string) ([]Marker, error) {
	skipDirs := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		"build":        true,
	}

	var markers []Marker
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			// Skip hidden dirs (.git, .parlay, .vscode, etc.) and known
			// non-source directories. The root itself may be a hidden
			// dir; don't skip the root.
			if path != rootDir && (strings.HasPrefix(name, ".") || skipDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		marker, err := ParseMarker(path)
		if err != nil {
			return nil // unreadable file is not a fatal error for the scan
		}
		if marker != nil {
			markers = append(markers, *marker)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return markers, nil
}
