package agent

// parlay-feature: studio-support/adapter-vocabulary-extension
// parlay-component: adapter-validator-deep-vocabulary-and-tokens
// parlay-artifact: test
//
// Tests the deep adapter validation surface for componentVocabulary and
// tokens — the vocabulary lookup passes (component, variant, property,
// allowed-children), the token lookup passes (raw-value-where-token-required,
// unknown-token), the version-mismatch fast-fail, the cross-adapter parity
// check, and the codegen emit-form translation.

import (
	"strings"
	"testing"
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
// Fixture: layout-validates-against-clarity-17

func TestValidateLayout_WellFormedClean(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes: []LayoutNode{
			{Type: "clarity.region"},
			{Type: "clarity.heading", ParentType: "clarity.region", Variant: "page", Properties: map[string]any{"text": "Hello"}},
			{Type: "clarity.button", ParentType: "clarity.region", Variant: "primary", Properties: map[string]any{"label": "Go"}},
			{Type: "clarity.datagrid", ParentType: "clarity.region", Variant: "compact"},
			{Type: "clarity.datagrid-column", ParentType: "clarity.datagrid", Properties: map[string]any{"headerLabel": "Name"}},
		},
	}
	errs := ValidateLayout(adapter, layout)
	if len(errs) != 0 {
		t.Fatalf("expected 0 validation errors; got %d: %+v", len(errs), errs)
	}
}

func TestValidateLayout_VersionMismatchFailsFast(t *testing.T) {
	adapter := fixtureClarity17()
	adapter.ComponentVocabulary.Name = "clarity@18"
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes: []LayoutNode{
			{Type: "clarity.foobar"}, // would also be unknown-component, but version-mismatch must fail fast
		},
	}
	errs := ValidateLayout(adapter, layout)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 validation error (version-mismatch fails fast); got %d: %+v", len(errs), errs)
	}
	if errs[0].Code != "version-mismatch" {
		t.Fatalf("expected code version-mismatch; got %q", errs[0].Code)
	}
	mustContainAll(t, errSurface(errs[0]), "version-mismatch", "clarity@17", "clarity@18")
}

func TestValidateLayout_UnknownComponent(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes:      []LayoutNode{{Type: "clarity.foobar"}},
	}
	errs := ValidateLayout(adapter, layout)
	if len(errs) != 1 || errs[0].Code != "unknown-component" {
		t.Fatalf("expected one unknown-component error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "unknown-component", "clarity.foobar")
}

func TestValidateLayout_UnknownVariantListsAllowedAlternatives(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes:      []LayoutNode{{Type: "clarity.button", Variant: "mega-button", Properties: map[string]any{"label": "Boom"}}},
	}
	errs := ValidateLayout(adapter, layout)
	if len(errs) != 1 || errs[0].Code != "unknown-variant" {
		t.Fatalf("expected one unknown-variant error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "unknown-variant", "mega-button", "primary, secondary, tertiary, danger")
}

func TestValidateLayout_DisallowedChildNamesParentAllowedSetAndOffender(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes: []LayoutNode{
			{Type: "clarity.button", ParentType: "clarity.datagrid", Properties: map[string]any{"label": "X"}},
		},
	}
	errs := ValidateLayout(adapter, layout)
	// Locate the disallowed-child error (variant may also fire if missing; here button is unvariant'd which is OK).
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
	mustContainAll(t, errSurface(*dc), "disallowed-child", "clarity.datagrid-column")
}

func TestValidateLayout_RawValueWhereTokenRequiredListsAvailableTokens(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes: []LayoutNode{
			{
				Type: "clarity.region",
				TokenRefs: []TokenReference{
					{Field: "gap", Value: "24px", Kind: "spacing", RawValue: true},
				},
			},
		},
	}
	errs := ValidateLayout(adapter, layout)
	if len(errs) != 1 || errs[0].Code != "raw-value-where-token-required" {
		t.Fatalf("expected one raw-value-where-token-required error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "raw-value-where-token-required", "spacing-xs, spacing-sm, spacing-md, spacing-lg, spacing-xl")
}

func TestValidateLayout_UnknownTokenLists(t *testing.T) {
	adapter := fixtureClarity17()
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes: []LayoutNode{
			{
				Type: "clarity.region",
				TokenRefs: []TokenReference{
					{Field: "gap", Value: "spacing-mega", Kind: "spacing"},
				},
			},
		},
	}
	errs := ValidateLayout(adapter, layout)
	if len(errs) != 1 || errs[0].Code != "unknown-token" {
		t.Fatalf("expected one unknown-token error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "unknown-token", "spacing-mega")
}

func TestValidateLayout_RemovedComponentStillReferencedInLayout(t *testing.T) {
	adapter := fixtureClarity17() // clarity.callout is NOT declared
	layout := &LayoutReference{
		Vocabulary: "clarity@17",
		Nodes:      []LayoutNode{{Type: "clarity.callout"}},
	}
	errs := ValidateLayout(adapter, layout)
	if len(errs) != 1 || errs[0].Code != "unknown-component" {
		t.Fatalf("expected one unknown-component error; got %+v", errs)
	}
	mustContainAll(t, errSurface(errs[0]), "clarity.callout", "clarity@17")
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
