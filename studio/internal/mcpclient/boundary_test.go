// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/official-mcp-sdk-adoption

package mcpclient_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestSDKImportBoundary walks the studio/ source tree and asserts that the
// official MCP Go SDK (github.com/modelcontextprotocol/go-sdk) is imported
// only from studio/internal/mcpclient. The walk runs from the studio module
// root and inspects every .go file it finds, ignoring vendor/ and
// build-artefact directories.
//
// The boundary is the load-bearing invariant of figma-mcp-client: keeping
// the SDK confined to a single package means SDK churn touches one file set,
// and alternative-library substitutions cannot land without rewriting the
// wrapper.
func TestSDKImportBoundary(t *testing.T) {
	root := studioRoot(t)
	const sdkPath = "github.com/modelcontextprotocol/go-sdk"

	violators := walkImports(t, root, func(file, imp string) bool {
		if !strings.HasPrefix(imp, sdkPath) {
			return false
		}
		// Imports from inside studio/internal/mcpclient itself are allowed.
		return !strings.Contains(file, "internal/mcpclient")
	})

	if len(violators) > 0 {
		t.Fatalf("github.com/modelcontextprotocol/go-sdk imported outside "+
			"studio/internal/mcpclient (figma-mcp-client violation): %v",
			violators)
	}
}

// TestNoAlternativeMCPLibraries walks the studio/ source tree and asserts
// that no alternative MCP client library (e.g. github.com/mark3labs/mcp-go)
// is imported anywhere — not even from inside studio/internal/mcpclient.
// Switching libraries is a v2-or-later spec revision against figma-mcp-client,
// not a runtime fallback.
func TestNoAlternativeMCPLibraries(t *testing.T) {
	root := studioRoot(t)
	forbidden := []string{
		"github.com/mark3labs/mcp-go",
	}

	violators := walkImports(t, root, func(file, imp string) bool {
		for _, f := range forbidden {
			if strings.HasPrefix(imp, f) {
				return true
			}
		}
		return false
	})

	if len(violators) > 0 {
		t.Fatalf("alternative MCP library imported under studio/ "+
			"(figma-mcp-client violation): %v", violators)
	}
}

// studioRoot resolves the studio module root from the test's working
// directory. The test file lives at studio/internal/mcpclient/boundary_test.go,
// so walking up two levels reaches the studio module root.
func studioRoot(t *testing.T) string {
	t.Helper()
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("studioRoot: %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

// walkImports scans every .go file under root and reports relative-path
// "<file> imports <import-path>" strings for each import that satisfies pred.
// Build artefacts and vendor directories are skipped.
func walkImports(t *testing.T, root string, pred func(file, imp string) bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	var violators []string

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			switch name {
			case "vendor", "node_modules", ".parlay", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			// Parse errors are surfaced by `go build`; the boundary test
			// stays focused on imports.
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for _, imp := range file.Imports {
			impPath := strings.Trim(imp.Path.Value, `"`)
			if pred(rel, impPath) {
				violators = append(violators, rel+" imports "+impPath)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	return violators
}
