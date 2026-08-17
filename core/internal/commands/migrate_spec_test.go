// parlay-feature: parlay-tool/multi-adapter
// parlay-component: spec-migration-report
// parlay-artifact: test

package commands

import (
	"bytes"
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

// --retire-md deletes surface.md only when surface.yaml covers every fragment
// the .md declares, and refuses per-feature otherwise. The refusal case is
// the important one: a fragment only the .md knows about would be silently
// lost by an unconditional delete.
//
// parlay-feature: parlay-tool/ledger-and-contract
func TestMigrateSpec_RetireMD(t *testing.T) {
	dir := setupTestDir(t)

	// covered: .yaml has the .md's one fragment -> retired.
	covered := filepath.Join(dir, "spec", "intents", "covered")
	os.MkdirAll(covered, 0o755)
	os.WriteFile(filepath.Join(covered, "surface.md"), []byte("## Thing List\n**Shows**: data-list\n**Source**: @covered/x\n"), 0o644)
	os.WriteFile(filepath.Join(covered, "surface.yaml"), []byte("feature: covered\nfragments:\n    - name: Thing List\n      shows: data-list\n      source: '@covered/x'\n"), 0o644)

	// holdout: .md carries a fragment the .yaml lacks -> refused.
	holdout := filepath.Join(dir, "spec", "intents", "holdout")
	os.MkdirAll(holdout, 0o755)
	os.WriteFile(filepath.Join(holdout, "surface.md"), []byte("## Kept\n**Shows**: summary\n**Source**: @holdout/x\n\n## Only In MD\n**Shows**: summary\n**Source**: @holdout/y\n"), 0o644)
	os.WriteFile(filepath.Join(holdout, "surface.yaml"), []byte("feature: holdout\nfragments:\n    - name: Kept\n      shows: summary\n      source: '@holdout/x'\n"), 0o644)

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateSpecRetireMD = true
	t.Cleanup(func() { migrateSpecRetireMD = false })
	if err := runMigrateSpec(cmd, nil); err != nil {
		t.Fatalf("runMigrateSpec: %v", err)
	}

	if _, err := os.Stat(filepath.Join(covered, "surface.md")); !os.IsNotExist(err) {
		t.Errorf("covered feature's surface.md should be retired (deleted)")
	}
	if _, err := os.Stat(filepath.Join(holdout, "surface.md")); err != nil {
		t.Errorf("holdout feature's surface.md must survive — the .yaml lacks a fragment it carries")
	}
	out := buf.String()
	if !strings.Contains(out, "retire refused") || !strings.Contains(out, "Only In MD") {
		t.Errorf("refusal should name the missing fragment; got:\n%s", out)
	}
	if !strings.Contains(out, "scaffold-signatures") {
		t.Errorf("retirement summary should point at the signature re-stamp; got:\n%s", out)
	}
}
