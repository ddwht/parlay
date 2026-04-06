package parser

import (
	"bufio"
	"os"
	"strings"
)

type Intent struct {
	Title       string
	Slug        string
	Goal        string
	Persona     string
	Context     string
	Action      string
	Objects     []string
	Constraints []string
	Hints       []string
}

func ParseIntentsFile(path string) ([]Intent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var intents []Intent
	var current *Intent
	var currentList *[]string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			if current != nil {
				intents = append(intents, *current)
			}
			title := strings.TrimPrefix(line, "## ")
			current = &Intent{
				Title: title,
				Slug:  Slugify(title),
			}
			currentList = nil
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "**Goal**:") {
			current.Goal = extractField(line, "**Goal**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Persona**:") {
			current.Persona = extractField(line, "**Persona**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Context**:") {
			current.Context = extractField(line, "**Context**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Action**:") {
			current.Action = extractField(line, "**Action**:")
			currentList = nil
		} else if strings.HasPrefix(line, "**Objects**:") {
			raw := extractField(line, "**Objects**:")
			for _, obj := range strings.Split(raw, ",") {
				obj = strings.TrimSpace(obj)
				if obj != "" {
					current.Objects = append(current.Objects, obj)
				}
			}
			currentList = nil
		} else if strings.HasPrefix(line, "**Constraints**:") {
			currentList = &current.Constraints
		} else if strings.HasPrefix(line, "**Hints**:") {
			currentList = &current.Hints
		} else if strings.HasPrefix(line, "- ") && currentList != nil {
			item := strings.TrimPrefix(line, "- ")
			*currentList = append(*currentList, item)
		} else if line == "---" {
			currentList = nil
		}
	}

	if current != nil {
		intents = append(intents, *current)
	}

	return intents, scanner.Err()
}

func extractField(line, prefix string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, prefix))
}
