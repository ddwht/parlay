package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// Every Markdown parser in this package must be blind to HTML comments. The
// tests below are one per parser, all asking the same question in that
// parser's own vocabulary: does commented-out text become real content?

func TestDialogParserIgnoresComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dialogs.md")
	content := `# Test — Dialogs

---

### Real Dialog

**Trigger**: Something happens

User: I want a thing.
System: Here it is.

<!-- @dwht: this turn is too terse -->

<!--
### Parked Dialog

**Trigger**: Never
User: this whole block is commented out
System: and so is this
-->
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dialogs, err := ParseDialogsFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(dialogs) != 1 {
		t.Fatalf("want 1 dialog, got %d: %+v", len(dialogs), dialogs)
	}
	if dialogs[0].Title != "Real Dialog" {
		t.Errorf("title = %q, want %q", dialogs[0].Title, "Real Dialog")
	}
	if len(dialogs[0].Turns) != 2 {
		t.Errorf("turns = %d, want 2: %+v", len(dialogs[0].Turns), dialogs[0].Turns)
	}
}

// The dialog parser's option lines are selected by indentation, so the
// re-trim in visibleLine must not reach a line the stripper never touched.
func TestDialogParserKeepsOptionIndentationBesideComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dialogs.md")
	content := `### Options

User: Choose.
System: Options:
  A: First
<!-- @dwht: B is missing -->
  B: Second
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dialogs, err := ParseDialogsFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(dialogs) != 1 || len(dialogs[0].Turns) != 2 {
		t.Fatalf("unexpected shape: %+v", dialogs)
	}
	opts := dialogs[0].Turns[1].Options
	if len(opts) != 2 || opts[0].Letter != "A" || opts[1].Letter != "B" {
		t.Errorf("options = %+v, want A and B", opts)
	}
}

func TestInfrastructureParserIgnoresComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "infrastructure.md")
	content := `# Infrastructure

## Real Fragment

**Affects**: operation
**Behavior**: It does the thing.
**Invariants**:
- Always true
<!-- @dwht: is this actually always true? -->
- Also true

<!--
## Parked Fragment

**Affects**: domain
**Behavior**: Not real.
-->
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	frags, err := ParseInfrastructureFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(frags) != 1 {
		t.Fatalf("want 1 fragment, got %d: %+v", len(frags), frags)
	}
	if frags[0].Name != "Real Fragment" {
		t.Errorf("name = %q", frags[0].Name)
	}
	if len(frags[0].Invariants) != 2 {
		t.Errorf("invariants = %v, want 2", frags[0].Invariants)
	}
}

func TestPageParserIgnoresCommentsOutsideLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.page.md")
	content := `# Board

> The board page.

**Owner**: design
**Status**: draft

## Header

1. @task-list/header
<!-- @dwht: the filter fragment belongs here too -->

<!--
## Parked Region

1. @task-list/parked
-->

## Layout

` + "```yaml" + `
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: root
    type: stack
    direction: vertical
` + "```" + `
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := ParsePageFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var names []string
	for _, r := range page.Regions {
		names = append(names, r.Name)
	}
	if len(page.Regions) != 1 || page.Regions[0].Name != "Header" {
		t.Fatalf("regions = %v, want [Header]", names)
	}
	if len(page.Regions[0].Components) != 1 {
		t.Errorf("components = %v, want 1", page.Regions[0].Components)
	}
	if page.Layout == nil || len(page.Layout.Nodes) != 1 {
		t.Errorf("layout = %+v, want one node", page.Layout)
	}
}

func TestAmendmentParserIgnoresCommentsInBody(t *testing.T) {
	content := []byte(`---
amendment: task-text-length
date: 2026-09-02
trigger: annotation
affects:
  - "@task-list/surface:add-task"
---

## Change

Raise the limit to 500.
<!-- @dwht: say which field -->

## Why

Product asked for it.

## Acceptance

- The limit is 500
<!-- @dwht: also assert the error message -->
<!--
- A commented-out criterion is not a criterion
-->
`)

	a, err := ParseAmendmentBytes("003-task-text-length.md", content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(a.Acceptance) != 1 || a.Acceptance[0] != "The limit is 500" {
		t.Errorf("acceptance = %v, want one criterion", a.Acceptance)
	}
	if a.Change != "Raise the limit to 500." {
		t.Errorf("change = %q", a.Change)
	}
}

func TestStripCommentsKeepsTextOutsideComment(t *testing.T) {
	tests := []struct {
		name        string
		lines       []string
		wantVisible []string
	}{
		{
			name:        "text after a closing marker stays visible",
			lines:       []string{"<!-- note --> ## Real"},
			wantVisible: []string{"## Real"},
		},
		{
			name:        "an untouched line is returned byte-for-byte",
			lines:       []string{"  A: indented option"},
			wantVisible: []string{"  A: indented option"},
		},
		{
			name:        "a multi-line comment hides every line it spans",
			lines:       []string{"<!-- start", "## Hidden", "end -->"},
			wantVisible: nil,
		},
		{
			name:        "a comment closing mid-line yields the remainder",
			lines:       []string{"<!-- start", "end --> ## Real"},
			wantVisible: []string{"## Real"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var comments mdComments
			var got []string
			for _, line := range tt.lines {
				rest, ok := comments.visible(line)
				if ok {
					got = append(got, rest)
				}
			}
			if len(got) != len(tt.wantVisible) {
				t.Fatalf("visible = %q, want %q", got, tt.wantVisible)
			}
			for i := range got {
				if got[i] != tt.wantVisible[i] {
					t.Errorf("visible[%d] = %q, want %q", i, got[i], tt.wantVisible[i])
				}
			}
		})
	}
}

// A comment opener inside a code span or a fenced block is literal text —
// which is not a nicety here. The claude-md-section-preservation feature
// quotes `<!-- parlay:begin -->` inside its founding intents and dialogs;
// stripping there deleted a quotation out of the middle of a frozen promise
// and the ledger reported an edit nobody had made.
func TestCommentsInCodeAreContent(t *testing.T) {
	tests := []struct {
		name        string
		lines       []string
		wantVisible []string
	}{
		{
			name:        "an inline code span protects its contents",
			lines:       []string{"System: Replacing content between `<!-- parlay:begin -->` and `<!-- parlay:end -->`."},
			wantVisible: []string{"System: Replacing content between `<!-- parlay:begin -->` and `<!-- parlay:end -->`."},
		},
		{
			name:        "a real comment after a code span is still stripped",
			lines:       []string{"- The marker is `<!-- parlay:begin -->` <!-- @dwht: name the file -->"},
			wantVisible: []string{"- The marker is `<!-- parlay:begin -->`"},
		},
		{
			name:        "an unclosed backtick does not protect the rest of the line",
			lines:       []string{"- A stray ` tick <!-- @dwht: hidden -->"},
			wantVisible: []string{"- A stray ` tick"},
		},
		{
			name:        "a fenced block is content, comment markers and all",
			lines:       []string{"```markdown", "<!-- @dwht: this is an example, not a comment -->", "```", "## Real"},
			wantVisible: []string{"```markdown", "<!-- @dwht: this is an example, not a comment -->", "```", "## Real"},
		},
		{
			name:        "a fence opened inside a comment was never opened",
			lines:       []string{"<!-- ```", "## Hidden", "``` -->", "## Real"},
			wantVisible: []string{"## Real"},
		},
		{
			name:        "a longer closing run does not close a shorter span early",
			lines:       []string{"Use ``a ` b`` then <!-- @dwht: gone -->"},
			wantVisible: []string{"Use ``a ` b`` then"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var comments mdComments
			var got []string
			for _, line := range tt.lines {
				rest, ok := comments.visible(line)
				if ok {
					got = append(got, rest)
				}
			}
			if len(got) != len(tt.wantVisible) {
				t.Fatalf("visible = %q, want %q", got, tt.wantVisible)
			}
			for i := range got {
				if got[i] != tt.wantVisible[i] {
					t.Errorf("visible[%d] = %q, want %q", i, got[i], tt.wantVisible[i])
				}
			}
		})
	}
}
