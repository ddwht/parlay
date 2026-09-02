package commands

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// refFor scans one file's content and returns the first thread's resolved ref,
// field and index, rendered compactly.
func refFor(t *testing.T, feature, path, content string) (string, string, string) {
	t.Helper()
	scan := parser.ScanAnnotations(path, []byte(content))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", scan.Findings)
	}
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %d, want 1: %+v", len(scan.Threads), scan.Threads)
	}
	resolveAnnotationRefs(feature, path, []byte(content), scan.Threads)
	anchor := scan.Threads[0].Anchor
	index := "—"
	if anchor.Index != nil {
		index = string(rune('0' + *anchor.Index))
	}
	return anchor.Ref, anchor.Field, index
}

func TestRefResolution(t *testing.T) {
	tests := []struct {
		name                   string
		feature, path, content string
		ref, field, index      string
	}{
		{
			name:    "an intent constraint",
			feature: "task-list",
			path:    "spec/intents/task-list/intents.md",
			content: `# Task List — Intents

## Add Task

**Constraints**:
- Task text must be 200 characters or fewer
- The list must not be empty
<!-- @dwht: product asked for 500 -->
`,
			ref: "@task-list/intent:add-task", field: "Constraints", index: "1",
		},
		{
			name:    "an intent field with no list under it",
			feature: "task-list",
			path:    "spec/intents/task-list/intents.md",
			content: `# Task List — Intents

## Add Task

**Goal**: A person can add a task
<!-- @dwht: which person? -->
`,
			ref: "@task-list/intent:add-task", field: "Goal", index: "—",
		},
		{
			name:    "a dialog turn",
			feature: "parlay-tool/authoring",
			path:    "spec/intents/parlay-tool/authoring/dialogs.md",
			content: `# Authoring — Dialogs

### Check Readiness

User: I want to check readiness.
System: Let me check.
<!-- @dwht: say what it checks -->
`,
			ref: "@parlay-tool/authoring/dialog:check-readiness", field: "turn", index: "1",
		},
		{
			name:    "an infrastructure invariant",
			feature: "task-list",
			path:    "spec/intents/task-list/infrastructure.md",
			content: `# Infrastructure

## Rate Limiting

**Invariants**:
- No more than 10 writes a second
<!-- @dwht: per user or per project? -->
`,
			ref: "@task-list/infrastructure:rate-limiting", field: "Invariants", index: "0",
		},
		{
			name:    "a surface fragment's verify entry",
			feature: "task-list",
			path:    "spec/intents/task-list/surface.yaml",
			content: `feature: task-list
fragments:
  - name: add-task
    shows: the composer
    verify:
      - A task longer than the limit is refused
      # @dwht: also assert the message
`,
			ref: "@task-list/surface:add-task", field: "verify[0]", index: "—",
		},
		{
			name:    "a capability operation",
			feature: "task-list",
			path:    "spec/intents/task-list/capabilities.yaml",
			content: `feature: task-list
operations:
  - id: task.list
    kind: query
  - id: task.create
    kind: command
    # @dwht: this is two operations
`,
			ref: "@task-list/operation:task.create", field: "kind", index: "—",
		},
		{
			name:    "a domain entity",
			feature: "task-list",
			path:    "spec/intents/task-list/domain-model.yaml",
			content: `entities:
  - name: Task
    fields:
      - name: text
  # @dwht: Task needs an owner
`,
			ref: "@task-list/domain:Task", field: "", index: "—",
		},
		{
			name:    "an amendment section",
			feature: "task-list",
			path:    "spec/intents/task-list/amendments/003-task-text-length.md",
			content: `---
amendment: task-text-length
---

## Change

Raise the limit to 500.
<!-- @dwht: say which field -->
`,
			ref: "@task-list/amendment:task-text-length", field: "Change", index: "—",
		},
		{
			name:    "a page region",
			feature: "task-list",
			path:    "spec/pages/board.page.md",
			content: `# Board

## Header

1. @task-list/header
<!-- @dwht: the filter belongs here too -->
`,
			ref: "page:board", field: "Header", index: "—",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, field, index := refFor(t, tt.feature, tt.path, tt.content)
			if ref != tt.ref {
				t.Errorf("ref = %q, want %q", ref, tt.ref)
			}
			if field != tt.field {
				t.Errorf("field = %q, want %q", field, tt.field)
			}
			if index != tt.index {
				t.Errorf("index = %q, want %q", index, tt.index)
			}
		})
	}
}

// A file parlay does not know keeps the generic identity and gains no ref.
// That is the normal case for a blueprint, an adapter or an authored.yaml, and
// it is what lets the scanner work on them at all.
func TestUnknownFileKeepsGenericIdentity(t *testing.T) {
	const content = `adapter: react
supports:
  - kind: command
  # @dwht: does this cover queries?
`
	scan := parser.ScanAnnotations(".parlay/adapters/react.yaml", []byte(content))
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	resolveAnnotationRefs("task-list", ".parlay/adapters/react.yaml", []byte(content), scan.Threads)
	if got := scan.Threads[0].Anchor.Ref; got != "" {
		t.Errorf("ref = %q, want none", got)
	}
	if got := scan.Threads[0].Anchor.YAMLPath; got != "supports[0]" {
		t.Errorf("yaml path = %q, want supports[0]", got)
	}
}

// A project-level file has no feature, and so no ref to resolve.
func TestProjectLevelFileHasNoRef(t *testing.T) {
	const content = `entities:
  - name: Task
  # @dwht: this belongs to a feature
`
	scan := parser.ScanAnnotations("spec/domain-model.yaml", []byte(content))
	resolveAnnotationRefs("", "spec/domain-model.yaml", []byte(content), scan.Threads)
	if len(scan.Threads) != 1 || scan.Threads[0].Anchor.Ref != "" {
		t.Errorf("threads = %+v", scan.Threads)
	}
}

// A reordered list must not silently re-point a ref: the index resolves to the
// entry's NAME, which is what survives the edit that answers the thread.
func TestYAMLRefResolvesIndexToName(t *testing.T) {
	const before = `operations:
  - id: task.list
  - id: task.create
    # @dwht: split this
`
	const after = `operations:
  - id: task.create
    # @dwht: split this
  - id: task.list
`
	for _, content := range []string{before, after} {
		scan := parser.ScanAnnotations("capabilities.yaml", []byte(content))
		resolveAnnotationRefs("task-list", "capabilities.yaml", []byte(content), scan.Threads)
		if len(scan.Threads) != 1 {
			t.Fatalf("threads = %+v", scan.Threads)
		}
		if got, want := scan.Threads[0].Anchor.Ref, "@task-list/operation:task.create"; got != want {
			t.Errorf("ref = %q, want %q", got, want)
		}
	}
}

// Ref resolution counts bullets, fields and turns, so it must count what the
// PARSERS see. A commented-out bullet is not a constraint, and counting it
// makes the ref name a different bullet than the reviewer read.
func TestRefResolutionIgnoresCommentedOutContent(t *testing.T) {
	const content = `# Task List — Intents

## Add Task

**Constraints**:
<!-- - A constraint someone parked while rewriting -->
- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 -->
`
	ref, field, index := refFor(t, "task-list", "spec/intents/task-list/intents.md", content)
	if ref != "@task-list/intent:add-task" || field != "Constraints" {
		t.Fatalf("ref = %q, field = %q", ref, field)
	}
	if index != "0" {
		t.Errorf("index = %q, want 0 — the commented-out bullet must not count", index)
	}
}

func TestDialogTurnIndexIgnoresCommentedOutTurns(t *testing.T) {
	const content = `# Authoring — Dialogs

### Check Readiness

<!-- User: a turn someone parked -->
User: I want to check readiness.
System: Let me check.
<!-- @dwht: say what it checks -->
`
	_, field, index := refFor(t, "authoring", "spec/intents/authoring/dialogs.md", content)
	if field != "turn" || index != "1" {
		t.Errorf("field = %q, index = %q, want turn[1]", field, index)
	}
}

// The same leak on the ref side: a field hidden behind a leading inline
// comment must still be found, and a trailing comment must not become part of
// an entry's name.
func TestRefResolutionSeesThroughInlineComments(t *testing.T) {
	const md = `# Task List — Intents

## Add Task

<!-- parked --> **Constraints**:
- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 -->
`
	ref, field, index := refFor(t, "task-list", "spec/intents/task-list/intents.md", md)
	if ref != "@task-list/intent:add-task" || field != "Constraints" || index != "0" {
		t.Errorf("ref = %q, field = %q, index = %q", ref, field, index)
	}

	const yml = `feature: task-list
operations:
  - id: task.create # the composer's write
    kind: command
    # @dwht: this is two operations
`
	ref, _, _ = refFor(t, "task-list", "spec/intents/task-list/capabilities.yaml", yml)
	if ref != "@task-list/operation:task.create" {
		t.Errorf("ref = %q — a trailing comment must not become part of the id", ref)
	}
}

// §4.4 promises "page name + region / layout node id". The region half is the
// Markdown body; the node half is inside the `## Layout` fence, which is YAML
// and which the page loader decodes with yaml.v3 — so a `#` comment there is
// invisible to the parser and available to the scanner.
func TestPageLayoutNodeRef(t *testing.T) {
	const content = "# Board\n\n## Header\n\n1. @task-list/header\n\n## Layout\n\n```yaml\n" +
		"componentVocabulary: clarity@17\n" +
		"schema_version: 1\n" +
		"nodes:\n" +
		"  - id: root\n" +
		"    type: stack\n" +
		"    direction: vertical\n" +
		"    # @dwht: this should be horizontal on wide viewports\n" +
		"  - id: sidebar\n" +
		"    type: panel\n" +
		"```\n"

	scan := parser.ScanAnnotations("spec/pages/board.page.md", []byte(content))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v", scan.Findings)
	}
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %d, want 1: %+v", len(scan.Threads), scan.Threads)
	}
	resolveAnnotationRefs("task-list", "spec/pages/board.page.md", []byte(content), scan.Threads)
	anchor := scan.Threads[0].Anchor
	if anchor.Ref != "page:board" {
		t.Errorf("ref = %q", anchor.Ref)
	}
	if anchor.Field != "node:root" {
		t.Errorf("field = %q, want node:root — the node, not the region", anchor.Field)
	}
	if !strings.Contains(anchor.Text, "direction: vertical") {
		t.Errorf("anchor text = %q", anchor.Text)
	}
	if strings.Contains(anchor.Text, "sidebar") {
		t.Errorf("the anchor claimed the next node: %q", anchor.Text)
	}
}

// The layout fence is the ONE fenced block the scanner reads into. A yaml
// fence anywhere else is an example — reading it would make this design's own
// schema document carry live annotations.
func TestOnlyThePageLayoutFenceIsScanned(t *testing.T) {
	const doc = "# Annotation Schema\n\n## Host forms\n\n```yaml\nverify:\n  - criterion A\n  # @dwht: an example, not a request\n```\n"
	for _, path := range []string{"annotation.schema.md", "infrastructure.md", "spec/pages/board.page.md"} {
		scan := parser.ScanAnnotations(path, []byte(doc))
		if len(scan.Threads) != 0 || len(scan.Findings) != 0 {
			t.Errorf("%s: threads = %+v, findings = %+v — no `## Layout` heading, so nothing here is a page layout",
				path, scan.Threads, scan.Findings)
		}
	}
}
