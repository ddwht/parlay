package parser

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Fragment struct {
	Name    string
	Shows   string
	Actions string
	Source  string
	Page    string
	Region  string
	Order   int
	Notes   []string
	Feature string // populated during scanning, not from file
}

// ParseSurfaceFile reads a surface artifact at the supplied path. When the
// file's basename ends in .yaml, the YAML form is loaded via
// LoadSurfaceYAML; otherwise the legacy markdown parser runs. Both forms
// produce identical []Fragment output, so callers do not branch on
// serialization.
//
// parlay-extends: parlay-tool/multi-adapter/surface-yaml-and-migrator
func ParseSurfaceFile(path string) ([]Fragment, error) {
	if strings.HasSuffix(path, ".yaml") {
		return LoadSurfaceYAML(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var fragments []Fragment
	var current *Fragment

	scanner := bufio.NewScanner(f)
	var currentList *[]string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			if current != nil {
				fragments = append(fragments, *current)
			}
			current = &Fragment{
				Name: strings.TrimPrefix(line, "## "),
			}
			currentList = nil
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "**Shows**:") {
			current.Shows = extractField(line, "**Shows**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Actions**:") {
			current.Actions = extractField(line, "**Actions**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Source**:") {
			current.Source = extractField(line, "**Source**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Page**:") {
			current.Page = extractField(line, "**Page**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Region**:") {
			current.Region = extractField(line, "**Region**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Order**:") {
			val := extractField(line, "**Order**:")
			current.Order, _ = strconv.Atoi(val)
			currentList = nil
		} else if strings.HasPrefix(line, "**Notes**:") {
			currentList = &current.Notes
		} else if strings.HasPrefix(line, "- ") && currentList != nil {
			*currentList = append(*currentList, strings.TrimPrefix(line, "- "))
		} else if line == "---" {
			currentList = nil
		}
	}

	if current != nil {
		fragments = append(fragments, *current)
	}

	return fragments, scanner.Err()
}

// ScanAllSurfaces finds all surface.md files across features and returns fragments with Feature populated.
func ScanAllSurfaces(specDir string) ([]Fragment, error) {
	intentsDir := filepath.Join(specDir, "intents")
	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		return nil, err
	}

	var all []Fragment
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		featureSlug := entry.Name()
		featureDir := filepath.Join(intentsDir, featureSlug)

		// A directory under spec/intents/ is either a feature or an
		// initiative holding features. Recurse one level so
		// initiative-nested features are scanned too — they were
		// previously invisible to page assembly entirely.
		if nested := scanNestedSurfaces(featureDir, featureSlug); len(nested) > 0 {
			all = append(all, nested...)
			continue
		}

		// Resolve surface.yaml ahead of surface.md rather than hardcoding
		// the legacy name. surface.yaml is the target format and what
		// create-artifacts emits, so hardcoding surface.md made page
		// assembly find nothing on any spec-conformant project — and say
		// so with "No fragments target page X", which reads as "you have
		// not authored them yet" rather than "this format is unsupported".
		surfacePath := ResolveSurfacePath(featureDir)
		if surfacePath == "" {
			continue // feature may not have a surface yet
		}

		fragments, err := ParseSurfaceFile(surfacePath)
		if err != nil {
			continue
		}

		for i := range fragments {
			fragments[i].Feature = featureSlug
		}
		all = append(all, fragments...)
	}

	return all, nil
}

// scanNestedSurfaces returns fragments for features nested one level under
// an initiative directory, with Feature set to the qualified
// "<initiative>/<feature>" form. Returns nil when dir holds no nested
// feature surfaces — i.e. when dir is itself a feature.
func scanNestedSurfaces(dir, initiativeSlug string) []Fragment {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Fragment
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childDir := filepath.Join(dir, e.Name())
		surfacePath := ResolveSurfacePath(childDir)
		if surfacePath == "" {
			continue
		}
		fragments, err := ParseSurfaceFile(surfacePath)
		if err != nil {
			continue
		}
		qualified := initiativeSlug + "/" + e.Name()
		for i := range fragments {
			fragments[i].Feature = qualified
		}
		out = append(out, fragments...)
	}
	return out
}
