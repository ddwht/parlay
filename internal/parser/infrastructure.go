// parlay-feature: infrastructure-layer
// parlay-component: InfrastructureValidationResult
// parlay-extends: infrastructure-layer/schema-framework-agnostic-fields

package parser

import (
	"bufio"
	"os"
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

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
