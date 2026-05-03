package commands

// parlay-feature: studio-support/adapter-vocabulary-extension
// parlay-component: adapter-parser-vocabulary-and-tokens
// parlay-artifact: test
//
// Tests the optional componentVocabulary and tokens sections of the adapter
// parser: well-formed parse, registration message counts, the closed-set
// validation for property types and component categories, the
// universal-field-redeclared rule, the at-least-one-mode rule, the
// per-mode-emit-form coverage rule, and the per-process parse cache.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAdapter(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "adapter.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
	return path
}

const wellFormedClarityAdapter = `name: clarity-cli
framework: clarity-cli
version: "1"
shows: {}
actions: {}
flows: {}
file-conventions:
  source-root: cmd/
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.region
      category: container
      allowed-children: [clarity.heading, clarity.button, clarity.datagrid]
    - type: clarity.heading
      category: leaf
      variants: [page, section, subsection]
      properties:
        - name: text
          type: string
          required: true
    - type: clarity.button
      category: leaf
      variants: [primary, secondary, tertiary, danger]
      properties:
        - name: label
          type: string
          required: true
    - type: clarity.datagrid
      category: container
      allowed-children: [clarity.datagrid-column]
      variants: [compact, comfortable]
      properties:
        - name: density
          type: enum
          enum-values: [compact, comfortable]
          required: false
    - type: clarity.datagrid-column
      category: data-shape
      properties:
        - name: headerLabel
          type: string
          required: true
tokens:
  modes: [light, dark]
  spacing:
    - {name: spacing-xs, order: 1, emit-form: var(--spacing-xs)}
    - {name: spacing-sm, order: 2, emit-form: var(--spacing-sm)}
    - {name: spacing-md, order: 3, emit-form: var(--spacing-md)}
    - {name: spacing-lg, order: 4, emit-form: var(--spacing-lg)}
    - {name: spacing-xl, order: 5, emit-form: var(--spacing-xl)}
  color:
    - name: color-surface
      tone: neutral
      emit-forms: ["light:var(--color-surface-light)", "dark:var(--color-surface-dark)"]
  typography:
    - name: heading-page
      use-site: heading-page
      emit-form: var(--type-heading-page)
`

func TestParseAdapterCached_WellFormedClarityVocabularyParsesCleanly(t *testing.T) {
	adapterCacheResetForTest()
	path := writeAdapter(t, wellFormedClarityAdapter)
	a, err := parseAdapterFileCached(path)
	if err != nil {
		t.Fatalf("expected clean parse; got %v", err)
	}
	if a.ComponentVocabulary == nil || a.ComponentVocabulary.Name != "clarity@17" {
		t.Fatalf("expected vocabulary name clarity@17; got %+v", a.ComponentVocabulary)
	}
	if len(a.ComponentVocabulary.Components) != 5 {
		t.Fatalf("expected 5 components; got %d", len(a.ComponentVocabulary.Components))
	}
	if a.Tokens == nil || len(a.Tokens.Modes) != 2 {
		t.Fatalf("expected 2 modes; got %+v", a.Tokens)
	}
}

func TestParseAdapter_BareVocabularyNameFailsParse(t *testing.T) {
	adapterCacheResetForTest()
	body := `name: bad
framework: bad
version: "1"
shows: {}
actions: {}
flows: {}
file-conventions: {source-root: cmd/}
componentVocabulary:
  name: clarity
  components: []
`
	path := writeAdapter(t, body)
	_, err := parseAdapterFileCached(path)
	if err == nil {
		t.Fatalf("expected bare vocabulary name to fail parse")
	}
	if !strings.Contains(err.Error(), "vocabulary name must include @<version>") {
		t.Fatalf("expected error about vocabulary name versioning; got %v", err)
	}
}

func TestParseAdapter_PropertyTypeOutsideClosedSetFails(t *testing.T) {
	adapterCacheResetForTest()
	body := `name: bad
framework: bad
version: "1"
shows: {}
actions: {}
flows: {}
file-conventions: {source-root: cmd/}
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.foo
      category: leaf
      properties:
        - name: config
          type: object
          required: true
`
	path := writeAdapter(t, body)
	_, err := parseAdapterFileCached(path)
	if err == nil {
		t.Fatalf("expected property type `object` to fail")
	}
	if !strings.Contains(err.Error(), "type `object` is not allowed") {
		t.Fatalf("expected closed-set error; got %v", err)
	}
}

func TestParseAdapter_UniversalFieldRedeclaredFailsParse(t *testing.T) {
	adapterCacheResetForTest()
	body := `name: bad
framework: bad
version: "1"
shows: {}
actions: {}
flows: {}
file-conventions: {source-root: cmd/}
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.region
      category: container
      properties:
        - name: direction
          type: enum
          enum-values: [horizontal, vertical]
          required: false
`
	path := writeAdapter(t, body)
	_, err := parseAdapterFileCached(path)
	if err == nil {
		t.Fatalf("expected universal-field redeclaration to fail parse")
	}
	if !strings.Contains(err.Error(), "universal container fields") {
		t.Fatalf("expected universal-field error; got %v", err)
	}
}

func TestParseAdapter_EmptyModeListFailsParse(t *testing.T) {
	adapterCacheResetForTest()
	body := `name: bad
framework: bad
version: "1"
shows: {}
actions: {}
flows: {}
file-conventions: {source-root: cmd/}
tokens:
  modes: []
`
	path := writeAdapter(t, body)
	_, err := parseAdapterFileCached(path)
	if err == nil {
		t.Fatalf("expected empty mode list to fail parse")
	}
	if !strings.Contains(err.Error(), "at least one mode") {
		t.Fatalf("expected at-least-one-mode error; got %v", err)
	}
}

func TestParseAdapter_ColorTokenMissingModeFailsParse(t *testing.T) {
	adapterCacheResetForTest()
	body := `name: bad
framework: bad
version: "1"
shows: {}
actions: {}
flows: {}
file-conventions: {source-root: cmd/}
tokens:
  modes: [light, dark]
  color:
    - name: color-surface
      tone: neutral
      emit-forms: ["light:var(--color-surface-light)"]
`
	path := writeAdapter(t, body)
	_, err := parseAdapterFileCached(path)
	if err == nil {
		t.Fatalf("expected missing-mode emit-form to fail parse")
	}
	msg := err.Error()
	if !strings.Contains(msg, "color-surface") || !strings.Contains(msg, "dark") {
		t.Fatalf("expected error to name token and missing mode; got %v", err)
	}
}

func TestParseAdapter_PerProcessCacheReusesParseResult(t *testing.T) {
	adapterCacheResetForTest()
	path := writeAdapter(t, wellFormedClarityAdapter)
	if _, err := parseAdapterFileCached(path); err != nil {
		t.Fatalf("first parse failed: %v", err)
	}
	if _, err := parseAdapterFileCached(path); err != nil {
		t.Fatalf("second parse failed: %v", err)
	}
	if got := adapterCacheParseCountForTest(); got != 1 {
		t.Fatalf("expected exactly 1 parse for two cached lookups; got %d", got)
	}
	// Second adapter file should bump the counter to 2 — caching is per path.
	other := writeAdapter(t, wellFormedClarityAdapter)
	if _, err := parseAdapterFileCached(other); err != nil {
		t.Fatalf("third parse failed: %v", err)
	}
	if got := adapterCacheParseCountForTest(); got != 2 {
		t.Fatalf("expected 2 distinct parses across two paths; got %d", got)
	}
}
