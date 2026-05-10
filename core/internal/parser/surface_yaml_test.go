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
