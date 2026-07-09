// parlay-feature: studio-support/page-layout-field
// parlay-cross-cutting-id: layout-precheck-contract
// parlay-artifact: test
//
// Tests the `parlay validate --type page` and `--type layout` CLI wiring
// (Phase 6.4): both route to agent.ValidateLayoutDeep, resolve --adapter
// via agent.LoadAdapterFile, and surface codes matching layout.schema.md's
// "Validation pass" table.

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const clarity17AdapterYAML = `
componentVocabulary:
  name: clarity@17
  components:
    - type: clarity.region
      category: container
      allowed-children: [clarity.heading, clarity.button]
    - type: clarity.heading
      category: leaf
      properties:
        - name: text
          type: string
          required: true
tokens:
  modes: [light]
  spacing:
    - name: spacing-lg
      order: 1
      emit-form: "var(--spacing-lg)"
`

func writeTempAdapter(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "clarity.adapter.yaml")
	if err := os.WriteFile(path, []byte(clarity17AdapterYAML), 0644); err != nil {
		t.Fatalf("write temp adapter: %v", err)
	}
	return path
}

func runValidateCmdForTest(t *testing.T, typ, path, adapterPath string) (stdout, stderr string, err error) {
	t.Helper()
	resetFlagsAfterTest(t, validateCmd.Flags())
	if e := validateCmd.Flags().Set("type", typ); e != nil {
		t.Fatalf("set --type: %v", e)
	}
	if adapterPath != "" {
		if e := validateCmd.Flags().Set("adapter", adapterPath); e != nil {
			t.Fatalf("set --adapter: %v", e)
		}
	}
	var out, errBuf bytes.Buffer
	validateCmd.SetOut(&out)
	validateCmd.SetErr(&errBuf)
	err = runValidate(validateCmd, []string{path})
	return out.String(), errBuf.String(), err
}

func TestValidateType_Page_NoLayoutBlockIsOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.page.md")
	body := "---\nname: dashboard\n---\n\nNo layout here.\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runValidateCmdForTest(t, "page", path, "")
	if err != nil {
		t.Fatalf("expected OK, got err=%v", err)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("expected OK output; got %q", stdout)
	}
}

func TestValidateType_Page_WellFormedLayoutIsOK(t *testing.T) {
	adapterPath := writeTempAdapter(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.page.md")
	body := "---\nname: dashboard\n---\n\n## Layout\n\n```yaml\ncomponentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n```\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runValidateCmdForTest(t, "page", path, adapterPath)
	if err != nil {
		t.Fatalf("expected OK, got err=%v stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("expected OK output; got %q", stdout)
	}
}

func TestValidateType_Page_MalformedLayoutBlockErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.page.md")
	body := "---\nname: dashboard\n---\n\n## Layout\n\n```yaml\ncomponentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n   bad-indent: oops\n```\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runValidateCmdForTest(t, "page", path, "")
	if err == nil {
		t.Fatal("expected a non-nil error for a malformed layout block")
	}
	if !strings.Contains(stderr, "malformed-layout-block") {
		t.Errorf("expected malformed-layout-block in output; got %q", stderr)
	}
}

func TestValidateType_Page_UnknownComponentTypeErrors(t *testing.T) {
	adapterPath := writeTempAdapter(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.page.md")
	body := "---\nname: dashboard\n---\n\n## Layout\n\n```yaml\ncomponentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.kanban\n```\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runValidateCmdForTest(t, "page", path, adapterPath)
	if err == nil {
		t.Fatal("expected a non-nil error for an unknown component type")
	}
	if !strings.Contains(stderr, "unknown-component-type") {
		t.Errorf("expected unknown-component-type in output; got %q", stderr)
	}
}

func TestValidateType_Layout_WellFormedStandaloneFileIsOK(t *testing.T) {
	adapterPath := writeTempAdapter(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.layout.yaml")
	body := "componentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runValidateCmdForTest(t, "layout", path, adapterPath)
	if err != nil {
		t.Fatalf("expected OK, got err=%v stderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("expected OK output; got %q", stdout)
	}
}

func TestValidateType_Layout_MissingSchemaVersionErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.layout.yaml")
	body := "componentVocabulary: clarity@17\nnodes:\n  - id: root\n    type: clarity.region\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runValidateCmdForTest(t, "layout", path, "")
	if err == nil {
		t.Fatal("expected a non-nil error for a missing schema_version")
	}
	if !strings.Contains(stderr, "missing-schema-version") {
		t.Errorf("expected missing-schema-version in output; got %q", stderr)
	}
}

func TestValidateType_Layout_WiringInLayoutErrors(t *testing.T) {
	adapterPath := writeTempAdapter(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.layout.yaml")
	body := "componentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n    dataSource: orders\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runValidateCmdForTest(t, "layout", path, adapterPath)
	if err == nil {
		t.Fatal("expected a non-nil error for a wiring field in the layout")
	}
	if !strings.Contains(stderr, "wiring-in-layout") {
		t.Errorf("expected wiring-in-layout in output; got %q", stderr)
	}
}

func TestValidateType_Layout_VocabularyVersionMismatchErrors(t *testing.T) {
	adapterPath := writeTempAdapter(t) // declares clarity@17
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.layout.yaml")
	body := "componentVocabulary: clarity@16\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runValidateCmdForTest(t, "layout", path, adapterPath)
	if err == nil {
		t.Fatal("expected a non-nil error for a vocabulary version mismatch")
	}
	if !strings.Contains(stderr, "vocabulary-version-mismatch") {
		t.Errorf("expected vocabulary-version-mismatch in output; got %q", stderr)
	}
}

func TestValidateType_Layout_RawValueWhereTokenRequiredErrors(t *testing.T) {
	adapterPath := writeTempAdapter(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.layout.yaml")
	body := "componentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n    gap: 24px\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runValidateCmdForTest(t, "layout", path, adapterPath)
	if err == nil {
		t.Fatal("expected a non-nil error for a raw spacing value")
	}
	if !strings.Contains(stderr, "raw-value-where-token-required") {
		t.Errorf("expected raw-value-where-token-required in output; got %q", stderr)
	}
}
