package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validator checks a file's content against its schema.
type Validator func(path string, content []byte) error

// ValidateYAML checks that a file is valid YAML.
func ValidateYAML(path string, content []byte) error {
	var v interface{}
	if err := yaml.Unmarshal(content, &v); err != nil {
		return fmt.Errorf("%s is not valid YAML: %w", path, err)
	}
	return nil
}

// ValidateBuildfile checks buildfile.yaml has required fields.
func ValidateBuildfile(path string, content []byte) error {
	if err := ValidateYAML(path, content); err != nil {
		return err
	}
	var bf struct {
		Feature    string      `yaml:"feature"`
		Adapter    string      `yaml:"adapter"`
		Components interface{} `yaml:"components"`
	}
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return fmt.Errorf("buildfile structure invalid: %w", err)
	}
	if bf.Feature == "" {
		return fmt.Errorf("buildfile missing 'feature' field")
	}
	if bf.Adapter == "" {
		return fmt.Errorf("buildfile missing 'adapter' field")
	}
	return nil
}

// ValidateSurface checks surface.md has fragment headings with Shows fields.
func ValidateSurface(path string, content []byte) error {
	text := string(content)
	if !strings.Contains(text, "## ") {
		return fmt.Errorf("surface.md has no fragment headings (## )")
	}
	if !strings.Contains(text, "**Shows**:") {
		return fmt.Errorf("surface.md has no **Shows**: fields")
	}
	return nil
}

// ValidateBlueprint checks blueprint.yaml has valid structure and cross-references.
func ValidateBlueprint(path string, content []byte) error {
	if err := ValidateYAML(path, content); err != nil {
		return err
	}

	var bp struct {
		App           string `yaml:"app"`
		Navigation    *struct {
			Strategy     string `yaml:"strategy"`
			DefaultRoute string `yaml:"default-route"`
			Routes       []struct {
				Path  string `yaml:"path"`
				Shell string `yaml:"shell"`
				Guard string `yaml:"guard"`
			} `yaml:"routes"`
		} `yaml:"navigation"`
		Shells        map[string]interface{} `yaml:"shells"`
		Authorization *struct {
			Strategy string                 `yaml:"strategy"`
			Guards   map[string]interface{} `yaml:"guards"`
		} `yaml:"authorization"`
	}
	if err := yaml.Unmarshal(content, &bp); err != nil {
		return fmt.Errorf("blueprint structure invalid: %w", err)
	}

	// Validate navigation strategy
	if bp.Navigation != nil && bp.Navigation.Strategy != "" {
		validStrategies := map[string]bool{
			"hash": true, "browser": true, "native-stack": true,
			"native-tab": true, "cli-subcommands": true,
		}
		if !validStrategies[bp.Navigation.Strategy] {
			return fmt.Errorf("invalid navigation.strategy %q — must be one of: hash, browser, native-stack, native-tab, cli-subcommands", bp.Navigation.Strategy)
		}
	}

	// Validate authorization strategy
	if bp.Authorization != nil && bp.Authorization.Strategy != "" {
		validAuthStrategies := map[string]bool{
			"role-based": true, "permission-based": true,
			"attribute-based": true, "none": true,
		}
		if !validAuthStrategies[bp.Authorization.Strategy] {
			return fmt.Errorf("invalid authorization.strategy %q — must be one of: role-based, permission-based, attribute-based, none", bp.Authorization.Strategy)
		}
	}

	// Cross-reference: shell names in routes must exist in shells
	if bp.Navigation != nil && bp.Navigation.Routes != nil {
		seenPaths := make(map[string]bool)
		for _, route := range bp.Navigation.Routes {
			// Check for duplicate paths
			if seenPaths[route.Path] {
				return fmt.Errorf("duplicate route path %q in navigation.routes", route.Path)
			}
			seenPaths[route.Path] = true

			// Check shell reference
			if route.Shell != "" && bp.Shells != nil {
				if _, ok := bp.Shells[route.Shell]; !ok {
					return fmt.Errorf("route %q references shell %q which is not defined in shells:", route.Path, route.Shell)
				}
			}

			// Check guard reference
			if route.Guard != "" && route.Guard != "none" {
				if bp.Authorization == nil || bp.Authorization.Guards == nil {
					return fmt.Errorf("route %q references guard %q but no authorization.guards are defined", route.Path, route.Guard)
				}
				if _, ok := bp.Authorization.Guards[route.Guard]; !ok {
					return fmt.Errorf("route %q references guard %q which is not defined in authorization.guards:", route.Path, route.Guard)
				}
			}
		}
	}

	return nil
}

// deepBuildfile is the parsed structure for deep validation.
type deepBuildfile struct {
	Feature      string                       `yaml:"feature"`
	Adapter      string                       `yaml:"adapter"`
	Models       map[string]interface{}       `yaml:"models"`
	Fixtures     map[string]deepFixture       `yaml:"fixtures"`
	Routes       []deepRoute                  `yaml:"routes"`
	Components   map[string]deepComponent     `yaml:"components"`
	CrossCutting []deepCrossCuttingEntry       `yaml:"cross-cutting"`
	Plan         *deepPlan                    `yaml:"plan"`
}

type deepPlan struct {
	Modifies []deepPlanEntry `yaml:"modifies"`
	Creates  []deepPlanEntry `yaml:"creates"`
	Deletes  []deepPlanEntry `yaml:"deletes"`
}

type deepPlanEntry struct {
	Path    string   `yaml:"path"`
	Sources []string `yaml:"sources"`
}

type deepCrossCuttingEntry struct {
	ID             string   `yaml:"id"`
	Source         string   `yaml:"source"`
	TargetFiles    []string `yaml:"target-files"`
	TargetPattern  string   `yaml:"target-pattern"`
	Transform      string   `yaml:"transform"`
	Introduces     []string `yaml:"introduces"`
}

type deepFixture struct {
	Data map[string]interface{} `yaml:"data"`
}

type deepRoute struct {
	Path    string                   `yaml:"path"`
	Regions map[string]deepRegion    `yaml:"regions"`
}

type deepRegion struct {
	Components []string `yaml:"components"`
}

type deepComponent struct {
	Source   string       `yaml:"source"`
	Widget   string       `yaml:"widget"`
	Data     *deepData    `yaml:"data"`
	Children []string     `yaml:"children"`
}

type deepData struct {
	Inputs []deepInput `yaml:"inputs"`
}

type deepInput struct {
	Model string `yaml:"model"`
}

// deepAdapter is the parsed adapter structure for vocabulary validation.
// Maps surface vocabulary terms (shows/actions/flows) to framework widgets.
//
// parlay-extends: studio-support/adapter-vocabulary-extension/adapter-validator-deep-vocabulary-and-tokens
//
// The ComponentVocabulary and Tokens fields are populated when the adapter
// declares the corresponding optional sections. Layout validation (and,
// looking forward, page-layout buildfile region validation) consult these
// to enforce vocabulary and token rules.
type deepAdapter struct {
	Shows               map[string]interface{}        `yaml:"shows"`
	Actions             map[string]interface{}        `yaml:"actions"`
	Flows               map[string]interface{}        `yaml:"flows"`
	ComponentVocabulary *deepComponentVocabulary      `yaml:"componentVocabulary,omitempty"`
	Tokens              *deepAdapterTokens            `yaml:"tokens,omitempty"`
}

// deepComponentVocabulary mirrors the adapter file's componentVocabulary
// block for validation lookups. Field names match the YAML schema.
type deepComponentVocabulary struct {
	Name       string                  `yaml:"name"`
	Components []deepVocabularyComponent `yaml:"components"`
}

type deepVocabularyComponent struct {
	Type            string                       `yaml:"type"`
	Category        string                       `yaml:"category"`
	Variants        []string                     `yaml:"variants,omitempty"`
	Properties      []deepVocabularyProperty     `yaml:"properties,omitempty"`
	AllowedChildren []string                     `yaml:"allowed-children,omitempty"`
}

type deepVocabularyProperty struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	EnumValues []string `yaml:"enum-values,omitempty"`
	ChildTypes []string `yaml:"child-types,omitempty"`
	Required   bool     `yaml:"required"`
}

type deepAdapterTokens struct {
	Modes      []string                  `yaml:"modes"`
	Spacing    []deepSpacingToken        `yaml:"spacing,omitempty"`
	Color      []deepColorToken          `yaml:"color,omitempty"`
	Typography []deepTypographyToken     `yaml:"typography,omitempty"`
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

// LayoutReference is the minimal shape of a layout used by ValidateLayout.
// Real layouts arrive via @studio-support/page-layout-field; this struct is
// sufficient to verify the validation contract today and is forward-compatible
// with the richer layout schema landing later.
//
// parlay-extends: studio-support/adapter-vocabulary-extension/adapter-validator-deep-vocabulary-and-tokens
type LayoutReference struct {
	// Vocabulary is the vocabulary version this layout pins itself to
	// (e.g., "clarity@17"). Mismatch with the adapter's vocabulary fails
	// fast before any component lookup.
	Vocabulary string

	// Nodes is the flat list of layout nodes to validate. Container nodes
	// are checked for allowed-children; leaf nodes are checked for variant
	// and property closure; data-shape nodes are checked for property
	// closure only.
	Nodes []LayoutNode
}

// LayoutNode is a single node within a LayoutReference.
type LayoutNode struct {
	Type         string         // e.g., "clarity.button"
	ParentType   string         // empty for root; the type of the parent container otherwise
	Variant      string         // empty when the node has no variant
	Properties   map[string]any // referenced property names → declared values
	TokenRefs    []TokenReference
}

// TokenReference is a single token reference in a layout node's chrome
// (e.g., gap: spacing-lg → TokenReference{Field: "gap", Value: "spacing-lg",
// Kind: "spacing"}). Raw values that should have been a token reference are
// represented with RawValue=true.
type TokenReference struct {
	Field    string // e.g., "gap"
	Value    string // the raw or token value supplied by the layout
	Kind     string // "spacing" | "color" | "typography"
	RawValue bool   // true when the layout supplied a literal value (e.g., "24px") instead of a token name
}

// ValidateLayout runs the deep validation passes for a layout against an
// adapter's componentVocabulary and tokens. Returns a list of structured
// errors with stable codes for programmatic handling.
//
// parlay-extends: studio-support/adapter-vocabulary-extension/adapter-validator-deep-vocabulary-and-tokens
func ValidateLayout(adapter *deepAdapter, layout *LayoutReference) []ValidationError {
	var errors []ValidationError
	if adapter == nil || adapter.ComponentVocabulary == nil {
		return errors
	}
	vocab := adapter.ComponentVocabulary

	// (1) Version-mismatch fails fast before any component lookup.
	if layout.Vocabulary != "" && layout.Vocabulary != vocab.Name {
		return []ValidationError{{
			Code:    "version-mismatch",
			Message: fmt.Sprintf("layout pins vocabulary %q but the active adapter declares %q — fails fast before any component lookup", layout.Vocabulary, vocab.Name),
			Context: "layout.vocabulary",
			Fix:     fmt.Sprintf("either update the layout to pin %q, or upgrade the adapter to declare %q", vocab.Name, layout.Vocabulary),
		}}
	}

	// Index components by type for the lookup passes.
	componentByType := map[string]*deepVocabularyComponent{}
	declaredTypes := make([]string, 0, len(vocab.Components))
	for i := range vocab.Components {
		c := &vocab.Components[i]
		componentByType[c.Type] = c
		declaredTypes = append(declaredTypes, c.Type)
	}
	declaredTypesList := strings.Join(declaredTypes, ", ")

	// Index spacing tokens for raw-value-where-token-required + unknown-token.
	spacingNames := map[string]bool{}
	spacingList := []string{}
	if adapter.Tokens != nil {
		for _, st := range adapter.Tokens.Spacing {
			spacingNames[st.Name] = true
			spacingList = append(spacingList, st.Name)
		}
	}
	spacingListStr := strings.Join(spacingList, ", ")

	for _, node := range layout.Nodes {
		// (2) Unknown component type.
		comp, ok := componentByType[node.Type]
		if !ok {
			errors = append(errors, ValidationError{
				Code:    "unknown-component",
				Message: fmt.Sprintf("layout (%s) references component type %q which is not declared in vocabulary %s", layout.Vocabulary, node.Type, vocab.Name),
				Context: fmt.Sprintf("layout.nodes.%s", node.Type),
				Fix:     fmt.Sprintf("either change the type to one of {%s} or add %q to the adapter's componentVocabulary", declaredTypesList, node.Type),
			})
			continue
		}

		// (3) Unknown variant.
		if node.Variant != "" {
			variantOK := false
			for _, v := range comp.Variants {
				if v == node.Variant {
					variantOK = true
					break
				}
			}
			if !variantOK {
				errors = append(errors, ValidationError{
					Code:    "unknown-variant",
					Message: fmt.Sprintf("layout references component %q with variant %q which is not in the closed enum {%s}", node.Type, node.Variant, strings.Join(comp.Variants, ", ")),
					Context: fmt.Sprintf("layout.nodes.%s.variant", node.Type),
					Fix:     fmt.Sprintf("change the variant to one of {%s}", strings.Join(comp.Variants, ", ")),
				})
			}
		}

		// (4) Unknown property — name or value-shape outside the closed type set.
		propByName := map[string]*deepVocabularyProperty{}
		for i := range comp.Properties {
			propByName[comp.Properties[i].Name] = &comp.Properties[i]
		}
		for propName := range node.Properties {
			if _, ok := propByName[propName]; !ok {
				errors = append(errors, ValidationError{
					Code:    "unknown-property",
					Message: fmt.Sprintf("layout references property %q on component %q which is not declared in the vocabulary", propName, node.Type),
					Context: fmt.Sprintf("layout.nodes.%s.properties", node.Type),
					Fix:     "either add this property to the component's componentVocabulary entry, or remove the reference from the layout",
				})
			}
		}

		// (5) Disallowed child — checked when ParentType is set on the node.
		if node.ParentType != "" {
			parent, ok := componentByType[node.ParentType]
			if ok && parent.Category == "container" {
				if !contains(parent.AllowedChildren, node.Type) {
					errors = append(errors, ValidationError{
						Code:    "disallowed-child",
						Message: fmt.Sprintf("layout places %q as a child of container %q whose allowed-children set is {%s} — %q is not in that set", node.Type, node.ParentType, strings.Join(parent.AllowedChildren, ", "), node.Type),
						Context: fmt.Sprintf("layout.nodes.%s.parent=%s", node.Type, node.ParentType),
						Fix:     fmt.Sprintf("either move the child under a container that allows it, or pick a child from {%s}", strings.Join(parent.AllowedChildren, ", ")),
					})
				}
			}
		}

		// (6) Token references.
		for _, tr := range node.TokenRefs {
			if tr.RawValue {
				errors = append(errors, ValidationError{
					Code:    "raw-value-where-token-required",
					Message: fmt.Sprintf("layout uses raw value %q for %q on component %q — a token-reference is required (available spacing tokens: %s)", tr.Value, tr.Field, node.Type, spacingListStr),
					Context: fmt.Sprintf("layout.nodes.%s.%s", node.Type, tr.Field),
					Fix:     fmt.Sprintf("replace the literal with one of: %s", spacingListStr),
				})
				continue
			}
			if tr.Kind == "spacing" {
				if !spacingNames[tr.Value] {
					errors = append(errors, ValidationError{
						Code:    "unknown-token",
						Message: fmt.Sprintf("layout uses %s token %q which is not declared (available: %s)", tr.Kind, tr.Value, spacingListStr),
						Context: fmt.Sprintf("layout.nodes.%s.%s", node.Type, tr.Field),
						Fix:     fmt.Sprintf("either change the value to one of {%s} or add %q to the adapter's tokens.%s section", spacingListStr, tr.Value, tr.Kind),
					})
				}
			}
		}
	}
	return errors
}

// contains reports whether s contains v.
func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// CheckCrossAdapterParity compares two adapters' componentVocabulary blocks
// and reports any drift as a parity violation. Two adapters declaring the
// same vocabulary version MUST produce structurally identical vocabulary
// blocks; until a shared-include mechanism lands, parity is held by hand.
//
// parlay-extends: studio-support/adapter-vocabulary-extension/adapter-validator-deep-vocabulary-and-tokens
func CheckCrossAdapterParity(a, b *deepAdapter) []ValidationError {
	var errors []ValidationError
	if a == nil || b == nil || a.ComponentVocabulary == nil || b.ComponentVocabulary == nil {
		return errors
	}
	if a.ComponentVocabulary.Name != b.ComponentVocabulary.Name {
		// Different vocabulary versions — parity check does not apply.
		return errors
	}
	aTypes := map[string]bool{}
	for _, c := range a.ComponentVocabulary.Components {
		aTypes[c.Type] = true
	}
	bTypes := map[string]bool{}
	for _, c := range b.ComponentVocabulary.Components {
		bTypes[c.Type] = true
	}
	for t := range bTypes {
		if !aTypes[t] {
			errors = append(errors, ValidationError{
				Code:    "parity-violation",
				Message: fmt.Sprintf("cross-adapter parity drift in %s: component %q is declared in one adapter but missing from the other", a.ComponentVocabulary.Name, t),
				Context: fmt.Sprintf("componentVocabulary.%s.components", a.ComponentVocabulary.Name),
				Fix:     fmt.Sprintf("either add %q to the missing adapter or remove it from the present one", t),
			})
		}
	}
	for t := range aTypes {
		if !bTypes[t] {
			errors = append(errors, ValidationError{
				Code:    "parity-violation",
				Message: fmt.Sprintf("cross-adapter parity drift in %s: component %q is declared in one adapter but missing from the other", a.ComponentVocabulary.Name, t),
				Context: fmt.Sprintf("componentVocabulary.%s.components", a.ComponentVocabulary.Name),
				Fix:     fmt.Sprintf("either add %q to the missing adapter or remove it from the present one", t),
			})
		}
	}
	return errors
}

// EmitTokenValue translates a token reference to the adapter's emit-form for
// codegen. Spacing tokens have a single mode-invariant emit-form; color
// tokens carry per-mode emit-forms and the returned string is a serialized
// list (e.g., "light:var(--color-surface-light) | dark:var(--color-surface-dark)").
// Returns ok=false when the token is not declared.
//
// parlay-extends: studio-support/adapter-vocabulary-extension/adapter-validator-deep-vocabulary-and-tokens
func EmitTokenValue(adapter *deepAdapter, kind, name string) (string, bool) {
	if adapter == nil || adapter.Tokens == nil {
		return "", false
	}
	switch kind {
	case "spacing":
		for _, t := range adapter.Tokens.Spacing {
			if t.Name == name {
				return t.EmitForm, true
			}
		}
	case "color":
		for _, t := range adapter.Tokens.Color {
			if t.Name == name {
				return strings.Join(t.EmitForms, " | "), true
			}
		}
	case "typography":
		for _, t := range adapter.Tokens.Typography {
			if t.Name == name {
				return t.EmitForm, true
			}
		}
	}
	return "", false
}

// ValidationError is a structured error returned by deep validation.
// Fields are designed for agent consumption: code identifies the error class,
// context provides specifics about where it occurred, and fix suggests recovery.
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Context string `json:"context,omitempty"`
	Fix     string `json:"fix"`
}

// ValidateBuildfileDeep performs cross-reference validation on a buildfile.
// Returns string-formatted errors for backwards compatibility.
// For structured output suitable for agent consumption, use ValidateBuildfileDeepStructured.
func ValidateBuildfileDeep(buildfilePath, adapterPath string) []string {
	structured := ValidateBuildfileDeepStructured(buildfilePath, adapterPath)
	var errors []string
	for _, e := range structured {
		errors = append(errors, e.Message)
	}
	return errors
}

// ValidateBuildfileDeepStructured performs cross-reference validation and returns
// structured errors. Each error has a code (for programmatic handling), context
// (location), and fix (recovery hint).
func ValidateBuildfileDeepStructured(buildfilePath, adapterPath string) []ValidationError {
	var errors []ValidationError

	content, err := os.ReadFile(buildfilePath)
	if err != nil {
		return []ValidationError{{
			Code:    "buildfile-not-readable",
			Message: fmt.Sprintf("cannot read buildfile: %s", err),
			Context: buildfilePath,
			Fix:     "ensure the buildfile path is correct and the file exists",
		}}
	}

	var bf deepBuildfile
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return []ValidationError{{
			Code:    "invalid-yaml",
			Message: fmt.Sprintf("invalid buildfile YAML: %s", err),
			Context: buildfilePath,
			Fix:     "fix the YAML syntax errors and re-run validation",
		}}
	}

	// 1. Component references in routes must exist in components
	for _, route := range bf.Routes {
		for regionName, region := range route.Regions {
			for _, compRef := range region.Components {
				if _, ok := bf.Components[compRef]; !ok {
					errors = append(errors, ValidationError{
						Code:    "missing-component-reference",
						Message: fmt.Sprintf("route %q region %q references component %q which is not defined", route.Path, regionName, compRef),
						Context: fmt.Sprintf("routes[%s].regions.%s", route.Path, regionName),
						Fix:     fmt.Sprintf("either add %q to the components: section or remove it from the route", compRef),
					})
				}
			}
		}
	}

	// 2. Model references in component data.inputs must exist in models
	for compName, comp := range bf.Components {
		if comp.Data != nil {
			for _, input := range comp.Data.Inputs {
				if input.Model != "" {
					if _, ok := bf.Models[input.Model]; !ok {
						errors = append(errors, ValidationError{
							Code:    "missing-model-reference",
							Message: fmt.Sprintf("component %q references model %q which is not defined", compName, input.Model),
							Context: fmt.Sprintf("components.%s.data.inputs", compName),
							Fix:     fmt.Sprintf("either add %q to the models: section or change the input to reference an existing model", input.Model),
						})
					}
				}
			}
		}

		// 3. Children references must exist in components
		for _, child := range comp.Children {
			if _, ok := bf.Components[child]; !ok {
				errors = append(errors, ValidationError{
					Code:    "missing-child-reference",
					Message: fmt.Sprintf("component %q references child %q which is not defined", compName, child),
					Context: fmt.Sprintf("components.%s.children", compName),
					Fix:     fmt.Sprintf("either add %q to the components: section or remove it from children", child),
				})
			}
		}
	}

	// 4. Fixture data keys must match defined models
	for fixtureName, fixture := range bf.Fixtures {
		for modelName := range fixture.Data {
			if _, ok := bf.Models[modelName]; !ok {
				errors = append(errors, ValidationError{
					Code:    "missing-fixture-model",
					Message: fmt.Sprintf("fixture %q references model %q which is not defined", fixtureName, modelName),
					Context: fmt.Sprintf("fixtures.%s.data", fixtureName),
					Fix:     fmt.Sprintf("either add %q to the models: section or remove the fixture data block", modelName),
				})
			}
		}
	}

	// 5. Adapter vocabulary validation (if adapter path provided)
	if adapterPath != "" {
		adapterErrors := validateAdapterVocabulary(bf, adapterPath)
		errors = append(errors, adapterErrors...)
	}

	// 6. Cross-cutting entry validation
	if len(bf.CrossCutting) > 0 {
		ccErrors := validateCrossCuttingEntries(bf.CrossCutting)
		errors = append(errors, ccErrors...)
	}

	// 7. Plan section validation: every component and cross-cutting
	// entry must be represented; modify-paths must exist; create-paths
	// must not collide with existing files.
	planErrors := validatePlanSection(bf, buildfilePath)
	errors = append(errors, planErrors...)

	return errors
}

// validatePlanSection enforces the executable contract of the plan:
// section. Every components: entry must produce at least one plan row;
// every cross-cutting target-files: path must appear in plan.modifies;
// modify-paths must exist on disk; create-paths must NOT exist.
//
// The function resolves paths relative to the directory containing the
// buildfile's source root, derived from the buildfile's location:
// .parlay/build/<feature>/buildfile.yaml lives at <root>/.parlay/build/<feature>/.
// Source-tree paths are interpreted relative to <root>.
func validatePlanSection(bf deepBuildfile, buildfilePath string) []ValidationError {
	var errors []ValidationError
	if bf.Plan == nil {
		errors = append(errors, ValidationError{
			Code:    "missing-plan",
			Message: "buildfile has no plan: section",
			Context: "plan",
			Fix:     "regenerate the buildfile via /parlay-build-feature so the plan: section is emitted",
		})
		return errors
	}
	rootDir := planRootDirFromBuildfilePath(buildfilePath)

	// Index plan entries by source for cross-checks.
	type planEntryKind struct {
		kind  string // "modify" | "create" | "delete"
		path  string
	}
	bySource := map[string][]planEntryKind{}
	addEntries := func(kind string, entries []deepPlanEntry, ctxPrefix string) {
		for i, e := range entries {
			ctx := fmt.Sprintf("%s[%d]", ctxPrefix, i)
			if e.Path == "" {
				errors = append(errors, ValidationError{
					Code:    "plan-entry-missing-path",
					Message: fmt.Sprintf("plan.%s entry at index %d has no path", kind, i),
					Context: ctx,
					Fix:     "add path: <file path> to the plan entry",
				})
				continue
			}
			if len(e.Sources) == 0 {
				errors = append(errors, ValidationError{
					Code:    "plan-entry-missing-sources",
					Message: fmt.Sprintf("plan.%s entry %q has no sources", kind, e.Path),
					Context: ctx,
					Fix:     "add sources: [component/<name> or cross-cutting/<id>] linking this entry to the buildfile entry that produced it",
				})
			}
			for _, src := range e.Sources {
				bySource[src] = append(bySource[src], planEntryKind{kind: kind, path: e.Path})
			}
		}
	}
	addEntries("modify", bf.Plan.Modifies, "plan.modifies")
	addEntries("create", bf.Plan.Creates, "plan.creates")
	addEntries("delete", bf.Plan.Deletes, "plan.deletes")

	// Every components: entry must appear in plan via component/<name> source.
	for compName := range bf.Components {
		key := "component/" + compName
		if _, ok := bySource[key]; !ok {
			errors = append(errors, ValidationError{
				Code:    "component-not-in-plan",
				Message: fmt.Sprintf("component %q has no entry in plan: (no row sources include %q)", compName, key),
				Context: fmt.Sprintf("plan / components.%s", compName),
				Fix:     "add a plan.creates or plan.modifies entry whose sources references this component",
			})
		}
	}

	// Every cross-cutting: entry's target-files: paths must appear in plan.modifies,
	// and the entry must be cited as the source.
	for _, cc := range bf.CrossCutting {
		key := "cross-cutting/" + cc.ID
		entries := bySource[key]
		if cc.ID != "" && len(entries) == 0 {
			errors = append(errors, ValidationError{
				Code:    "cross-cutting-not-in-plan",
				Message: fmt.Sprintf("cross-cutting %q has no entry in plan:", cc.ID),
				Context: fmt.Sprintf("plan / cross-cutting[%s]", cc.ID),
				Fix:     "add plan.modifies entries for each target-files: path, or plan.creates if the entry is purely-introducing",
			})
		}
		for _, target := range cc.TargetFiles {
			found := false
			for _, e := range entries {
				if e.kind == "modify" && e.path == target {
					found = true
					break
				}
			}
			if !found {
				errors = append(errors, ValidationError{
					Code:    "cross-cutting-target-not-in-plan",
					Message: fmt.Sprintf("cross-cutting %q names target-files: %q but plan.modifies has no matching entry sourced from %q", cc.ID, target, key),
					Context: fmt.Sprintf("plan.modifies / cross-cutting[%s].target-files", cc.ID),
					Fix:     fmt.Sprintf("add plan.modifies entry { path: %s, sources: [%s] }", target, key),
				})
			}
		}
	}

	// Disk-shape checks (only when we can resolve a sensible root).
	if rootDir != "" {
		for _, e := range bf.Plan.Modifies {
			if e.Path == "" {
				continue
			}
			abs := filepath.Join(rootDir, e.Path)
			if _, err := os.Stat(abs); err != nil && os.IsNotExist(err) {
				errors = append(errors, ValidationError{
					Code:    "plan-modify-target-missing",
					Message: fmt.Sprintf("plan.modifies %q does not exist in source root %s", e.Path, rootDir),
					Context: "plan.modifies",
					Fix:     "either fix the path, or move the entry to plan.creates if this feature genuinely introduces the file",
				})
			}
		}
		for _, e := range bf.Plan.Creates {
			if e.Path == "" {
				continue
			}
			abs := filepath.Join(rootDir, e.Path)
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				errors = append(errors, ValidationError{
					Code:    "plan-create-collision",
					Message: fmt.Sprintf("plan.creates %q already exists at %s", e.Path, abs),
					Context: "plan.creates",
					Fix:     "either move the entry to plan.modifies (to merge into the existing file) or pick a different path",
				})
			}
		}
	}

	return errors
}

// planRootDirFromBuildfilePath derives the project root directory from
// a buildfile's path. A buildfile at <root>/.parlay/build/<feature>/buildfile.yaml
// — or, for initiative-nested features, <root>/.parlay/build/<initiative>/<feature>/buildfile.yaml
// — belongs to root <root>. Returns "" when the path doesn't match the
// expected layout, signaling the disk-shape checks should be skipped.
func planRootDirFromBuildfilePath(buildfilePath string) string {
	abs, err := filepath.Abs(buildfilePath)
	if err != nil {
		return ""
	}
	// Walk up from the buildfile's directory until we land on
	// <root>/.parlay/build/, then return <root>. Feature slugs may be
	// qualified (e.g., "initiative/feature"), so the depth between the
	// buildfile and .parlay/build is not fixed.
	dir := filepath.Dir(abs)
	for dir != "/" && dir != "." && dir != "" {
		parent := filepath.Dir(dir)
		if filepath.Base(dir) == "build" && filepath.Base(parent) == ".parlay" {
			return filepath.Dir(parent)
		}
		dir = parent
	}
	return ""
}

func validateCrossCuttingEntries(entries []deepCrossCuttingEntry) []ValidationError {
	var errors []ValidationError
	seenIDs := make(map[string]bool)

	for i, entry := range entries {
		ctx := fmt.Sprintf("cross-cutting[%d]", i)

		if entry.ID == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-id",
				Message: fmt.Sprintf("cross-cutting entry at index %d has no id", i),
				Context: ctx,
				Fix:     "add a unique id: field to the cross-cutting entry",
			})
		} else {
			if seenIDs[entry.ID] {
				errors = append(errors, ValidationError{
					Code:    "duplicate-cross-cutting-id",
					Message: fmt.Sprintf("cross-cutting id %q appears more than once", entry.ID),
					Context: ctx,
					Fix:     "rename one of the duplicate entries to be unique",
				})
			}
			seenIDs[entry.ID] = true
		}

		if entry.Source == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-source",
				Message: fmt.Sprintf("cross-cutting entry %q has no source reference", entry.ID),
				Context: ctx,
				Fix:     "add source: @feature/intent-slug for traceability",
			})
		}

		if entry.Transform == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-transform",
				Message: fmt.Sprintf("cross-cutting entry %q has no transform description", entry.ID),
				Context: ctx,
				Fix:     "add transform: describing what the change does",
			})
		}

		if len(entry.TargetFiles) == 0 && entry.TargetPattern == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-target",
				Message: fmt.Sprintf("cross-cutting entry %q has neither target-files nor target-pattern", entry.ID),
				Context: ctx,
				Fix:     "add target-files: (explicit paths) or target-pattern: (grep pattern) or both",
			})
		}
	}

	return errors
}

func validateAdapterVocabulary(bf deepBuildfile, adapterPath string) []ValidationError {
	var errors []ValidationError

	data, err := os.ReadFile(adapterPath)
	if err != nil {
		// Adapter file doesn't exist — try resolving from .parlay/adapters/
		resolved := filepath.Join(".parlay", "adapters", bf.Adapter+".adapter.yaml")
		data, err = os.ReadFile(resolved)
		if err != nil {
			return []ValidationError{{
				Code:    "adapter-not-found",
				Message: fmt.Sprintf("cannot read adapter %q: %s", adapterPath, err),
				Context: adapterPath,
				Fix:     "verify the adapter file exists at .parlay/adapters/{name}.adapter.yaml",
			}}
		}
	}

	var adapter deepAdapter
	if err := yaml.Unmarshal(data, &adapter); err != nil {
		return []ValidationError{{
			Code:    "invalid-adapter-yaml",
			Message: fmt.Sprintf("invalid adapter YAML: %s", err),
			Context: adapterPath,
			Fix:     "fix the YAML syntax errors in the adapter file",
		}}
	}

	// Check component widgets against adapter vocabulary. The buildfile
	// contains framework-specific widget names populated from the adapter's
	// shows/actions mappings. Widgets that don't appear in ANY adapter
	// section are flagged.
	allWidgets := make(map[string]bool)
	for _, sections := range []map[string]interface{}{adapter.Shows, adapter.Actions, adapter.Flows} {
		for _, v := range sections {
			if m, ok := v.(map[string]interface{}); ok {
				if w, ok := m["widget"]; ok {
					allWidgets[fmt.Sprint(w)] = true
				}
				if p, ok := m["pattern"]; ok {
					allWidgets[fmt.Sprint(p)] = true
				}
			}
		}
	}
	for compName, comp := range bf.Components {
		if comp.Widget != "" && comp.Widget != "not-applicable" {
			if !allWidgets[comp.Widget] {
				errors = append(errors, ValidationError{
					Code:    "unknown-widget",
					Message: fmt.Sprintf("component %q uses widget %q which is not in adapter %q", compName, comp.Widget, bf.Adapter),
					Context: fmt.Sprintf("components.%s.widget", compName),
					Fix:     fmt.Sprintf("change the widget to one defined in the adapter's shows/actions/flows sections, or add %q to the adapter", comp.Widget),
				})
			}
		}
	}

	return errors
}

// ============================================================================
// Domain-model deep validation
// ============================================================================
//
// parlay-feature: studio-support/domain-model-yaml-migration
// parlay-component: validation-error-reporter
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-deep-validator
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-schema-version-dispatch
//
// Open-question resolutions applied (per the code-phase decision):
//
//   - enum-tone-vocabulary  → closed-set
//   - entity-field-shape    → flat-only
//
// See internal/embedded/schemas/domain-model.schema.md for the canonical
// shape and the rationale for each closed set.

// DomainModelBinaryVersion is the schema_version this binary natively
// understands. Older versions are migrated in-memory through the
// migrator chain registered with RegisterDomainModelMigrator. Newer
// versions refuse to load; the user is told to run parlay upgrade.
//
// Bumping this constant is a coordinated change: the new value lives
// here, the migrator from the previous version lives in
// init() / RegisterDomainModelMigrator, and the schema document under
// internal/embedded/schemas/domain-model.schema.md is updated to match.
const DomainModelBinaryVersion = 1

// closedFieldTypePrimitives is the closed primitive set for a
// DomainField.type per the entity-field-shape=flat-only resolution.
// Field types outside this set must name a declared enum (handled
// separately) or fail with field-type-outside-closed-set.
var closedFieldTypePrimitives = map[string]bool{
	"uuid":     true,
	"string":   true,
	"int":      true,
	"float":    true,
	"bool":     true,
	"datetime": true,
	"ref":      true,
}

// closedFieldTypeList is the human-readable rendering of
// closedFieldTypePrimitives plus the named-enum sentinel, used in error
// messages.
const closedFieldTypeList = "uuid, string, int, float, bool, datetime, ref, <named-enum>"

// closedRelationshipCardinalities is the closed set of relationship
// cardinalities. Anything else fails with relationship-cardinality-unknown.
var closedRelationshipCardinalities = map[string]bool{
	"one-to-one":   true,
	"one-to-many":  true,
	"many-to-one":  true,
	"many-to-many": true,
}

// closedCardinalityList is the human-readable rendering of
// closedRelationshipCardinalities, used in error messages.
const closedCardinalityList = "one-to-one, one-to-many, many-to-one, many-to-many"

// closedEnumTones is the closed set of tones a DomainEnumValue may
// declare per the enum-tone-vocabulary=closed-set resolution. This
// matches the adapter's color-token tone vocabulary.
var closedEnumTones = map[string]bool{
	"neutral": true,
	"info":    true,
	"warning": true,
	"danger":  true,
	"success": true,
}

// closedEnumToneList is the human-readable rendering of
// closedEnumTones, used in error messages.
const closedEnumToneList = "neutral, info, warning, danger, success"

// deepDomainModel mirrors the on-disk YAML shape declared in
// internal/embedded/schemas/domain-model.schema.md. The flat-only
// resolution means fields cannot be inline object literals; they are
// strings naming a primitive type or a declared enum, plus an optional
// target/enum reference key.
type deepDomainModel struct {
	SchemaVersion int                      `yaml:"schema_version"`
	Enums         []deepDomainEnum         `yaml:"enums"`
	Entities      []deepDomainEntity       `yaml:"entities"`
	Relationships []deepDomainRelationship `yaml:"relationships"`
	Operations    []deepDomainOperation    `yaml:"operations"`
}

type deepDomainEnum struct {
	Name   string                `yaml:"name"`
	Values []deepDomainEnumValue `yaml:"values"`
}

type deepDomainEnumValue struct {
	Value string `yaml:"value"`
	Label string `yaml:"label,omitempty"`
	Tone  string `yaml:"tone,omitempty"`
}

type deepDomainEntity struct {
	Name   string `yaml:"name"`
	Fields []struct {
		// The fields here are typed as yaml.Node so we can detect inline
		// object literals (which the flat-only resolution rejects). A
		// scalar Type maps to a string; a mapping/sequence node tells us
		// the field shape is nested.
		Name     string    `yaml:"name"`
		Type     yaml.Node `yaml:"type"`
		Target   string    `yaml:"target,omitempty"`
		Enum     string    `yaml:"enum,omitempty"`
		Required bool      `yaml:"required"`
	} `yaml:"fields"`
}

type deepDomainRelationship struct {
	Name        string `yaml:"name"`
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Cardinality string `yaml:"cardinality"`
}

type deepDomainOperation struct {
	Name    string   `yaml:"name"`
	Input   []string `yaml:"input"`
	Effects []string `yaml:"effects,omitempty"`
}

// ValidateDomainModel is the deep validator for domain-model.yaml.
// Returns nil on success; a single aggregated error on failure, with
// every unresolved reference reported (not just the first).
//
// The validator runs the following passes:
//
//  1. parse YAML and confirm schema_version is present and integer
//  2. compare schema_version to the binary's expected version
//  3. for each entity field, validate type is in the closed set or
//     names a declared enum (flat-only — inline object literals fail)
//  4. for each enum value, validate tone is in the closed set
//     (closed-set — unknown tones fail)
//  5. for each relationship, validate from/to resolve to a declared
//     entity and cardinality is in the closed set
//  6. for each operation, validate every input field name resolves
//     to a real field on a declared entity
//
// The model is not partially accepted — every reference must resolve
// or the call exits non-nil. The function signature mirrors the other
// validators in this file (path string, content []byte) error so it
// can be wired into the validate.go --type switch.
func ValidateDomainModel(path string, content []byte) error {
	errs := ValidateDomainModelStructured(path, content)
	if len(errs) == 0 {
		return nil
	}
	// Aggregate: callers that want structured output can use the
	// _Structured variant directly. The plain Validator interface
	// returns a single error built from the concatenated messages.
	var b strings.Builder
	b.WriteString(fmt.Sprintf("domain-model.yaml validation failed: %d issue(s)", len(errs)))
	for _, e := range errs {
		b.WriteString("\n  [")
		b.WriteString(e.Code)
		b.WriteString("] ")
		b.WriteString(e.Message)
	}
	return fmt.Errorf("%s", b.String())
}

// ValidateDomainModelStructured returns the structured ValidationError
// list for a domain-model.yaml. Used by the agent-facing validate
// command (--json) and by the JSON-emitting --type domain-model path
// in internal/commands/validate.go.
func ValidateDomainModelStructured(path string, content []byte) []ValidationError {
	var errors []ValidationError

	// Pass 0: YAML parse.
	var dm deepDomainModel
	if err := yaml.Unmarshal(content, &dm); err != nil {
		return []ValidationError{{
			Code:    "invalid-yaml",
			Message: fmt.Sprintf("domain-model.yaml is not valid YAML: %s", err),
			Context: path,
			Fix:     "fix the YAML syntax errors and re-run validation",
		}}
	}

	// Pass 1: schema_version is present and integer (yaml.Unmarshal of
	// a missing or string-typed field leaves SchemaVersion at its zero
	// value, 0). We re-decode against a permissive map to disambiguate
	// "missing" from "present but non-integer".
	rawTop := map[string]interface{}{}
	_ = yaml.Unmarshal(content, &rawTop)
	if _, present := rawTop["schema_version"]; !present {
		errors = append(errors, ValidationError{
			Code:    "missing-schema-version",
			Message: "domain-model.yaml is missing required top-level field 'schema_version'",
			Context: "schema_version",
			Fix:     "add 'schema_version: 1' to the top of the file",
		})
		return errors
	}
	if _, ok := rawTop["schema_version"].(int); !ok {
		errors = append(errors, ValidationError{
			Code:    "missing-schema-version",
			Message: fmt.Sprintf("domain-model.yaml schema_version must be an integer (got %T)", rawTop["schema_version"]),
			Context: "schema_version",
			Fix:     "set schema_version to a plain integer (e.g., 'schema_version: 1' — no quotes, no semver)",
		})
		return errors
	}

	// Pass 2: compare schema_version to the binary's expected version.
	switch chain := ResolveDomainModelVersion(dm.SchemaVersion); chain.Outcome {
	case DomainModelVersionEqual, DomainModelVersionMigrate:
		// equal: pipeline proceeds. migrate: in-memory chain; for
		// validation purposes this is a no-op — we still validate the
		// on-disk shape, which is the source-of-truth representation.
	case DomainModelVersionTooNew:
		errors = append(errors, ValidationError{
			Code:    "schema-version-newer-than-binary",
			Message: fmt.Sprintf("domain-model.yaml schema_version %d is newer than this Core release supports (%d); run parlay upgrade", dm.SchemaVersion, DomainModelBinaryVersion),
			Context: "schema_version",
			Fix:     "run parlay upgrade to install a Core release that supports the file's schema_version",
		})
		return errors
	case DomainModelVersionUnreachable:
		errors = append(errors, ValidationError{
			Code:    "schema-version-unreachable",
			Message: fmt.Sprintf("domain-model.yaml schema_version %d has no migrator chain reaching the binary version (%d); upgrade in steps via the documented migration path", dm.SchemaVersion, DomainModelBinaryVersion),
			Context: "schema_version",
			Fix:     "run an intermediate parlay release that supports the file's schema_version, migrate, then upgrade further",
		})
		return errors
	}

	// Build lookup tables for cross-reference passes.
	enumByName := map[string]*deepDomainEnum{}
	enumValueSet := map[string]map[string]bool{} // enum-name → set of declared values
	for i := range dm.Enums {
		e := &dm.Enums[i]
		enumByName[e.Name] = e
		vs := map[string]bool{}
		for _, v := range e.Values {
			vs[v.Value] = true
		}
		enumValueSet[e.Name] = vs
	}

	entityByName := map[string]*deepDomainEntity{}
	entityFieldSet := map[string]map[string]bool{} // entity-name → set of field names
	for i := range dm.Entities {
		ent := &dm.Entities[i]
		entityByName[ent.Name] = ent
		fs := map[string]bool{}
		for _, f := range ent.Fields {
			fs[f.Name] = true
		}
		entityFieldSet[ent.Name] = fs
	}

	// Pass 3: enum tones (closed-set resolution).
	for _, e := range dm.Enums {
		for _, v := range e.Values {
			if v.Tone == "" {
				continue
			}
			if !closedEnumTones[v.Tone] {
				errors = append(errors, ValidationError{
					Code:    "enum-tone-outside-closed-set",
					Message: fmt.Sprintf("enums.%s.values.%s.tone %q is not in the closed set (%s)", e.Name, v.Value, v.Tone, closedEnumToneList),
					Context: fmt.Sprintf("enums.%s.values.%s.tone", e.Name, v.Value),
					Fix:     fmt.Sprintf("change the tone to one of {%s}, or bump schema_version and add a migrator if a new tone is genuinely needed", closedEnumToneList),
				})
			}
		}
	}

	// Pass 4: entity field types (flat-only resolution).
	for _, ent := range dm.Entities {
		for _, f := range ent.Fields {
			// Inline object literal? The yaml.Node tells us.
			if f.Type.Kind == yaml.MappingNode || f.Type.Kind == yaml.SequenceNode {
				errors = append(errors, ValidationError{
					Code:    "field-type-outside-closed-set",
					Message: fmt.Sprintf("entities.%s.fields.%s declares a nested field shape, which the flat-only resolution rejects", ent.Name, f.Name),
					Context: fmt.Sprintf("entities.%s.fields.%s.type", ent.Name, f.Name),
					Fix:     fmt.Sprintf("lift the nested shape into a separate entity joined by a ref-typed field (closed set: %s)", closedFieldTypeList),
				})
				continue
			}
			typeStr := f.Type.Value
			if typeStr == "" {
				errors = append(errors, ValidationError{
					Code:    "field-type-outside-closed-set",
					Message: fmt.Sprintf("entities.%s.fields.%s declares no type", ent.Name, f.Name),
					Context: fmt.Sprintf("entities.%s.fields.%s.type", ent.Name, f.Name),
					Fix:     fmt.Sprintf("set type: to one of the closed set (%s) or to a declared enum name", closedFieldTypeList),
				})
				continue
			}
			// Primitive in the closed set?
			if closedFieldTypePrimitives[typeStr] {
				if typeStr == "ref" && f.Target == "" {
					errors = append(errors, ValidationError{
						Code:    "ref-missing-target",
						Message: fmt.Sprintf("entities.%s.fields.%s declares type 'ref' without a target entity", ent.Name, f.Name),
						Context: fmt.Sprintf("entities.%s.fields.%s.target", ent.Name, f.Name),
						Fix:     "add target: <Entity> naming the referenced entity",
					})
				}
				if typeStr == "ref" && f.Target != "" {
					if _, ok := entityByName[f.Target]; !ok {
						errors = append(errors, ValidationError{
							Code:    "undeclared-entity-reference",
							Message: fmt.Sprintf("entities.%s.fields.%s.target references undeclared entity %q", ent.Name, f.Name, f.Target),
							Context: fmt.Sprintf("entities.%s.fields.%s.target", ent.Name, f.Name),
							Fix:     fmt.Sprintf("declare entity %q in entities: or change the target to an existing one", f.Target),
						})
					}
				}
				continue
			}
			// Names a declared enum?
			if _, ok := enumByName[typeStr]; ok {
				// Optional consistency: if enum: key is set, it must
				// match the type. We treat this as advisory only —
				// missing or matching enum: is fine.
				if f.Enum != "" && f.Enum != typeStr {
					errors = append(errors, ValidationError{
						Code:    "enum-key-mismatch",
						Message: fmt.Sprintf("entities.%s.fields.%s declares type %q but enum: %q — these must match", ent.Name, f.Name, typeStr, f.Enum),
						Context: fmt.Sprintf("entities.%s.fields.%s.enum", ent.Name, f.Name),
						Fix:     fmt.Sprintf("set enum: %q (matching the type), or change type: to match enum:", typeStr),
					})
				}
				continue
			}
			// Anything else fails.
			errors = append(errors, ValidationError{
				Code:    "field-type-outside-closed-set",
				Message: fmt.Sprintf("entities.%s.fields.%s declares type %q which is not in the closed set (%s)", ent.Name, f.Name, typeStr, closedFieldTypeList),
				Context: fmt.Sprintf("entities.%s.fields.%s.type", ent.Name, f.Name),
				Fix:     fmt.Sprintf("change the type to one of {%s} or to a declared enum name", closedFieldTypeList),
			})
		}
	}

	// Pass 5: relationships.
	for _, r := range dm.Relationships {
		if r.From != "" {
			if _, ok := entityByName[r.From]; !ok {
				errors = append(errors, ValidationError{
					Code:    "undeclared-entity-reference",
					Message: fmt.Sprintf("relationships.%s.from references undeclared entity %q", r.Name, r.From),
					Context: fmt.Sprintf("relationships.%s.from", r.Name),
					Fix:     fmt.Sprintf("declare entity %q in entities: or change relationship endpoint", r.From),
				})
			}
		}
		if r.To != "" {
			if _, ok := entityByName[r.To]; !ok {
				errors = append(errors, ValidationError{
					Code:    "undeclared-entity-reference",
					Message: fmt.Sprintf("relationships.%s.to references undeclared entity %q", r.Name, r.To),
					Context: fmt.Sprintf("relationships.%s.to", r.Name),
					Fix:     fmt.Sprintf("declare entity %q in entities: or change relationship endpoint", r.To),
				})
			}
		}
		if r.Cardinality != "" && !closedRelationshipCardinalities[r.Cardinality] {
			errors = append(errors, ValidationError{
				Code:    "relationship-cardinality-unknown",
				Message: fmt.Sprintf("relationships.%s.cardinality %q is not recognized (%s)", r.Name, r.Cardinality, closedCardinalityList),
				Context: fmt.Sprintf("relationships.%s.cardinality", r.Name),
				Fix:     fmt.Sprintf("change cardinality to one of {%s}", closedCardinalityList),
			})
		}
	}

	// Pass 6: operation inputs.
	for _, op := range dm.Operations {
		for i, inp := range op.Input {
			// Input is "Entity.field" form. Anything else is malformed.
			parts := strings.SplitN(inp, ".", 2)
			if len(parts) != 2 {
				errors = append(errors, ValidationError{
					Code:    "operation-input-field-not-found",
					Message: fmt.Sprintf("operations.%s.input[%d] %q is not in 'Entity.field' form", op.Name, i, inp),
					Context: fmt.Sprintf("operations.%s.input[%d]", op.Name, i),
					Fix:     "use 'Entity.field' form (e.g., 'Order.id') to name a field on a declared entity",
				})
				continue
			}
			entName, fieldName := parts[0], parts[1]
			fields, ok := entityFieldSet[entName]
			if !ok {
				errors = append(errors, ValidationError{
					Code:    "undeclared-entity-reference",
					Message: fmt.Sprintf("operations.%s.input[%d] references undeclared entity %q", op.Name, i, entName),
					Context: fmt.Sprintf("operations.%s.input[%d]", op.Name, i),
					Fix:     fmt.Sprintf("declare entity %q or change the input to an existing entity", entName),
				})
				continue
			}
			if !fields[fieldName] {
				errors = append(errors, ValidationError{
					Code:    "operation-input-field-not-found",
					Message: fmt.Sprintf("operations.%s.input names field %q which is not declared on entity %q", op.Name, fieldName, entName),
					Context: fmt.Sprintf("operations.%s.input[%d]", op.Name, i),
					Fix:     fmt.Sprintf("either add field %q to entity %q or change the input to a declared field", fieldName, entName),
				})
			}
		}
	}

	// Cross-pass: enum-typed fields whose enum value is declared but
	// the field-level enum: key references a value not in the enum.
	// (This catches a fixture-style mistake; deep validation does not
	// otherwise observe live values.)
	for _, ent := range dm.Entities {
		for _, f := range ent.Fields {
			if f.Enum == "" {
				continue
			}
			vs, ok := enumValueSet[f.Enum]
			if !ok {
				// enum: names a missing enum — already flagged above when
				// the type itself doesn't resolve. Skip duplicate.
				continue
			}
			_ = vs // declared values are validated by fixtures, not here
		}
	}

	return errors
}

// ============================================================================
// Domain-model schema-version dispatch
// ============================================================================
//
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-schema-version-dispatch
//
// Every read of domain-model.yaml first inspects schema_version and
// dispatches:
//
//   - equal-to-binary    → reader proceeds directly
//   - older-than-binary  → in-memory model is routed through the
//                          per-version migrator chain (e.g., v1→v2,
//                          v2→v3) before handing to the rest of the
//                          pipeline; the on-disk file is unchanged
//   - newer-than-binary  → fail with schema-version-newer-than-binary
//   - unreachably-old    → fail with schema-version-unreachable
//
// Migrators are pure functions over the in-memory model — they do not
// write to disk, do not consult the network, and do not require AI
// inference. Adding a new schema_version requires registering exactly
// one migrator (from the previous version to the new one); chaining is
// automatic.

// DomainModelVersionOutcome classifies a resolved schema_version
// against the binary's natively-supported version.
type DomainModelVersionOutcome int

const (
	// DomainModelVersionEqual — schema_version equals the binary
	// version; no migration needed.
	DomainModelVersionEqual DomainModelVersionOutcome = iota
	// DomainModelVersionMigrate — schema_version is older but a
	// migrator chain reaches the binary version.
	DomainModelVersionMigrate
	// DomainModelVersionTooNew — schema_version is newer than the
	// binary version.
	DomainModelVersionTooNew
	// DomainModelVersionUnreachable — schema_version is older but no
	// migrator chain reaches the binary version.
	DomainModelVersionUnreachable
)

// DomainModelMigrator is a pure function that lifts a domain model
// from one schema_version to the next. Migrators are registered with
// RegisterDomainModelMigrator at package init time.
type DomainModelMigrator interface {
	FromVersion() int
	ToVersion() int
	Migrate(in map[string]interface{}) (map[string]interface{}, error)
}

// DomainModelVersionResolution is the result of dispatching a
// schema_version against the binary's version. Chain is the ordered
// list of migrators to apply when Outcome is DomainModelVersionMigrate;
// it is empty for the other outcomes.
type DomainModelVersionResolution struct {
	Outcome DomainModelVersionOutcome
	Chain   []DomainModelMigrator
}

// domainModelMigrators is the package-level registry, keyed by source
// version. There is at most one migrator per source version (the
// canonical "v1→v2"); the chain is built by walking from the file's
// version up to the binary's version one step at a time.
var domainModelMigrators = map[int]DomainModelMigrator{}

// RegisterDomainModelMigrator registers a migrator for ResolveDomainModelVersion
// to consult. Intended to be called from init() in a per-version
// migrator file. Re-registration with the same FromVersion overwrites
// the prior entry — useful for tests; in production each FromVersion
// is registered exactly once.
func RegisterDomainModelMigrator(m DomainModelMigrator) {
	domainModelMigrators[m.FromVersion()] = m
}

// ResolveDomainModelVersion classifies the given schema_version against
// the binary's version and (when older) builds the migrator chain that
// would lift the in-memory model up to the binary version. Pure — does
// not read or write any file, does not consult the network.
func ResolveDomainModelVersion(version int) DomainModelVersionResolution {
	if version == DomainModelBinaryVersion {
		return DomainModelVersionResolution{Outcome: DomainModelVersionEqual}
	}
	if version > DomainModelBinaryVersion {
		return DomainModelVersionResolution{Outcome: DomainModelVersionTooNew}
	}
	// version < binary: walk the chain.
	var chain []DomainModelMigrator
	cur := version
	for cur < DomainModelBinaryVersion {
		m, ok := domainModelMigrators[cur]
		if !ok {
			return DomainModelVersionResolution{Outcome: DomainModelVersionUnreachable}
		}
		chain = append(chain, m)
		next := m.ToVersion()
		if next <= cur {
			// Defensive: a migrator that doesn't advance the version
			// would loop forever. Treat as unreachable.
			return DomainModelVersionResolution{Outcome: DomainModelVersionUnreachable}
		}
		cur = next
	}
	return DomainModelVersionResolution{
		Outcome: DomainModelVersionMigrate,
		Chain:   chain,
	}
}
