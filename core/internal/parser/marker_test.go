package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func TestParseMarker_SectionTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.go")
	content := `// parlay-feature: task-list
// parlay-section: models
// Generated from buildfile — do not edit by hand

package main

type Task struct {}
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker == nil {
		t.Fatal("expected marker, got nil")
	}
	if marker.Section != "models" {
		t.Errorf("Section = %q, want models", marker.Section)
	}
	if marker.Component != "" {
		t.Errorf("Component = %q, want empty (section-derived file)", marker.Component)
	}
}

func TestParseMarker_ArtifactTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "add_test.go")
	content := `// parlay-feature: task-list
// parlay-component: add-task-command
// parlay-artifact: test
// Generated from testcases.yaml — do not edit by hand

package main

func TestAddTask(t *testing.T) {}
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker == nil {
		t.Fatal("expected marker, got nil")
	}
	if marker.Component != "add-task-command" {
		t.Errorf("Component = %q, want add-task-command", marker.Component)
	}
	if marker.Artifact != "test" {
		t.Errorf("Artifact = %q, want test", marker.Artifact)
	}
}

func TestParseMarker_SectionOnly_NoComponent(t *testing.T) {
	// A file with only parlay-section: (no component) is valid.
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	content := `// parlay-section: routes
package main
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker == nil {
		t.Fatal("expected marker for section-only file, got nil")
	}
	if marker.Section != "routes" {
		t.Errorf("Section = %q, want routes", marker.Section)
	}
}

func TestParseMarker_ProjectScope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	content := `// parlay-scope: project
// parlay-section: routes
// Generated — do not edit by hand

package main
`
	os.WriteFile(path, []byte(content), 0644)

	marker, err := ParseMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker == nil {
		t.Fatal("expected marker, got nil")
	}
	if marker.Scope != "project" {
		t.Errorf("Scope = %q, want project", marker.Scope)
	}
	if marker.Section != "routes" {
		t.Errorf("Section = %q, want routes", marker.Section)
	}
	if marker.Feature != "" {
		t.Errorf("Feature = %q, want empty (project-scoped)", marker.Feature)
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

// TestParseMarker_BlockCommentForms covers markers carried in HTML and CSS
// block comments. Template-based adapters (Angular, Vue, Svelte) have no
// `//` line-comment form, so before this was supported every generated
// template was invisible to ScanGenerated — never hashed, so a hand-edit to
// one could not be detected and was silently lost on regeneration.
func TestParseMarker_BlockCommentForms(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		wantFeat  string
		wantComp  string
		wantScope string
	}{
		{
			name:     "angular template, inline html comments",
			content:  "<!-- parlay-feature: expense-list -->\n<!-- parlay-component: my-expense-reports-datagrid -->\n<div>x</div>\n",
			wantFeat: "expense-list",
			wantComp: "my-expense-reports-datagrid",
		},
		{
			name:     "css block comment",
			content:  "/* parlay-feature: expense-list */\n/* parlay-component: draft-row-actions */\n.a { color: red; }\n",
			wantFeat: "expense-list",
			wantComp: "draft-row-actions",
		},
		{
			name:      "html block spanning lines, bare fields inside",
			content:   "<!--\nparlay-scope: project\nparlay-section: routes\n-->\n\n<router-outlet/>\n",
			wantScope: "project",
		},
		{
			name:     "extra whitespace inside the delimiters",
			content:  "<!--   parlay-feature: f   -->\n<!--\tparlay-component: c\t-->\n",
			wantFeat: "f",
			wantComp: "c",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseMarkerFromReader(strings.NewReader(tc.content), "x")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if m == nil {
				t.Fatal("no marker parsed — the file would be invisible to scan-generated and never hashed")
			}
			if m.Feature != tc.wantFeat {
				t.Errorf("Feature = %q, want %q", m.Feature, tc.wantFeat)
			}
			if m.Component != tc.wantComp {
				t.Errorf("Component = %q, want %q", m.Component, tc.wantComp)
			}
			if tc.wantScope != "" && m.Scope != tc.wantScope {
				t.Errorf("Scope = %q, want %q", m.Scope, tc.wantScope)
			}
		})
	}
}

// TestParseMarker_BlockCommentDoesNotOverreach guards against treating an
// ordinary template comment as a marker.
func TestParseMarker_BlockCommentDoesNotOverreach(t *testing.T) {
	m, err := parseMarkerFromReader(strings.NewReader("<!-- just an ordinary comment -->\n<div/>\n"), "x")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m != nil {
		t.Errorf("parsed a marker from a non-marker comment: %+v", m)
	}
}

// A Tier-2 merged file carries a primary feature plus one or more
// parlay-extends: lines. Nothing parsed those and the validity gate did not
// count them, so such a file read as user-owned: never hashed, never
// verifiable, silently clobberable on the next generation.
func TestExtendsAloneIdentifiesAGeneratedFile(t *testing.T) {
	const src = `// parlay-feature: studio-support
// parlay-extends: studio-support/studio-cli-hooks/hook-dispatch-trio-create-artifacts
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands

package commands
`
	m, err := parseMarkerFromReader(strings.NewReader(src), "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("a file claimed by parlay-extends: is generated, not user-owned")
	}
	if len(m.Extends) != 2 {
		t.Fatalf("Extends = %v, want both lines — matchField returns one value per line, so these must append", m.Extends)
	}
	// Stored verbatim: the live shapes differ in arity, and a parser that
	// split them into feature+component would drop what it could not fit.
	if m.Extends[0] != "studio-support/studio-cli-hooks/hook-dispatch-trio-create-artifacts" {
		t.Errorf("Extends[0] = %q, want the value as written", m.Extends[0])
	}
}

func TestCrossCuttingAloneIdentifiesAGeneratedFile(t *testing.T) {
	const src = `// parlay-cross-cutting: approvals/review-queue/audit-trail-on-status-changes

package audit
`
	m, err := parseMarkerFromReader(strings.NewReader(src), "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("a file claimed by parlay-cross-cutting: is generated")
	}
	if len(m.CrossCutting) != 1 {
		t.Fatalf("CrossCutting = %v", m.CrossCutting)
	}
}

// parlay-feature: on its own must NOT claim a file. It names which feature a
// file relates to, not that parlay wrote it — counting it would sweep in
// hand-written files that merely mention their feature, and parlay would
// then consider itself free to overwrite them.
func TestFeatureAloneDoesNotClaimAFile(t *testing.T) {
	const src = `// parlay-feature: expenses-list

package handwritten
`
	m, err := parseMarkerFromReader(strings.NewReader(src), "x.go")
	if err != nil {
		t.Fatal(err)
	}
	if m != nil {
		t.Errorf("parlay-feature: alone must not mark a file as generated, got %+v", m)
	}
}
