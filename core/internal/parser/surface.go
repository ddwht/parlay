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

func ParseSurfaceFile(path string) ([]Fragment, error) {
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
		surfacePath := filepath.Join(intentsDir, featureSlug, "surface.md")

		fragments, err := ParseSurfaceFile(surfacePath)
		if err != nil {
			continue // feature may not have a surface yet
		}

		for i := range fragments {
			fragments[i].Feature = featureSlug
		}
		all = append(all, fragments...)
	}

	return all, nil
}
