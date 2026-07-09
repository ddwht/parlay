// parlay-feature: studio-support/page-layout-field
// parlay-section: cross-cutting
// parlay-cross-cutting-id: layout-precheck-contract
//
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
//   - Sub-millisecond on common-case pages.
//   - Returns the FIRST failure within a single page; aggregation across
//     pages is the caller's responsibility.
//   - Never auto-fixes. Never enforces policy. The caller decides whether
//     to refuse-to-proceed, annotate state, or surface to the human.
//   - When page.Layout == nil: returns Verdict{Code: "ok"}. A page without
//     a layout is trivially valid for this contract.

package agent

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/core/internal/parser"
)

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
// exhaustive surface for this feature.
const (
	VerdictMalformedLayoutBlock       = "malformed-layout-block"
	VerdictMissingSchemaVersion       = "missing-schema-version"
	VerdictVocabularyVersionMismatch  = "vocabulary-version-mismatch"
	VerdictUnknownComponentType       = "unknown-component-type"
	VerdictRawValueWhereTokenRequired = "raw-value-where-token-required"
	VerdictWiringInLayout             = "wiring-in-layout"
)

// LayoutPrecheck aggregates layout validation for a single page into a
// single Verdict. Pages without a layout return Verdict{Code: "ok"}.
//
// The check order is deterministic:
//
//  1. nil-layout shortcut (ok)
//  2. block-level shape (missing schema_version)
//  3. schema-version compatibility
//  4. vocabulary version match
//  5. per-node walk (membership, tokens, wiring)
//
// The first failure short-circuits — callers get one verdict per page.
func LayoutPrecheck(page *parser.Page, adapter *deepAdapter) Verdict {
	if page == nil || page.Layout == nil {
		return Verdict{Code: "ok"}
	}
	pagePath := pageFilePath(page)
	layout := page.Layout

	// (1) Block-level shape. The parser already rejects layouts that
	// omit componentVocabulary or schema_version, but a synthetically
	// constructed Page (e.g., from a non-parser test harness) can still
	// reach LayoutPrecheck with the field zero — guard accordingly.
	if layout.SchemaVersion == 0 {
		return Verdict{
			Code:     VerdictMissingSchemaVersion,
			File:     pagePath,
			Found:    "",
			Expected: "schema_version: <integer>",
			Fix:      "add schema_version at the top of the ## Layout block",
		}
	}

	// (2) Schema-version compatibility — the build understands a single
	// version at a time. Future versions land via parlay upgrade.
	if layout.SchemaVersion != SupportedLayoutSchemaVersion {
		return Verdict{
			Code:     VerdictMalformedLayoutBlock,
			File:     pagePath,
			NodePath: "",
			Found:    fmt.Sprintf("schema_version: %d", layout.SchemaVersion),
			Expected: fmt.Sprintf("schema_version: %d (the version this build supports)", SupportedLayoutSchemaVersion),
			Fix:      fmt.Sprintf("either downgrade the layout to schema_version %d, or run parlay upgrade to install a build that understands schema_version %d", SupportedLayoutSchemaVersion, layout.SchemaVersion),
		}
	}

	// (3) Vocabulary version match. The adapter's componentVocabulary
	// declares the version this build is configured to render against;
	// the layout pins itself to a specific version. Mismatch is a
	// dedicated verdict so the caller can phrase remediation precisely.
	if adapter != nil && adapter.ComponentVocabulary != nil {
		if layout.ComponentVocabulary != "" && layout.ComponentVocabulary != adapter.ComponentVocabulary.Name {
			return Verdict{
				Code:     VerdictVocabularyVersionMismatch,
				File:     pagePath,
				NodePath: "",
				Found:    layout.ComponentVocabulary,
				Expected: adapter.ComponentVocabulary.Name,
				Fix: fmt.Sprintf(
					"either update the layout to pin %q, or upgrade the adapter to declare %q",
					adapter.ComponentVocabulary.Name, layout.ComponentVocabulary,
				),
			}
		}
	}

	// (4) Per-node walk. Returns the FIRST failure encountered.
	if v := walkPagePrecheck(adapter, layout, layout.Nodes, "", pagePath); v.Code != "" && v.Code != "ok" {
		return v
	}

	return Verdict{Code: "ok"}
}

// ParsePagePrecheck is a convenience for callers that have a path
// rather than a parsed Page. It parses the file, then calls
// LayoutPrecheck. Parse failures translate to Verdict{Code:
// malformed-layout-block} carrying the verbatim parser error.
func ParsePagePrecheck(path string, adapter *deepAdapter) Verdict {
	page, err := parser.ParsePageFile(path)
	if err != nil {
		// Translate parser-level missing-schema-version into the
		// dedicated verdict so callers can distinguish "schema_version
		// absent" from "block YAML didn't even parse".
		emsg := err.Error()
		if strings.Contains(emsg, "missing required field 'schema_version'") {
			return Verdict{
				Code:     VerdictMissingSchemaVersion,
				File:     path,
				Found:    "",
				Expected: "schema_version: <integer>",
				Fix:      "add schema_version at the top of the ## Layout block",
			}
		}
		// Wiring-in-layout — parser surfaces it as "wiring field 'X' on node 'Y'".
		if strings.Contains(emsg, "wiring field '") {
			found := extractQuoted(emsg, "wiring field '")
			node := extractQuoted(emsg, "node '")
			return Verdict{
				Code:     VerdictWiringInLayout,
				File:     path,
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
			return Verdict{
				Code:     VerdictRawValueWhereTokenRequired,
				File:     path,
				NodePath: node,
				Found:    found,
				Expected: spacingExpectedForAdapter(adapter),
				Fix:      fmt.Sprintf("replace %q on field %q with a spacing token from the active adapter", found, field),
			}
		}
		// Catch-all: malformed layout block.
		nextStep := "rerun the parser locally and address the surfaced error before regenerating the page artifact"
		return Verdict{
			Code:     VerdictMalformedLayoutBlock,
			File:     path,
			NodePath: "",
			Found:    emsg,
			Expected: "well-formed YAML conforming to the layout schema",
			Fix:      nextStep,
		}
	}
	return LayoutPrecheck(page, adapter)
}

// pageFilePath returns the source path the page was parsed from. The
// parser stores the path on the *parser.Page indirectly (via the
// frontmatter), but Verdict needs it explicitly. When the field isn't
// available, we fall back to the page Name.
func pageFilePath(page *parser.Page) string {
	if page == nil {
		return ""
	}
	// parser.Page does not currently surface its own path; downstream
	// callers populate it via wrapper structs. For the contract here,
	// fall back to the page Name when no other channel is available.
	return page.Name
}

// walkPagePrecheck walks the layout tree and returns the first failure
// verdict it finds, or {} on full pass. Recursion is bounded by tree
// depth.
func walkPagePrecheck(adapter *deepAdapter, layout *parser.Layout, nodes []parser.LayoutNode, parentPath string, pagePath string) Verdict {
	for i, node := range nodes {
		nodePath := nodePathOf(parentPath, i, node)

		// Wiring fields land in node.Properties because they are not
		// part of the universal field set. The parser already rejects
		// these on disk; this branch covers synthetically-constructed
		// pages.
		for name := range node.Properties {
			if isWiringField(name) {
				return Verdict{
					Code:     VerdictWiringInLayout,
					File:     pagePath,
					NodePath: nodePath,
					Found:    name,
					Expected: "layout node fields without wiring",
					Fix:      "move wiring out of the layout block — wiring is the responsibility of the layout-aware codegen pass",
				}
			}
		}

		// Raw-value-where-token-required for universal spacing fields.
		if v := checkRawSpacing(node.Gap, "gap", node, nodePath, pagePath, adapter); v.Code != "" {
			return v
		}
		if v := checkRawSpacing(node.Padding, "padding", node, nodePath, pagePath, adapter); v.Code != "" {
			return v
		}

		// Component-type membership.
		if adapter != nil && adapter.ComponentVocabulary != nil {
			known := false
			for _, c := range adapter.ComponentVocabulary.Components {
				if c.Type == node.Type {
					known = true
					break
				}
			}
			if !known {
				return Verdict{
					Code:     VerdictUnknownComponentType,
					File:     pagePath,
					NodePath: nodePath,
					Found:    node.Type,
					Expected: typeListForVocabulary(adapter),
					Fix:      fmt.Sprintf("either pick a known type from %s or upgrade the adapter to declare %q", adapter.ComponentVocabulary.Name, node.Type),
				}
			}
		}

		// Recurse.
		if v := walkPagePrecheck(adapter, layout, node.Children, nodePath, pagePath); v.Code != "" {
			return v
		}
	}
	return Verdict{}
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

// checkRawSpacing returns a non-empty Verdict when value is a literal
// dimension (e.g. "24px", "16") instead of a token name. It is the
// runtime mirror of parser.isRawSpacingValue, scoped to this package.
func checkRawSpacing(value, field string, node parser.LayoutNode, nodePath, pagePath string, adapter *deepAdapter) Verdict {
	if value == "" {
		return Verdict{}
	}
	if !looksRawSpacing(value) {
		return Verdict{}
	}
	return Verdict{
		Code:     VerdictRawValueWhereTokenRequired,
		File:     pagePath,
		NodePath: nodePath,
		Found:    value,
		Expected: spacingExpectedForAdapter(adapter),
		Fix:      fmt.Sprintf("replace %q on field %q with a spacing token from the active adapter", value, field),
	}
}

func looksRawSpacing(v string) bool {
	if v == "" {
		return false
	}
	first := v[0]
	return first == '-' || first == '+' || (first >= '0' && first <= '9')
}

func spacingExpectedForAdapter(adapter *deepAdapter) string {
	if adapter == nil || adapter.Tokens == nil || len(adapter.Tokens.Spacing) == 0 {
		return "a spacing token name (no tokens are declared in the active adapter)"
	}
	names := make([]string, 0, len(adapter.Tokens.Spacing))
	for _, t := range adapter.Tokens.Spacing {
		names = append(names, t.Name)
	}
	return "one of {" + strings.Join(names, ", ") + "}"
}

func typeListForVocabulary(adapter *deepAdapter) string {
	if adapter == nil || adapter.ComponentVocabulary == nil {
		return "the active adapter's vocabulary"
	}
	names := make([]string, 0, len(adapter.ComponentVocabulary.Components))
	for _, c := range adapter.ComponentVocabulary.Components {
		names = append(names, c.Type)
	}
	return adapter.ComponentVocabulary.Name + " types {" + strings.Join(names, ", ") + "}"
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

