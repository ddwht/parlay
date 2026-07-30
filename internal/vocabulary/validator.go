// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/vocabulary-validator-library

package vocabulary

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Node is the typed shape the validator walks. Callers may supply either
// a single Node (the read-back-mode invocation) or a Layout whose Root is
// a Node (the pre-flight invocation). The shape is intentionally loose —
// the validator does not own the layout schema, it just walks whatever
// the caller hands it as long as the field names line up.
type Node struct {
	// Path is the address of this node within the layout — e.g.
	// "root.sidebar[0].button". Reported verbatim in entry.NodePath.
	Path string `yaml:"path" json:"path"`
	// Type is the component type to validate against vocab.Components.
	Type string `yaml:"type" json:"type"`
	// Properties is the per-node property bag. Property names not in the
	// resolved ComponentSpec.Properties trigger RulePropertyCheck.
	Properties map[string]any `yaml:"properties" json:"properties"`
	// Variants is the per-axis variant selection. Values not in the
	// resolved ComponentSpec.Variants[axis] enum trigger RuleVariantCheck.
	Variants map[string]string `yaml:"variants" json:"variants"`
	// Spacing is the spacing/padding/gap bag — values are inspected by the
	// spacing-token check. Token names resolve against vocab.SpacingTokens.
	Spacing map[string]any `yaml:"spacing" json:"spacing"`
	// Color is the color-bag — values are inspected by the color-token
	// check. Token names resolve against vocab.ColorTokens.
	Color map[string]any `yaml:"color" json:"color"`
	// LayoutParameters is the layout-container parameter bag. Names + values
	// resolve against the matching LayoutContainerSpec.
	LayoutParameters map[string]any `yaml:"layout_parameters" json:"layout_parameters"`
	// Aliases lets callers declare that a spacing or color token is an
	// alias-to-non-canonical — used by the token checks to surface a
	// warning (not an error) for "resolves but isn't canonical."
	Aliases map[string]string `yaml:"aliases" json:"aliases"`
	// Children is the typed-tree continuation. Walk order is deterministic
	// (slice order); the validator does not sort children.
	Children []Node `yaml:"children" json:"children"`
}

// Layout is the optional outer wrapper. Callers may pass either a Layout
// or a Node directly to Validate.
type Layout struct {
	Root Node `yaml:"root" json:"root"`
}

// Validate runs the six closed checks against the layout. The layout
// argument is `any` so callers can pass Layout (full-layout mode), Node
// (single-node read-back mode), or a *Layout / *Node pointer. Any other
// type returns a Report with no entries — the validator is non-fatal on
// unknown inputs by design; the schema-level "this layout failed to
// parse" surface lives in the caller, not here.
//
// Purity invariants:
//   - Zero network sockets.
//   - Zero filesystem reads.
//   - Zero AI calls.
//   - Zero Figma MCP tool invocations.
//
// The validator walks the tree in deterministic order (depth-first,
// children in slice order) and runs the six checks per applicable node.
// It does NOT short-circuit: every applicable check fires against every
// applicable node, so the report carries the full set of failures in one
// pass. The validator never emits the derived in-vocabulary /
// out-of-vocabulary signal; that lives in callers.
func Validate(ctx context.Context, layout any, vocab Vocabulary) Report {
	// Suppress unused-import warning while keeping the contextual hook
	// for cancellation in future. We accept ctx so callers can plumb in a
	// cancellable context; the current six checks are O(nodes) and cheap,
	// so we do not check ctx.Done() inside the walk. If this changes,
	// add a select on ctx.Done() inside walk().
	_ = ctx
	r := Report{}
	switch v := layout.(type) {
	case Layout:
		walk(v.Root, &r, vocab)
	case *Layout:
		if v != nil {
			walk(v.Root, &r, vocab)
		}
	case Node:
		walk(v, &r, vocab)
	case *Node:
		if v != nil {
			walk(*v, &r, vocab)
		}
	}
	return r
}

// walk descends the typed tree depth-first in declaration order, running
// the six checks against each applicable node. Type errors do not skip
// the remaining checks — Suite 1 "No short-circuiting" pins that
// invariant.
func walk(n Node, r *Report, vocab Vocabulary) {
	spec, knownType := lookupComponent(vocab, n.Type)

	// Check 1: type-check.
	if !knownType {
		r.Entries = append(r.Entries, Entry{
			NodePath: n.Path,
			Rule:     RuleTypeCheck,
			Expected: componentTypeNames(vocab),
			Actual:   n.Type,
			Severity: SeverityError,
		})
	}

	// Check 2: property-check.
	if knownType {
		for _, propName := range sortedKeys(n.Properties) {
			if !contains(spec.Properties, propName) {
				r.Entries = append(r.Entries, Entry{
					NodePath: n.Path,
					Rule:     RulePropertyCheck,
					Expected: spec.Properties,
					Actual:   propName,
					Severity: SeverityError,
				})
			}
		}
	}

	// Check 3: variant-check.
	if knownType {
		for _, axis := range sortedKeys(n.Variants) {
			value := n.Variants[axis]
			allowed, axisKnown := spec.Variants[axis]
			if !axisKnown {
				r.Entries = append(r.Entries, Entry{
					NodePath: n.Path,
					Rule:     RuleVariantCheck,
					Expected: fmt.Sprintf("known variant axis on %s", n.Type),
					Actual:   axis,
					Severity: SeverityError,
				})
				continue
			}
			if !contains(allowed, value) {
				r.Entries = append(r.Entries, Entry{
					NodePath: n.Path,
					Rule:     RuleVariantCheck,
					Expected: allowed,
					Actual:   value,
					Severity: SeverityError,
				})
			}
		}
	}

	// Check 4: spacing-token-check.
	for _, name := range sortedKeys(n.Spacing) {
		raw := n.Spacing[name]
		emitTokenCheck(r, n, raw, n.Aliases[stringifyKey(raw)], vocab.SpacingTokens, RuleSpacingTokenCheck)
	}

	// Check 5: color-token-check.
	for _, name := range sortedKeys(n.Color) {
		raw := n.Color[name]
		emitTokenCheck(r, n, raw, n.Aliases[stringifyKey(raw)], vocab.ColorTokens, RuleColorTokenCheck)
	}

	// Check 6: layout-container-check.
	if knownType {
		lcSpec, isContainer := lookupLayoutContainer(vocab, n.Type)
		if isContainer {
			for _, name := range sortedKeys(n.LayoutParameters) {
				value := n.LayoutParameters[name]
				if !contains(lcSpec.AdmissibleParameters, name) {
					r.Entries = append(r.Entries, Entry{
						NodePath: n.Path,
						Rule:     RuleLayoutContainerCheck,
						Expected: lcSpec.AdmissibleParameters,
						Actual:   name,
						Severity: SeverityError,
					})
					continue
				}
				if pc, ok := lcSpec.ParameterConstraints[name]; ok && len(pc.AllowedValues) > 0 {
					if !contains(pc.AllowedValues, stringifyKey(value)) {
						r.Entries = append(r.Entries, Entry{
							NodePath: n.Path,
							Rule:     RuleLayoutContainerCheck,
							Expected: pc.AllowedValues,
							Actual:   value,
							Severity: SeverityError,
						})
					}
				}
			}
		}
	}

	// Recurse into children in slice order — deterministic walk.
	for _, child := range n.Children {
		walk(child, r, vocab)
	}
}

// emitTokenCheck handles the shared spacing-token-check + color-token-check
// emission. Raw literals (numeric, hex strings, anything that does not
// resolve as a token name) produce SeverityError. Values that match a
// token name in the admissible set produce no entry. Values that match a
// declared alias to a NON-canonical-token produce SeverityWarning — the
// "resolves but is not canonical" branch from infrastructure 1.
func emitTokenCheck(r *Report, n Node, raw any, alias string, admissible []string, rule Rule) {
	value := stringifyKey(raw)
	if isRawLiteral(rule, value) {
		r.Entries = append(r.Entries, Entry{
			NodePath: n.Path,
			Rule:     rule,
			Expected: admissible,
			Actual:   raw,
			Severity: SeverityError,
		})
		return
	}
	if contains(admissible, value) {
		return
	}
	if alias != "" {
		// Resolves via alias, but the alias itself is not in the
		// canonical admissible set. Warning, not error.
		if !contains(admissible, alias) {
			r.Entries = append(r.Entries, Entry{
				NodePath: n.Path,
				Rule:     rule,
				Expected: admissible,
				Actual:   raw,
				Severity: SeverityWarning,
			})
			return
		}
		// Alias points at a canonical token — pass.
		return
	}
	// Unknown token name (no alias, not admissible) — error.
	r.Entries = append(r.Entries, Entry{
		NodePath: n.Path,
		Rule:     rule,
		Expected: admissible,
		Actual:   raw,
		Severity: SeverityError,
	})
}

// isRawLiteral classifies values that are obviously raw (not token names)
// for the spacing and color rules: a leading '#' for color, a pure-digit
// or "16px" / "1rem" pattern for spacing.
func isRawLiteral(rule Rule, value string) bool {
	switch rule {
	case RuleColorTokenCheck:
		return strings.HasPrefix(value, "#")
	case RuleSpacingTokenCheck:
		if value == "" {
			return false
		}
		// Any digit-prefixed value is a raw literal (e.g. "16", "16px",
		// "1.5rem"). Token names never start with a digit.
		c := value[0]
		return c >= '0' && c <= '9'
	}
	return false
}

func lookupComponent(vocab Vocabulary, t string) (ComponentSpec, bool) {
	for _, c := range vocab.Components {
		if c.Name == t {
			return c, true
		}
	}
	return ComponentSpec{}, false
}

func lookupLayoutContainer(vocab Vocabulary, t string) (LayoutContainerSpec, bool) {
	for _, lc := range vocab.LayoutContainers {
		if lc.ContainerType == t {
			return lc, true
		}
	}
	return LayoutContainerSpec{}, false
}

func componentTypeNames(vocab Vocabulary) []string {
	out := make([]string, 0, len(vocab.Components))
	for _, c := range vocab.Components {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func stringifyKey(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
