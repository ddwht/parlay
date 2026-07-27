// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: cross-cutting/out-of-process-validate-endpoint
// parlay-artifact: test
//
// The no-Core-import guard. Studio reaches Core's validator ONLY out of
// process (`parlay validate --type domain-model --json`); it must NOT import
// any Core package. This walks the studio module's own source (every .go file
// under the module root) and fails if any file imports a package under Core's
// module path. Failing it blocks the build. Mirrors the forbidden-import guard
// style in internal/deployer/subcommands_test.go, widened from one package to
// the whole module's import surface.

package domain

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// coreModulePath is Core's Go module path. Any import whose path is this
// module (or a package under it) is a forbidden Studio→Core dependency.
const coreModulePath = "github.com/ddwht/parlay"

// findStudioModuleRoot ascends from the test's working directory until it finds
// the go.mod whose module line is the studio module.
func findStudioModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(gomod); err == nil {
			if strings.Contains(string(data), "module github.com/parlay-tool/parlay/studio") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate the studio module root above %s", dir)
		}
		dir = parent
	}
}

// TestNoStudioPackageImportsCore walks every .go file in the studio module and
// asserts none imports a Core package. A single offending import fails the
// build.
func TestNoStudioPackageImportsCore(t *testing.T) {
	root := findStudioModuleRoot(t)
	fset := token.NewFileSet()

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip vendored / third-party / build-output trees.
			switch d.Name() {
			case "node_modules", "vendor", "dist", ".git", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			// A parse failure here is not this guard's concern; the compiler
			// and the rest of the suite catch malformed source.
			return nil
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p == coreModulePath || strings.HasPrefix(p, coreModulePath+"/") {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel+" imports "+p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk studio module: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("Studio must not import Core (reach the validator only out of process); found:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
