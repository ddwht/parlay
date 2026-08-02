// parlay-feature: parlay-tool
// parlay-component: DialogValidationResult

package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

// dialog.schema.md documents exactly four turn forms. The parser
// recognises those four and silently ignores every other line, which is
// correct for prose but means a near-miss — `System (foo):`,
// `Systems:`, `System (condition: x) : y` — disappears without a word.
// The turn is gone from the parsed dialog, the file still validates, and
// nothing downstream can tell the difference between a turn that was
// never written and one that was written wrong.
//
// speakerColonLine matches only a line whose speaker region is a
// closed-vocabulary shape: `User`/`System`, optionally followed by a
// single word or a parenthesised modifier, then a colon. Prose that
// happens to begin with the word System ("System behaviour notes: ...")
// has words and spaces before its colon and does not match — a lint that
// misfires on a correct file is worse than no lint.
var speakerColonLine = regexp.MustCompile(`^(User|System)([A-Za-z]*|\s*\([^)]*\))\s*:`)

// ValidateDialogsDeep checks a dialogs.md against dialog.schema.md.
//
// Dialogs have no required fields — an empty dialogs.md is what
// `parlay add-feature` writes and is a valid starting point — so there
// is no "no dialogs" finding here. What is checkable is the closed set
// of turn forms, and that is an error in both modes: the pattern below
// only matches a finished speaker-and-colon line, so a half-typed turn
// never reaches it and what it does catch is wrong at any stage.
func ValidateDialogsDeep(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	if _, err := parser.ParseDialogsFile(path); err != nil {
		return []ValidationOutcome{outcomeWith(mode, "dialogs-not-readable",
			fmt.Sprintf("cannot parse dialogs.md: %s", err), path,
			"ensure the file exists and is valid markdown")}
	}

	var outcomes []ValidationOutcome
	inFence := false

	for i, line := range strings.Split(string(content), "\n") {
		// Fenced blocks are illustrative, not transcript. The schema's
		// own template is a fenced block; so are most examples people
		// paste into a dialogs.md.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if !speakerColonLine.MatchString(line) {
			continue
		}
		if parser.IsRecognisedTurn(line) {
			continue
		}
		outcomes = append(outcomes, outcomeWith(mode, "unknown-turn-form",
			fmt.Sprintf("line %d is not one of the four documented turn forms, so it is dropped rather than read as a turn: %q",
				i+1, strings.TrimSpace(line)),
			fmt.Sprintf("%s:%d", path, i+1),
			"use `User: <content>`, `System: <content>`, `System (background): <content>` or `System (condition: <when>): <content>`"))
	}

	return outcomes
}
