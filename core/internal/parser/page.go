// parlay-feature: parlay-tool/page-layout-field
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

	// Parse the markdown body — the form page.schema.md's Template section
	// documents and the one lock-page actually writes.
	//
	// Only the frontmatter and the ## Layout block were read before, and a
	// conforming manifest has neither: it opens with "# <Page Name>", carries
	// **Owner**/**Status** lines, and lists its regions as "## <Region Name>"
	// headings with numbered @feature/fragment items. So a manifest in the
	// documented form parsed to a zero-valued Page — no name, no status, and
	// NO REGIONS. The writer and the reader had never agreed on a format.
	//
	// That is also why `validate --type page` returned OK on a manifest
	// naming a page, a region and a fragment that all did not exist: nothing
	// had been decoded to check.
	//
	// Frontmatter still wins where present, so a page authored in the YAML
	// form keeps working; the markdown scan only fills fields left empty.
	parseMarkdownBody(body, &page)

	layoutYAML, found, err := extractLayoutSection(body)
	if err != nil {
		return nil, fmt.Errorf("page %s: %w", path, err)
	}
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

// parseMarkdownBody fills Name, Description, Owner, Status and Regions from
// the markdown form documented in page.schema.md's Fields table. Fields
// already set from frontmatter are left alone.
//
// The `## Layout` section is skipped: its body is a fenced YAML block owned
// by extractLayoutSection, and treating it as a region would invent one
// named "Layout" carrying no fragments.
func parseMarkdownBody(body []byte, page *Page) {
	var current *PageRegion
	flush := func() {
		if current != nil {
			page.Regions = append(page.Regions, *current)
			current = nil
		}
	}

	lines := pageLines(body)
	// The layout section is located ONCE, by the locator every reader of a
	// page shares (pageLayoutSpan). This scan used to decide for itself where
	// the layout began and ended, on rules that differed from the loader's and
	// from the annotation scanner's.
	layout, hasLayout := pageLayoutSpan(lines)

	// HTML comments are invisible here as they are in every other Markdown
	// parser (comments.go) — a commented-out region must not become a real
	// region. The layout block is exempt: its body is fenced YAML, where
	// `<!--` is not a comment opener but whatever the YAML says it is.
	var comments mdComments
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		// The whole section, not just the fenced body: trailing prose under
		// `## Layout` belongs to the layout section and is not a page region.
		inLayout := hasLayout && i >= layout.heading && i < layout.sectionEnd
		if !inLayout {
			rest, ok := comments.visible(line)
			if !ok {
				continue
			}
			line = rest
		}
		trimmed := strings.TrimSpace(line)

		switch {
		case inLayout:
			// The heading and the fenced YAML both belong to the layout, not
			// to us: a region named "Layout" carrying no fragments would be
			// invented from the heading alone.
			flush()
			continue
		case strings.HasPrefix(trimmed, "## "):
			flush()
			current = &PageRegion{Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))}
		case strings.HasPrefix(trimmed, "# "):
			flush()
			if page.Name == "" {
				page.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			}
		case strings.HasPrefix(trimmed, "> "):
			if page.Description == "" {
				page.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "> "))
			}
		case strings.HasPrefix(trimmed, "**Owner**:"):
			if page.Owner == "" {
				page.Owner = strings.TrimSpace(strings.TrimPrefix(trimmed, "**Owner**:"))
			}
		case strings.HasPrefix(trimmed, "**Status**:"):
			if page.Status == "" {
				page.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "**Status**:"))
			}
		default:
			if current == nil {
				continue
			}
			if ref, ok := numberedFragmentRef(trimmed); ok {
				current.Components = append(current.Components, ref)
			}
		}
	}
	flush()
}

// numberedFragmentRef matches a "1. @feature/fragment-name" list item and
// returns the reference.
//
// The list ORDER is the manifest's whole purpose — page.schema.md says it
// "overrides feature surface Order values" — so the slice position carries
// the meaning and the printed number does not.
func numberedFragmentRef(line string) (string, bool) {
	dot := strings.Index(line, ".")
	if dot <= 0 {
		return "", false
	}
	for _, r := range line[:dot] {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	ref := strings.TrimSpace(line[dot+1:])
	if !strings.HasPrefix(ref, "@") {
		return "", false
	}
	return ref, true
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

// pageLayout is where a page's `## Layout` section is, and how well-formed it
// is. The three states are kept apart on purpose: "no Layout section",
// "Layout section with no fenced block", and "Layout section whose fence is
// never closed" are different mistakes, and only the first is not one.
type pageLayout struct {
	heading    int // line of the `## Layout` heading
	sectionEnd int // one past the section's last line
	bodyStart  int // first line inside the fence, -1 when there is no fence
	bodyEnd    int // one past the last line inside the fence
	closed     bool
}

func (l pageLayout) fenced() bool { return l.bodyStart >= 0 }

// pageLayoutSpan locates a page's `## Layout` section. The bool reports only
// whether the heading exists; the struct says whether what follows it is
// usable.
//
// ONE locator, shared by everything that reads a page. There were three, and
// they disagreed: `extractLayoutSection` accepted only an exact unindented
// `## Layout` with a backtick fence closed by any line starting with three
// backticks, `parseMarkdownBody` matched the heading case-insensitively on the
// trimmed line, and the annotation scanner applied CommonMark's fence rules.
// So a `~~~` fence, a `## layout`, an indented heading or a four-backtick
// opener each put the same bytes in a different category for each reader — and
// the annotation scanner could offer a resolver an actionable request inside
// bytes the page loader never consumed as YAML. That is the one thing the
// design forbids outright: the same bytes must not be content to one reader
// and a request to another.
//
// The rules are the permissive ones, because the strict version was the
// outlier — `parseMarkdownBody` was already case-insensitive — and because a
// tilde-fenced layout previously decoded to nothing at all, silently dropping
// a layout its author had written.
func pageLayoutSpan(lines []string) (pageLayout, bool) {
	out := pageLayout{heading: -1, bodyStart: -1, bodyEnd: -1, sectionEnd: len(lines)}

	// One pass, tracking fences throughout. A heading inside a fenced block is
	// not a heading — an indented `# ...` in a YAML example would otherwise end
	// the section it is inside and make its own fence look unterminated.
	fence := ""
	for i, line := range lines {
		if fence != "" {
			if closesFence(line, fence) {
				if out.heading >= 0 && out.bodyStart >= 0 && !out.closed {
					out.bodyEnd = i
					out.closed = true
					out.sectionEnd = len(lines)
					// Keep scanning: the section runs to the next heading.
				}
				fence = ""
			}
			continue
		}
		if marker, isFence := opensFence(line); isFence {
			fence = marker
			if out.heading >= 0 && out.bodyStart < 0 {
				out.bodyStart = i + 1
				out.bodyEnd = len(lines)
			}
			continue
		}
		level, title, isHeading := markdownHeading(line)
		if !isHeading || level > 2 {
			continue
		}
		if out.heading >= 0 {
			out.sectionEnd = i
			break
		}
		if level == 2 && strings.EqualFold(title, "Layout") {
			out.heading = i
		}
	}

	if out.heading < 0 {
		return out, false
	}
	if out.bodyStart >= 0 && !out.closed {
		out.bodyEnd = out.sectionEnd
	}
	if out.bodyEnd > out.sectionEnd {
		out.bodyEnd = out.sectionEnd
	}
	return out, true
}

// extractLayoutSection returns the fenced YAML body of the `## Layout`
// section, whether the section exists, and an error when it exists but is not
// usable.
//
// The error is the point. A `## Layout` heading with no fence under it used to
// yield an empty body that decodeLayout then refused for missing required
// fields — accidentally, but it refused. Answering "no layout section" for
// that shape would silently reclassify a malformed layout as an ordinary page
// region named "Layout": a page that means something other than its author
// wrote. An unterminated fence is the same class of mistake, and leaving it to
// the YAML decoder catches it only when the collected text also happens to be
// invalid YAML — which is exactly when the mistake is hardest to see.
func extractLayoutSection(body []byte) ([]byte, bool, error) {
	lines := pageLines(body)
	loc, ok := pageLayoutSpan(lines)
	if !ok {
		return nil, false, nil
	}
	if !loc.fenced() {
		return nil, true, fmt.Errorf("the `## Layout` section has no fenced YAML block")
	}
	if !loc.closed {
		return nil, true, fmt.Errorf("the `## Layout` fence is never closed")
	}
	return []byte(strings.Join(lines[loc.bodyStart:loc.bodyEnd], "\n")), true, nil
}

// pageLines splits a page body the way every reader of it must, so that a line
// index means the same thing to all of them.
func pageLines(body []byte) []string {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
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
