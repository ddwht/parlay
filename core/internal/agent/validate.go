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
// belongs to root <root>. Returns "" when the path doesn't match the
// expected layout, signaling the disk-shape checks should be skipped.
func planRootDirFromBuildfilePath(buildfilePath string) string {
	abs, err := filepath.Abs(buildfilePath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs) // <root>/.parlay/build/<feature>
	for i := 0; i < 3; i++ {
		dir = filepath.Dir(dir)
	}
	// Verify <dir>/.parlay/build/<feature>/buildfile.yaml round-trips.
	if !strings.HasPrefix(abs, dir+string(filepath.Separator)) {
		return ""
	}
	return dir
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
