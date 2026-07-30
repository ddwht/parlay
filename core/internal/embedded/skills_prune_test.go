package embedded

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPruneStaleModulesRemovesOrphanTmp covers the .tmp collection ported from
// the editor's deployer.
//
// atomicfile writes through a `<target>.tmp` sibling. A crash between its
// creation and the rename leaves one behind; for a target still in the embedded
// set that self-heals on the next write, but a target that has left the set is
// never written again and its debris would persist in the user's .parlay/.
func TestPruneStaleModulesRemovesOrphanTmp(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteModules(dir); err != nil {
		t.Fatalf("WriteModules: %v", err)
	}

	// Debris for a module that no longer exists, and for one that does.
	orphanTmp := filepath.Join(dir, "long-gone-module.md.tmp")
	if err := os.WriteFile(orphanTmp, []byte("partial"), 0o644); err != nil {
		t.Fatalf("seed orphan tmp: %v", err)
	}
	liveTmp := filepath.Join(dir, "create-artifacts.md.tmp")
	if err := os.WriteFile(liveTmp, []byte("partial"), 0o644); err != nil {
		t.Fatalf("seed live tmp: %v", err)
	}

	if err := PruneStaleModules(dir); err != nil {
		t.Fatalf("PruneStaleModules: %v", err)
	}

	if _, err := os.Stat(orphanTmp); !os.IsNotExist(err) {
		t.Fatalf("orphan .tmp for a departed module survived: %v", err)
	}
	// A live module's .tmp is left alone: the next write O_TRUNCs and renames
	// it, and removing it here would race a concurrent deploy.
	if _, err := os.Stat(liveTmp); err != nil {
		t.Fatalf("live module's .tmp was removed: %v", err)
	}
	// The real modules must survive the sweep.
	if _, err := os.Stat(filepath.Join(dir, "create-artifacts.md")); err != nil {
		t.Fatalf("prune removed a wanted module: %v", err)
	}
}
