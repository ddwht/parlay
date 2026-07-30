//go:build ignore

// dumpskills writes every embedded skill, marker-expanded, into the directory
// named by its first argument, and prints a manifest to stdout — one
// tab-separated `name<TAB>surface<TAB>path` row per skill.
//
// It exists so the "expected body" side of `make verify-skills` is produced by
// the SAME code path deployers run: embedded.ReadAllSkills, which parses each
// skill's frontmatter and expands the `<!-- parlay:expand-... -->` markers.
// Doing that expansion in shell instead would create a second copy of the
// expansion text, one that drifts silently the moment a marker is added.
//
// The surface column is the load-bearing part. A skill does not have one
// destination: SurfaceCommand skills deploy to .claude/skills/parlay-<name>/
// SKILL.md with YAML frontmatter, SurfaceModule skills to .parlay/modules/
// <name>.md under a markdown title-and-description header. Deriving that from
// the source filename is impossible — the surface lives in the frontmatter —
// which is why the caller is told rather than left to guess.
//
// `//go:build ignore` keeps this out of ./... for build, vet, and test; naming
// it explicitly on the `go run` command line is what compiles it, because an
// explicit file list bypasses build-constraint filtering. Module imports still
// resolve normally, so it reaches the embedded package beside it.
//
// It replaces a copy of this program that the Makefile generated on the fly
// with `$(file >...)` and deleted afterwards. That function is GNU Make 4.0+;
// macOS ships 3.81, where it expands to nothing at all — no error, no file —
// so the target failed with "no Go files in ..." pointing at a directory whose
// real problem was that nobody had written to it. A committed file has no
// version floor and shows up in a diff.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/embedded"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run dumpskills.go <output-dir>")
		os.Exit(2)
	}
	outDir := os.Args[1]

	skills, err := embedded.ReadAllSkills()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, s := range skills {
		path := filepath.Join(outDir, s.Name+".expanded")
		if err := os.WriteFile(path, s.Content, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s\t%s\t%s\n", s.Name, s.Surface, path)
	}
}
