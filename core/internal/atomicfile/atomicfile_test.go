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
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/testsupport"
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
// deployment package may call os.WriteFile, os.Create, or ioutil.WriteFile — the
// canonical path is this package.
//
// It walks each file's AST rather than grepping its text. The inherited version
// did a substring scan, which reports any file that so much as names the
// primitive: the first comment written here explaining why the digest write moved
// mentioned "os.WriteFile" and failed the guard that comment was describing. A
// guard that cannot distinguish a call from prose about a call will eventually be
// silenced by rewording, which is the wrong repair.
//
// The package list is explicit, and that is the real weak point — worth stating
// plainly, because it has already cost something. core/internal/commands writes
// plenty of files legitimately (intents.md, buildfiles), so it cannot be scanned
// wholesale; DIGEST.md was deployed from there and stayed an unconditional write
// after the other nine were converted, with this guard green throughout. The fix
// was to move that write into a package the list already covers, not to widen the
// list. A deployment write outside these packages is still invisible here, so put
// deployment writes in a deployment package.
//
// The root is anchored on go.mod rather than a ".." hop count from this file: a
// guard whose root is computed from its own depth silently narrows when the file
// moves, which is worse than breaking.
func TestNoDirectWritePrimitives(t *testing.T) {
	root, err := testsupport.ModuleRoot(".")
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	// The packages whose job is deploying files. atomicfile itself is exempt —
	// it is the primitive being mandated.
	guarded := []string{
		filepath.Join("core", "internal", "deployer"),
		filepath.Join("core", "internal", "embedded"),
	}
	// receiver -> forbidden selectors on it.
	forbidden := map[string]map[string]bool{
		"os":     {"WriteFile": true, "Create": true},
		"ioutil": {"WriteFile": true},
	}

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
			full := filepath.Join(dir, name)
			file, err := parser.ParseFile(token.NewFileSet(), full, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse %s: %v", full, err)
			}
			checked++
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				recv, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if forbidden[recv.Name][sel.Sel.Name] {
					t.Errorf("forbidden write primitive %s.%s called in %s/%s; use atomicfile.WriteIfChanged",
						recv.Name, sel.Sel.Name, pkg, name)
				}
				return true
			})
		}
	}
	// A scan that silently matched nothing would pass forever.
	if checked == 0 {
		t.Fatal("scanned no files; the guarded package list is wrong")
	}
}
