package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// TestDeployToRoot_BareParent verifies that deployToRoot succeeds at a
// multi-root parent registry — which has roots.yaml + schemas/ but no
// config.yaml of its own. Regression test for the previous failure
// mode where parlay upgrade errored with "run parlay init first" on
// the parent root, blocking schema redeployment in multi-root setups.
func TestDeployToRoot_BareParent(t *testing.T) {
	tmp := t.TempDir()
	parlayDir := filepath.Join(tmp, config.ParlayDir)
	if err := os.MkdirAll(parlayDir, 0o755); err != nil {
		t.Fatalf("mkdir .parlay: %v", err)
	}
	rootsPath := filepath.Join(parlayDir, config.RootsIndexFile)
	if err := os.WriteFile(rootsPath, []byte("children: []\n"), 0o644); err != nil {
		t.Fatalf("write roots.yaml: %v", err)
	}

	result, err := deployToRoot(tmp)
	if err != nil {
		t.Fatalf("deployToRoot at bare parent: %v", err)
	}
	if result.SchemaCount == 0 {
		t.Fatalf("expected schemas to be deployed at bare parent, got 0")
	}
	if result.SkillCount != 0 {
		t.Fatalf("expected no skill deployment without config.yaml, got %d", result.SkillCount)
	}

	schemasPath := filepath.Join(parlayDir, config.SchemasDir)
	entries, err := os.ReadDir(schemasPath)
	if err != nil {
		t.Fatalf("read deployed schemas dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected schema files written to %s, found none", schemasPath)
	}
}

// TestDeployToRoot_MissingConfigAndRoots preserves the original "run
// parlay init first" error for the case where neither config.yaml nor
// roots.yaml exists — the user really hasn't initialized anything.
func TestDeployToRoot_MissingConfigAndRoots(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, config.ParlayDir), 0o755); err != nil {
		t.Fatalf("mkdir .parlay: %v", err)
	}

	_, err := deployToRoot(tmp)
	if err == nil {
		t.Fatalf("expected error when both config.yaml and roots.yaml are missing")
	}
}
