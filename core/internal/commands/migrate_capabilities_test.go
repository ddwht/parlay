// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-migration-operations-extraction
// parlay-artifact: test
// parlay-extends: parlay-tool/architectural-prose-artifact/partial-migration-semantics-in-migrate-capabilities

package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrateCapabilities_OperationOnlyDeletesInfra exercises the
// all-extracted case: every fragment in infrastructure.md is
// operation-shaped, so capabilities.yaml is written and the now-empty
// infrastructure.md is deleted.
func TestMigrateCapabilities_OperationOnlyDeletesInfra(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "op-only")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	infra := `# Op only — Infrastructure

---

## Task create-one

**Affects**: task entity persistence
**Behavior**: validate-input then create-one Task in storage
**Source**: @op-only/create
`
	infraPath := filepath.Join(featDir, "infrastructure.md")
	if err := os.WriteFile(infraPath, []byte(infra), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateCapabilitiesDryRun = false
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
	if !strings.Contains(string(capContent), "feature: op-only") {
		t.Errorf("expected feature: op-only header; got:\n%s", capContent)
	}

	if _, err := os.Stat(infraPath); !os.IsNotExist(err) {
		t.Errorf("expected infrastructure.md to be deleted; stat err = %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"Extracted to capabilities.yaml:",
		"Retained in infrastructure.md: (none)",
		"Deleted: infrastructure.md (was empty after extraction)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected stdout to contain %q; got:\n%s", want, out)
		}
	}
}

// TestMigrateCapabilities_MixedPartitionsBoth exercises the mixed case:
// the fragment partition produces both an extracted operation in
// capabilities.yaml and a retained architectural fragment in
// infrastructure.md.
func TestMigrateCapabilities_MixedPartitionsBoth(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "mixed")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	infra := `# Mixed — Infrastructure

---

## Task storage boundary

**Affects**: package import boundary
**Behavior**: tasks package may not import internal/storage directly
**Source**: @mixed/boundary

---

## Task create-one

**Affects**: task entity persistence
**Behavior**: validate-input then create-one Task in storage
**Source**: @mixed/create
`
	infraPath := filepath.Join(featDir, "infrastructure.md")
	if err := os.WriteFile(infraPath, []byte(infra), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateCapabilitiesDryRun = false
	if err := runMigrateCapabilities(cmd, nil); err != nil {
		t.Fatalf("runMigrateCapabilities: %v", err)
	}

	capContent, err := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	if err != nil {
		t.Fatalf("read capabilities.yaml: %v", err)
	}
	if !strings.Contains(string(capContent), "task-create-one") {
		t.Errorf("expected operation id task-create-one in capabilities.yaml; got:\n%s", capContent)
	}

	retainedInfra, err := os.ReadFile(infraPath)
	if err != nil {
		t.Fatalf("read retained infrastructure.md: %v", err)
	}
	if !strings.Contains(string(retainedInfra), "Task storage boundary") {
		t.Errorf("expected Task storage boundary retained in infrastructure.md; got:\n%s", retainedInfra)
	}
	if strings.Contains(string(retainedInfra), "Task create-one") {
		t.Errorf("did not expect Task create-one to remain in infrastructure.md; got:\n%s", retainedInfra)
	}

	out := buf.String()
	for _, want := range []string{
		"Extracted to capabilities.yaml:",
		"Retained in infrastructure.md:",
		"Task storage boundary",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected stdout to contain %q; got:\n%s", want, out)
		}
	}
}

// TestMigrateCapabilities_ArchOnlyLeavesInfraInPlace exercises the
// all-retained case: no operation-shaped fragments are present, so no
// capabilities.yaml is created and infrastructure.md is byte-identical
// to before the run. Exit code is zero.
func TestMigrateCapabilities_ArchOnlyLeavesInfraInPlace(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "arch-only")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	infra := `# Arch only — Infrastructure

---

## SDK import boundary

**Affects**: package import boundary
**Behavior**: only internal/sdk may import the upstream SDK
**Source**: @arch-only/boundary
`
	infraPath := filepath.Join(featDir, "infrastructure.md")
	if err := os.WriteFile(infraPath, []byte(infra), 0o644); err != nil {
		t.Fatal(err)
	}
	preHash := hashFile(t, infraPath)

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateCapabilitiesDryRun = false
	if err := runMigrateCapabilities(cmd, nil); err != nil {
		t.Fatalf("runMigrateCapabilities: %v", err)
	}

	if _, err := os.Stat(filepath.Join(featDir, "capabilities.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no capabilities.yaml; stat err = %v", err)
	}
	postHash := hashFile(t, infraPath)
	if preHash != postHash {
		t.Errorf("infrastructure.md hash changed across all-retained run\n  pre:  %s\n  post: %s", preHash, postHash)
	}
	out := buf.String()
	if !strings.Contains(out, "no operation-shaped fragments to migrate") {
		t.Errorf("expected stdout to mention 'no operation-shaped fragments to migrate'; got:\n%s", out)
	}
}

// TestMigrateCapabilities_DryRunIsByteIdentical exercises --dry-run:
// the partition prints with a dry-run header and the feature folder is
// byte-identical to before the run.
func TestMigrateCapabilities_DryRunIsByteIdentical(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "dryrun-mixed")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	infra := `# Mixed — Infrastructure

---

## Task storage boundary

**Affects**: package import boundary
**Behavior**: tasks package may not import internal/storage directly
**Source**: @dryrun-mixed/boundary

---

## Task create-one

**Affects**: task entity persistence
**Behavior**: validate-input then create-one Task in storage
**Source**: @dryrun-mixed/create
`
	infraPath := filepath.Join(featDir, "infrastructure.md")
	if err := os.WriteFile(infraPath, []byte(infra), 0o644); err != nil {
		t.Fatal(err)
	}
	preHash := hashFile(t, infraPath)

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateCapabilitiesDryRun = true
	defer func() { migrateCapabilitiesDryRun = false }()
	if err := runMigrateCapabilities(cmd, nil); err != nil {
		t.Fatalf("runMigrateCapabilities --dry-run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(featDir, "capabilities.yaml")); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote capabilities.yaml; stat err = %v", err)
	}
	postHash := hashFile(t, infraPath)
	if preHash != postHash {
		t.Errorf("dry-run mutated infrastructure.md\n  pre:  %s\n  post: %s", preHash, postHash)
	}
	out := buf.String()
	for _, want := range []string{
		"(dry run — no files written)",
		"Extracted to capabilities.yaml:",
		"Retained in infrastructure.md:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected dry-run stdout to contain %q; got:\n%s", want, out)
		}
	}
}

// TestMigrateCapabilities_SkipsExistingCapabilities preserves the
// pre-existing idempotency invariant.
func TestMigrateCapabilities_SkipsExistingCapabilities(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "x")
	os.MkdirAll(featDir, 0o755)
	os.WriteFile(filepath.Join(featDir, "infrastructure.md"),
		[]byte("# x — Infrastructure\n\n---\n\n## something\n\nvalidate-input then create-one a User entity.\n"), 0o644)
	existingCap := []byte("schema_version: 1\nfeature: x\noperations: []\n")
	os.WriteFile(filepath.Join(featDir, "capabilities.yaml"), existingCap, 0o644)

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	migrateCapabilitiesDryRun = false
	if err := runMigrateCapabilities(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(featDir, "capabilities.yaml"))
	if string(got) != string(existingCap) {
		t.Errorf("capabilities.yaml was overwritten; got %q", got)
	}
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
