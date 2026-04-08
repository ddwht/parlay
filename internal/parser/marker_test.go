package parser

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestParseMarker_GoStyle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upgrade_prompt.go")
	content := `// parlay-feature: upgrade-plan-creation
// parlay-component: upgrade-prompt
// Generated from .parlay/build/upgrade-plan-creation/buildfile.yaml — do not edit by hand

package main

func UpgradePrompt() {}
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker == nil {
		t.Fatal("expected marker, got nil")
	}
	if marker.Feature != "upgrade-plan-creation" {
		t.Errorf("Feature = %q, want upgrade-plan-creation", marker.Feature)
	}
	if marker.Component != "upgrade-prompt" {
		t.Errorf("Component = %q, want upgrade-prompt", marker.Component)
	}
	if marker.Path != path {
		t.Errorf("Path = %q, want %q", marker.Path, path)
	}
}

func TestParseMarker_HashStyle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `# parlay-feature: my-feature
# parlay-component: app-config
# Generated — do not edit

key: value
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker == nil || marker.Component != "app-config" {
		t.Errorf("expected app-config marker, got %+v", marker)
	}
}

func TestParseMarker_NoMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "user.go")
	content := `package main

// Just a regular comment, no parlay marker here.
func UserCode() {}
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker != nil {
		t.Errorf("expected nil marker, got %+v", marker)
	}
}

func TestParseMarker_MissingComponent(t *testing.T) {
	// A marker that only declares feature is incomplete and should not
	// be returned — component is the load-bearing field.
	dir := t.TempDir()
	path := filepath.Join(dir, "incomplete.go")
	content := `// parlay-feature: my-feature
// (no component field)
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker != nil {
		t.Errorf("expected nil marker for incomplete metadata, got %+v", marker)
	}
}

func TestParseMarker_TooDeep(t *testing.T) {
	// Marker fields buried beyond the scan limit should be ignored —
	// the marker must be at the top of the file.
	dir := t.TempDir()
	path := filepath.Join(dir, "deep.go")
	var content string
	for i := 0; i < 25; i++ {
		content += "// padding\n"
	}
	content += "// parlay-component: too-deep\n"
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker != nil {
		t.Errorf("expected nil marker for marker buried below scan limit, got %+v", marker)
	}
}

func TestScanGenerated_FindsMarkedFiles(t *testing.T) {
	dir := t.TempDir()

	// File 1: marked
	os.WriteFile(filepath.Join(dir, "comp_a.go"),
		[]byte("// parlay-feature: f\n// parlay-component: comp-a\npackage main\n"), 0644)
	// File 2: marked, in subdirectory
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "comp_b.go"),
		[]byte("// parlay-feature: f\n// parlay-component: comp-b\npackage sub\n"), 0644)
	// File 3: unmarked (user-owned)
	os.WriteFile(filepath.Join(dir, "user.go"),
		[]byte("package main\n// no marker\n"), 0644)
	// File 4: marked but inside a skipped dir
	os.MkdirAll(filepath.Join(dir, "node_modules"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "skipped.go"),
		[]byte("// parlay-component: skipped\n"), 0644)
	// File 5: marked but inside a hidden dir
	os.MkdirAll(filepath.Join(dir, ".cache"), 0755)
	os.WriteFile(filepath.Join(dir, ".cache", "hidden.go"),
		[]byte("// parlay-component: hidden\n"), 0644)

	markers, err := ScanGenerated(dir)
	if err != nil {
		t.Fatal(err)
	}

	var components []string
	for _, m := range markers {
		components = append(components, m.Component)
	}
	sort.Strings(components)

	expected := []string{"comp-a", "comp-b"}
	if len(components) != len(expected) {
		t.Fatalf("Components = %v, want %v", components, expected)
	}
	for i := range expected {
		if components[i] != expected[i] {
			t.Errorf("Components[%d] = %q, want %q", i, components[i], expected[i])
		}
	}
}

func TestScanGenerated_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	markers, err := ScanGenerated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 0 {
		t.Errorf("expected no markers, got %v", markers)
	}
}
