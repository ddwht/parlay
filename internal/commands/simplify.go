// parlay-feature: helper-extraction
// parlay-component: DuplicationScanResults

package commands

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	parlayParser "github.com/ddwht/parlay/internal/parser"
	"github.com/spf13/cobra"
)

var simplifyCmd = &cobra.Command{
	Use:   "simplify",
	Short: "Detect duplicated helpers across generated files and propose extractions",
	RunE:  runSimplify,
}

type duplicateGroup struct {
	FunctionName    string
	Similarity      string // "identical" or "near-identical"
	SourceFiles     []string
	ProposedTarget  string
	BodyHash        string
	Differences     string
}

func runSimplify(cmd *cobra.Command, args []string) error {
	sourceRoot := "internal/commands/"

	markers, scanErr := parlayParser.ScanGenerated(sourceRoot)
	if scanErr != nil {
		return fmt.Errorf("scanning generated files: %w", scanErr)
	}
	if len(markers) == 0 {
		fmt.Println("No parlay-generated files found in the source tree.")
		return nil
	}

	var generatedPaths []string
	for _, m := range markers {
		generatedPaths = append(generatedPaths, m.Path)
	}

	groups, err := findDuplicateFunctions(generatedPaths)
	if err != nil {
		return fmt.Errorf("scanning for duplicates: %w", err)
	}

	if len(groups) == 0 {
		fmt.Println("No duplicated helpers found across generated files. Nothing to extract.")
		return nil
	}

	fmt.Printf("Found %d duplicated helper(s) across generated files:\n", len(groups))
	for i, g := range groups {
		fmt.Printf("  %d. `%s` — %s in %s\n", i+1, g.FunctionName, g.Similarity, strings.Join(g.SourceFiles, ", "))
	}
	fmt.Println()
	fmt.Println("Run with --extract to review and apply extractions interactively.")

	return nil
}

func findDuplicateFunctions(paths []string) ([]duplicateGroup, error) {
	type funcInfo struct {
		Name     string
		BodyHash string
		File     string
	}

	var allFuncs []funcInfo

	fset := token.NewFileSet()
	for _, path := range paths {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}

		src, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if !fn.Name.IsExported() {
				start := fset.Position(fn.Body.Pos()).Offset
				end := fset.Position(fn.Body.End()).Offset
				body := string(src[start:end])
				h := sha256.Sum256([]byte(body))
				allFuncs = append(allFuncs, funcInfo{
					Name:     fn.Name.Name,
					BodyHash: fmt.Sprintf("%x", h[:8]),
					File:     path,
				})
			}
		}
	}

	hashGroups := make(map[string][]funcInfo)
	for _, fi := range allFuncs {
		key := fi.Name + ":" + fi.BodyHash
		hashGroups[key] = append(hashGroups[key], fi)
	}

	nameGroups := make(map[string][]funcInfo)
	for _, fi := range allFuncs {
		nameGroups[fi.Name] = append(nameGroups[fi.Name], fi)
	}

	var groups []duplicateGroup

	seen := make(map[string]bool)
	for key, fis := range hashGroups {
		if len(fis) < 2 {
			continue
		}
		name := fis[0].Name
		if seen[name] {
			continue
		}
		seen[name] = true

		var files []string
		for _, fi := range fis {
			files = append(files, filepath.Base(fi.File))
		}

		groups = append(groups, duplicateGroup{
			FunctionName:   name,
			Similarity:     "identical",
			SourceFiles:    files,
			ProposedTarget: proposeTarget(name),
			BodyHash:       fis[0].BodyHash,
		})
		_ = key
	}

	for name, fis := range nameGroups {
		if seen[name] || len(fis) < 2 {
			continue
		}
		hashes := make(map[string]bool)
		for _, fi := range fis {
			hashes[fi.BodyHash] = true
		}
		if len(hashes) <= 1 {
			continue
		}

		var files []string
		for _, fi := range fis {
			files = append(files, filepath.Base(fi.File))
		}

		groups = append(groups, duplicateGroup{
			FunctionName:   name,
			Similarity:     "near-identical",
			SourceFiles:    files,
			ProposedTarget: proposeTarget(name),
			Differences:    "function bodies differ in literals or error messages",
		})
	}

	return groups, nil
}

func proposeTarget(funcName string) string {
	return "internal/config/helpers.go"
}
