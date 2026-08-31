package agent

// parlay-feature: parlay-tool/adapter-vocabulary-extension
// parlay-feature: parlay-tool/page-layout-field
// parlay-component: adapter-validator-deep-vocabulary-and-tokens
// parlay-cross-cutting-id: layout-precheck-contract
// parlay-artifact: test
//
// Tests the unified layout-validation walk (Phase 6.3/6.4): the deep
// adapter validation surface for componentVocabulary and tokens (the
// vocabulary lookup passes — component, variant, property,
// allowed-children — the token lookup passes — raw-value-where-token-
// required, unknown-token — the version-mismatch fast-fail, the
// cross-adapter parity check, and the codegen emit-form translation), plus
// the closed-shape LayoutPrecheck contract both entry points share a
// single walk with. Formerly split across validate_layout_test.go
// (ValidateLayout, agent.LayoutNode) and precheck_test.go (LayoutPrecheck,
// parser.LayoutNode) — consolidated onto parser.LayoutNode / ValidateLayoutDeep
// / LayoutPrecheck now that both wrap the same walk.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// fixtureClarity17 returns a representative deepAdapter mirroring the
// adapter-with-clarity-vocabulary fixture from the buildfile.
func fixtureClarity17() *deepAdapter {
	return &deepAdapter{
		ComponentVocabulary: &deepComponentVocabulary{
			Name: "clarity@17",
			Components: []deepVocabularyComponent{
				{
					Type:            "clarity.region",
					Category:        "container",
					AllowedChildren: []string{"clarity.heading", "clarity.button", "clarity.datagrid"},
				},
				{
					Type:     "clarity.heading",
					Category: "leaf",
					Variants: []string{"page", "section", "subsection"},
					Properties: []deepVocabularyProperty{
						{Name: "text", Type: "string", Required: true},
					},
				},
				{
					Type:     "clarity.button",
					Category: "leaf",
					Variants: []string{"primary", "secondary", "tertiary", "danger"},
					Properties: []deepVocabularyProperty{
						{Name: "label", Type: "string", Required: true},
					},
				},
				{
					Type:            "clarity.datagrid",
					Category:        "container",
					AllowedChildren: []string{"clarity.datagrid-column"},
					Variants:        []string{"compact", "comfortable"},
					Properties: []deepVocabularyProperty{
						{Name: "density", Type: "enum", EnumValues: []string{"compact", "comfortable"}, Required: false},
					},
				},
				{
					Type:     "clarity.datagrid-column",
					Category: "data-shape",
					Properties: []deepVocabularyProperty{
						{Name: "headerLabel", Type: "string", Required: true},
					},
				},
			},
		},
		Tokens: &deepAdapterTokens{
			Modes: []string{"light", "dark"},
			Spacing: []deepSpacingToken{
				{Name: "spacing-xs", Order: 1, EmitForm: "var(--spacing-xs)"},
				{Name: "spacing-sm", Order: 2, EmitForm: "var(--spacing-sm)"},
				{Name: "spacing-md", Order: 3, EmitForm: "var(--spacing-md)"},
				{Name: "spacing-lg", Order: 4, EmitForm: "var(--spacing-lg)"},
				{Name: "spacing-xl", Order: 5, EmitForm: "var(--spacing-xl)"},
			},
			Color: []deepColorToken{
				{Name: "color-surface", Tone: "neutral", EmitForms: []string{"light:var(--color-surface-light)", "dark:var(--color-surface-dark)"}},
			},
			Typography: []deepTypographyToken{
				{Name: "heading-page", UseSite: "heading-page", EmitForm: "var(--type-heading-page)"},
			},
		},
	}
}

func mustContainAll(t *testing.T, message string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(message, n) {
			t.Errorf("expected error message to contain %q; got %q", n, message)
		}
	}
}

// errSurface returns a single string carrying the structured error's Code,
// Message, Context, and Fix — the union surface a caller's "expected error
// contains X" testcase looks at, since the Code is the agent-consumable
// identifier and the Message is the human-readable rendering.
func errSurface(e ValidationError) string {
	return e.Code + " | " + e.Message + " | " + e.Context + " | " + e.Fix
}

// Suite: adapter validator enforces vocabulary and token rules in deep validation

func TestValidateLayoutDeep_WellFormedClean(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{
				ID:   "root",
				Type: "clarity.region",
				Children: []parser.LayoutNode{
					{ID: "hd", Type: "clarity.heading", Properties: map[string]interface{}{"variant": "page", "text": "Hello"}},
					{ID: "btn", Type: "clarity.button", Properties: map[string]interface{}{"variant": "primary", "label": "Go"}},
					{
						ID: "grid", Type: "clarity.datagrid", Properties: map[string]interface{}{"variant": "compact"},
						Children: []parser.LayoutNode{
							{ID: "col", Type: "clarity.datagrid-column", Properties: map[string]interface{}{"headerLabel": "Name"}},
						},
					},
				},
			},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 0 {
		t.Fatalf("expected 0 validation errors; got %d: %+v", len(errs), errs)
	}
}

func TestValidateLayoutDeep_MissingSchemaVersionFailsFast(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.foobar"}},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 || errs[0].Code != "missing-schema-version" {
		t.Fatalf("expected exactly one missing-schema-version error; got %+v", errs)
	}
}

func TestValidateLayoutDeep_UnsupportedSchemaVersionFailsFast(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       2,
		Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.foobar"}},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 || errs[0].Code != "layout-schema-version-unsupported" {
		t.Fatalf("expected exactly one layout-schema-version-unsupported error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "2", "1")
}

func TestValidateLayoutDeep_VersionMismatchFailsFast(t *testing.T) {
	adapter := fixtureClarity17()
	adapter.ComponentVocabulary.Name = "clarity@18"
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.foobar"}}, // would also be unknown-component-type, but vocabulary-version-mismatch must fail fast
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 validation error (vocabulary-version-mismatch fails fast); got %d: %+v", len(errs), errs)
	}
	if errs[0].Code != "vocabulary-version-mismatch" {
		t.Fatalf("expected code vocabulary-version-mismatch; got %q", errs[0].Code)
	}
	mustContainAll(t, errSurface(errs[0]), "vocabulary-version-mismatch", "clarity@17", "clarity@18")
}

func TestValidateLayoutDeep_UnknownComponentType(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.foobar"}},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 || errs[0].Code != "unknown-component-type" {
		t.Fatalf("expected one unknown-component-type error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "unknown-component-type", "clarity.foobar")
}

func TestValidateLayoutDeep_UnknownVariantListsAllowedAlternatives(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{ID: "root", Type: "clarity.button", Properties: map[string]interface{}{"variant": "mega-button", "label": "Boom"}},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 || errs[0].Code != "unknown-variant" {
		t.Fatalf("expected one unknown-variant error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "unknown-variant", "mega-button", "primary, secondary, tertiary, danger")
}

func TestValidateLayoutDeep_UnknownPropertyNamesOffendingKey(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{ID: "root", Type: "clarity.button", Properties: map[string]interface{}{"label": "Go", "tooltip": "extra"}},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	var up *ValidationError
	for i := range errs {
		if errs[i].Code == "unknown-property" {
			up = &errs[i]
			break
		}
	}
	if up == nil {
		t.Fatalf("expected an unknown-property error; got %+v", errs)
	}
	mustContainAll(t, errSurface(*up), "unknown-property", "tooltip", "clarity.button")
}

func TestValidateLayoutDeep_DisallowedChildNamesParentAllowedSetAndOffender(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{
				ID: "grid", Type: "clarity.datagrid", Properties: map[string]interface{}{"variant": "compact"},
				Children: []parser.LayoutNode{
					{ID: "btn", Type: "clarity.button", Properties: map[string]interface{}{"label": "X"}},
				},
			},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	var dc *ValidationError
	for i := range errs {
		if errs[i].Code == "disallowed-child" {
			dc = &errs[i]
			break
		}
	}
	if dc == nil {
		t.Fatalf("expected a disallowed-child error; got %+v", errs)
	}
	mustContainAll(t, errSurface(*dc), "disallowed-child", "clarity.datagrid-column", "clarity.button")
}

func TestValidateLayoutDeep_RawValueWhereTokenRequiredListsAvailableTokens(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{ID: "root", Type: "clarity.region", Gap: "24px"},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 || errs[0].Code != "raw-value-where-token-required" {
		t.Fatalf("expected one raw-value-where-token-required error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "raw-value-where-token-required", "spacing-xs, spacing-sm, spacing-md, spacing-lg, spacing-xl")
}

func TestValidateLayoutDeep_UnknownTokenLists(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{ID: "root", Type: "clarity.region", Padding: "spacing-mega"},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 || errs[0].Code != "unknown-token" {
		t.Fatalf("expected one unknown-token error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "unknown-token", "spacing-mega")
}

func TestValidateLayoutDeep_WiringInLayout(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{ID: "grid", Type: "clarity.datagrid", Properties: map[string]interface{}{"dataSource": "orders"}},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	var w *ValidationError
	for i := range errs {
		if errs[i].Code == "wiring-in-layout" {
			w = &errs[i]
			break
		}
	}
	if w == nil {
		t.Fatalf("expected a wiring-in-layout error; got %+v", errs)
	}
	mustContainAll(t, errSurface(*w), "wiring-in-layout", "dataSource", "codegen")
}

func TestValidateLayoutDeep_RemovedComponentStillReferencedInLayout(t *testing.T) {
	adapter := fixtureClarity17() // clarity.callout is NOT declared
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.callout"}},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	if len(errs) != 1 || errs[0].Code != "unknown-component-type" {
		t.Fatalf("expected one unknown-component-type error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "clarity.callout", "clarity@17")
}

func TestValidateLayoutDeep_NilLayoutReturnsNil(t *testing.T) {
	if errs := ValidateLayoutDeep(nil, fixtureClarity17()); errs != nil {
		t.Fatalf("expected nil for nil layout; got %+v", errs)
	}
}

// Suite: cross-adapter parity

func TestCrossAdapterParity_IdenticalNoViolations(t *testing.T) {
	a := fixtureClarity17()
	b := fixtureClarity17()
	violations := CheckCrossAdapterParity(a, b)
	if len(violations) != 0 {
		t.Fatalf("expected 0 parity violations; got %d: %+v", len(violations), violations)
	}
}

func TestCrossAdapterParity_DriftReported(t *testing.T) {
	a := fixtureClarity17()
	b := fixtureClarity17()
	// Add clarity.tooltip to b only — drift.
	b.ComponentVocabulary.Components = append(b.ComponentVocabulary.Components, deepVocabularyComponent{
		Type:     "clarity.tooltip",
		Category: "leaf",
	})
	violations := CheckCrossAdapterParity(a, b)
	if len(violations) == 0 {
		t.Fatalf("expected at least one parity violation; got 0")
	}
	found := false
	for _, v := range violations {
		if strings.Contains(v.Message, "clarity.tooltip") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected parity violation to name clarity.tooltip; got %+v", violations)
	}
}

// Suite: codegen translation

func TestEmitTokenValue_SpacingTranslatesToEmitForm(t *testing.T) {
	adapter := fixtureClarity17()
	got, ok := EmitTokenValue(adapter, "spacing", "spacing-lg")
	if !ok {
		t.Fatalf("expected spacing-lg to resolve")
	}
	if !strings.Contains(got, "var(--spacing-lg)") {
		t.Fatalf("expected emit-form to contain var(--spacing-lg); got %q", got)
	}
}

func TestEmitTokenValue_PerModeColorEmissionCarriesEveryMode(t *testing.T) {
	adapter := fixtureClarity17()
	got, ok := EmitTokenValue(adapter, "color", "color-surface")
	if !ok {
		t.Fatalf("expected color-surface to resolve")
	}
	mustContainAll(t, got, "color-surface-light", "color-surface-dark")
}

func TestEmitTokenValue_UnknownTokenReturnsFalse(t *testing.T) {
	adapter := fixtureClarity17()
	if _, ok := EmitTokenValue(adapter, "spacing", "spacing-mega"); ok {
		t.Fatalf("expected unknown spacing token to return ok=false")
	}
}

func TestValidateLayoutParseError_MalformedYAMLYieldsMalformedLayoutBlockValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.md")
	contents := `---
name: dashboard
---

## Layout

` + "```" + `yaml
componentVocabulary: clarity@17
schema_version: 1
nodes:
  - id: root
    type: clarity.region
   bad-indent: oops
` + "```" + `
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	_, parseErr := parser.ParsePageFile(path)
	if parseErr == nil {
		t.Fatalf("expected parse error from malformed layout block")
	}
	errs := ValidateLayoutParseError(parseErr, fixtureClarity17())
	if len(errs) != 1 || errs[0].Code != "malformed-layout-block" {
		t.Fatalf("expected one malformed-layout-block error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "malformed-layout-block")
}

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
schema_version: 1
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

func TestLayoutPrecheck_UnsupportedSchemaVersionYieldsMalformedLayoutBlockVerdict(t *testing.T) {
	// The full-list validator (ValidateLayoutDeep) surfaces this with the
	// finer-grained layout-schema-version-unsupported code; the precheck's
	// closed-shape Verdict folds the same violation into
	// malformed-layout-block since that code isn't in the precheck's
	// registered closed set (layout.schema.md's "Validation pass" section).
	page := &parser.Page{
		Name: "dashboard",
		Layout: &parser.Layout{
			ComponentVocabulary: "clarity@17",
			SchemaVersion:       2,
			Nodes:               []parser.LayoutNode{{ID: "root", Type: "clarity.region"}},
		},
	}
	verdict := LayoutPrecheck(page, fixtureClarity17())
	if verdict.Code != VerdictMalformedLayoutBlock {
		t.Fatalf("expected Code=%s; got %+v", VerdictMalformedLayoutBlock, verdict)
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

// Suite: universal container fields with schema-fixed enums (D10)
//
// direction and alignment were parsed into parser.LayoutNode scalars and then
// read by nothing. This looked covered because the vocabulary validator does
// check container parameters against an adapter's parameter_constraints — but
// that path reads vocabulary.Node.LayoutParameters, and these two are decoded
// out of the property map into typed fields before it sees them. A layout could
// declare `direction: sideways` and pass every validator in the pipeline.
//
// These are behavioural, not structural: the conformance suite only proves a
// documented code is reachable in source, which a code emitted behind a
// condition that never matches also satisfies.

func TestValidateLayoutDeep_InvalidDirectionRejected(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{ID: "root", Type: "clarity.region", Direction: "sideways"},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	found := false
	for _, e := range errs {
		if e.Code != "universal-field-value-invalid" {
			continue
		}
		found = true
		surface := errSurface(e)
		// The offending value, the field, and the full allowed set all have to
		// be in the message: "invalid direction" without the alternatives makes
		// the author guess at a set the schema owns and they cannot widen.
		for _, want := range []string{"sideways", "direction", "horizontal", "vertical"} {
			if !strings.Contains(surface, want) {
				t.Errorf("diagnostic omits %q: %s", want, surface)
			}
		}
	}
	if !found {
		t.Fatalf("direction \"sideways\" produced no universal-field-value-invalid; got %+v", errs)
	}
}

func TestValidateLayoutDeep_InvalidAlignmentRejected(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{ID: "root", Type: "clarity.region", Alignment: "middle"},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	found := false
	for _, e := range errs {
		if e.Code == "universal-field-value-invalid" {
			found = true
			if !strings.Contains(errSurface(e), "stretch") {
				t.Errorf("diagnostic omits the allowed alignment set: %s", errSurface(e))
			}
		}
	}
	if !found {
		t.Fatalf("alignment \"middle\" produced no universal-field-value-invalid; got %+v", errs)
	}
}

// Every legal value must pass, and absent must pass. An over-strict check here
// would reject valid layouts, which is a worse failure than the gap it closes.
func TestValidateLayoutDeep_ValidAndAbsentUniversalFieldsAccepted(t *testing.T) {
	adapter := fixtureClarity17()
	for _, dir := range []string{"", "horizontal", "vertical"} {
		for _, align := range []string{"", "start", "center", "end", "stretch"} {
			layout := &parser.Layout{
				ComponentVocabulary: "clarity@17",
				SchemaVersion:       1,
				Nodes: []parser.LayoutNode{
					{ID: "root", Type: "clarity.region", Direction: dir, Alignment: align},
				},
			}
			for _, e := range ValidateLayoutDeep(layout, adapter) {
				if e.Code == "universal-field-value-invalid" {
					t.Errorf("direction=%q alignment=%q rejected: %s", dir, align, errSurface(e))
				}
			}
		}
	}
}

// A nested node's violation must be reported against its own path, not the
// root's — the walk has to reach children, and the locator has to say where.
func TestValidateLayoutDeep_InvalidDirectionOnNestedNodeNamesItsPath(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &parser.Layout{
		ComponentVocabulary: "clarity@17",
		SchemaVersion:       1,
		Nodes: []parser.LayoutNode{
			{
				ID: "root", Type: "clarity.region", Direction: "vertical",
				Children: []parser.LayoutNode{
					{ID: "inner", Type: "clarity.region", Direction: "diagonal"},
				},
			},
		},
	}
	errs := ValidateLayoutDeep(layout, adapter)
	found := false
	for _, e := range errs {
		if e.Code == "universal-field-value-invalid" {
			found = true
			if !strings.Contains(errSurface(e), "inner") {
				t.Errorf("violation does not name the offending nested node: %s", errSurface(e))
			}
			if strings.Contains(e.Context, "root") && !strings.Contains(e.Context, "inner") {
				t.Errorf("violation anchored on the root instead of the child: %s", e.Context)
			}
		}
	}
	if !found {
		t.Fatalf("nested invalid direction produced no violation; got %+v", errs)
	}
}
