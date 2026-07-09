// parlay-feature: parlay-tool/schema-consolidation
// parlay-component: vocabulary-block-parity-check
// parlay-artifact: test

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAdapterFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.adapter.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckVocabularyBlockParity_ConsistentPasses(t *testing.T) {
	path := writeAdapterFixture(t, `
name: test-adapter
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.button
      category: leaf
    - type: clarity.region
      category: container
tokens:
  modes: [light]
  spacing:
    - name: spacing-md
      order: 1
      emit-form: "var(--spacing-md)"
  color:
    - name: color-status-danger
      emit-forms: ["light:var(--color-danger)"]
vocabulary:
  components:
    - name: clarity.button
    - name: clarity.region
  spacing_tokens: [spacing-md]
  color_tokens: [color-status-danger]
`)
	outcomes, err := CheckVocabularyBlockParity(ModeBuild, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outcomes) != 0 {
		t.Errorf("expected no drift; got %+v", outcomes)
	}
}

func TestCheckVocabularyBlockParity_MissingComponentDetected(t *testing.T) {
	path := writeAdapterFixture(t, `
name: test-adapter
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.button
      category: leaf
    - type: clarity.region
      category: container
vocabulary:
  components:
    - name: clarity.button
  spacing_tokens: []
  color_tokens: []
`)
	outcomes, err := CheckVocabularyBlockParity(ModeBuild, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !findCode(outcomes, "vocabulary-block-parity-drift") {
		t.Errorf("expected vocabulary-block-parity-drift; got %+v", outcomes)
	}
	if !findMessage(outcomes, "clarity.region") {
		t.Errorf("expected drift to name clarity.region; got %+v", outcomes)
	}
}

func TestCheckVocabularyBlockParity_TokenDriftDetected(t *testing.T) {
	path := writeAdapterFixture(t, `
name: test-adapter
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.button
      category: leaf
tokens:
  modes: [light]
  spacing:
    - name: spacing-md
      order: 1
      emit-form: "var(--spacing-md)"
    - name: spacing-lg
      order: 2
      emit-form: "var(--spacing-lg)"
vocabulary:
  components:
    - name: clarity.button
  spacing_tokens: [spacing-md]
  color_tokens: []
`)
	outcomes, err := CheckVocabularyBlockParity(ModeBuild, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !findCode(outcomes, "vocabulary-block-parity-drift") {
		t.Errorf("expected vocabulary-block-parity-drift; got %+v", outcomes)
	}
	if !findMessage(outcomes, "spacing-lg") {
		t.Errorf("expected drift to name spacing-lg; got %+v", outcomes)
	}
}

func TestCheckVocabularyBlockParity_EitherBlockAbsentIsNoOp(t *testing.T) {
	path := writeAdapterFixture(t, `
name: test-adapter
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.button
      category: leaf
`)
	outcomes, err := CheckVocabularyBlockParity(ModeBuild, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outcomes) != 0 {
		t.Errorf("expected no outcomes when vocabulary: is absent; got %+v", outcomes)
	}
}
