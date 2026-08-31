// parlay-feature: parlay-tool/adapter-authoring
// parlay-component: canonical-adapter-model
//
// The canonical parsed shape of an adapter file, and the closed vocabularies
// its sections are validated against.
//
// Before this file the adapter format was unmarshalled by ten different Go
// structs with conflicting field sets — one that saw `file-conventions` as an
// untyped map, one that typed only `source-root`, one that typed `paths` fully,
// two that saw only `supports.steps`, and several that saw no `kind:` at all.
// A rule could therefore be enforced in one command and invisible in another,
// which is how `validate --type adapter` came to check only the toolchain block
// while reporting OK for an adapter with no name, no kind, and no
// file-conventions. One shape, one validator (validate_adapter.go), every
// consumer reading the same fields.

package agent

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// deepAdapter is the canonical adapter shape. Every section of
// adapter.schema.md is represented; consumers project the parts they need.
type deepAdapter struct {
	// Identity (top-level)
	Name      string `yaml:"name"`
	Framework string `yaml:"framework"`
	Version   string `yaml:"version"`
	Kind      string `yaml:"kind"`

	// §0.5 — backend capability contract
	Supports *AdapterSupports `yaml:"supports,omitempty"`

	// §1 — framework vocabulary (presentation)
	Shows   map[string]interface{} `yaml:"shows"`
	Actions map[string]interface{} `yaml:"actions"`
	Flows   map[string]interface{} `yaml:"flows"`

	// §2/§3 — recipes and team rules
	Compositions map[string]AdapterComposition `yaml:"compositions,omitempty"`
	Conventions  map[string]AdapterConvention  `yaml:"conventions,omitempty"`

	// §4 — where generated code goes
	FileConventions *AdapterFileConventions `yaml:"file-conventions,omitempty"`

	// §5/§6 — design-system provenance and interaction defaults
	DesignSystem map[string]AdapterDesignSystemEntry `yaml:"design-system,omitempty"`
	Patterns     *AdapterPatterns                    `yaml:"patterns,omitempty"`

	// §7 — brownfield insertion points
	MountStrategies map[string]AdapterMountStrategy `yaml:"mount-strategies,omitempty"`

	// §8/§9 — layout vocabulary and design tokens
	ComponentVocabulary *deepComponentVocabulary `yaml:"componentVocabulary,omitempty"`
	Tokens              *deepAdapterTokens       `yaml:"tokens,omitempty"`

	// §10 — external skills / MCP servers
	Toolchain *Toolchain `yaml:"toolchain,omitempty"`
}

// AdapterSupports is the non-presentation capability declaration. It mirrors
// the shape validate_supports.go reads; kept here so one struct owns it.
type AdapterSupports struct {
	OperationKinds []string `yaml:"operation_kinds,omitempty"`
	Steps          []string `yaml:"steps,omitempty"`
	Policies       []string `yaml:"policies,omitempty"`
	Errors         []string `yaml:"errors,omitempty"`
}

type AdapterComposition struct {
	Trigger     string   `yaml:"trigger"`
	State       []string `yaml:"state,omitempty"`
	Wiring      string   `yaml:"wiring"`
	Description string   `yaml:"description"`
}

type AdapterConvention struct {
	Rule      string `yaml:"rule"`
	AppliesTo string `yaml:"applies-to"`
}

// AdapterFileConventions carries BOTH path shapes, which answer different
// questions and are both load-bearing:
//
//   - Paths: per-artifact file templates. Machine-read — scaffold-plan derives
//     plan.creates rows from these. Without them a plan can only be hand-written,
//     and every hand-written plan drifted from what codegen emitted.
//   - Packages: the shared-code directory map ("where do reusable components,
//     hooks, utils live"). No paths template expresses this, which is why
//     `parlay simplify` needs it to place an extracted helper.
type AdapterFileConventions struct {
	ProjectRoot      string            `yaml:"project-root"`
	SourceRoot       string            `yaml:"source-root"`
	ComponentPattern string            `yaml:"component-pattern,omitempty"`
	Naming           string            `yaml:"naming"`
	EntryPoint       string            `yaml:"entry-point,omitempty"`
	Paths            adapterPathsBlock `yaml:"paths,omitempty"`
	Packages         map[string]string `yaml:"packages,omitempty"`
}

// adapterPathsBlock mirrors file-conventions.paths. Field set matches the
// deriver in commands/scaffold_plan.go.
type adapterPathsBlock struct {
	Component       string   `yaml:"component,omitempty"`
	ComponentExtras []string `yaml:"component-extras,omitempty"`
	Test            string   `yaml:"test,omitempty"`
	Model           string   `yaml:"model,omitempty"`
	Service         string   `yaml:"service,omitempty"`
	Types           string   `yaml:"types,omitempty"`
	FeatureRoutes   string   `yaml:"feature-routes,omitempty"`
	Routes          string   `yaml:"routes,omitempty"`
	Controller      string   `yaml:"controller,omitempty"`
	Module          string   `yaml:"module,omitempty"`
	Seed            string   `yaml:"seed,omitempty"`
	Store           string   `yaml:"store,omitempty"`
}

// All returns every declared path template keyed by its field name, so the
// validator can check placeholders without restating the field list.
func (p adapterPathsBlock) All() map[string]string {
	out := map[string]string{}
	for k, v := range map[string]string{
		"component": p.Component, "test": p.Test, "model": p.Model,
		"service": p.Service, "types": p.Types, "feature-routes": p.FeatureRoutes,
		"routes": p.Routes, "controller": p.Controller, "module": p.Module,
		"seed": p.Seed, "store": p.Store,
	} {
		if v != "" {
			out[k] = v
		}
	}
	for i, e := range p.ComponentExtras {
		if e != "" {
			out[fmt.Sprintf("component-extras[%d]", i)] = e
		}
	}
	return out
}

// IsEmpty reports whether the adapter declares no path templates at all —
// plan derivation is unavailable for such an adapter.
func (p adapterPathsBlock) IsEmpty() bool { return len(p.All()) == 0 }

type AdapterDesignSystemEntry struct {
	Source string `yaml:"source"`
	Format string `yaml:"format,omitempty"`
	Usage  string `yaml:"usage,omitempty"`
}

type AdapterPatterns struct {
	Interaction        map[string]interface{} `yaml:"interaction,omitempty"`
	InformationDensity *struct {
		Default   string `yaml:"default"`
		Rationale string `yaml:"rationale,omitempty"`
	} `yaml:"information-density,omitempty"`
	ErrorPlacement *struct {
		Default   string `yaml:"default"`
		Rationale string `yaml:"rationale,omitempty"`
	} `yaml:"error-placement,omitempty"`
	Confirmation *struct {
		RequiredFor []string `yaml:"required-for,omitempty"`
		Style       string   `yaml:"style"`
	} `yaml:"confirmation,omitempty"`
	Content map[string]interface{} `yaml:"content,omitempty"`
}

type AdapterMountStrategy struct {
	Detection   string `yaml:"detection"`
	Template    string `yaml:"template"`
	Description string `yaml:"description"`
}

// deepComponentVocabulary mirrors the adapter file's componentVocabulary
// block for validation lookups. Field names match the YAML schema.
type deepComponentVocabulary struct {
	Name       string                    `yaml:"name"`
	Components []deepVocabularyComponent `yaml:"components"`
}

type deepVocabularyComponent struct {
	Type            string                   `yaml:"type"`
	Category        string                   `yaml:"category"`
	Variants        []string                 `yaml:"variants,omitempty"`
	Properties      []deepVocabularyProperty `yaml:"properties,omitempty"`
	AllowedChildren []string                 `yaml:"allowed-children,omitempty"`
}

type deepVocabularyProperty struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	EnumValues []string `yaml:"enum-values,omitempty"`
	ChildTypes []string `yaml:"child-types,omitempty"`
	Required   bool     `yaml:"required"`
}

type deepAdapterTokens struct {
	Modes      []string              `yaml:"modes"`
	Spacing    []deepSpacingToken    `yaml:"spacing,omitempty"`
	Color      []deepColorToken      `yaml:"color,omitempty"`
	Typography []deepTypographyToken `yaml:"typography,omitempty"`
}

type deepSpacingToken struct {
	Name     string `yaml:"name"`
	Order    int    `yaml:"order"`
	EmitForm string `yaml:"emit-form"`
}

type deepColorToken struct {
	Name      string   `yaml:"name"`
	Tone      string   `yaml:"tone,omitempty"`
	EmitForms []string `yaml:"emit-forms"`
}

type deepTypographyToken struct {
	Name     string `yaml:"name"`
	UseSite  string `yaml:"use-site"`
	EmitForm string `yaml:"emit-form"`
}

// Adapter is the exported alias for the canonical parsed adapter shape. It is
// a type alias (not a new type) so any value produced by LoadAdapterFile is
// directly usable wherever the package already expects *deepAdapter, and so
// callers outside this package can hold and pass one.
type Adapter = deepAdapter

// ResolvedKind returns the adapter's kind, defaulting an absent one to
// presentation per adapter.schema.md Section 0.
func (a *deepAdapter) ResolvedKind() string {
	if a == nil || a.Kind == "" {
		return "presentation"
	}
	return a.Kind
}

// LoadAdapterFile reads and parses an adapter YAML file.
func LoadAdapterFile(path string) (*Adapter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read adapter file %s: %w", path, err)
	}
	var a deepAdapter
	if err := yaml.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("parse adapter file %s: %w", path, err)
	}
	return &a, nil
}

// ---------------------------------------------------------------------------
// Closed vocabularies
//
// These mirror the schema tables the way ClosedSetSteps/Policies/Errors mirror
// the capability vocabularies: declared in Go, kept honest by tests. The
// Shows/Actions/Flows sets come from surface.schema.md's Interaction
// Vocabulary — surface.schema.md:177 makes the adapter responsible for mapping
// every one of them, and every bundled presentation adapter already does.
// ---------------------------------------------------------------------------

var ClosedSetAdapterKinds = map[string]bool{
	"presentation": true, "transport": true, "application": true, "persistence": true,
}

var ClosedSetShows = map[string]bool{
	"data-value": true, "data-list": true, "data-table": true, "data-tree": true,
	"data-chart": true, "status": true, "progress": true, "message": true,
	"media": true, "empty-state": true, "summary": true, "diff": true,
	"timeline": true, "code": true,
}

var ClosedSetActions = map[string]bool{
	"provide-text": true, "provide-rich-text": true, "provide-value": true,
	"provide-file": true, "provide-structured-input": true, "select-one": true,
	"select-many": true, "select-toggle": true, "select-range": true,
	"confirm": true, "dismiss": true, "acknowledge": true, "navigate": true,
	"navigate-back": true, "navigate-drill": true, "navigate-tab": true,
	"navigate-search": true, "reorder": true, "move": true, "edit-inline": true,
	"invoke": true, "invoke-destructive": true, "invoke-batch": true,
	"undo": true, "redo": true, "export": true, "hand-off": true,
	"expand": true, "collapse": true, "filter": true, "sort": true, "inspect": true,
}

var ClosedSetFlows = map[string]bool{
	"guided-flow": true, "crud-collection": true, "search-and-act": true,
	"review-and-approve": true, "bulk-operation": true, "configure": true,
	"monitor": true, "import-export": true, "onboarding": true, "compare": true,
}

var ClosedSetNaming = map[string]bool{
	"kebab-case": true, "snake_case": true, "PascalCase": true, "camelCase": true,
}

var ClosedSetDesignSystemSources = map[string]bool{
	"framework": true, "not-defined": true,
}

var ClosedSetComponentCategories = map[string]bool{
	"container": true, "leaf": true, "data-shape": true,
}

var ClosedSetPropertyTypes = map[string]bool{
	"string": true, "token-reference": true, "enum": true,
	"boolean": true, "int": true, "child-list": true,
}

var ClosedSetColorTones = map[string]bool{
	"": true, "neutral": true, "info": true, "warning": true,
	"danger": true, "success": true,
}

var ClosedSetTypographyUseSites = map[string]bool{
	"heading-page": true, "heading-section": true, "body": true, "caption": true,
}

// UniversalContainerFields are owned by the layout schema; a componentVocabulary
// entry must not re-declare them.
var UniversalContainerFields = map[string]bool{
	"direction": true, "gap": true, "padding": true, "alignment": true,
}

// KnownPathPlaceholders are the substitutions expandTemplate performs. A path
// template naming anything else expands to a literal brace and is not a path.
var KnownPathPlaceholders = map[string]bool{
	"feature": true, "name": true, "entity": true,
	"Feature": true, "Name": true, "Entity": true,
}
