// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-artifact: test
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

package deployer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAtomicHappyPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skills", "parlay-design-loop", "SKILL.md")
	content := []byte("---\nname: parlay-design-loop\ndescription: x\n---\nbody\n")
	if err := writeAtomic(target, content); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("ReadFile mismatch: got %q, want %q", got, content)
	}
	// No .tmp file should remain on disk after a successful write.
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp file unexpectedly present after happy path: %v", err)
	}
}

func TestWriteAtomicRenameFailureLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skills", "SKILL.md")
	// Seed the target with a known original.
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := []byte("ORIGINAL")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	// Inject a failing renamer.
	failingRename := func(_, _ string) error { return errors.New("rename injected failure") }
	err := writeAtomicWith(target, []byte("NEW"), failingRename)
	if err == nil {
		t.Fatalf("expected an error from writeAtomicWith with failing renamer")
	}
	if !strings.Contains(err.Error(), "rename injected failure") {
		t.Fatalf("expected the injected error to propagate; got %v", err)
	}
	// Original target is byte-equal to its pre-call content.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("target changed after failed rename: got %q, want %q", got, original)
	}
	// The .tmp file should be observable mid-run; on the next Run, the
	// cleanup helper would remove it. Here we just assert it exists.
	if _, err := os.Stat(target + ".tmp"); err != nil {
		t.Fatalf(".tmp file missing after failed rename: %v", err)
	}
}

func TestCleanupOrphanTmpFiles(t *testing.T) {
	dir := t.TempDir()
	skills := filepath.Join(dir, "skills")
	parlay := filepath.Join(skills, "parlay-design-loop")
	if err := os.MkdirAll(parlay, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifestPath := filepath.Join(parlay, "SKILL.md")
	tmpPath := manifestPath + ".tmp"
	// .tmp file corresponding to a manifest path → should be removed.
	if err := os.WriteFile(tmpPath, []byte("debris"), 0o644); err != nil {
		t.Fatalf("seed manifest tmp: %v", err)
	}
	// .tmp file NOT on the manifest → should be left alone.
	otherTmp := filepath.Join(skills, "some-other.tmp")
	if err := os.WriteFile(otherTmp, []byte("other"), 0o644); err != nil {
		t.Fatalf("seed other tmp: %v", err)
	}
	manifest := map[string]struct{}{manifestPath: {}}
	if err := cleanupOrphanTmpFiles(skills, manifest); err != nil {
		t.Fatalf("cleanupOrphanTmpFiles: %v", err)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected manifest-sibling .tmp to be removed; got err = %v", err)
	}
	if _, err := os.Stat(otherTmp); err != nil {
		t.Fatalf("expected non-manifest .tmp to be preserved; got err = %v", err)
	}
}

// TestNoDirectWritePrimitives is the build-time guardrail. Every Go source
// file in this package EXCEPT atomic_write.go must NOT contain calls to
// os.WriteFile, ioutil.WriteFile, or os.Create — the canonical write path
// is writeAtomic.
//
// Test files (*_test.go) are explicitly exempt: tests can seed fixtures
// using os.WriteFile freely.
func TestNoDirectWritePrimitives(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	forbidden := []string{"os.WriteFile", "ioutil.WriteFile", "os.Create("}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name == "atomic_write.go" {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for _, f := range forbidden {
			if strings.Contains(string(data), f) {
				t.Fatalf("forbidden write primitive %q found in %s; use writeAtomic instead", f, name)
			}
		}
	}
}
