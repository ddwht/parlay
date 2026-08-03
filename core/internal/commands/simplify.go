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

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	parlayParser "github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var simplifyCmd = &cobra.Command{
	Use:   "simplify <source-root>",
	Short: "Detect duplicated helpers across generated files and propose extractions",
	Args:  cobra.ExactArgs(1),
	RunE:  runSimplify,
}

type duplicateGroup struct {
	FunctionName   string
	Similarity     string // "identical" or "near-identical"
	SourceFiles    []string
	ProposedTarget string
	BodyHash       string
	Differences    string
}

func runSimplify(cmd *cobra.Command, args []string) error {
	// source-root is a required positional argument, matching
	// scan-generated's and save-build-state --source-root's convention
	// — this command has no adapter-resolution machinery of its own, so
	// it takes the same explicit input every other source-tree-walking
	// command takes rather than hardcoding a path (the previous
	// "internal/commands/" default only ever made sense for scanning
	// this repo's own dev tree, not a caller's actual project).
	sourceRoot := args[0]

	markers, scanErr := parlayParser.ScanGenerated(sourceRoot)
	if scanErr != nil {
		return fmt.Errorf("scanning generated files: %w", scanErr)
	}
	if len(markers) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No parlay-generated files found in the source tree.")
		return nil
	}

	var generatedPaths []string
	for _, m := range markers {
		generatedPaths = append(generatedPaths, m.Path)
	}

	// Resolve the shared-helper destination from the adapter rather than
	// assuming one. mustContext failing is not fatal here — simplify can run
	// against a bare source tree, and sharedHelperDestination falls back to it.
	cfg, _ := mustContext(cmd)
	groups, err := findDuplicateFunctions(generatedPaths, sharedHelperDestination(cfg, sourceRoot))
	if err != nil {
		return fmt.Errorf("scanning for duplicates: %w", err)
	}

	if len(groups) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No duplicated helpers found across generated files. Nothing to extract.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Found %d duplicated helper(s) across generated files:\n", len(groups))
	for i, g := range groups {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d. `%s` — %s in %s\n", i+1, g.FunctionName, g.Similarity, strings.Join(g.SourceFiles, ", "))
	}
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Run with --extract to review and apply extractions interactively.")

	return nil
}

func findDuplicateFunctions(paths []string, target string) ([]duplicateGroup, error) {
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
			ProposedTarget: target,
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
			ProposedTarget: target,
			Differences:    "function bodies differ in literals or error messages",
		})
	}

	return groups, nil
}

// sharedHelperDestination resolves where an extracted helper should live from
// the active adapter's file-conventions, per helper-extraction/intents.md:
// "The target shared package should be determined from the adapter's
// file-conventions and the project's existing package structure."
//
// This used to return a hardcoded "internal/config/helpers.go" — parlay's own
// layout — so every project was told to extract into a directory that may not
// exist in it. `packages:` is the block that answers this question: no `paths:`
// template names a shared-code destination, because `paths:` is per-artifact
// (component, model, service, routes) while this is "where does reusable code
// live". That is why both blocks exist.
//
// Falls back to the source root when the adapter declares no shared package,
// which is a correct answer rather than a guess: a flat project genuinely has
// nowhere else to put it.
func sharedHelperDestination(cfg *config.Context, sourceRoot string) string {
	dir := ""
	if cfg != nil {
		if adapterPath := presentationAdapterFile(cfg); adapterPath != "" {
			if a, err := agent.LoadAdapterFile(adapterPath); err == nil && a.FileConventions != nil {
				fc := a.FileConventions
				// Preference order: the most-shared package first.
				for _, key := range []string{"utils", "shared", "core", "lib"} {
					if v := strings.TrimSpace(fc.Packages[key]); v != "" {
						dir = v
						break
					}
				}
				if dir == "" && strings.TrimSpace(fc.SourceRoot) != "" {
					dir = fc.SourceRoot
				}
			}
		}
	}
	if dir == "" {
		dir = sourceRoot
	}
	// The duplicate analyzer parses Go only (see findDuplicateFunctions), so
	// the emitted destination is a .go file. When it grows to other languages
	// the extension follows the adapter the same way the directory now does.
	return filepath.Join(dir, "helpers.go")
}
