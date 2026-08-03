// parlay-feature: helper-extraction
// parlay-component: DuplicationScanResults
// parlay-artifact: test

package commands

import (
	"bytes"
	"github.com/ddwht/parlay/core/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunSimplify_UsesGivenSourceRoot is a regression test for the
// hardcoded "internal/commands/" source root runSimplify used to scan
// unconditionally, regardless of the caller's actual project layout.
// It writes duplicated helpers under a project-shaped root that is NOT
// named internal/commands/ and confirms runSimplify finds them there —
// proving it scans args[0], not a fixed path.
func TestRunSimplify_UsesGivenSourceRoot(t *testing.T) {
	setupTestDir(t)

	sourceRoot := filepath.Join("cmd", "myproject")
	if err := os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatal(err)
	}
	body := `// parlay-feature: test
// parlay-component: x
package myproject

func sharedHelper() []string {
	return []string{"a", "b", "c"}
}
`
	os.WriteFile(filepath.Join(sourceRoot, "a.go"), []byte(body), 0644)
	os.WriteFile(filepath.Join(sourceRoot, "b.go"), []byte(body), 0644)

	cmd := testCommandWithContext(t, testContext(t))
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runSimplify(cmd, []string{sourceRoot}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "sharedHelper") {
		t.Errorf("expected sharedHelper duplicate found under %s, got: %s", sourceRoot, buf.String())
	}
}

func TestSimplify_NoDuplicates(t *testing.T) {
	setupTestDir(t)

	dir := "internal/commands"
	os.MkdirAll(dir, 0755)

	os.WriteFile(filepath.Join(dir, "a.go"), []byte(`// parlay-feature: test
// parlay-component: a
package commands

func uniqueA() string { return "a" }
`), 0644)

	os.WriteFile(filepath.Join(dir, "b.go"), []byte(`// parlay-feature: test
// parlay-component: b
package commands

func uniqueB() string { return "b" }
`), 0644)

	groups, err := findDuplicateFunctions([]string{
		filepath.Join(dir, "a.go"),
		filepath.Join(dir, "b.go"),
	}, "internal/config/helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("expected 0 duplicate groups, got %d", len(groups))
	}
}

func TestSimplify_IdenticalDuplicates(t *testing.T) {
	setupTestDir(t)

	dir := "internal/commands"
	os.MkdirAll(dir, 0755)

	body := `// parlay-feature: test
// parlay-component: x
package commands

func sharedHelper() []string {
	return []string{"a", "b", "c"}
}
`
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(body), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(body), 0644)

	groups, err := findDuplicateFunctions([]string{
		filepath.Join(dir, "a.go"),
		filepath.Join(dir, "b.go"),
	}, "internal/config/helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}
	if groups[0].FunctionName != "sharedHelper" {
		t.Errorf("expected function name 'sharedHelper', got %q", groups[0].FunctionName)
	}
	if groups[0].Similarity != "identical" {
		t.Errorf("expected similarity 'identical', got %q", groups[0].Similarity)
	}
	if len(groups[0].SourceFiles) != 2 {
		t.Errorf("expected 2 source files, got %d", len(groups[0].SourceFiles))
	}
}

func TestSimplify_SkipsTestFiles(t *testing.T) {
	setupTestDir(t)

	dir := "internal/commands"
	os.MkdirAll(dir, 0755)

	body := `package commands

func sharedHelper() []string {
	return []string{"a", "b", "c"}
}
`
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("// parlay-feature: test\n// parlay-component: x\n"+body), 0644)
	os.WriteFile(filepath.Join(dir, "a_test.go"), []byte(body), 0644)

	groups, err := findDuplicateFunctions([]string{
		filepath.Join(dir, "a.go"),
		filepath.Join(dir, "a_test.go"),
	}, "internal/config/helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("test files should be excluded, got %d groups", len(groups))
	}
}

// The extraction destination comes from the adapter's file-conventions, not a
// hardcoded path. helper-extraction/intents.md:20 requires it ("determined from
// the adapter's file-conventions"); the previous implementation returned
// parlay's own "internal/config/helpers.go" to every project.
func TestSimplify_DestinationComesFromAdapterPackages(t *testing.T) {
	dir := t.TempDir()
	adapters := filepath.Join(dir, ".parlay", "adapters")
	if err := os.MkdirAll(adapters, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := `name: react-antd
kind: presentation
file-conventions:
  source-root: "src/"
  packages:
    utils: "src/utils/"
`
	if err := os.WriteFile(filepath.Join(adapters, "react-antd.adapter.yaml"), []byte(adapter), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "p", Path: dir, Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)

	if got, want := sharedHelperDestination(cfg, "src/"), filepath.Join("src/utils/", "helpers.go"); got != want {
		t.Errorf("destination = %q, want %q", got, want)
	}

	// No adapter at all: fall back to the source root rather than inventing one.
	bare := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "b", Path: t.TempDir(), Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)
	if got, want := sharedHelperDestination(bare, "cmd/"), filepath.Join("cmd/", "helpers.go"); got != want {
		t.Errorf("fallback destination = %q, want %q", got, want)
	}
}
