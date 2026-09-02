package parser

import (
	"strings"
	"testing"
)

// scanned is a compact view of a scan for table assertions: one line per
// thread ("state unit path|first-entry-text") and one per finding ("code@line").
func scanned(t *testing.T, path, content string) ([]string, []string) {
	t.Helper()
	scan := ScanAnnotations(path, []byte(content))
	var threads, findings []string
	for _, thread := range scan.Threads {
		id := thread.Anchor.YAMLPath
		if id == "" {
			id = strings.Join(thread.Anchor.HeadingPath, " › ")
		}
		threads = append(threads, thread.State+" "+thread.Anchor.Unit+" "+id+"|"+firstLine(thread.Anchor.Text))
	}
	for _, f := range scan.Findings {
		findings = append(findings, f.Code)
	}
	return threads, findings
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func assertEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %q, want %q", label, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", label, i, got[i], want[i])
		}
	}
}

func TestMarkdownAnchors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		threads []string
	}{
		{
			name: "a bullet is the unit above a comment",
			content: `## Add Task

**Constraints**:
- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 -->
`,
			threads: []string{"open list-item Add Task|- Task text must be 200 characters or fewer"},
		},
		{
			name: "a blank line between the text and the comment is transparent",
			content: `## Add Task

- Task text must be 200 characters or fewer

<!-- @dwht: product asked for 500 -->
`,
			threads: []string{"open list-item Add Task|- Task text must be 200 characters or fewer"},
		},
		{
			name: "a heading anchors to its own line, never the section under it",
			content: `## Add Task
<!-- @dwht: this title says less than the goal does -->

**Goal**: A person can add a task
`,
			threads: []string{"open heading Add Task|## Add Task"},
		},
		{
			name: "the scope word widens to the enclosing section",
			content: `## Add Task

**Verify**:
- A malformed item is refused
- The item names the run

<!-- @dwht section: this intent overlaps "Decide what to do" — merge the personas -->

## Next Intent
`,
			threads: []string{"open section Add Task|## Add Task"},
		},
		{
			name: "a field line with no bullets under it",
			content: `## Add Task

**Goal**: A person can add a task
<!-- @dwht: which person? -->
`,
			threads: []string{"open field Add Task|**Goal**: A person can add a task"},
		},
		{
			name: "a dialog turn takes its option lines with it",
			content: `### Add Task

User: Add a task.
System: Options:
  A: Save
  B: Discard
<!-- @dwht: there is no cancel -->
`,
			threads: []string{"open turn Add Task|System: Options:"},
		},
		{
			name: "an ordinary paragraph is the contiguous run ending above",
			content: `## Notes

The first line of the paragraph
and the second.
<!-- @dwht: split this -->
`,
			threads: []string{"open paragraph Notes|The first line of the paragraph"},
		},
		{
			name: "a nested list level is selected by the comment's own indent",
			content: `## Add Task

- Outer item
  - Inner item
<!-- @dwht: the outer one is wrong -->
`,
			threads: []string{"open list-item Add Task|- Outer item"},
		},
		{
			// Align with the dash you mean. This is §4.2's YAML rule for the
			// same gesture; the amended §3.5 makes Markdown match it, because
			// two opposite conventions for one visual alignment is a defect
			// in the syntax rather than a detail of the implementation.
			name: "a comment aligned with the nested dash is about the nested item",
			content: `## Add Task

- Outer item
  - Inner item
  <!-- @dwht: the inner one is wrong -->
`,
			threads: []string{"open list-item Add Task|  - Inner item"},
		},
		{
			name: "a comment at the outer dash's column is about the outer item",
			content: `## Add Task

- Outer item
  - Inner item
<!-- @dwht: the outer one is wrong -->
`,
			threads: []string{"open list-item Add Task|- Outer item"},
		},
		{
			name: "frontmatter is anchored by its closing fence",
			content: `---
amendment: task-text-length
---
<!-- @dwht: the trigger is missing -->

## Change
`,
			threads: []string{"open frontmatter |---"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threads, findings := scanned(t, "intents.md", tt.content)
			assertEqual(t, "threads", threads, tt.threads)
			assertEqual(t, "findings", findings, nil)
		})
	}
}

func TestYAMLAnchorsByColumn(t *testing.T) {
	const doc = `fragments:
  - name: add-task
    verify:
      - criterion A
      - criterion B
      # @dwht: B duplicates A
    notes: |
      Free prose.
  # @dwht: only one fragment?
`
	scan := ScanAnnotations("surface.yaml", []byte(doc))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", scan.Findings)
	}
	if len(scan.Threads) != 2 {
		t.Fatalf("threads = %d, want 2: %+v", len(scan.Threads), scan.Threads)
	}
	if got, want := scan.Threads[0].Anchor.YAMLPath, "fragments[0].verify[1]"; got != want {
		t.Errorf("first path = %q, want %q", got, want)
	}
	if got := scan.Threads[0].Anchor.Text; strings.TrimSpace(got) != "- criterion B" {
		t.Errorf("first text = %q", got)
	}
	if got, want := scan.Threads[1].Anchor.YAMLPath, "fragments[0]"; got != want {
		t.Errorf("second path = %q, want %q", got, want)
	}
}

func TestYAMLColumnSelectsWholeListOrOneItem(t *testing.T) {
	const doc = `verify:
  - criterion A
  - criterion B
# @dwht: none of these are testable
`
	scan := ScanAnnotations("surface.yaml", []byte(doc))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	if got, want := scan.Threads[0].Anchor.YAMLPath, "verify"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if got, want := scan.Threads[0].Anchor.Unit, annUnitMapping; got != want {
		t.Errorf("unit = %q, want %q", got, want)
	}
}

// `- key: value` has two starts on one line, and the design resolves the
// ambiguity by column: outdenting to the dash is the same move a YAML author
// makes to add a sibling, which is why it reads as "the whole item".
func TestYAMLDashLineHasTwoStarts(t *testing.T) {
	const doc = `operations:
  - id: task.create
    kind: command
    # @dwht: this is two operations
  - id: task.list
`
	scan := ScanAnnotations("capabilities.yaml", []byte(doc))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	if got, want := scan.Threads[0].Anchor.YAMLPath, "operations[0].kind"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}

	const wider = `operations:
  - id: task.create
    kind: command
  # @dwht: this is two operations
  - id: task.list
`
	scan = ScanAnnotations("capabilities.yaml", []byte(wider))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	if got, want := scan.Threads[0].Anchor.YAMLPath, "operations[0]"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if got, want := scan.Threads[0].Anchor.Span, [2]int{2, 3}; got != want {
		t.Errorf("span = %v, want %v", got, want)
	}
}

func TestThreadStates(t *testing.T) {
	tests := []struct {
		name    string
		content string
		state   string
		entries int
	}{
		{
			name: "a request alone is open",
			content: `- A bullet
<!-- @dwht: wrong -->
`,
			state: AnnotationOpen, entries: 1,
		},
		{
			name: "a reply under a request is answered",
			content: `- A bullet
<!-- @dwht: wrong -->
<!-- @claude done: raised to 500 in amendment 003-task-text-length -->
`,
			state: AnnotationAnswered, entries: 2,
		},
		{
			name: "a further request after a reply reopens the thread",
			content: `- A bullet
<!-- @dwht: wrong -->
<!-- @claude done: raised to 500 -->
<!-- @dwht: the dialog still says 200 -->
`,
			state: AnnotationOpen, entries: 3,
		},
		{
			name: "close ends it",
			content: `- A bullet
<!-- @dwht: wrong -->
<!-- @claude done: raised to 500 -->
<!-- @dwht close -->
`,
			state: AnnotationClosed, entries: 3,
		},
		{
			name: "close may follow a request directly",
			content: `- A bullet
<!-- @dwht: wrong -->
<!-- @dwht close: never mind -->
`,
			state: AnnotationClosed, entries: 2,
		},
		{
			name: "an ask is a request",
			content: `- A bullet
<!-- @dwht ask: why 200? -->
<!-- @claude answer: the store column is varchar(200) -->
`,
			state: AnnotationAnswered, entries: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scan := ScanAnnotations("intents.md", []byte(tt.content))
			if len(scan.Findings) != 0 {
				t.Fatalf("findings = %+v, want none", scan.Findings)
			}
			if len(scan.Threads) != 1 {
				t.Fatalf("threads = %+v, want 1", scan.Threads)
			}
			if scan.Threads[0].State != tt.state {
				t.Errorf("state = %q, want %q", scan.Threads[0].State, tt.state)
			}
			if len(scan.Threads[0].Entries) != tt.entries {
				t.Errorf("entries = %d, want %d", len(scan.Threads[0].Entries), tt.entries)
			}
		})
	}
}

func TestAnnotationFindings(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		want    []string
	}{
		{
			name: "a sigil with no colon and no kind",
			file: "intents.md",
			content: `- A bullet
<!-- @dwht -->
`,
			want: []string{AnnotationMalformed},
		},
		{
			name: "a word that is neither a kind nor the scope word",
			file: "intents.md",
			content: `- A bullet
<!-- @dwht maybe: not sure -->
`,
			want: []string{AnnotationWordUnknown},
		},
		{
			name: "two kinds in one annotation",
			file: "intents.md",
			content: `- A bullet
<!-- @dwht ask done: which is it -->
`,
			want: []string{AnnotationWordUnknown},
		},
		{
			name: "the scope word in YAML",
			file: "surface.yaml",
			content: `verify:
  - criterion A
  # @dwht section: too wide
`,
			want: []string{AnnotationMalformed},
		},
		{
			name: "an unterminated quote",
			file: "intents.md",
			content: `- Task text must be 200 characters or fewer
<!-- @dwht "200 characters: too low -->
`,
			want: []string{AnnotationMalformed},
		},
		{
			name: "a phrase that is not in the unit",
			file: "intents.md",
			content: `- Task text must be 200 characters or fewer
<!-- @dwht "500 characters": too low -->
`,
			want: []string{AnnotationPhraseNotFound},
		},
		{
			name: "an annotation sharing its line with content",
			file: "dialogs.md",
			content: `### D

User: hello <!-- @dwht: too terse -->
`,
			want: []string{AnnotationInline},
		},
		{
			name: "an annotation with nothing above it",
			file: "intents.md",
			content: `<!-- @dwht: this file is wrong -->

## Add Task
`,
			want: []string{AnnotationUnanchored},
		},
		{
			name: "an annotation above a section separator",
			file: "intents.md",
			content: `## Add Task

---
<!-- @dwht: wrong -->
`,
			want: []string{AnnotationUnanchored},
		},
		{
			name: "a reply with no request",
			file: "intents.md",
			content: `- A bullet
<!-- @claude done: changed it -->
`,
			want: []string{AnnotationReplyOrphaned},
		},
		{
			name: "a reply at a different column",
			file: "surface.yaml",
			content: `verify:
  - criterion A
  # @dwht: wrong
# @claude done: fixed
`,
			want: []string{AnnotationReplyColumn},
		},
		{
			name: "an entry after a close",
			file: "intents.md",
			content: `- A bullet
<!-- @dwht: wrong -->
<!-- @dwht close -->
<!-- @dwht: actually still wrong -->
`,
			want: []string{AnnotationAfterClose},
		},
		{
			name: "a sigil inside a block scalar",
			file: "capabilities.yaml",
			content: `operations:
  - id: task.create
    notes: |
      Some prose.
      # @dwht: this is content, not a comment
`,
			want: []string{AnnotationInBlockScalar},
		},
		{
			name:    "a tab-indented YAML annotation",
			file:    "surface.yaml",
			content: "verify:\n  - criterion A\n\t# @dwht: wrong\n",
			want:    []string{AnnotationMalformed},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, findings := scanned(t, tt.file, tt.content)
			assertEqual(t, "findings", findings, tt.want)
		})
	}
}

// A well-formed thread beside a broken one is still reported: one broken sigil
// must not hide the review around it.
func TestBrokenAnnotationDoesNotHideGoodOnes(t *testing.T) {
	const doc = `## Add Task

- First bullet
<!-- @dwht maybe: not sure -->

- Second bullet
<!-- @dwht: this one is fine -->
`
	scan := ScanAnnotations("intents.md", []byte(doc))
	if len(scan.Findings) != 1 || scan.Findings[0].Code != AnnotationWordUnknown {
		t.Fatalf("findings = %+v", scan.Findings)
	}
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v, want the well-formed one", scan.Threads)
	}
	if scan.Threads[0].Entries[0].Text != "this one is fine" {
		t.Errorf("text = %q", scan.Threads[0].Entries[0].Text)
	}
}

// The scanner and the parsers share one definition of a comment. A spec that
// QUOTES the annotation syntax — this design's own schema does — must not be
// scanned as carrying an annotation.
func TestSigilInCodeIsNotAnAnnotation(t *testing.T) {
	const doc = "## Grammar\n\nAn annotation is `<!-- @dwht: text -->` inside a comment.\n\n```markdown\n- A bullet\n<!-- @dwht: an example, not a request -->\n```\n\n- A real bullet\n<!-- @dwht: a real request -->\n"
	scan := ScanAnnotations("infrastructure.md", []byte(doc))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", scan.Findings)
	}
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %d, want 1: %+v", len(scan.Threads), scan.Threads)
	}
	if got := scan.Threads[0].Entries[0].Text; got != "a real request" {
		t.Errorf("text = %q", got)
	}
}

func TestMultiLineAnnotations(t *testing.T) {
	const md = `- A bullet
<!-- @dwht: this is two operations — create, and the confirmation that
     returns the id. Split it. -->
`
	scan := ScanAnnotations("intents.md", []byte(md))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	want := "this is two operations — create, and the confirmation that returns the id. Split it."
	if got := scan.Threads[0].Entries[0].Text; got != want {
		t.Errorf("markdown text = %q, want %q", got, want)
	}

	const yml = `operations:
  - id: task.create
    # @dwht: this is two operations — create, and the confirmation that
    #        returns the id. Split it.
`
	scan = ScanAnnotations("capabilities.yaml", []byte(yml))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	if got := scan.Threads[0].Entries[0].Text; got != want {
		t.Errorf("yaml text = %q, want %q", got, want)
	}
}

func TestPhraseNarrowing(t *testing.T) {
	const doc = `- Task text must be 200 characters or fewer
<!-- @dwht "200 characters": too low -->
`
	scan := ScanAnnotations("intents.md", []byte(doc))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v", scan.Findings)
	}
	if scan.Threads[0].Anchor.Phrase == nil || *scan.Threads[0].Anchor.Phrase != "200 characters" {
		t.Errorf("phrase = %v", scan.Threads[0].Anchor.Phrase)
	}
}

func TestHostSelection(t *testing.T) {
	for path, want := range map[string]string{
		"spec/intents/x/intents.md":   AnnotationHostMarkdown,
		"spec/intents/x/surface.yaml": AnnotationHostYAML,
		"a.yml":                       AnnotationHostYAML,
		"main.go":                     "",
	} {
		if got := AnnotationHostFor(path); got != want {
			t.Errorf("host(%q) = %q, want %q", path, got, want)
		}
	}
	scan := ScanAnnotations("main.go", []byte("// @dwht: not a spec file\n"))
	if len(scan.Threads) != 0 || len(scan.Findings) != 0 {
		t.Errorf("a non-spec file yielded %+v / %+v", scan.Threads, scan.Findings)
	}
}

// A `.page.md` and an amendment carry both hosts: YAML frontmatter above,
// Markdown below.
func TestBothHostsInOneFile(t *testing.T) {
	const doc = `---
amendment: task-text-length
affects:
  - "@task-list/surface:add-task"
  # @dwht: this should name the operation too
---

## Change

Raise the limit to 500.
<!-- @dwht: say which field -->
`
	scan := ScanAnnotations("003-task-text-length.md", []byte(doc))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v", scan.Findings)
	}
	if len(scan.Threads) != 2 {
		t.Fatalf("threads = %d: %+v", len(scan.Threads), scan.Threads)
	}
	if got, want := scan.Threads[0].Anchor.YAMLPath, "affects[0]"; got != want {
		t.Errorf("frontmatter path = %q, want %q", got, want)
	}
	if got, want := scan.Threads[1].Anchor.Unit, annUnitParagraph; got != want {
		t.Errorf("body unit = %q, want %q", got, want)
	}
}

// An `@feature/name` ref that happens to open a comment line is not an
// annotation. The design claimed the mandatory colon ruled this out; scanning
// this repo showed it does not — a wrapped comment in
// core/.parlay/build/parlay-tool/page-layout-field/buildfile.yaml opens with
// `# @design-loop/design-loop respectively and stay byte-equivalent`, with no
// colon anywhere. The slash after the handle is the discriminator.
func TestFeatureRefIsNotAnAnnotation(t *testing.T) {
	const doc = `units:
  - id: layout
    # The layout.schema.md sections were landed by
    # @design-loop/design-loop respectively and stay byte-equivalent
    # through this edit.
    # @dwht: but say which sections
`
	scan := ScanAnnotations("buildfile.yaml", []byte(doc))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", scan.Findings)
	}
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %d, want 1: %+v", len(scan.Threads), scan.Threads)
	}
	if got := scan.Threads[0].Entries[0].Text; got != "but say which sections" {
		t.Errorf("text = %q", got)
	}

	// A handle with no slash after it is still an annotation, broken or not.
	scan = ScanAnnotations("intents.md", []byte("- A bullet\n<!-- @dwht -->\n"))
	if len(scan.Findings) != 1 || scan.Findings[0].Code != AnnotationMalformed {
		t.Errorf("findings = %+v, want one annotation-malformed", scan.Findings)
	}
}

// Every case below is one Codex found by reading WP1 rather than by running
// it: none was covered, and each was a real defect.
func TestScannerEdgeCasesFoundInReview(t *testing.T) {
	t.Run("an annotation after an ordinary comment on one line is found", func(t *testing.T) {
		scan := ScanAnnotations("intents.md", []byte("- A bullet\n<!-- ordinary --><!-- @dwht: request -->\n"))
		if len(scan.Findings) != 0 {
			t.Fatalf("findings = %+v", scan.Findings)
		}
		if len(scan.Threads) != 1 || scan.Threads[0].Entries[0].Text != "request" {
			t.Fatalf("threads = %+v", scan.Threads)
		}
	})

	t.Run("an ordinary comment after an annotation is not content", func(t *testing.T) {
		scan := ScanAnnotations("intents.md", []byte("- A bullet\n<!-- @dwht: request --><!-- ordinary -->\n"))
		if len(scan.Findings) != 0 {
			t.Fatalf("findings = %+v, want none — a comment beside an annotation is not content", scan.Findings)
		}
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v", scan.Threads)
		}
	})

	t.Run("a close with nothing above it is orphaned", func(t *testing.T) {
		scan := ScanAnnotations("intents.md", []byte("- A bullet\n<!-- @dwht close -->\n"))
		if len(scan.Findings) != 1 || scan.Findings[0].Code != AnnotationReplyOrphaned {
			t.Fatalf("findings = %+v", scan.Findings)
		}
		if len(scan.Threads) != 0 {
			t.Errorf("threads = %+v, want none", scan.Threads)
		}
	})

	t.Run("a yaml annotation at the top of a file is unanchored", func(t *testing.T) {
		scan := ScanAnnotations("surface.yaml", []byte("# @dwht: this whole file is wrong\nfeature: task-list\n"))
		if len(scan.Findings) != 1 || scan.Findings[0].Code != AnnotationUnanchored {
			t.Fatalf("findings = %+v", scan.Findings)
		}
	})

	t.Run("a unit never claims text below the annotation", func(t *testing.T) {
		scan := ScanAnnotations("surface.yaml", []byte("verify:\n  - criterion A\n# @dwht: only A is testable\n  - criterion B\n"))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v", scan.Threads)
		}
		if strings.Contains(scan.Threads[0].Anchor.Text, "criterion B") {
			t.Errorf("anchor claimed text below the annotation:\n%s", scan.Threads[0].Anchor.Text)
		}
		if got, want := scan.Threads[0].Anchor.Span, [2]int{1, 2}; got != want {
			t.Errorf("span = %v, want %v", got, want)
		}
	})

	t.Run("markdown fields do not claim bullets below the annotation", func(t *testing.T) {
		scan := ScanAnnotations("intents.md", []byte("## X\n\n**Verify**:\n<!-- @dwht: these are not testable -->\n- criterion A\n"))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v", scan.Threads)
		}
		if strings.Contains(scan.Threads[0].Anchor.Text, "criterion A") {
			t.Errorf("anchor claimed a bullet below the annotation:\n%s", scan.Threads[0].Anchor.Text)
		}
	})

	t.Run("a hash inside a quoted scalar is not a comment", func(t *testing.T) {
		for _, line := range []string{
			`  shows: "he said \"x\" # not a comment"`,
			`  shows: 'it''s fine # not a comment'`,
			"  shows: don't lose the draft",
		} {
			scan := ScanAnnotations("surface.yaml", []byte("fragments:\n"+line+"\n"))
			if len(scan.Threads) != 0 || len(scan.Findings) != 0 {
				t.Errorf("%s\n  threads = %+v, findings = %+v", line, scan.Threads, scan.Findings)
			}
		}
	})

	t.Run("an apostrophe does not hide a later annotation", func(t *testing.T) {
		scan := ScanAnnotations("surface.yaml", []byte("fragments:\n  - shows: don't lose the draft\n    # @dwht: say where it is kept\n"))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v, findings = %+v", scan.Threads, scan.Findings)
		}
	})

	t.Run("a deeply indented fence does not close the real one", func(t *testing.T) {
		doc := "## X\n\n```markdown\nnested example:\n    ```\n<!-- @dwht: inside the fence, an example -->\n```\n\n- A real bullet\n<!-- @dwht: a real request -->\n"
		scan := ScanAnnotations("infrastructure.md", []byte(doc))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v", scan.Threads)
		}
		if got := scan.Threads[0].Entries[0].Text; got != "a real request" {
			t.Errorf("text = %q", got)
		}
	})

	t.Run("an unmatched backtick does not hide a later annotation", func(t *testing.T) {
		doc := "## X\n\nA line with one stray ` backtick in it.\n\n- A bullet\n<!-- @dwht: still found -->\n"
		scan := ScanAnnotations("infrastructure.md", []byte(doc))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v, findings = %+v", scan.Threads, scan.Findings)
		}
		if got := scan.Threads[0].Entries[0].Text; got != "still found" {
			t.Errorf("text = %q", got)
		}
	})
}

// Codex's second review, and the two it called fatal.
func TestAnchorTextExcludesTheThread(t *testing.T) {
	// `section` runs FORWARD from its heading, so its span covers the
	// annotation itself. Building the text from raw lines let a quoted phrase
	// narrow against words the reviewer wrote in their own request.
	const doc = `## Add Task

**Verify**:
- A malformed item is refused

<!-- @dwht section: this overlaps the neighbouring intent -->
<!-- @claude answer: they differ in persona -->

## Next Intent
`
	scan := ScanAnnotations("intents.md", []byte(doc))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v, findings = %+v", scan.Threads, scan.Findings)
	}
	text := scan.Threads[0].Anchor.Text
	for _, leak := range []string{"@dwht", "@claude", "overlaps the neighbouring", "differ in persona"} {
		if strings.Contains(text, leak) {
			t.Errorf("anchor text contains the thread (%q):\n%s", leak, text)
		}
	}
	if !strings.Contains(text, "A malformed item is refused") {
		t.Errorf("anchor text lost its real content:\n%s", text)
	}
}

func TestPhraseCannotMatchTheRequestsOwnWords(t *testing.T) {
	const doc = `## Add Task

- A malformed item is refused

<!-- @dwht section "merge the personas": do that -->

## Next Intent
`
	scan := ScanAnnotations("intents.md", []byte(doc))
	if len(scan.Findings) != 1 || scan.Findings[0].Code != AnnotationPhraseNotFound {
		t.Fatalf("findings = %+v, want annotation-phrase-not-found — the phrase occurs only in the request", scan.Findings)
	}
}

func TestOrdinaryCommentsDoNotLeakIntoAnchorText(t *testing.T) {
	const doc = `verify:
  - criterion A
  # an ordinary note about the list
  - criterion B
  # @dwht: is B testable?
`
	scan := ScanAnnotations("surface.yaml", []byte(doc))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	if strings.Contains(scan.Threads[0].Anchor.Text, "ordinary note") {
		t.Errorf("anchor text carries an ordinary comment:\n%s", scan.Threads[0].Anchor.Text)
	}
}

// A blank line separates two conversations. Without this, "reopen by starting
// a new thread on the same text" under a closed one became
// annotation-after-close and the new request vanished from the listing.
func TestBlankLineStartsANewThread(t *testing.T) {
	const doc = `- A bullet
<!-- @dwht: wrong -->
<!-- @claude done: fixed -->
<!-- @dwht close -->

<!-- @dwht: still wrong, for a different reason -->
`
	scan := ScanAnnotations("intents.md", []byte(doc))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", scan.Findings)
	}
	if len(scan.Threads) != 2 {
		t.Fatalf("threads = %d, want 2: %+v", len(scan.Threads), scan.Threads)
	}
	if scan.Threads[0].State != AnnotationClosed || scan.Threads[1].State != AnnotationOpen {
		t.Errorf("states = %q, %q; want closed then open", scan.Threads[0].State, scan.Threads[1].State)
	}
}

func TestEntryDirectlyUnderACloseIsStillRefused(t *testing.T) {
	const doc = `- A bullet
<!-- @dwht: wrong -->
<!-- @dwht close -->
<!-- @dwht: actually still wrong -->
`
	scan := ScanAnnotations("intents.md", []byte(doc))
	if len(scan.Findings) != 1 || scan.Findings[0].Code != AnnotationAfterClose {
		t.Fatalf("findings = %+v", scan.Findings)
	}
}

// The leak Codex found after the first "visible lines" fix: dropping whole
// comment lines is not enough, because an INLINE comment leaves a line the
// parsers read differently from the raw bytes. Whole-line tests never
// exercise it.
func TestInlineCommentsDoNotLeakIntoAnchorText(t *testing.T) {
	t.Run("markdown", func(t *testing.T) {
		scan := ScanAnnotations("intents.md", []byte("## X\n\n- Task text is limited <!-- an ordinary note -->\n<!-- @dwht: to what? -->\n"))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v, findings = %+v", scan.Threads, scan.Findings)
		}
		text := scan.Threads[0].Anchor.Text
		if strings.Contains(text, "ordinary note") || strings.Contains(text, "<!--") {
			t.Errorf("anchor text carries an inline comment: %q", text)
		}
		if text != "- Task text is limited" {
			t.Errorf("anchor text = %q", text)
		}
	})

	t.Run("markdown keeps indentation of a line it filtered", func(t *testing.T) {
		scan := ScanAnnotations("intents.md", []byte("## X\n\n- Outer\n  - Inner <!-- note -->\n  <!-- @dwht: which? -->\n"))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v", scan.Threads)
		}
		if got := scan.Threads[0].Anchor.Text; got != "  - Inner" {
			t.Errorf("anchor text = %q, want the inner item with its indent", got)
		}
	})

	t.Run("yaml", func(t *testing.T) {
		scan := ScanAnnotations("surface.yaml", []byte("verify:\n  - criterion A # an ordinary note\n  # @dwht: is it testable?\n"))
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v, findings = %+v", scan.Threads, scan.Findings)
		}
		if got := scan.Threads[0].Anchor.Text; got != "  - criterion A" {
			t.Errorf("anchor text = %q", got)
		}
	})
}

func TestStructuralLinesFiltersInlineComments(t *testing.T) {
	md := StructuralLines("intents.md", []byte("<!-- an ordinary note --> **Goal**: a person can add a task\n- bullet <!-- note -->\n<!-- whole line -->\n"))
	if md[0] != "**Goal**: a person can add a task" {
		t.Errorf("line 0 = %q — the field must be visible with the leading comment gone", md[0])
	}
	if md[1] != "- bullet" {
		t.Errorf("line 1 = %q", md[1])
	}
	if md[2] != "" {
		t.Errorf("line 2 = %q, want empty", md[2])
	}

	yml := StructuralLines("capabilities.yaml", []byte("operations:\n  - id: task.create # a note\n"))
	if yml[1] != "  - id: task.create" {
		t.Errorf("yaml line 1 = %q — a trailing comment must not become part of the id", yml[1])
	}
}

// One locator, or the same bytes are content to one reader and an actionable
// request to the other. Each row below is a shape where the three old
// locators disagreed: a tilde fence, a lower-case heading, an indented
// heading, and a four-backtick opener that a three-backtick line must not
// close.
//
// The assertion is not "the scanner finds it" or "the parser reads it" — it is
// that they AGREE, whatever the answer.
func TestPageLayoutLocatorAgreesWithThePageParser(t *testing.T) {
	fence := "```"
	tests := []struct {
		name    string
		body    string
		wantOne bool
	}{
		{
			name:    "backtick fence",
			body:    "## Layout\n\n" + fence + "yaml\nnodes:\n  - id: root\n    # @dwht: wrong\n" + fence + "\n",
			wantOne: true,
		},
		{
			name:    "tilde fence",
			body:    "## Layout\n\n~~~yaml\nnodes:\n  - id: root\n    # @dwht: wrong\n~~~\n",
			wantOne: true,
		},
		{
			name:    "lower-case heading",
			body:    "## layout\n\n" + fence + "yaml\nnodes:\n  - id: root\n    # @dwht: wrong\n" + fence + "\n",
			wantOne: true,
		},
		{
			name:    "indented heading",
			body:    "  ## Layout\n\n" + fence + "yaml\nnodes:\n  - id: root\n    # @dwht: wrong\n" + fence + "\n",
			wantOne: true,
		},
		{
			name:    "a three-backtick line does not close a four-backtick fence",
			body:    "## Layout\n\n````yaml\nnodes:\n  - id: root\n" + fence + "\n    # @dwht: wrong\n````\n",
			wantOne: true,
		},
		{
			name:    "no layout section at all",
			body:    "## Header\n\n" + fence + "yaml\nnodes:\n  - id: root\n    # @dwht: an example\n" + fence + "\n",
			wantOne: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "# Board\n\n" + tt.body

			// What the page loader consumes as YAML. An error here means the
			// loader refuses the page outright, which is neither "sees a
			// layout" nor "sees none" — the agreement rows below are all
			// well-formed, and the malformed shapes are tested through
			// ParsePageFile in page_test.go.
			_, present, err := extractLayoutSection([]byte(content))
			if err != nil {
				t.Fatalf("unexpected layout error: %v", err)
			}
			parserSees := present

			// What the scanner treats as a layout region.
			scan := ScanAnnotations("board.page.md", []byte(content))
			scannerFound := len(scan.Threads) == 1

			if parserSees != tt.wantOne {
				t.Errorf("the page parser found a layout = %v, want %v", parserSees, tt.wantOne)
			}
			if scannerFound != tt.wantOne {
				t.Errorf("the scanner found %d threads, want %v",
					len(scan.Threads), tt.wantOne)
			}
			if parserSees != scannerFound {
				t.Fatalf("the two readers disagree: parser=%v scanner=%v — the same bytes are YAML to one and not the other",
					parserSees, scannerFound)
			}
		})
	}
}
