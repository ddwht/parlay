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
	Priority    string
	Context     string
	Action      string
	Objects     []string
	Constraints []string
	Verify      []string
	Questions   []string
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

	// inComment tracks an open HTML comment across lines.
	//
	// This scanner has no Markdown state: before this, any line starting `## `
	// was an intent heading even inside `<!-- ... -->`, and the `**Goal**:`
	// lines under it were consumed as that intent's fields. So a commented-out
	// intent — a template in a scaffold, or a block an author parked while
	// rewriting — parsed as a real one, and `no-intents` was satisfied by a
	// feature nobody had authored. A comment is the one construct whose whole
	// purpose is "the tools should not read this", so reading it was the bug.
	inComment := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Comment state is resolved before any content rule. Open and close
		// are both handled on a single line, so `<!-- ## X -->` hides its
		// heading and text after a `-->` on the same line stays visible.
		rest, hidden := stripComments(line, &inComment)
		if hidden {
			continue
		}
		if rest != line {
			// Only a line the stripper actually touched is re-trimmed. The
			// prefix rules below match exact starts, and `<!-- n --> ## X`
			// leaves a leading space that would otherwise hide a real
			// heading. Untouched lines keep their original bytes so nothing
			// about existing files changes.
			rest = strings.TrimLeft(rest, " \t")
			if strings.TrimSpace(rest) == "" {
				continue
			}
		}
		line = rest

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
		} else if strings.HasPrefix(line, "**Priority**:") {
			current.Priority = extractField(line, "**Priority**:")
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
		} else if strings.HasPrefix(line, "**Verify**:") {
			currentList = &current.Verify
		} else if strings.HasPrefix(line, "**Questions**:") {
			currentList = &current.Questions
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

// stripComments removes HTML-comment regions from one line, carrying open
// state across lines via inComment.
//
// Returns the visible remainder and whether the line is entirely hidden. A
// line that is only a comment yields hidden=true; a line with text outside the
// comment yields that text, so `<!-- note --> ## Real` still parses.
func stripComments(line string, inComment *bool) (string, bool) {
	var out strings.Builder
	for {
		if *inComment {
			end := strings.Index(line, "-->")
			if end < 0 {
				// Whole remainder is inside the comment.
				return out.String(), out.Len() == 0
			}
			*inComment = false
			line = line[end+len("-->"):]
			continue
		}
		start := strings.Index(line, "<!--")
		if start < 0 {
			out.WriteString(line)
			break
		}
		out.WriteString(line[:start])
		*inComment = true
		line = line[start+len("<!--"):]
	}
	return out.String(), strings.TrimSpace(out.String()) == ""
}
