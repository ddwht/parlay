// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-migration-operations-extraction
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateCapabilities_ExtractsOperationFragment(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "task-list")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	infra := `# Infrastructure

Task creation runs validate-input then create-one to persist a new Task entity.

A router registry that lists registered handlers.
`
	if err := os.WriteFile(filepath.Join(featDir, "infrastructure.md"), []byte(infra), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := testCommandWithContext(t, testContext(t))
	if err := runMigrateCapabilities(cmd, nil); err != nil {
		t.Fatalf("runMigrateCapabilities: %v", err)
	}

	capContent, err := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	if err != nil {
		t.Fatalf("read capabilities.yaml: %v", err)
	}
	if !strings.Contains(string(capContent), "kind: unknown") {
		t.Errorf("expected kind: unknown stub; got:\n%s", capContent)
	}
	if !strings.Contains(string(capContent), "task-list") {
		t.Errorf("expected feature task-list; got:\n%s", capContent)
	}

	report, err := os.ReadFile(filepath.Join(featDir, "migration-report.md"))
	if err != nil {
		t.Fatalf("read migration-report.md: %v", err)
	}
	if !strings.Contains(string(report), "registry") {
		t.Errorf("expected registry classification in report; got:\n%s", report)
	}
}

func TestMigrateCapabilities_SkipsExistingCapabilities(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "x")
	os.MkdirAll(featDir, 0o755)
	os.WriteFile(filepath.Join(featDir, "infrastructure.md"),
		[]byte("Validate-input then create-one a User entity.\n"), 0o644)
	existingCap := []byte("schema_version: 1\nfeature: x\noperations: []\n")
	os.WriteFile(filepath.Join(featDir, "capabilities.yaml"), existingCap, 0o644)

	cmd := testCommandWithContext(t, testContext(t))
	if err := runMigrateCapabilities(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	if string(got) != string(existingCap) {
		t.Errorf("capabilities.yaml was overwritten; got %q", got)
	}
}
