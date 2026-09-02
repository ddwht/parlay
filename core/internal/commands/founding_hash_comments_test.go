package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// The founding-document freeze hashes PARSED content, and the Markdown
// parsers cannot see HTML comments. Together those two facts are what lets a
// reviewer write a comment into a frozen intents.md or dialogs.md without
// producing a ledger_integrity violation. This test pins the pair: if either
// half regresses, adding a comment moves a founding hash and the freeze starts
// accusing a reviewer of editing a promise they only annotated.
func TestFoundingHashesIgnoreComments(t *testing.T) {
	dir := t.TempDir()

	dialogsPlain := `### Add Task

**Trigger**: User opens the composer

User: Add "buy milk".
System: Options:
  A: Save
  B: Discard
System: Added.
`
	dialogsAnnotated := `### Add Task

**Trigger**: User opens the composer

User: Add "buy milk".
<!-- @dwht: the example should use a task at the length limit -->
System: Options:
  A: Save
  B: Discard
<!-- @claude answer: the limit case is covered by the constraint, not the dialog -->
System: Added.
`

	intentsPlain := `## Add Task

**Goal**: A person can add a task
**Persona**: Anyone with the list open
**Priority**: must
**Constraints**:
- Task text must be 200 characters or fewer
**Verify**:
- A task longer than the limit is refused
`
	intentsAnnotated := `## Add Task

**Goal**: A person can add a task
**Persona**: Anyone with the list open
**Priority**: must
**Constraints**:
- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 -->
**Verify**:
- A task longer than the limit is refused
`

	dialogHash := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		dialogs, err := parser.ParseDialogsFile(path)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if len(dialogs) != 1 {
			t.Fatalf("%s: want 1 dialog, got %d", name, len(dialogs))
		}
		return hashDialogContent(dialogs[0])
	}

	if plain, annotated := dialogHash("plain-dialogs.md", dialogsPlain), dialogHash("annotated-dialogs.md", dialogsAnnotated); plain != annotated {
		t.Errorf("dialog hash moved when a comment was added:\n  plain     = %s\n  annotated = %s", plain, annotated)
	}

	intentHash := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		intents, err := parser.ParseIntentsFile(path)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if len(intents) != 1 {
			t.Fatalf("%s: want 1 intent, got %d", name, len(intents))
		}
		return hashIntentContent(intents[0])
	}

	if plain, annotated := intentHash("plain-intents.md", intentsPlain), intentHash("annotated-intents.md", intentsAnnotated); plain != annotated {
		t.Errorf("intent hash moved when a comment was added:\n  plain     = %s\n  annotated = %s", plain, annotated)
	}
}

// The exemption that makes the rule above safe: a comment opener inside
// backticks is a QUOTATION, and stripping it deletes bytes out of the middle
// of a frozen promise.
//
// The text here is the real shape from
// core/spec/intents/claude-md-section-preservation — a founding feature whose
// whole subject is the `<!-- parlay:begin -->` marker, so it necessarily
// writes the marker down. The naive stripper turned that line into "Replacing
// content between “ and “ with updated parlay section" and the ledger
// reported an edit nobody had made. This test fails if that regression ever
// comes back, at the level where it did damage: the founding hash.
func TestFoundingHashesKeepQuotedCommentMarkers(t *testing.T) {
	dir := t.TempDir()

	const quoted = "System (condition: CLAUDE.md exists with markers): Found existing CLAUDE.md with parlay markers. Replacing content between `<!-- parlay:begin -->` and `<!-- parlay:end -->` with updated parlay section."

	path := filepath.Join(dir, "dialogs.md")
	content := "### Preserve user sections in CLAUDE.md during upgrade\n\n**Trigger**: parlay upgrade\n\nUser: parlay init\n" + quoted + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	dialogs, err := parser.ParseDialogsFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(dialogs) != 1 {
		t.Fatalf("want 1 dialog, got %d", len(dialogs))
	}
	var conditional *parser.Turn
	for i := range dialogs[0].Turns {
		if dialogs[0].Turns[i].Type == "conditional" {
			conditional = &dialogs[0].Turns[i]
		}
	}
	if conditional == nil {
		t.Fatalf("no conditional turn parsed from %+v", dialogs[0].Turns)
	}
	if !strings.Contains(conditional.Content, "`<!-- parlay:begin -->`") {
		t.Errorf("the quoted marker was stripped out of the turn:\n  got %q", conditional.Content)
	}

	// And the same at the level the ledger reads: a founding hash computed
	// over this dialog must not depend on whether the parser is comment-aware.
	sameTextDifferentQuoting := filepath.Join(dir, "dialogs-again.md")
	if err := os.WriteFile(sameTextDifferentQuoting, []byte(content+"\n<!-- @dwht: name the fallback -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := parser.ParseDialogsFile(sameTextDifferentQuoting)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := hashDialogContent(again[0]), hashDialogContent(dialogs[0]); got != want {
		t.Errorf("hash moved when an annotation was added beside a quoted marker:\n  with    = %s\n  without = %s", got, want)
	}
}
