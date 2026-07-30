package embedded

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/atomicfile"
)

//go:embed schemas/*.schema.md
var schemasFS embed.FS

// WriteSchemas copies all embedded schemas to the target directory and returns
// the number it actually wrote. A schema whose on-disk copy already matches is
// skipped, so a re-deploy over unchanged sources returns 0 — the count is what
// changed, not what was considered.
func WriteSchemas(targetDir string) (int, error) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return 0, err
	}

	entries, err := fs.ReadDir(schemasFS, "schemas")
	if err != nil {
		return 0, err
	}

	written := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := schemasFS.ReadFile(filepath.Join("schemas", entry.Name()))
		if err != nil {
			return written, err
		}
		dst := filepath.Join(targetDir, entry.Name())
		wrote, err := atomicfile.WriteIfChanged(dst, data)
		if err != nil {
			return written, err
		}
		if wrote {
			written++
		}
	}
	return written, nil
}

// SchemaNames returns the list of embedded schema file names.
func SchemaNames() ([]string, error) {
	entries, err := fs.ReadDir(schemasFS, "schemas")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// WriteSchemaDigest materializes DIGEST.md alongside the deployed schemas and
// reports whether it wrote. Called from init and upgrade so the digest can never
// be staler than the schemas it summarizes — a stale cheat sheet is worse than
// none, because it is trusted.
//
// The write lives here rather than in core/internal/commands, where it used to,
// for a reason worth stating. It was the one deployment write outside the
// packages TestNoDirectWritePrimitives scans, so it stayed an unconditional
// os.WriteFile after the other nine were converted — and a no-op `parlay upgrade`
// rewrote DIGEST.md every time while reporting that nothing changed. Documenting
// an exception in the guard would have left the next such write equally free;
// putting the write beside its builder and renderer, in a package the guard
// already covers, removes the category.
func WriteSchemaDigest(schemasDir string) (bool, error) {
	d, err := BuildSchemaDigest()
	if err != nil {
		return false, err
	}
	return atomicfile.WriteIfChanged(
		filepath.Join(schemasDir, "DIGEST.md"),
		[]byte(RenderSchemaDigestMarkdown(d)),
	)
}

// PruneStaleSchemas removes .parlay/schemas/*.schema.md files that no longer
// correspond to an embedded schema, and returns how many it removed.
//
// This is the counterpart to PruneStaleModules, which existed while this did not.
// The asymmetry was invisible until a schema was actually retired: WriteSchemas
// only ever adds and overwrites, so deleting one from the embedded set was a
// no-op for every project already on disk. Retiring design-loop deleted
// design-loop-result and design-loop-conflicts here and left both deployed
// everywhere, describing a skill that no longer exists — and a stale schema is
// worse than a missing one, because agents are told to read it as authoritative.
//
// DIGEST.md is deliberately not a candidate: it is generated beside the schemas
// rather than being one, and WriteSchemaDigest owns it. Files that are not
// *.schema.md are left alone.
func PruneStaleSchemas(targetDir string) (int, error) {
	names, err := SchemaNames()
	if err != nil {
		return 0, err
	}
	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[n] = true
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		// Missing directory on a fresh project — nothing to prune.
		return 0, nil
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || wanted[name] || !endsWithSchemaSuffix(name) {
			continue
		}
		if err := os.Remove(filepath.Join(targetDir, name)); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// endsWithSchemaSuffix reports whether name is a schema file. Lives here rather
// than in the test file it started in: PruneStaleSchemas needs it, and a helper
// only the test binary can see compiles under `go test` while breaking
// `go build` — which is exactly how it was first noticed.
func endsWithSchemaSuffix(name string) bool {
	const suffix = ".schema.md"
	return strings.HasSuffix(name, suffix)
}
