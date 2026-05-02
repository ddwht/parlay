// parlay-feature: helper-extraction
// parlay-component: DuplicationScanResults
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"testing"
)

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
	})
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
	})
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
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("test files should be excluded, got %d groups", len(groups))
	}
}

func TestSimplify_ProposesTarget(t *testing.T) {
	target := proposeTarget("threeTreeRoots")
	if target == "" {
		t.Error("expected non-empty proposed target")
	}
}
