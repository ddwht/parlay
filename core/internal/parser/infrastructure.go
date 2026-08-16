// parlay-feature: infrastructure-layer
// parlay-component: InfrastructureValidationResult
// parlay-extends: infrastructure-layer/schema-framework-agnostic-fields

package parser

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type InfraFragment struct {
	Name               string
	Affects            string
	Behavior           string
	Invariants         []string
	Source             string
	Caching            string
	BackwardCompatible string
	Notes              []string
	// DeliberatelySpecific carries a one-line justification for naming a
	// specific thing, and suppresses the portability lint for this
	// fragment. The justification is the mechanism, not decoration: an
	// empty marker is not honoured, because the point is to put a claim on
	// the record that a reviewer can disagree with. Nobody can disagree
	// with a warning that was never read.
	DeliberatelySpecific string
	// Feature is populated during a cross-feature scan
	// (ScanAllInfrastructure), not from the file — the same convention as
	// Fragment.Feature on the surface side. A single-file parse leaves it
	// empty.
	Feature string
}

func ParseInfrastructureFile(path string) ([]InfraFragment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var fragments []InfraFragment
	var current *InfraFragment
	var currentList *[]string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			if current != nil {
				fragments = append(fragments, *current)
			}
			current = &InfraFragment{
				Name: strings.TrimPrefix(line, "## "),
			}
			currentList = nil
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "**Affects**:") {
			current.Affects = extractField(line, "**Affects**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Behavior**:") {
			current.Behavior = extractField(line, "**Behavior**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Invariants**:") {
			currentList = &current.Invariants
		} else if strings.HasPrefix(line, "**Source**:") {
			current.Source = extractField(line, "**Source**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Caching**:") {
			current.Caching = extractField(line, "**Caching**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Deliberately-Specific**:") {
			current.DeliberatelySpecific = extractField(line, "**Deliberately-Specific**:")
		} else if strings.HasPrefix(line, "**Backward-Compatible**:") {
			current.BackwardCompatible = extractField(line, "**Backward-Compatible**:")
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

// ScanAllInfrastructure finds every infrastructure.md under spec/intents/ —
// including features nested one level under an initiative directory — and
// returns their fragments with Feature populated. It is the infrastructure
// counterpart of ScanAllSurfaces, and shares its structure so the two
// cross-feature scans stay recognisably the same walk: a directory holding
// nested feature directories is an initiative and is recursed one level; a
// directory holding an infrastructure.md is a feature. A feature with no
// infrastructure.md simply contributes nothing.
func ScanAllInfrastructure(specDir string) ([]InfraFragment, error) {
	intentsDir := filepath.Join(specDir, "intents")
	entries, err := os.ReadDir(intentsDir)
	if err != nil {
		return nil, err
	}

	var all []InfraFragment
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		featureSlug := entry.Name()
		featureDir := filepath.Join(intentsDir, featureSlug)

		if nested := scanNestedInfrastructure(featureDir, featureSlug); len(nested) > 0 {
			all = append(all, nested...)
			continue
		}

		infraPath := filepath.Join(featureDir, "infrastructure.md")
		fragments, err := parseInfrastructureFragmentsIfPresent(infraPath, featureSlug)
		if err != nil {
			continue
		}
		all = append(all, fragments...)
	}

	return all, nil
}

// scanNestedInfrastructure returns fragments for features nested one level
// under an initiative directory, with Feature set to the qualified
// "<initiative>/<feature>" form. Returns nil when dir holds no nested feature
// infrastructure — i.e. when dir is itself a feature.
func scanNestedInfrastructure(dir, initiativeSlug string) []InfraFragment {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []InfraFragment
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childDir := filepath.Join(dir, e.Name())
		infraPath := filepath.Join(childDir, "infrastructure.md")
		qualified := initiativeSlug + "/" + e.Name()
		fragments, err := parseInfrastructureFragmentsIfPresent(infraPath, qualified)
		if err != nil {
			continue
		}
		out = append(out, fragments...)
	}
	return out
}

// parseInfrastructureFragmentsIfPresent parses infrastructure.md at infraPath
// and stamps Feature on every fragment. A missing file is not an error — it
// yields no fragments — so callers can treat "feature has no infrastructure"
// and "feature has an empty infrastructure" identically.
func parseInfrastructureFragmentsIfPresent(infraPath, feature string) ([]InfraFragment, error) {
	if _, err := os.Stat(infraPath); err != nil {
		return nil, nil
	}
	fragments, err := ParseInfrastructureFile(infraPath)
	if err != nil {
		return nil, err
	}
	for i := range fragments {
		fragments[i].Feature = feature
	}
	return fragments, nil
}
