package agent

// parlay-feature: studio-support/page-layout-field
// parlay-feature: studio-support/adapter-vocabulary-extension
// parlay-section: cross-cutting
// parlay-cross-cutting-id: layout-precheck-contract
//
// Layout validation, consolidated (Phase 6.3/6.4) out of validate.go and
// precheck.go into a single file and a single walk. Two stacks used to
// exist here — agent.ValidateLayout over a package-private agent.LayoutNode
// tree, and agent.LayoutPrecheck over parser.LayoutNode — with duplicated
// checks, different node types, different result shapes, and non-canonical
// error codes on the full-list side. This file picks parser.LayoutNode (the
// type pages actually parse into) as the single node representation,
// implements every check once in walkLayoutViolations, and exposes two thin
// wrappers: ValidateLayoutDeep (full list, for programmatic/CLI use) and
// LayoutPrecheck (closed-shape first-failure Verdict, for gate consumers —
// codegen, view-page, lock-page, status, repair, sync).

import (
	"fmt"
	"os"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

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
	Shows               map[string]interface{}   `yaml:"shows"`
	Actions             map[string]interface{}   `yaml:"actions"`
	Flows               map[string]interface{}   `yaml:"flows"`
	ComponentVocabulary *deepComponentVocabulary `yaml:"componentVocabulary,omitempty"`
	Tokens              *deepAdapterTokens       `yaml:"tokens,omitempty"`
	// Vocabulary is a minimal local mirror of the top-level `vocabulary:`
	// block — just enough fields for CheckVocabularyBlockParity to compare
	// against ComponentVocabulary/Tokens. This is deliberately NOT the full
	// Vocabulary/ComponentSpec/LayoutContainerSpec shape studio/pkg/vocabulary
	// owns — importing that package from core would cross a module boundary
	// this consolidation doesn't take on. See vocabulary.schema.md's
	// "Cross-block parity check" section.
	Vocabulary *deepVocabularyBlock `yaml:"vocabulary,omitempty"`
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

// deepVocabularyBlock is a minimal local mirror of the adapter's top-level
// `vocabulary:` block — see the Vocabulary field comment on deepAdapter for
// why this isn't studio/pkg/vocabulary.Vocabulary itself. Only the fields
// CheckVocabularyBlockParity compares are represented.
type deepVocabularyBlock struct {
	Components    []deepVocabularyBlockComponent `yaml:"components"`
	SpacingTokens []string                       `yaml:"spacing_tokens"`
	ColorTokens   []string                       `yaml:"color_tokens"`
}

type deepVocabularyBlockComponent struct {
	Name string `yaml:"name"`
}

// SupportedLayoutSchemaVersion is the layout schema version this Core
// build understands. Layouts with a different (non-zero) schema_version
// fail validation with code "layout-schema-version-unsupported" (full-list
// shape) / "malformed-layout-block" (precheck's closed-shape Verdict — see
// toVerdictCode).
//
// parlay-extends: studio-support/page-layout-field/layout-tree-schema-validator
const SupportedLayoutSchemaVersion = 1

// Adapter is the exported alias for the parsed adapter shape that layout
// validation and precheck operate against. It is a type alias (not a new
// type) so any value produced by LoadAdapterFile is directly usable
// wherever the package already expects *deepAdapter, and so callers
// outside this package (CLI wiring, view-page, lock-page) can hold and
// pass one without needing an unexported type.
type Adapter = deepAdapter

// LoadAdapterFile reads and parses an adapter YAML file for callers
// outside this package that need to run layout validation or precheck —
// e.g. the CLI wiring in commands/validate.go and the view-page/lock-page
// precheck gate. Mirrors the read+unmarshal shape validateAdapterVocabulary
// already uses internally for buildfile-deep validation.
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

// Verdict is the closed-shape result of a single LayoutPrecheck call.
// On success, Code is "ok" and ALL OTHER FIELDS ARE EMPTY. On failure,
// Code is one of the stable failure codes registered below and ALL SIX
// FIELDS are populated. There is no severity field — every failure is
// an error. Adding a field requires bumping the contract.
type Verdict struct {
	Code     string // "ok" | one of the stable failure codes below
	File     string // page path; empty on success
	NodePath string // dot-path inside the layout tree; empty on success or block-level failures
	Found    string // offending value or verbatim parser error; empty on success
	Expected string // expected shape or list of valid values; empty on success
	Fix      string // human-readable remediation; empty on success
}

// Stable failure codes registered by the page-layout-field feature.
// Sibling features add their own codes in lockstep; this set is the
// exhaustive closed set the precheck registers — see layout.schema.md's
// "Validation pass" section. Adjacent codes owned by sibling features
// (unknown-variant, unknown-token, universal-field-redeclared,
// missing-mode-emit-form) surface through the same Verdict shape without
// a dedicated constant here — their string values are what the shared
// walk already produces.
const (
	VerdictMalformedLayoutBlock       = "malformed-layout-block"
	VerdictMissingSchemaVersion       = "missing-schema-version"
	VerdictVocabularyVersionMismatch  = "vocabulary-version-mismatch"
	VerdictUnknownComponentType       = "unknown-component-type"
	VerdictRawValueWhereTokenRequired = "raw-value-where-token-required"
	VerdictWiringInLayout             = "wiring-in-layout"
)

// layoutViolation is the internal shape every layout check produces. Both
// exported entry points — ValidateLayoutDeep (full list) and LayoutPrecheck
// (closed-shape first failure) — are thin wrappers over the same walk
// (walkLayoutViolations), never two separate walks: ValidateLayoutDeep maps
// every violation to a ValidationError; LayoutPrecheck takes the first one
// and maps it to a Verdict.
type layoutViolation struct {
	Code     string
	Message  string
	NodePath string
	Found    string
	Expected string
	Fix      string
}

// toVerdictCode translates a layoutViolation's code into the precheck's
// closed-shape code set. Every code passes through unchanged except
// layout-schema-version-unsupported: layout.schema.md's "Validation pass"
// section registers that finer-grained code only for the per-rule
// (full-list) validator and folds the same underlying violation into
// malformed-layout-block for the precheck's closed contract.
func toVerdictCode(code string) string {
	if code == "layout-schema-version-unsupported" {
		return VerdictMalformedLayoutBlock
	}
	return code
}

// ValidateLayoutDeep is the per-rule (full-list) layout validator described
// in layout.schema.md's "Validation pass" section: it walks every node in
// the tree and returns every violation found, with stable codes for
// programmatic handling. Nil layout returns nil. LayoutPrecheck wraps the
// same underlying walk into the closed-shape Verdict precheck contract —
// see walkLayoutViolations.
//
// parlay-extends: studio-support/adapter-vocabulary-extension/adapter-validator-deep-vocabulary-and-tokens
// parlay-extends: studio-support/page-layout-field/layout-tree-schema-validator
func ValidateLayoutDeep(layout *parser.Layout, adapter *Adapter) []ValidationError {
	violations := walkLayoutViolations(layout, adapter)
	if len(violations) == 0 {
		return nil
	}
	errs := make([]ValidationError, len(violations))
	for i, v := range violations {
		errs[i] = ValidationError{Code: v.Code, Message: v.Message, Context: v.NodePath, Fix: v.Fix}
	}
	return errs
}

// LayoutPrecheck — the closed-shape validation contract every layout
// consumer (codegen, status, repair, sync) calls uniformly. Aggregates
// every layout check (well-formedness, schema compliance, vocabulary
// membership, token correctness, no-wiring-in-layout) into a structured
// Verdict that callers branch on without having to know which subsystem
// owns each check.
//
// The contract:
//
//   - Pure function over its inputs. No file I/O. No network. No AI calls.
//     No external state.
//   - Deterministic: same (page, adapter) → byte-identical Verdict every
//     call.
//   - Returns the FIRST failure within a single page; aggregation across
//     pages is the caller's responsibility.
//   - Never auto-fixes. Never enforces policy. The caller decides whether
//     to refuse-to-proceed, annotate state, or surface to the human.
//   - When page.Layout == nil: returns Verdict{Code: "ok"}. A page without
//     a layout is trivially valid for this contract.
//
// parlay-feature: studio-support/page-layout-field
// parlay-cross-cutting-id: layout-precheck-contract
func LayoutPrecheck(page *parser.Page, adapter *Adapter) Verdict {
	if page == nil || page.Layout == nil {
		return Verdict{Code: "ok"}
	}
	pagePath := pageFilePath(page)
	violations := walkLayoutViolations(page.Layout, adapter)
	if len(violations) == 0 {
		return Verdict{Code: "ok"}
	}
	v := violations[0]
	return Verdict{
		Code:     toVerdictCode(v.Code),
		File:     pagePath,
		NodePath: v.NodePath,
		Found:    v.Found,
		Expected: v.Expected,
		Fix:      v.Fix,
	}
}

// ParsePagePrecheck is a convenience for callers that have a path
// rather than a parsed Page. It parses the file, then calls
// LayoutPrecheck. Parse failures translate to Verdict{Code:
// malformed-layout-block} carrying the verbatim parser error.
func ParsePagePrecheck(path string, adapter *Adapter) Verdict {
	page, err := parser.ParsePageFile(path)
	if err != nil {
		v := parseErrorToLayoutViolation(err, adapter)
		return Verdict{
			Code:     toVerdictCode(v.Code),
			File:     path,
			NodePath: v.NodePath,
			Found:    v.Found,
			Expected: v.Expected,
			Fix:      v.Fix,
		}
	}
	return LayoutPrecheck(page, adapter)
}

// ValidateLayoutParseError translates a page/layout parse failure (e.g.
// from parser.ParsePageFile) into the same stable-coded ValidationError
// shape ValidateLayoutDeep produces, so a CLI caller sees one consistent
// error surface whether the page failed to parse at all or parsed but
// failed a semantic check. Shares parseErrorToLayoutViolation with
// ParsePagePrecheck's Verdict-shaped translation — one translation, two
// wrapper shapes, same as the rest of this file.
func ValidateLayoutParseError(err error, adapter *Adapter) []ValidationError {
	v := parseErrorToLayoutViolation(err, adapter)
	return []ValidationError{{Code: v.Code, Message: v.Message, Context: v.NodePath, Fix: v.Fix}}
}

// LayoutParseErrorVerdict translates a page/layout parse failure into the
// closed-shape Verdict, for callers that already hold the parse error
// from a call of their own (e.g. a caller resolving a standalone
// *.layout.yaml through a synthetic wrapper) rather than a path
// ParsePagePrecheck could re-parse — re-parsing would either fail
// differently (the path isn't a page artifact) or, worse, silently
// succeed as a layout-free page. Shares parseErrorToLayoutViolation with
// ParsePagePrecheck and ValidateLayoutParseError.
func LayoutParseErrorVerdict(path string, err error, adapter *Adapter) Verdict {
	v := parseErrorToLayoutViolation(err, adapter)
	return Verdict{
		Code:     toVerdictCode(v.Code),
		File:     path,
		NodePath: v.NodePath,
		Found:    v.Found,
		Expected: v.Expected,
		Fix:      v.Fix,
	}
}

// parseErrorToLayoutViolation translates a parser.ParsePageFile error
// string into the layoutViolation shape. The parser rejects a handful of
// shape violations before agent-level validation ever runs (missing
// schema_version, wiring fields, raw spacing values) — this recovers the
// stable code and Found/Expected/Fix for those cases from the parser's
// error text so a parse failure and a semantic validation failure look
// the same to a caller. Anything else falls through to malformed-layout-block.
func parseErrorToLayoutViolation(err error, adapter *Adapter) layoutViolation {
	emsg := err.Error()
	if strings.Contains(emsg, "missing required field 'schema_version'") {
		return layoutViolation{
			Code:     "missing-schema-version",
			Message:  emsg,
			Found:    "",
			Expected: "schema_version: <integer>",
			Fix:      "add schema_version at the top of the ## Layout block",
		}
	}
	// Wiring-in-layout — parser surfaces it as "wiring field 'X' on node 'Y'".
	if strings.Contains(emsg, "wiring field '") {
		found := extractQuoted(emsg, "wiring field '")
		node := extractQuoted(emsg, "node '")
		return layoutViolation{
			Code:     "wiring-in-layout",
			Message:  emsg,
			NodePath: node,
			Found:    found,
			Expected: "layout node fields without wiring",
			Fix:      "move wiring out of the layout block — wiring is the responsibility of the layout-aware codegen pass",
		}
	}
	// Raw-value-where-token-required — parser surfaces it as
	// "raw value 'X' for field 'Y' on node 'Z'".
	if strings.Contains(emsg, "raw value '") && strings.Contains(emsg, "spacing token reference is required") {
		found := extractQuoted(emsg, "raw value '")
		field := extractQuoted(emsg, "field '")
		node := extractQuoted(emsg, "node '")
		return layoutViolation{
			Code:     "raw-value-where-token-required",
			Message:  emsg,
			NodePath: node,
			Found:    found,
			Expected: spacingExpectedForAdapter(adapter),
			Fix:      fmt.Sprintf("replace %q on field %q with a spacing token from the active adapter", found, field),
		}
	}
	// Catch-all: malformed layout block.
	return layoutViolation{
		Code:     "malformed-layout-block",
		Message:  emsg,
		NodePath: "",
		Found:    emsg,
		Expected: "well-formed YAML conforming to the layout schema",
		Fix:      "rerun the parser locally and address the surfaced error before regenerating the page artifact",
	}
}

// pageFilePath returns the source path the page was parsed from. The
// parser stores the path on the *parser.Page indirectly (via the
// frontmatter), but Verdict needs it explicitly. When the field isn't
// available, we fall back to the page Name.
func pageFilePath(page *parser.Page) string {
	if page == nil {
		return ""
	}
	return page.Name
}

// walkLayoutViolations is the single shared walk both ValidateLayoutDeep
// and LayoutPrecheck derive from. Block-level gates (missing/unsupported
// schema_version, vocabulary mismatch) fail fast with exactly one
// violation before any node is examined; the per-node walk otherwise
// collects every violation in the tree, in a fixed deterministic order.
func walkLayoutViolations(layout *parser.Layout, adapter *Adapter) []layoutViolation {
	if layout == nil {
		return nil
	}

	// (1) Missing schema_version — layout-intrinsic, adapter-independent.
	// The parser already rejects on-disk layouts omitting schema_version;
	// this guards synthetically-constructed *parser.Layout values (e.g.
	// from a non-parser test harness or a hand-built standalone layout.yaml
	// read some other way) that can still reach this walk with the field
	// zero.
	if layout.SchemaVersion == 0 {
		return []layoutViolation{{
			Code:     "missing-schema-version",
			Message:  "layout is missing schema_version",
			NodePath: "",
			Found:    "",
			Expected: "schema_version: <integer>",
			Fix:      "add schema_version at the top of the layout block",
		}}
	}

	// (2) Schema-version compatibility — the build understands a single
	// version at a time. Future versions land via parlay upgrade.
	if layout.SchemaVersion != SupportedLayoutSchemaVersion {
		return []layoutViolation{{
			Code:     "layout-schema-version-unsupported",
			Message:  fmt.Sprintf("layout schema_version %d is not supported by this build (expected %d)", layout.SchemaVersion, SupportedLayoutSchemaVersion),
			NodePath: "",
			Found:    fmt.Sprintf("schema_version: %d", layout.SchemaVersion),
			Expected: fmt.Sprintf("schema_version: %d (the version this build supports)", SupportedLayoutSchemaVersion),
			Fix:      fmt.Sprintf("either downgrade the layout to schema_version %d, or run parlay upgrade to install a build that understands schema_version %d", SupportedLayoutSchemaVersion, layout.SchemaVersion),
		}}
	}

	// (3) Vocabulary version match — fails fast before any node lookup.
	if adapter != nil && adapter.ComponentVocabulary != nil &&
		layout.ComponentVocabulary != "" && layout.ComponentVocabulary != adapter.ComponentVocabulary.Name {
		return []layoutViolation{{
			Code:     "vocabulary-version-mismatch",
			Message:  fmt.Sprintf("layout pins vocabulary %q but the active adapter declares %q — fails fast before any component lookup", layout.ComponentVocabulary, adapter.ComponentVocabulary.Name),
			NodePath: "",
			Found:    layout.ComponentVocabulary,
			Expected: adapter.ComponentVocabulary.Name,
			Fix: fmt.Sprintf("either update the layout to pin %q, or upgrade the adapter to declare %q",
				adapter.ComponentVocabulary.Name, layout.ComponentVocabulary),
		}}
	}

	// Index vocabulary + tokens for the per-node walk, when an adapter is
	// available. A nil adapter (or one with no componentVocabulary/tokens)
	// degrades gracefully: node-shape checks that don't need the adapter
	// (wiring, raw-vs-token) still run; adapter-dependent checks
	// (component-type membership, variant, property, disallowed-child,
	// token membership) are skipped rather than firing spuriously.
	var componentByType map[string]*deepVocabularyComponent
	var declaredTypesList, vocabName string
	if adapter != nil && adapter.ComponentVocabulary != nil {
		vocabName = adapter.ComponentVocabulary.Name
		componentByType = map[string]*deepVocabularyComponent{}
		declaredTypes := make([]string, 0, len(adapter.ComponentVocabulary.Components))
		for i := range adapter.ComponentVocabulary.Components {
			c := &adapter.ComponentVocabulary.Components[i]
			componentByType[c.Type] = c
			declaredTypes = append(declaredTypes, c.Type)
		}
		declaredTypesList = strings.Join(declaredTypes, ", ")
	}
	spacingNames := map[string]bool{}
	var spacingListStr string
	if adapter != nil && adapter.Tokens != nil {
		spacingList := make([]string, 0, len(adapter.Tokens.Spacing))
		for _, st := range adapter.Tokens.Spacing {
			spacingNames[st.Name] = true
			spacingList = append(spacingList, st.Name)
		}
		spacingListStr = strings.Join(spacingList, ", ")
	}

	var violations []layoutViolation
	walkLayoutNodes(adapter, layout.Nodes, "", componentByType, declaredTypesList, vocabName, spacingNames, spacingListStr, "", &violations)
	return violations
}

// walkLayoutNodes recurses into a layout tree collecting every violation.
// parentType is the actual parent node's declared type (empty at the
// root) — real parser.Layout trees are properly nested, so the effective
// parent for disallowed-child purposes is always the true parent, unlike
// the old agent.LayoutNode test-only override.
func walkLayoutNodes(adapter *Adapter, nodes []parser.LayoutNode, parentType string, componentByType map[string]*deepVocabularyComponent, declaredTypesList, vocabName string, spacingNames map[string]bool, spacingListStr string, parentPath string, violations *[]layoutViolation) {
	for i, node := range nodes {
		nodePath := nodePathOf(parentPath, i, node)

		// (a) wiring-in-layout. Wiring fields land in node.Properties
		// because they are not part of the universal field set. The
		// parser already rejects these on disk; this covers
		// synthetically-constructed layouts.
		for name := range node.Properties {
			if isWiringField(name) {
				*violations = append(*violations, layoutViolation{
					Code:     "wiring-in-layout",
					Message:  fmt.Sprintf("layout node %q carries wiring field %q — wiring is codegen's concern, not the layout's", nodePath, name),
					NodePath: nodePath,
					Found:    name,
					Expected: "layout node fields without wiring",
					Fix:      "move wiring out of the layout block — wiring is the responsibility of the layout-aware codegen pass",
				})
			}
		}

		// (b) raw-value-where-token-required / unknown-token for the
		// universal spacing fields.
		checkSpacingField(node.Gap, "gap", nodePath, spacingNames, spacingListStr, adapter, violations)
		checkSpacingField(node.Padding, "padding", nodePath, spacingNames, spacingListStr, adapter, violations)

		// (b2) universal-field-value-invalid for the two universal container
		// fields whose values are fixed enums rather than adapter tokens.
		checkUniversalEnumField(node.Direction, "direction", layoutDirections, nodePath, violations)
		checkUniversalEnumField(node.Alignment, "alignment", layoutAlignments, nodePath, violations)

		// (c) unknown-component-type.
		comp, known := componentByType[node.Type]
		if componentByType != nil && !known {
			*violations = append(*violations, layoutViolation{
				Code:     "unknown-component-type",
				Message:  fmt.Sprintf("layout references component type %q which is not declared in vocabulary %s", node.Type, vocabName),
				NodePath: nodePath,
				Found:    node.Type,
				Expected: fmt.Sprintf("%s types {%s}", vocabName, declaredTypesList),
				Fix:      fmt.Sprintf("either pick a known type from {%s} or upgrade the adapter %q to declare %q", declaredTypesList, vocabName, node.Type),
			})
		}

		if known {
			// (d) unknown-variant. parser.LayoutNode has no first-class
			// Variant field (unlike the old agent.LayoutNode) — a node's
			// variant is an ordinary inline property, so it's read from
			// Properties["variant"] and excluded from the unknown-property
			// check below.
			if variant, ok := node.Properties["variant"].(string); ok && variant != "" {
				variantOK := false
				for _, v := range comp.Variants {
					if v == variant {
						variantOK = true
						break
					}
				}
				if !variantOK {
					*violations = append(*violations, layoutViolation{
						Code:     "unknown-variant",
						Message:  fmt.Sprintf("layout references component %q with variant %q which is not in the closed enum {%s}", node.Type, variant, strings.Join(comp.Variants, ", ")),
						NodePath: nodePath,
						Found:    variant,
						Expected: strings.Join(comp.Variants, ", "),
						Fix:      fmt.Sprintf("change the variant to one of {%s}", strings.Join(comp.Variants, ", ")),
					})
				}
			}

			// (e) unknown-property — name outside the closed property set
			// declared for this component.
			propByName := map[string]*deepVocabularyProperty{}
			for i := range comp.Properties {
				propByName[comp.Properties[i].Name] = &comp.Properties[i]
			}
			for propName := range node.Properties {
				if propName == "variant" || isWiringField(propName) {
					continue
				}
				if _, ok := propByName[propName]; !ok {
					*violations = append(*violations, layoutViolation{
						Code:     "unknown-property",
						Message:  fmt.Sprintf("layout references property %q on component %q which is not declared in the vocabulary", propName, node.Type),
						NodePath: nodePath,
						Found:    propName,
						Expected: fmt.Sprintf("declared properties of %q", node.Type),
						Fix:      "either add this property to the component's componentVocabulary entry, or remove the reference from the layout",
					})
				}
			}
		}

		// (f) disallowed-child — checked when the real parent's type is
		// known and is a container.
		if parentType != "" && componentByType != nil {
			if parent, ok := componentByType[parentType]; ok && parent.Category == "container" {
				if !contains(parent.AllowedChildren, node.Type) {
					*violations = append(*violations, layoutViolation{
						Code:     "disallowed-child",
						Message:  fmt.Sprintf("layout places %q as a child of container %q whose allowed-children set is {%s} — %q is not in that set", node.Type, parentType, strings.Join(parent.AllowedChildren, ", "), node.Type),
						NodePath: nodePath,
						Found:    node.Type,
						Expected: strings.Join(parent.AllowedChildren, ", "),
						Fix:      fmt.Sprintf("either move the child under a container that allows it, or pick a child from {%s}", strings.Join(parent.AllowedChildren, ", ")),
					})
				}
			}
		}

		// Recurse. The current node becomes the parent type for its
		// children's disallowed-child checks, even when its own type is
		// unknown — deeper errors should still surface.
		walkLayoutNodes(adapter, node.Children, node.Type, componentByType, declaredTypesList, vocabName, spacingNames, spacingListStr, nodePath, violations)
	}
}

// checkSpacingField checks a universal spacing field (gap or padding)
// against the raw-value and declared-token-membership rules. A raw
// literal (e.g. "24px") is raw-value-where-token-required; a non-raw
// value that isn't a declared spacing token name is unknown-token.
// layoutDirections and layoutAlignments are the fixed enums layout.schema.md
// declares for the two universal container fields that are not token
// references. They are schema-owned rather than adapter-declared — "direction
// is always one of the fixed enum {horizontal, vertical}" — so they live here
// as constants rather than being read from the active adapter.
//
// Both were parsed into parser.LayoutNode scalars and then never checked. The
// vocabulary validator does check container parameters against an adapter's
// parameter_constraints, which is why this looked covered: that path reads
// vocabulary.Node.LayoutParameters, and these two are decoded out of the
// property map into typed fields before it ever sees them. So a layout could
// say `direction: sideways` and pass every validator the pipeline runs.
var (
	layoutDirections = []string{"horizontal", "vertical"}
	layoutAlignments = []string{"start", "center", "end", "stretch"}
)

// checkUniversalEnumField reports a universal container field whose value falls
// outside its schema-fixed enum. Empty is not a violation: every universal field
// is optional, and absent means "the framework default", not "invalid".
func checkUniversalEnumField(value, field string, allowed []string, nodePath string, violations *[]layoutViolation) {
	if value == "" {
		return
	}
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	allowedStr := strings.Join(allowed, ", ")
	*violations = append(*violations, layoutViolation{
		Code:     "universal-field-value-invalid",
		Message:  fmt.Sprintf("layout node %q sets %s to %q, which is not in the fixed enum {%s}", nodePath, field, value, allowedStr),
		NodePath: nodePath,
		Found:    value,
		Expected: allowedStr,
		Fix:      fmt.Sprintf("change %s to one of {%s} — the set is fixed by the layout schema, not by the adapter, so it cannot be extended by declaring a token", field, allowedStr),
	})
}

func checkSpacingField(value, field, nodePath string, spacingNames map[string]bool, spacingListStr string, adapter *Adapter, violations *[]layoutViolation) {
	if value == "" {
		return
	}
	if looksRawSpacing(value) {
		*violations = append(*violations, layoutViolation{
			Code:     "raw-value-where-token-required",
			Message:  fmt.Sprintf("layout uses raw value %q for %q — a spacing token reference is required (%s)", value, field, spacingExpectedForAdapter(adapter)),
			NodePath: nodePath,
			Found:    value,
			Expected: spacingExpectedForAdapter(adapter),
			Fix:      fmt.Sprintf("replace %q on field %q with a spacing token from the active adapter", value, field),
		})
		return
	}
	if !spacingNames[value] {
		*violations = append(*violations, layoutViolation{
			Code:     "unknown-token",
			Message:  fmt.Sprintf("layout uses spacing token %q which is not declared (available: %s)", value, spacingListStr),
			NodePath: nodePath,
			Found:    value,
			Expected: spacingListStr,
			Fix:      fmt.Sprintf("either change the value to one of {%s} or add %q to the adapter's tokens.spacing section", spacingListStr, value),
		})
	}
}

// nodePathOf renders a stable dot-path locator for a node — preferring
// the node's id when present, falling back to the index.
func nodePathOf(parentPath string, index int, node parser.LayoutNode) string {
	segment := node.ID
	if segment == "" {
		segment = fmt.Sprintf("[%d]", index)
	}
	if parentPath == "" {
		return segment
	}
	return parentPath + "." + segment
}

// isWiringField mirrors parser.wiringFieldNames — kept local to avoid
// exporting that set across packages.
func isWiringField(name string) bool {
	switch name {
	case "data-source", "dataSource", "binding", "expression", "expression-string":
		return true
	}
	return false
}

// looksRawSpacing reports whether v looks like a literal dimension (e.g.
// "24px", "16") instead of a token name — the runtime mirror of
// parser.isRawSpacingValue, scoped to this package.
func looksRawSpacing(v string) bool {
	if v == "" {
		return false
	}
	first := v[0]
	return first == '-' || first == '+' || (first >= '0' && first <= '9')
}

func spacingExpectedForAdapter(adapter *Adapter) string {
	if adapter == nil || adapter.Tokens == nil || len(adapter.Tokens.Spacing) == 0 {
		return "a spacing token name (no tokens are declared in the active adapter)"
	}
	names := make([]string, 0, len(adapter.Tokens.Spacing))
	for _, t := range adapter.Tokens.Spacing {
		names = append(names, t.Name)
	}
	return "one of {" + strings.Join(names, ", ") + "}"
}

// extractQuoted returns the text between the first occurrence of marker
// and the next single-quote character. Returns "" when not found.
func extractQuoted(s, marker string) string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(marker):]
	end := strings.Index(rest, "'")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

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
