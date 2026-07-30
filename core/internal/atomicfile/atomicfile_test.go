// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency
// parlay-artifact: test

// Salvaged from internal/editor/deployer/atomic_write_test.go along with the
// primitive it covers. TestNoDirectWritePrimitives is generalized: it used to
// scan one package, and now scans every package that deploys files, since the
// guarantee is about parlay's deployment paths rather than about one deployer.

package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAtomicCreatesFileAndParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "out.md")
	if err := WriteAtomic(path, []byte("hello")); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q, want hello", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file survived a successful write: %v", err)
	}
}

// TestWriteAtomicRenameFailureLeavesOriginal is the crash-safety claim: when the
// rename fails, the pre-existing target must still hold its old bytes. A
// half-written skill file is the outcome this primitive exists to prevent.
func TestWriteAtomicRenameFailureLeavesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	boom := errors.New("rename refused")
	err := WriteAtomicWith(path, []byte("replacement"), func(string, string) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want the injected rename error, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("target was modified despite a failed rename: %q", got)
	}
	// The .tmp is deliberately left for the next run to overwrite.
	if _, err := os.Stat(path + ".tmp"); err != nil {
		t.Fatalf("expected the .tmp to remain after a failed rename: %v", err)
	}
}

// TestWriteAtomicOverwritesStaleTemp covers the O_TRUNC choice: debris from a
// previously crashed run must not corrupt the next write by being appended to.
func TestWriteAtomicOverwritesStaleTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	if err := os.WriteFile(path+".tmp", []byte("LEFTOVER GARBAGE FROM A CRASH"), 0o644); err != nil {
		t.Fatalf("seed stale tmp: %v", err)
	}
	if err := WriteAtomic(path, []byte("fresh")); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "fresh" {
		t.Fatalf("stale temp contaminated the write: %q", got)
	}
}

func TestWriteIfChangedSkipsIdenticalContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")

	wrote, err := WriteIfChanged(path, []byte("v1"))
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if !wrote {
		t.Fatal("first write reported no write; the file did not exist")
	}

	// A skip must not reach the rename at all. Proving that needs the skip
	// decision to be observable independently of the return value, so the file
	// is made read-only: a rewrite would fail, a skip cannot notice.
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	wrote, err = WriteIfChanged(path, []byte("v1"))
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if wrote {
		t.Fatal("identical content was rewritten; the content-hash skip did not fire")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod back: %v", err)
	}

	wrote, err = WriteIfChanged(path, []byte("v2"))
	if err != nil {
		t.Fatalf("changed write: %v", err)
	}
	if !wrote {
		t.Fatal("changed content was skipped")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "v2" {
		t.Fatalf("content = %q, want v2", got)
	}
}

// TestWriteIfChangedUnreadableFileIsAnError asserts an unreadable existing file
// is reported rather than overwritten. Treating a read failure as "differs"
// turns a permissions problem into data loss.
func TestWriteIfChangedUnreadableFileIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; mode bits do not deny reads")
	}
	path := filepath.Join(t.TempDir(), "out.md")
	if err := os.WriteFile(path, []byte("secret"), 0o000); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := WriteIfChanged(path, []byte("replacement")); err == nil {
		t.Fatal("want an error for an unreadable target, got nil")
	}
	// Restore so t.TempDir cleanup can remove it.
	_ = os.Chmod(path, 0o644)
}

// TestNoDirectWritePrimitives is the guardrail. No non-test Go file in any
// deployment package may call os.WriteFile, ioutil.WriteFile, or os.Create — the
// canonical path is this package.
//
// It scans by walking the module from this package's location, so a new
// deployment package is covered the day it is added rather than the day someone
// remembers to list it here. Anchored on go.mod rather than a ".." hop count: a
// guard whose root is computed from its own depth silently narrows when the file
// moves, which is a worse failure than breaking.
func TestNoDirectWritePrimitives(t *testing.T) {
	root := moduleRoot(t)
	// The packages whose job is deploying files. atomicfile itself is exempt —
	// it is the primitive being mandated.
	guarded := []string{
		filepath.Join("core", "internal", "deployer"),
		filepath.Join("core", "internal", "embedded"),
	}
	forbidden := []string{"os.WriteFile", "ioutil.WriteFile", "os.Create("}

	var checked int
	for _, pkg := range guarded {
		dir := filepath.Join(root, pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			// dumpskills.go is a //go:build ignore generator that writes the
			// expanded skills to a caller-supplied temp directory for
			// `make verify-skills`. It deploys nothing.
			if name == "dumpskills.go" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("ReadFile %s: %v", name, err)
			}
			checked++
			for _, f := range forbidden {
				if strings.Contains(string(data), f) {
					t.Errorf("forbidden write primitive %q in %s/%s; use atomicfile.WriteIfChanged", f, pkg, name)
				}
			}
		}
	}
	// A scan that silently matched nothing would pass forever.
	if checked == 0 {
		t.Fatal("scanned no files; the guarded package list is wrong")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found walking up from %s", dir)
		}
		dir = parent
	}
}
