// parlay-feature: studio-support/page-layout-field
// parlay-section: cross-cutting
// parlay-cross-cutting-id: layout-precheck-contract
// parlay-artifact: test
//
// Tests the LayoutPrecheck contract — closed-shape Verdict, deterministic
// output, ok-on-nil-layout, and one verdict per stable failure code.
// Hews to the testcases enumerated under the layout-precheck-contract-*
// suites in testcases.yaml.

package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// Suite: layout-precheck-contract-passing

func TestLayoutPrecheck_NilLayoutReturnsOk(t *testing.T) {
	page := &parser.Page{Name: "dashboard"}
	verdict := LayoutPrecheck(page, fixtureClarity17())
	if verdict.Code != "ok" {
		t.Fatalf("expected Code=ok for nil-layout page; got %+v", verdict)
	}
	if verdict.File != "" || verdict.NodePath != "" || verdict.Found != "" || verdict.Expected != "" || verdict.Fix != "" {
		t.Fatalf("expected ok-verdict to carry only Code; got %+v", verdict)
	}
}

func TestLayoutPrecheck_PassingPageReturnsOnlyOkCode(t *testing.T) {
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       1,
			Nodes: []parser.LayoutNode{
				{
					ID:        "root",
					Type:      "clarity.region",
					Direction: "vertical",
					Gap:       "spacing-lg",
					Children: []parser.LayoutNode{
						{ID: "header", Type: "clarity.heading"},
					},
				},
			},
		},
	}
	got := LayoutPrecheck(page, fixtureClarity17())
	want := Verdict{Code: "ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected ok-verdict with all-empty fields; got %+v", got)
	}
}

func TestLayoutPrecheck_DeterministicByteForByte(t *testing.T) {
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       1,
			Nodes: []parser.LayoutNode{
				{ID: "root", Type: "clarity.region", Gap: "spacing-lg"},
			},
		},
	}
	adapter := fixtureClarity17()
	a := LayoutPrecheck(page, adapter)
	b := LayoutPrecheck(page, adapter)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("LayoutPrecheck not deterministic: a=%+v b=%+v", a, b)
	}
}

// Suite: layout-precheck-contract-failures

func TestLayoutPrecheck_MalformedYAMLBlockYieldsMalformedLayoutBlockVerdict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.md")
	contents := `---
name: dashboard
---

## Layout

` + "```" + `yaml
componentVocabulary: clarity@17
schemaVersion: 1
nodes:
  - id: root
    type: clarity.region
   bad-indent: oops
` + "```" + `
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	verdict := ParsePagePrecheck(path, fixtureClarity17())
	if verdict.Code != VerdictMalformedLayoutBlock {
		t.Fatalf("expected Code=%s; got %+v", VerdictMalformedLayoutBlock, verdict)
	}
	if verdict.File != path {
		t.Fatalf("expected File=%q; got %q", path, verdict.File)
	}
	if verdict.Found == "" {
		t.Fatalf("expected Found to carry the verbatim parser error")
	}
	if !strings.Contains(verdict.Expected, "well-formed YAML") {
		t.Fatalf("expected Expected to mention well-formed YAML; got %q", verdict.Expected)
	}
	if verdict.Fix == "" {
		t.Fatalf("expected Fix to carry a next-step instruction")
	}
}

func TestLayoutPrecheck_VocabularyVersionMismatchVerdict(t *testing.T) {
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       1,
			Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.region"}},
		},
	}
	adapter := fixtureClarity17()
	adapter.ComponentVocabulary.Name = "clarity@16"

	verdict := LayoutPrecheck(page, adapter)
	if verdict.Code != VerdictVocabularyVersionMismatch {
		t.Fatalf("expected Code=%s; got %+v", VerdictVocabularyVersionMismatch, verdict)
	}
	if verdict.Found != "clarity@17" {
		t.Fatalf("expected Found=clarity@17; got %q", verdict.Found)
	}
	if verdict.Expected != "clarity@16" {
		t.Fatalf("expected Expected=clarity@16; got %q", verdict.Expected)
	}
	if !strings.Contains(verdict.Fix, "clarity@17") || !strings.Contains(verdict.Fix, "clarity@16") {
		t.Fatalf("expected Fix to name both remediation paths; got %q", verdict.Fix)
	}
}

func TestLayoutPrecheck_RawValueWhereTokenRequiredVerdict(t *testing.T) {
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       1,
			Nodes: []parser.LayoutNode{
				{ID: "root", Type: "clarity.region", Gap: "24px"},
			},
		},
	}
	verdict := LayoutPrecheck(page, fixtureClarity17())
	if verdict.Code != VerdictRawValueWhereTokenRequired {
		t.Fatalf("expected Code=%s; got %+v", VerdictRawValueWhereTokenRequired, verdict)
	}
	if verdict.NodePath == "" {
		t.Fatalf("expected non-empty NodePath; got %q", verdict.NodePath)
	}
	if verdict.Found != "24px" {
		t.Fatalf("expected Found=24px; got %q", verdict.Found)
	}
	if !strings.Contains(verdict.Expected, "spacing-lg") {
		t.Fatalf("expected Expected to list valid spacing tokens; got %q", verdict.Expected)
	}
	if !strings.Contains(verdict.Fix, "24px") {
		t.Fatalf("expected Fix to mention substitution; got %q", verdict.Fix)
	}
}

func TestLayoutPrecheck_UnknownComponentTypeVerdict(t *testing.T) {
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       1,
			Nodes: []parser.LayoutNode{
				{ID: "root", Type: "clarity.kanban"},
			},
		},
	}
	verdict := LayoutPrecheck(page, fixtureClarity17())
	if verdict.Code != VerdictUnknownComponentType {
		t.Fatalf("expected Code=%s; got %+v", VerdictUnknownComponentType, verdict)
	}
	if verdict.Found != "clarity.kanban" {
		t.Fatalf("expected Found=clarity.kanban; got %q", verdict.Found)
	}
	if !strings.Contains(verdict.Expected, "clarity@17") {
		t.Fatalf("expected Expected to name the vocabulary's known type list; got %q", verdict.Expected)
	}
	if !strings.Contains(verdict.Fix, "clarity@17") || !strings.Contains(verdict.Fix, "clarity.kanban") {
		t.Fatalf("expected Fix to name both remediation paths; got %q", verdict.Fix)
	}
}

func TestLayoutPrecheck_MissingSchemaVersionVerdict(t *testing.T) {
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       0, // explicitly missing
			Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.region"}},
		},
	}
	verdict := LayoutPrecheck(page, fixtureClarity17())
	if verdict.Code != VerdictMissingSchemaVersion {
		t.Fatalf("expected Code=%s; got %+v", VerdictMissingSchemaVersion, verdict)
	}
}

func TestLayoutPrecheck_WiringInLayoutVerdict(t *testing.T) {
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       1,
			Nodes: []parser.LayoutNode{
				{
					ID:   "main-table",
					Type: "clarity.datagrid",
					Properties: map[string]interface{}{
						"dataSource": "orders",
					},
				},
			},
		},
	}
	verdict := LayoutPrecheck(page, fixtureClarity17())
	if verdict.Code != VerdictWiringInLayout {
		t.Fatalf("expected Code=%s; got %+v", VerdictWiringInLayout, verdict)
	}
	if verdict.Found != "dataSource" {
		t.Fatalf("expected Found=dataSource; got %q", verdict.Found)
	}
	if !strings.Contains(verdict.Fix, "codegen") {
		t.Fatalf("expected Fix to mention codegen pass; got %q", verdict.Fix)
	}
}

// Suite: layout-precheck-aggregation-across-pages

func TestLayoutPrecheck_WalkOfMultiplePagesYieldsOneVerdictPerPage(t *testing.T) {
	pages := []*parser.Page{
		// (a) ok
		{
			Name: "a",
			Layout: &parser.Layout{
				ComponentVocabulary: "clarity@17",
				SchemaVersion:       1,
				Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.region"}},
			},
		},
		// (b) unknown-component-type
		{
			Name: "b",
			Layout: &parser.Layout{
				ComponentVocabulary: "clarity@17",
				SchemaVersion:       1,
				Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.kanban"}},
			},
		},
		// (c) ok (no layout)
		{
			Name: "c",
		},
	}
	adapter := fixtureClarity17()
	verdicts := make([]Verdict, 0, len(pages))
	for _, p := range pages {
		verdicts = append(verdicts, LayoutPrecheck(p, adapter))
	}
	if len(verdicts) != len(pages) {
		t.Fatalf("expected one verdict per page; got %d", len(verdicts))
	}
	codes := []string{}
	for _, v := range verdicts {
		codes = append(codes, v.Code)
	}
	// Mix of ok and failure.
	hasOK := false
	hasFail := false
	for _, c := range codes {
		if c == "ok" {
			hasOK = true
		} else {
			hasFail = true
		}
	}
	if !hasOK || !hasFail {
		t.Fatalf("expected mix of ok and failure across pages; got codes=%v", codes)
	}
}

func TestLayoutPrecheck_PurityNoSideEffects(t *testing.T) {
	// Sanity: invoking LayoutPrecheck does not mutate its inputs and
	// does not write to disk. We assert by snapshot/diff on the inputs.
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       1,
			Nodes: []parser.LayoutNode{
				{ID: "root", Type: "clarity.region"},
			},
		},
	}
	adapter := fixtureClarity17()

	pageBefore := *page
	layoutBefore := *page.Layout
	nodesBefore := make([]parser.LayoutNode, len(page.Layout.Nodes))
	copy(nodesBefore, page.Layout.Nodes)
	adapterBefore := *adapter

	_ = LayoutPrecheck(page, adapter)

	if !reflect.DeepEqual(*page, pageBefore) {
		t.Fatalf("page mutated by LayoutPrecheck")
	}
	if !reflect.DeepEqual(*page.Layout, layoutBefore) {
		t.Fatalf("layout mutated by LayoutPrecheck")
	}
	if !reflect.DeepEqual(page.Layout.Nodes, nodesBefore) {
		t.Fatalf("nodes mutated by LayoutPrecheck")
	}
	if !reflect.DeepEqual(*adapter, adapterBefore) {
		t.Fatalf("adapter mutated by LayoutPrecheck")
	}
}
