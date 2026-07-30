// Package testsupport holds helpers shared by tests in more than one package.
//
// It is an ordinary package rather than a _test.go file because Go test helpers
// cannot cross package boundaries: a helper in one package's _test.go is
// invisible to another's. It deliberately does not import "testing" — that would
// register test flags into any binary linking this package — so helpers return
// errors and each caller wraps them in its own t.Fatalf.
package testsupport

import (
	"fmt"
	"os"
	"path/filepath"
)

// ModuleRoot walks up from startDir and returns the first directory containing
// go.mod — the module root.
//
// It exists because two guard tests need to scan the whole module and both had
// computed the root by counting ".." hops from their own location. That is the
// wrong shape for this: when a package moves, the hop count silently addresses a
// narrower tree and the guard keeps passing over less code. It already happened
// once here — internal/editor/config's boundary scan counted two levels, which
// pointed at the module root before the merge and at internal/ after, so it would
// have stopped covering core/ without ever failing.
//
// A landmark moves with the module. A hop count moves with the file.
func ModuleRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("testsupport: resolve %s: %w", startDir, err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("testsupport: no go.mod found walking up from %s", startDir)
		}
		dir = parent
	}
}
