// parlay-feature: studio-support/page-layout-field
// parlay-section: cross-cutting
// parlay-cross-cutting-id: page-artifact-loader-with-optional-layout
//
// Page-artifact loader that surfaces an optional Layout block. Existing
// pages without a `## Layout` section parse identically to today and
// expose Layout == nil — bit-for-bit-equivalent backward compatibility.
//
// Pages that declare a `## Layout` section have its fenced YAML body
// decoded into a Layout struct with universal-container-field semantics
// (direction, gap, padding, alignment). Wiring fields (data-source,
// dataSource, binding, expression-string fields) are rejected — wiring
// belongs to the layout-aware codegen pass, not to the page artifact.
//
// Vocabulary-against-adapter membership and component-type lookups live
// downstream in agent.ValidateLayout / agent.LayoutPrecheck. This loader
// only enforces shape rules intrinsic to the layout block.

package parser

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Page is the in-memory representation of a parsed page artifact (a
// `spec/pages/<page>.page.md` file). Top-level metadata fields mirror
// the page schema. Layout is optional — nil for pages without a
// `## Layout` section.
type Page struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Owner       string       `yaml:"owner"`
	Status      string       `yaml:"status"`
	Regions     []PageRegion `yaml:"regions"`
	Layout      *Layout      `yaml:"-"` // populated separately from the ## Layout section
}

// PageRegion is a placeholder for the existing page-region structure. The
// concrete shape is owned by the page schema and is not changed by this
// feature; we keep the struct generic so existing pages decode unchanged.
type PageRegion struct {
	Name       string                 `yaml:"name"`
	Components []string               `yaml:"components"`
	Extra      map[string]interface{} `yaml:",inline"`
}

// Layout is the optional layout block carried inside a page artifact.
// ComponentVocabulary pins the layout to a specific design-system
// vocabulary version (e.g., "clarity@17"). SchemaVersion is the layout
// schema's own version, distinct from the vocabulary version. Nodes is
// the recursive tree of layout nodes.
//
// SchemaVersion's YAML tag is schema_version (snake_case) per the
// cross-schema versioning house rule (schema-versioning.schema.md) —
// this was previously the one schema out of step, tagged schemaVersion
// (camelCase). Renamed directly with no dual-key acceptance window:
// the layout feature is new enough (v1, freshly landed) that there is
// no installed base of hand-authored layouts worth bridging.
type Layout struct {
	ComponentVocabulary string       `yaml:"componentVocabulary"`
	SchemaVersion       int          `yaml:"schema_version"`
	Nodes               []LayoutNode `yaml:"nodes"`
}

// LayoutNode is a single node in a layout tree. Universal container
// fields {direction, gap, padding, alignment} live here; vocabulary-
// specific fields are decoded into Properties for downstream validation.
//
// Children is the recursive tree — a LayoutNode may itself contain
// further LayoutNodes to arbitrary depth.
type LayoutNode struct {
	ID         string                 `yaml:"id"`
	Type       string                 `yaml:"type"`
	Children   []LayoutNode           `yaml:"children,omitempty"`
	Direction  string                 `yaml:"direction,omitempty"`
	Gap        string                 `yaml:"gap,omitempty"`
	Padding    string                 `yaml:"padding,omitempty"`
	Alignment  string                 `yaml:"alignment,omitempty"`
	Properties map[string]interface{} `yaml:",inline"`
}

// universalContainerFields is the closed set of layout-schema-owned
// container chrome fields. These are decoded directly into LayoutNode
// scalars, not into Properties.
var universalContainerFields = map[string]bool{
	"id":        true,
	"type":      true,
	"children":  true,
	"direction": true,
	"gap":       true,
	"padding":   true,
	"alignment": true,
}

// wiringFieldNames is the closed set of node-level wiring field names
// that are forbidden inside a layout block. Wiring is the responsibility
// of the layout-aware codegen pass, which consumes the buildfile's
// resolved bindings rather than reading wiring directly from the layout.
var wiringFieldNames = map[string]bool{
	"data-source":       true,
	"dataSource":        true,
	"binding":           true,
	"expression":        true,
	"expression-string": true,
}

// ParsePageFile reads a page artifact from disk and returns the parsed
// Page. Top-level metadata is decoded from the page's frontmatter; if a
// `## Layout` section is present anywhere in the body (matched by the
// exact heading "## Layout", regardless of position among siblings), its
// fenced YAML body is decoded into Page.Layout.
//
// Errors are wrapped with the page path so callers know which file
// failed.
func ParsePageFile(path string) (*Page, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read page file %s: %w", path, err)
	}
	return parsePageContent(path, content)
}

// parsePageContent is the in-memory parse path used by ParsePageFile and
// by tests. Splitting the disk read off keeps the unit-test surface
// small.
func parsePageContent(path string, content []byte) (*Page, error) {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("split frontmatter %s: %w", path, err)
	}

	var page Page
	if len(frontmatter) > 0 {
		if err := yaml.Unmarshal(frontmatter, &page); err != nil {
			return nil, fmt.Errorf("decode page frontmatter %s: %w", path, err)
		}
	}

	layoutYAML, found := extractLayoutSection(body)
	if !found {
		// No layout section present — Layout stays nil.
		return &page, nil
	}

	layout, err := decodeLayout(path, layoutYAML)
	if err != nil {
		return nil, err
	}
	page.Layout = layout
	return &page, nil
}

// splitFrontmatter separates a leading YAML frontmatter block (delimited
// by `---` lines) from the markdown body. Pages that have no
// frontmatter return (nil, content, nil).
func splitFrontmatter(content []byte) ([]byte, []byte, error) {
	text := string(content)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return nil, content, nil
	}
	// Find the closing fence.
	rest := strings.TrimPrefix(strings.TrimPrefix(text, "---\n"), "---\r\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		end = strings.Index(rest, "\n---\r\n")
	}
	if end < 0 {
		return nil, nil, fmt.Errorf("frontmatter has opening --- but no closing ---")
	}
	fm := rest[:end]
	body := rest[end:]
	body = strings.TrimPrefix(body, "\n---\n")
	body = strings.TrimPrefix(body, "\n---\r\n")
	return []byte(fm), []byte(body), nil
}

// extractLayoutSection walks the body's top-level `## ` headings and
// returns the fenced YAML body of the section whose heading is exactly
// "Layout". Position among siblings is irrelevant — the loader matches
// by heading, not by position. Returns (nil, false) when no `## Layout`
// section exists.
func extractLayoutSection(body []byte) ([]byte, bool) {
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inLayout := false
	inFence := false
	var collected []string
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// A new top-level heading ends any prior section.
		if strings.HasPrefix(line, "## ") {
			if inLayout {
				// We exited the layout section without ever closing the
				// fence — return what we collected anyway; YAML decode
				// will fail loudly.
				break
			}
			if trimmed == "## Layout" {
				inLayout = true
				inFence = false
				continue
			}
			continue
		}

		if !inLayout {
			continue
		}

		if !inFence {
			// Look for the opening ``` or ```yaml fence.
			if strings.HasPrefix(trimmed, "```") {
				inFence = true
			}
			continue
		}

		// Inside the fence — collect until the closing ```.
		if strings.HasPrefix(trimmed, "```") {
			// End of the fenced block; we have everything we need.
			return []byte(strings.Join(collected, "\n")), true
		}
		collected = append(collected, line)
	}
	if inLayout && inFence {
		// Reached EOF while still inside the fence; return what we have.
		return []byte(strings.Join(collected, "\n")), true
	}
	if inLayout {
		// Heading present but no fenced YAML body found.
		return []byte{}, true
	}
	return nil, false
}

// decodeLayout decodes the YAML body of a `## Layout` section into a
// Layout struct and runs the parser-side shape checks (required fields,
// forbidden wiring fields, raw-value-where-token-required for the
// universal spacing fields).
func decodeLayout(path string, layoutYAML []byte) (*Layout, error) {
	// First decode into a generic map so we can detect missing required
	// keys with field-level precision (yaml.Unmarshal silently leaves
	// missing fields zero, which would otherwise fold into a generic
	// "schema_version: 0" rather than the actionable "missing required
	// field" error this contract requires).
	var raw map[string]interface{}
	if err := yaml.Unmarshal(layoutYAML, &raw); err != nil {
		return nil, fmt.Errorf("decode layout block in %s: %w", path, err)
	}
	if _, ok := raw["componentVocabulary"]; !ok {
		return nil, fmt.Errorf("layout block in %s: missing required field 'componentVocabulary'", path)
	}
	if _, ok := raw["schema_version"]; !ok {
		return nil, fmt.Errorf("layout block in %s: missing required field 'schema_version'", path)
	}

	var layout Layout
	if err := yaml.Unmarshal(layoutYAML, &layout); err != nil {
		return nil, fmt.Errorf("decode layout block in %s: %w", path, err)
	}

	if err := walkLayoutNodes(path, layout.Nodes); err != nil {
		return nil, err
	}
	return &layout, nil
}

// walkLayoutNodes runs the per-node shape checks recursively. Errors
// name the offending node id, the offending field, and the page path so
// authors can locate the problem immediately.
func walkLayoutNodes(path string, nodes []LayoutNode) error {
	for i := range nodes {
		if err := checkLayoutNode(path, &nodes[i]); err != nil {
			return err
		}
		if err := walkLayoutNodes(path, nodes[i].Children); err != nil {
			return err
		}
	}
	return nil
}

// checkLayoutNode enforces wiring-free and token-correct rules on a
// single node. Universal spacing fields (gap, padding) must reference a
// token name; raw values like "24px" or bare integers fail loudly.
func checkLayoutNode(path string, node *LayoutNode) error {
	// Wiring fields are rejected at the inline-properties level.
	for name := range node.Properties {
		if wiringFieldNames[name] {
			return fmt.Errorf("layout block in %s: wiring field '%s' on node '%s' is not allowed — wiring belongs to the codegen pass", path, name, node.ID)
		}
		// A property that re-declares a universal field is also rejected
		// here — universal fields must live on the LayoutNode scalars
		// (they are decoded directly), not in the inline Properties map.
		// Decoding into Properties only happens when the field name is
		// outside the universal set, so a presence here is by definition
		// a redeclaration attempt or an unknown field.
		if universalContainerFields[name] {
			return fmt.Errorf("layout block in %s: universal field '%s' redeclared on node '%s'", path, name, node.ID)
		}
	}

	// Spacing fields must be token references. A token reference is a
	// non-empty string that does NOT end in a unit suffix (px, em, rem)
	// and is NOT a bare integer. The downstream validator
	// (agent.ValidateLayout) confirms membership against the adapter's
	// declared spacing tokens; this parser-level check rejects only the
	// shape mistakes that are visible without consulting the adapter.
	if node.Gap != "" && isRawSpacingValue(node.Gap) {
		return fmt.Errorf("layout block in %s: raw value '%s' for field 'gap' on node '%s' — a spacing token reference is required", path, node.Gap, node.ID)
	}
	if node.Padding != "" && isRawSpacingValue(node.Padding) {
		return fmt.Errorf("layout block in %s: raw value '%s' for field 'padding' on node '%s' — a spacing token reference is required", path, node.Padding, node.ID)
	}
	return nil
}

// isRawSpacingValue reports whether a value looks like a literal
// dimension (e.g., "24px", "1.5em", "16") rather than a token name
// (e.g., "spacing-lg"). Token names are alphanumeric-with-dashes and do
// not start with a digit.
func isRawSpacingValue(v string) bool {
	if v == "" {
		return false
	}
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return false
	}
	// Bare integer or float — first char is a digit or sign.
	first := trimmed[0]
	if first == '-' || first == '+' || (first >= '0' && first <= '9') {
		return true
	}
	return false
}
