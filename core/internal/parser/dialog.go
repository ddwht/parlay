package parser

import (
	"bufio"
	"os"
	"strings"
)

type Dialog struct {
	Title   string
	Slug    string
	Trigger string
	Turns   []Turn
}

type Turn struct {
	Speaker   string // "user" or "system"
	Type      string // "regular", "background", "conditional"
	Condition string
	Content   string
	Options   []Option
}

type Option struct {
	Letter    string
	Desc      string
	IsFreeform bool
}

func ParseDialogsFile(path string) ([]Dialog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var dialogs []Dialog
	var current *Dialog

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Dialog title
		if strings.HasPrefix(line, "### ") && !strings.HasPrefix(line, "#### ") {
			if current != nil {
				dialogs = append(dialogs, *current)
			}
			title := strings.TrimPrefix(line, "### ")
			current = &Dialog{
				Title: title,
				Slug:  Slugify(title),
			}
			continue
		}

		if current == nil {
			continue
		}

		// Metadata
		if strings.HasPrefix(line, "**Trigger**:") {
			current.Trigger = extractField(line, "**Trigger**:")
			continue
		}

		// Segment separator — finalize current dialog
		if line == "---" {
			if current != nil {
				dialogs = append(dialogs, *current)
				current = nil
			}
			continue
		}

		// Parse turns
		turn, ok := parseTurn(line)
		if ok {
			current.Turns = append(current.Turns, turn)
			continue
		}

		// Parse options (indented A:, B:, C: lines)
		if strings.HasPrefix(line, "  ") && len(current.Turns) > 0 {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 2 && trimmed[1] == ':' {
				opt := Option{
					Letter: string(trimmed[0]),
					Desc:   strings.TrimSpace(trimmed[2:]),
				}
				opt.IsFreeform = strings.HasPrefix(opt.Desc, "==") && strings.HasSuffix(opt.Desc, "==")
				lastTurn := &current.Turns[len(current.Turns)-1]
				lastTurn.Options = append(lastTurn.Options, opt)
			}
		}
	}

	if current != nil {
		dialogs = append(dialogs, *current)
	}

	return dialogs, scanner.Err()
}

func parseTurn(line string) (Turn, bool) {
	if strings.HasPrefix(line, "User:") {
		return Turn{
			Speaker: "user",
			Type:    "regular",
			Content: strings.TrimSpace(strings.TrimPrefix(line, "User:")),
		}, true
	}

	if strings.HasPrefix(line, "System (background):") {
		return Turn{
			Speaker: "system",
			Type:    "background",
			Content: strings.TrimSpace(strings.TrimPrefix(line, "System (background):")),
		}, true
	}

	if strings.HasPrefix(line, "System (condition:") {
		// Extract condition and content
		rest := strings.TrimPrefix(line, "System (condition:")
		idx := strings.Index(rest, "):")
		if idx >= 0 {
			condition := strings.TrimSpace(rest[:idx])
			content := strings.TrimSpace(rest[idx+2:])
			return Turn{
				Speaker:   "system",
				Type:      "conditional",
				Condition: condition,
				Content:   content,
			}, true
		}
	}

	if strings.HasPrefix(line, "System:") {
		return Turn{
			Speaker: "system",
			Type:    "regular",
			Content: strings.TrimSpace(strings.TrimPrefix(line, "System:")),
		}, true
	}

	return Turn{}, false
}
