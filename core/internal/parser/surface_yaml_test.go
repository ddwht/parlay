// parlay-feature: parlay-tool/multi-adapter
// parlay-component: surface-yaml-and-migrator
// parlay-artifact: test

package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSurfaceYAML_BasicShape(t *testing.T) {
	content := []byte(`feature: task-list
fragments:
  - name: TaskList
    shows: data-list
    actions: invoke, navigate-drill
    source: "@task-list/list-tasks"
    page: tasks
    region: main
    order: 1
    notes:
      - "Sorted by priority"
`)
	frags, err := LoadSurfaceYAMLBytes("test.yaml", content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(frags); got != 1 {
		t.Fatalf("fragments: got %d, want 1", got)
	}
	f := frags[0]
	if f.Name != "TaskList" || f.Shows != "data-list" || f.Page != "tasks" {
		t.Errorf("got %+v", f)
	}
	if f.Feature != "task-list" {
		t.Errorf("feature: got %q, want task-list", f.Feature)
	}
	if got := len(f.Notes); got != 1 || f.Notes[0] != "Sorted by priority" {
		t.Errorf("notes: got %v", f.Notes)
	}
}

// TestLoadSurfaceYAML_CompositionFields verifies supersedes: and the tri-state
// interactive: field parse from the YAML form. Absent interactive: reads as
// interactive (nil pointer, IsInteractive true); an explicit false is
// output-only.
func TestLoadSurfaceYAML_CompositionFields(t *testing.T) {
	content := []byte(`fragments:
  - name: New Viewport
    shows: media
    source: "@viewer/render"
    page: canvas
    region: main
    supersedes: "@old-viewer/viewport"
  - name: Readout
    shows: data-value
    source: "@viewer/stats"
    page: canvas
    region: sidebar
    interactive: false
  - name: Plain
    shows: data-value
    source: "@viewer/x"
    page: canvas
    region: footer
`)
	frags, err := LoadSurfaceYAMLBytes("test.yaml", content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(frags) != 3 {
		t.Fatalf("fragments: got %d, want 3", len(frags))
	}
	if frags[0].Supersedes != "@old-viewer/viewport" {
		t.Errorf("supersedes: got %q", frags[0].Supersedes)
	}
	if !frags[0].IsInteractive() {
		t.Errorf("fragment with no interactive: should default interactive")
	}
	if frags[1].Interactive == nil || *frags[1].Interactive {
		t.Errorf("interactive: false should parse to explicit false, got %v", frags[1].Interactive)
	}
	if frags[1].IsInteractive() {
		t.Errorf("interactive: false fragment must not be interactive")
	}
	if frags[2].Interactive != nil {
		t.Errorf("absent interactive: should stay nil, got %v", frags[2].Interactive)
	}
}

// TestParseSurfaceFile_MarkdownCompositionFields verifies the legacy markdown
// parser reads **Supersedes** and **Interactive** identically to the YAML form.
func TestParseSurfaceFile_MarkdownCompositionFields(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "surface.md")
	body := `# Viewer — Surface

## New Viewport

**Shows**: media
**Source**: @viewer/render
**Page**: canvas
**Region**: main
**Supersedes**: @old-viewer/viewport

## Readout

**Shows**: data-value
**Source**: @viewer/stats
**Page**: canvas
**Region**: sidebar
**Interactive**: false
`
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	frags, err := ParseSurfaceFile(mdPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(frags) != 2 {
		t.Fatalf("fragments: got %d, want 2", len(frags))
	}
	if frags[0].Supersedes != "@old-viewer/viewport" {
		t.Errorf("supersedes: got %q", frags[0].Supersedes)
	}
	if frags[1].IsInteractive() {
		t.Errorf("**Interactive**: false must parse to non-interactive")
	}
}

func TestParseSurfaceFile_AutoDetectYAML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "surface.yaml")
	if err := os.WriteFile(yamlPath, []byte(`fragments:
  - { name: Foo, shows: data-value, actions: invoke, source: "@x/y", page: home, region: main }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	frags, err := ParseSurfaceFile(yamlPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := len(frags); got != 1 || frags[0].Name != "Foo" {
		t.Errorf("got %+v", frags)
	}
}
