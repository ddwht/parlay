package embedded

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed schemas/*.schema.md
var schemasFS embed.FS

// WriteSchemas copies all embedded schemas to the target directory.
func WriteSchemas(targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	entries, err := fs.ReadDir(schemasFS, "schemas")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := schemasFS.ReadFile(filepath.Join("schemas", entry.Name()))
		if err != nil {
			return err
		}
		dst := filepath.Join(targetDir, entry.Name())
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return err
		}
	}
	return nil
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
