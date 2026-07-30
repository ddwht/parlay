package embedded

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"

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
