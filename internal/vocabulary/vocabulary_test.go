// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/vocabulary-validator-library
// parlay-artifact: test

package vocabulary

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adapterFixture is a minimal but realistic adapter YAML. Includes a
// componentVocabulary name AND a top-level vocabulary block so the loader
// has something to extract. Adapters without vocabulary: are exercised
// separately.
const adapterFixture = `name: react-vite-radix-tailwind
framework: React + Radix + Tailwind
version: 0.1.0
kind: presentation
componentVocabulary:
  name: clarity@17
vocabulary:
  components:
    - name: clarity.button
      properties: [label, disabled]
      variants:
        kind: [primary, secondary, tertiary]
  spacing_tokens: [spacing-sm, spacing-md, spacing-lg]
  color_tokens: [color-status-info, color-status-danger]
  layout_containers:
    - container_type: clarity.region
      admissible_parameters: [direction, gap]
      parameter_constraints:
        direction:
          type: enum
          allowed_values: [horizontal, vertical]
`

const adapterFixtureNoVocabBlock = `name: bare-adapter
framework: Bare
version: 0.0.1
kind: presentation
componentVocabulary:
  name: bare@0
`

// TestLoadFromAdapterFileHappyPath pins Suite 2 / infrastructure 2's
// happy-path contract: a well-formed adapter YAML produces a Vocabulary
// with the four populated subfields.
func TestLoadFromAdapterFileHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "react-vite-radix-tailwind.adapter.yaml")
	if err := os.WriteFile(path, []byte(adapterFixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	resetVocabularyCacheForTest()

	v, err := LoadFromAdapterFile(path)
	if err != nil {
		t.Fatalf("LoadFromAdapterFile: %v", err)
	}
	if len(v.Components) != 1 || v.Components[0].Name != "clarity.button" {
		t.Fatalf("components: %+v", v.Components)
	}
	if len(v.SpacingTokens) != 3 {
		t.Fatalf("spacing_tokens count: %d (%v)", len(v.SpacingTokens), v.SpacingTokens)
	}
	if len(v.ColorTokens) != 2 {
		t.Fatalf("color_tokens count: %d (%v)", len(v.ColorTokens), v.ColorTokens)
	}
	if len(v.LayoutContainers) != 1 || v.LayoutContainers[0].ContainerType != "clarity.region" {
		t.Fatalf("layout_containers: %+v", v.LayoutContainers)
	}
}

// TestLoadFromAdapterFileMissingBlock pins the wire-contract for the
// "no vocabulary block" path: ErrVocabularyMissingFromAdapter is returned
// and the error string includes "vocabulary-missing-from-adapter".
func TestLoadFromAdapterFileMissingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bare.adapter.yaml")
	if err := os.WriteFile(path, []byte(adapterFixtureNoVocabBlock), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	resetVocabularyCacheForTest()

	_, err := LoadFromAdapterFile(path)
	if err == nil {
		t.Fatal("expected ErrVocabularyMissingFromAdapter, got nil")
	}
	if !errors.Is(err, ErrVocabularyMissingFromAdapter) {
		t.Fatalf("expected errors.Is(err, ErrVocabularyMissingFromAdapter); got %v", err)
	}
	if !strings.Contains(err.Error(), "vocabulary-missing-from-adapter") {
		t.Fatalf("error message missing the wire-contract string: %v", err)
	}
}

// TestResolveForLayoutUnknownAdapter pins the second wire-contract:
// ErrVocabularyUnknownAdapter is returned when no adapter resolves the
// layout's componentVocabulary value. The error message names both the
// referenced value AND the registered-adapter list.
func TestResolveForLayoutUnknownAdapter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "react-vite-radix-tailwind.adapter.yaml"), []byte(adapterFixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	resetVocabularyCacheForTest()

	_, err := ResolveForLayout("unknown-vocab@99", []string{"react-vite-radix-tailwind"}, dir)
	if err == nil {
		t.Fatal("expected ErrVocabularyUnknownAdapter, got nil")
	}
	if !errors.Is(err, ErrVocabularyUnknownAdapter) {
		t.Fatalf("expected errors.Is(err, ErrVocabularyUnknownAdapter); got %v", err)
	}
	if !strings.Contains(err.Error(), "unknown-vocab@99") {
		t.Fatalf("error message missing referenced componentVocabulary value: %v", err)
	}
	if !strings.Contains(err.Error(), "react-vite-radix-tailwind") {
		t.Fatalf("error message missing the registered-adapter list: %v", err)
	}
}

// TestResolveForLayoutHappyPath pins the resolution happy path: the
// adapter declares componentVocabulary.name matching the layout's value;
// ResolveForLayout returns the loaded vocabulary.
func TestResolveForLayoutHappyPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "react-vite-radix-tailwind.adapter.yaml"), []byte(adapterFixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	resetVocabularyCacheForTest()

	v, err := ResolveForLayout("clarity@17", []string{"react-vite-radix-tailwind"}, dir)
	if err != nil {
		t.Fatalf("ResolveForLayout: %v", err)
	}
	if len(v.Components) == 0 {
		t.Fatalf("expected non-empty components, got %+v", v.Components)
	}
}

// TestVocabularyLoadIgnoresSiblingFile pins the strict file-IO contract.
// A sibling <adapter>.vocabulary.yaml exists alongside the adapter, but
// the loader MUST read exactly the adapter file — never the sibling.
// Suite 1 / infrastructure 1 invariant.
func TestVocabularyLoadIgnoresSiblingFile(t *testing.T) {
	dir := t.TempDir()
	adapterPath := filepath.Join(dir, "react-vite-radix-tailwind.adapter.yaml")
	siblingPath := filepath.Join(dir, "react-vite-radix-tailwind.vocabulary.yaml")
	if err := os.WriteFile(adapterPath, []byte(adapterFixture), 0644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
	// Sibling carries an incompatible block — if the loader read it, the
	// happy-path assertion below would see different components.
	if err := os.WriteFile(siblingPath, []byte("components: [{name: sibling-only}]\n"), 0644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}
	resetVocabularyCacheForTest()

	v, err := LoadFromAdapterFile(adapterPath)
	if err != nil {
		t.Fatalf("LoadFromAdapterFile: %v", err)
	}
	if v.Components[0].Name != "clarity.button" {
		t.Fatalf("loader read the sibling file (saw %q) — file-IO contract violated", v.Components[0].Name)
	}
}

// TestVocabularyCacheOnFirstAccess pins the caching: on-first-access
// contract. The first ResolveForLayout call populates the cache; a second
// call against the same adapter does not re-parse — we observe by
// mutating the on-disk file between calls and asserting the cached value
// remains the original.
func TestVocabularyCacheOnFirstAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "react-vite-radix-tailwind.adapter.yaml")
	if err := os.WriteFile(path, []byte(adapterFixture), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	resetVocabularyCacheForTest()

	v1, err := ResolveForLayout("clarity@17", []string{"react-vite-radix-tailwind"}, dir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Mutate the file by writing a divergent fixture.
	if err := os.WriteFile(path, []byte(adapterFixtureNoVocabBlock), 0644); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	v2, err := ResolveForLayout("clarity@17", []string{"react-vite-radix-tailwind"}, dir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	// The cache hit means we still see the original component count.
	if len(v1.Components) != len(v2.Components) {
		t.Fatalf("cache miss between calls: v1=%d v2=%d", len(v1.Components), len(v2.Components))
	}
}
