// parlay-feature: parlay-tool/multi-adapter
// parlay-component: spec-migration-report
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateSpec_EmitsSurfaceYAML(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "task-list")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mdContent := `## TaskList
**Shows**: data-list
**Actions**: invoke
**Source**: @task-list/list
**Page**: tasks
**Region**: main
`
	if err := os.WriteFile(filepath.Join(featDir, "surface.md"), []byte(mdContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := testCommandWithContext(t, testContext(t))
	if err := runMigrateSpec(cmd, nil); err != nil {
		t.Fatalf("runMigrateSpec: %v", err)
	}

	yamlPath := filepath.Join(featDir, "surface.yaml")
	content, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read surface.yaml: %v", err)
	}
	if !strings.Contains(string(content), "TaskList") {
		t.Errorf("expected TaskList in yaml; got:\n%s", content)
	}
	if !strings.Contains(string(content), "data-list") {
		t.Errorf("expected data-list in yaml; got:\n%s", content)
	}
}

func TestMigrateSpec_Idempotent(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "x")
	os.MkdirAll(featDir, 0o755)
	os.WriteFile(filepath.Join(featDir, "surface.md"),
		[]byte("## A\n**Shows**: data-value\n**Source**: @x/a\n"), 0o644)
	existingYAML := []byte("# pre-existing\nfeature: x\nfragments: []\n")
	os.WriteFile(filepath.Join(featDir, "surface.yaml"), existingYAML, 0o644)

	cmd := testCommandWithContext(t, testContext(t))
	if err := runMigrateSpec(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(featDir, "surface.yaml"))
	if string(got) != string(existingYAML) {
		t.Errorf("surface.yaml was overwritten; got %q", got)
	}
}
