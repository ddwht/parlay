// parlay-feature: parlay-tool/multi-adapter
// parlay-component: bundled-adapter-set-presets
package embedded

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed presets/*.adapter-set.yaml
var presetsFS embed.FS

// ReadPreset returns the content of a bundled adapter-set preset by name.
//
// The name is the slug (filename minus the .adapter-set.yaml suffix) — e.g.,
// "react-nest-prisma" or "react-antd-only".
func ReadPreset(name string) ([]byte, error) {
	filename := fmt.Sprintf("presets/%s.adapter-set.yaml", name)
	content, err := presetsFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read preset %q: %w", name, err)
	}
	return content, nil
}

// PresetNames returns the slugs of every bundled adapter-set preset, sorted
// alphabetically.
func PresetNames() ([]string, error) {
	entries, err := fs.ReadDir(presetsFS, "presets")
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".adapter-set.yaml")
		if name == e.Name() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
