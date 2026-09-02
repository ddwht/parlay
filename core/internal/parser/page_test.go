// parlay-feature: parlay-tool/page-layout-field
// parlay-section: cross-cutting
// parlay-cross-cutting-id: page-artifact-loader-with-optional-layout
// parlay-artifact: test
//
// Tests for parser.ParsePageFile. Hews to the Verify-bullets in the
// page-layout-field intents and to the testcases enumerated under the
// page-artifact-loader-* suites in testcases.yaml.

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePageFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write page %s: %v", path, err)
	}
	return path
}

// Suite: page-artifact-loader-without-layout-block

func TestParsePageFile_WithoutLayoutSectionReturnsNilLayout(t *testing.T) {
	dir := t.TempDir()
	path := writePageFile(t, dir, "page.md", `---
name: dashboard
description: Top-level dashboard
owner: studio-team
status: in-design
---

## Overview

Some prose.

## Regions

- name: main
`)
	page, err := ParsePageFile(path)
	if err != nil {
		t.Fatalf("expected clean parse; got %v", err)
	}
	if page.Layout != nil {
		t.Fatalf("expected Layout == nil for page without ## Layout section; got %+v", page.Layout)
	}
	if page.Name != "dashboard" {
		t.Fatalf("expected page.Name=dashboard; got %q", page.Name)
	}
}

// Suite: page-artifact-loader-with-layout-block

func TestParsePageFile_WithWellFormedLayoutSectionDecodes(t *testing.T) {
	dir := t.TempDir()
	path := writePageFile(t, dir, "page.md", `---
name: dashboard
---

## Layout

`+"```"+`yaml
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: root
    type: clarity.region
    direction: vertical
    gap: spacing-lg
    children:
      - id: header
        type: clarity.heading
      - id: main-table
        type: clarity.datagrid
`+"```"+`
`)
	page, err := ParsePageFile(path)
	if err != nil {
		t.Fatalf("expected clean parse; got %v", err)
	}
	if page.Layout == nil {
		t.Fatalf("expected Layout to be populated")
	}
	if page.Layout.ComponentVocabulary != "clarity@17" {
		t.Fatalf("expected componentVocabulary=clarity@17; got %q", page.Layout.ComponentVocabulary)
	}
	if page.Layout.SchemaVersion != 1 {
		t.Fatalf("expected schema_version=1; got %d", page.Layout.SchemaVersion)
	}
	if len(page.Layout.Nodes) == 0 {
		t.Fatalf("expected at least one node")
	}
	root := page.Layout.Nodes[0]
	if root.ID != "root" || root.Type != "clarity.region" {
		t.Fatalf("expected root node id=root type=clarity.region; got id=%q type=%q", root.ID, root.Type)
	}
	if root.Direction != "vertical" {
		t.Fatalf("expected direction=vertical; got %q", root.Direction)
	}
	if root.Gap != "spacing-lg" {
		t.Fatalf("expected gap=spacing-lg; got %q", root.Gap)
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children; got %d", len(root.Children))
	}
}

func TestParsePageFile_LayoutSectionPositionAmongSiblingsIsIrrelevant(t *testing.T) {
	dir := t.TempDir()
	layoutBody := `componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: root
    type: clarity.region
`
	pageA := writePageFile(t, dir, "a.md", `---
name: dashboard
---

## Layout

`+"```"+`yaml
`+layoutBody+"```"+`

## Notes

Trailing prose.
`)
	pageB := writePageFile(t, dir, "b.md", `---
name: dashboard
---

## Notes

Leading prose.

## Layout

`+"```"+`yaml
`+layoutBody+"```"+`
`)
	a, err := ParsePageFile(pageA)
	if err != nil {
		t.Fatalf("a parse: %v", err)
	}
	b, err := ParsePageFile(pageB)
	if err != nil {
		t.Fatalf("b parse: %v", err)
	}
	if a.Layout == nil || b.Layout == nil {
		t.Fatalf("both pages should expose a Layout; got a=%v b=%v", a.Layout, b.Layout)
	}
	if a.Layout.ComponentVocabulary != b.Layout.ComponentVocabulary || a.Layout.SchemaVersion != b.Layout.SchemaVersion {
		t.Fatalf("layouts diverged across orderings: a=%+v b=%+v", a.Layout, b.Layout)
	}
	if len(a.Layout.Nodes) != len(b.Layout.Nodes) || a.Layout.Nodes[0].ID != b.Layout.Nodes[0].ID {
		t.Fatalf("layout nodes diverged across orderings")
	}
}

func TestParsePageFile_LayoutMissingComponentVocabularyFails(t *testing.T) {
	dir := t.TempDir()
	path := writePageFile(t, dir, "page.md", `---
name: dashboard
---

## Layout

`+"```"+`yaml
schema_version: 1
nodes: []
`+"```"+`
`)
	_, err := ParsePageFile(path)
	if err == nil {
		t.Fatalf("expected error for missing componentVocabulary")
	}
	if !strings.Contains(err.Error(), "missing required field 'componentVocabulary'") {
		t.Fatalf("expected error to name missing componentVocabulary; got %v", err)
	}
}

func TestParsePageFile_LayoutMissingSchemaVersionFails(t *testing.T) {
	dir := t.TempDir()
	path := writePageFile(t, dir, "page.md", `---
name: dashboard
---

## Layout

`+"```"+`yaml
componentVocabulary: clarity@17
nodes: []
`+"```"+`
`)
	_, err := ParsePageFile(path)
	if err == nil {
		t.Fatalf("expected error for missing schema_version")
	}
	if !strings.Contains(err.Error(), "missing required field 'schema_version'") {
		t.Fatalf("expected error to name missing schema_version; got %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("expected error to include page path; got %v", err)
	}
}

func TestParsePageFile_LayoutWithWiringFieldFails(t *testing.T) {
	dir := t.TempDir()
	path := writePageFile(t, dir, "page.md", `---
name: dashboard
---

## Layout

`+"```"+`yaml
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: main-table
    type: clarity.datagrid
    dataSource: orders
`+"```"+`
`)
	_, err := ParsePageFile(path)
	if err == nil {
		t.Fatalf("expected error for wiring field on layout node")
	}
	msg := err.Error()
	if !strings.Contains(msg, "wiring field 'dataSource' on node 'main-table'") {
		t.Fatalf("expected error to name wiring field, node id, and page path; got %v", err)
	}
	if !strings.Contains(msg, path) {
		t.Fatalf("expected error to include page path; got %v", err)
	}
}

func TestParsePageFile_LayoutWithRawSpacingValueFails(t *testing.T) {
	dir := t.TempDir()
	path := writePageFile(t, dir, "page.md", `---
name: dashboard
---

## Layout

`+"```"+`yaml
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: header-row
    type: clarity.region
    gap: 24px
`+"```"+`
`)
	_, err := ParsePageFile(path)
	if err == nil {
		t.Fatalf("expected error for raw spacing value")
	}
	msg := err.Error()
	if !strings.Contains(msg, "raw value '24px'") {
		t.Fatalf("expected error to quote raw value; got %v", err)
	}
	if !strings.Contains(msg, "field 'gap'") {
		t.Fatalf("expected error to name field; got %v", err)
	}
	if !strings.Contains(msg, "node 'header-row'") {
		t.Fatalf("expected error to name node id; got %v", err)
	}
}

func TestParsePageFile_LayoutWithRawIntegerPaddingFails(t *testing.T) {
	dir := t.TempDir()
	path := writePageFile(t, dir, "page.md", `---
name: dashboard
---

## Layout

`+"```"+`yaml
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: header-row
    type: clarity.region
    padding: "16"
`+"```"+`
`)
	_, err := ParsePageFile(path)
	if err == nil {
		t.Fatalf("expected error for raw padding value")
	}
	if !strings.Contains(err.Error(), "field 'padding'") {
		t.Fatalf("expected error to name padding; got %v", err)
	}
}

func TestParsePageFile_RoundTripPreservesNodeIDs(t *testing.T) {
	dir := t.TempDir()
	original := `---
name: dashboard
---

## Layout

` + "```" + `yaml
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: root
    type: clarity.region
    children:
      - id: header
        type: clarity.heading
      - id: main-table
        type: clarity.datagrid
        children:
          - id: col-a
            type: clarity.datagrid-column
          - id: col-b
            type: clarity.datagrid-column
` + "```" + `
`
	path := writePageFile(t, dir, "page.md", original)
	a, err := ParsePageFile(path)
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	// Re-parse without modifying the file: every id must survive.
	b, err := ParsePageFile(path)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	idsA := collectIDs(a.Layout.Nodes)
	idsB := collectIDs(b.Layout.Nodes)
	if len(idsA) != len(idsB) {
		t.Fatalf("id count drifted across parses: a=%d b=%d", len(idsA), len(idsB))
	}
	for i := range idsA {
		if idsA[i] != idsB[i] {
			t.Fatalf("id at position %d drifted: a=%q b=%q", i, idsA[i], idsB[i])
		}
	}
	want := []string{"root", "header", "main-table", "col-a", "col-b"}
	if len(idsA) != len(want) {
		t.Fatalf("expected %d ids; got %d (%v)", len(want), len(idsA), idsA)
	}
	for i, w := range want {
		if idsA[i] != w {
			t.Fatalf("id[%d]: expected %q got %q", i, w, idsA[i])
		}
	}
}

func collectIDs(nodes []LayoutNode) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.ID)
		out = append(out, collectIDs(n.Children)...)
	}
	return out
}

// A `## Layout` section that is not usable must be REFUSED, not reclassified.
//
// Before the shared locator, a heading with no fence yielded an empty body
// that decodeLayout then rejected for missing required fields — accidentally,
// but it rejected. Answering "no layout section" instead would turn a
// malformed layout into an ordinary page region named "Layout": a page that
// means something other than its author wrote.
//
// The unterminated case was never really covered: leaving it to the YAML
// decoder catches it only when the collected text also happens to be invalid
// YAML, which is exactly when the mistake is hardest to see. Both are tested
// through ParsePageFile, because that is what every caller uses.
func TestPageRefusesAnUnusableLayoutSection(t *testing.T) {
	fence := "```"
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "a Layout heading with no fenced block",
			content: "# Board\n\n## Layout\n\nThe layout is still being designed.\n",
			wantErr: "no fenced YAML block",
		},
		{
			name: "a fence that is never closed, whose contents are valid YAML",
			content: "# Board\n\n## Layout\n\n" + fence + "yaml\n" +
				"componentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: stack\n",
			wantErr: "never closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "board.page.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			page, err := ParsePageFile(path)
			if err == nil {
				t.Fatalf("the page was accepted; layout = %+v, regions = %+v", page.Layout, page.Regions)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to name %q", err, tt.wantErr)
			}
		})
	}
}

// A page with frontmatter: the thread's line numbers and the anchor's span are
// the FILE's, not the body's. Everything a reviewer opens the file to find is
// addressed by absolute line.
func TestPageLayoutAnnotationUsesAbsoluteLineNumbers(t *testing.T) {
	fence := "```"
	content := "---\nname: Board\nowner: design\n---\n\n# Board\n\n## Layout\n\n" + fence + "yaml\n" +
		"componentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: stack\n" +
		"    # @dwht: should be horizontal on wide viewports\n" + fence + "\n"

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	wantAnnotation := 0
	for i, l := range lines {
		if strings.Contains(l, "@dwht") {
			wantAnnotation = i + 1
		}
	}

	scan := ScanAnnotations("board.page.md", []byte(content))
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v", scan.Findings)
	}
	if len(scan.Threads) != 1 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	thread := scan.Threads[0]
	if thread.Line != wantAnnotation {
		t.Errorf("thread line = %d, want the file's line %d", thread.Line, wantAnnotation)
	}
	// The anchored unit is the `type: stack` pair on the line above it.
	if got, want := thread.Anchor.Span, [2]int{wantAnnotation - 1, wantAnnotation - 1}; got != want {
		t.Errorf("span = %v, want %v", got, want)
	}
	if got := strings.TrimSpace(thread.Anchor.Text); got != "type: stack" {
		t.Errorf("anchor text = %q", got)
	}
}
