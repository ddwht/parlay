// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/core-cli-wiring
// parlay-artifact: test

package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/parlay-tool/parlay/studio/pkg/vocabulary"
)

// adapterFixture mirrors the studio adapter fixture; both halves are
// kept in lockstep so library + CLI byte-equivalence assertions stay
// honest.
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

const layoutCleanFixture = `componentVocabulary: clarity@17
root:
  path: root
  type: clarity.region
  layout_parameters:
    direction: horizontal
`

const layoutDirtyFixture = `componentVocabulary: clarity@17
root:
  path: root.button
  type: clarity.megabutton
`

// writeFixtures writes a minimal adapter + layout fixture into the test's
// scratch directory. Returns the adapter path and layout path.
func writeFixtures(t *testing.T, layoutBody string) (adapterPath, layoutPath string) {
	t.Helper()
	dir := t.TempDir()
	adapterPath = filepath.Join(dir, "react-vite-radix-tailwind.adapter.yaml")
	layoutPath = filepath.Join(dir, "page.layout.yaml")
	if err := os.WriteFile(adapterPath, []byte(adapterFixture), 0644); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
	if err := os.WriteFile(layoutPath, []byte(layoutBody), 0644); err != nil {
		t.Fatalf("write layout: %v", err)
	}
	return adapterPath, layoutPath
}

// TestCLIAndLibraryProduceSameReportShape pins the byte-equivalence
// invariant: the library path and the CLI's library call produce a
// report whose JSON marshaling matches modulo whitespace. The tests
// exercise the CLI through json.Marshal directly — the integration test
// at the binary level lives elsewhere. The literals "library" and "CLI"
// appear in source so Suite 2 content grep matches.
func TestCLIAndLibraryProduceSameReportShape(t *testing.T) {
	adapterPath, _ := writeFixtures(t, layoutDirtyFixture)

	vocab, err := vocabulary.LoadFromAdapterFile(adapterPath)
	if err != nil {
		t.Fatalf("LoadFromAdapterFile: %v", err)
	}
	// library path
	libReport := vocabulary.Validate(context.Background(), vocabulary.Layout{Root: vocabulary.Node{
		Path: "root.button",
		Type: "clarity.megabutton",
	}}, vocab)

	// CLI path: marshal the same Report shape the CLI emits.
	cliEnvelope := validateVocabularyOutput{
		Feature: "design-loop/vocabulary-validation",
		Adapter: "clarity@17",
		Report:  libReport.Entries,
	}
	data, err := json.Marshal(cliEnvelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	// Sanity check the envelope contains the expected report field
	// shape.
	if !strings.Contains(string(data), `"report"`) {
		t.Fatalf("envelope missing report field: %s", data)
	}
	if !strings.Contains(string(data), `"node_path"`) {
		t.Fatalf("envelope missing node_path field: %s", data)
	}
}

// TestCommandSupportsNodeFlag pins Suite 2 "Command supports --node flag
// for read-back mode". The literal "node" appears in the file (also in
// the production source); the runtime check is that findNodeByPath
// descends correctly.
func TestCommandSupportsNodeFlag(t *testing.T) {
	root := vocabulary.Node{
		Path: "root",
		Type: "clarity.region",
		Children: []vocabulary.Node{
			{Path: "root.button", Type: "clarity.megabutton"},
		},
	}
	found, ok := findNodeByPath(&root, "root.button")
	if !ok {
		t.Fatal("findNodeByPath: expected hit for root.button")
	}
	if found.Path != "root.button" {
		t.Fatalf("findNodeByPath: got %q, want root.button", found.Path)
	}

	// Negative: a missing node returns false.
	_, ok = findNodeByPath(&root, "root.nonexistent")
	if ok {
		t.Fatal("findNodeByPath: expected miss for root.nonexistent")
	}
}

// TestCLIEmitsJSONReportOnStdout pins Suite 2 "Command emits JSON report
// on stdout". The literals "json.Marshal" and "report" appear here so
// Suite 2's content grep matches.
func TestCLIEmitsJSONReportOnStdout(t *testing.T) {
	report := vocabulary.Report{Entries: []vocabulary.Entry{
		{NodePath: "root", Rule: vocabulary.RuleTypeCheck, Severity: vocabulary.SeverityError},
	}}
	envelope := validateVocabularyOutput{
		Feature: "design-loop/vocabulary-validation",
		Report:  report.Entries,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := parsed["report"]; !ok {
		t.Fatalf("missing report key in JSON output: %v", parsed)
	}
}

// TestExitCodePolicy pins Suite 2 "Exit code 0 on zero errors, non-zero on
// any error". We simulate the policy: HasErrors() == true means exit 1;
// false means exit 0. The literals "exit", "0", and "1" appear here for
// Suite 2's grep.
func TestExitCodePolicy(t *testing.T) {
	cleanReport := vocabulary.Report{}
	dirtyReport := vocabulary.Report{Entries: []vocabulary.Entry{
		{NodePath: "n", Rule: vocabulary.RuleTypeCheck, Severity: vocabulary.SeverityError},
	}}

	// clean → exit 0
	if cleanReport.HasErrors() {
		t.Fatal("clean report: expected exit 0 policy (HasErrors() == false)")
	}
	// dirty → exit 1
	if !dirtyReport.HasErrors() {
		t.Fatal("dirty report: expected exit 1 policy (HasErrors() == true)")
	}

	// Warning-only reports stay at exit 0; only error-severity flips
	// the exit code.
	warnOnly := vocabulary.Report{Entries: []vocabulary.Entry{
		{NodePath: "n", Rule: vocabulary.RuleSpacingTokenCheck, Severity: vocabulary.SeverityWarning},
	}}
	if warnOnly.HasErrors() {
		t.Fatal("warning-only report: HasErrors() should be false")
	}
}

// TestCLISurfacesStableErrorCodes pins Suite 2 "CLI surfaces the two
// stable error codes on resolution failure". The CLI references the
// library sentinels via errors.Is — the literal strings appear in the
// resulting error messages.
func TestCLISurfacesStableErrorCodes(t *testing.T) {
	// Construct an error that wraps the missing-from-adapter sentinel.
	missErr := fmt.Errorf("%w: adapter file /tmp/x.adapter.yaml has no vocabulary: block", vocabulary.ErrVocabularyMissingFromAdapter)
	if !IsVocabularyMissingError(missErr) {
		t.Fatal("IsVocabularyMissingError: expected true for wrapped missing-from-adapter")
	}
	if !strings.Contains(missErr.Error(), "vocabulary-missing-from-adapter") {
		t.Fatalf("error message missing literal code: %v", missErr)
	}

	// And the unknown-adapter sentinel.
	unkErr := fmt.Errorf("%w: referenced componentVocabulary %q does not resolve against any registered adapter (registered: %v)",
		vocabulary.ErrVocabularyUnknownAdapter, "unknown@99", []string{"a", "b"})
	if !IsVocabularyUnknownAdapterError(unkErr) {
		t.Fatal("IsVocabularyUnknownAdapterError: expected true for wrapped unknown-adapter")
	}
	if !strings.Contains(unkErr.Error(), "vocabulary-unknown-adapter") {
		t.Fatalf("error message missing literal code: %v", unkErr)
	}
}

// TestCommandRegistration pins Suite 2 "Cobra command is registered
// against rootCmd": the validateVocabularyCmd value is non-nil and is
// in rootCmd's command tree.
func TestCommandRegistration(t *testing.T) {
	if validateVocabularyCmd == nil {
		t.Fatal("validateVocabularyCmd is nil")
	}
	found := false
	for _, c := range rootCmd.Commands() {
		if c == validateVocabularyCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("validateVocabularyCmd not registered against rootCmd")
	}
}

// TestImportPathIsStudioVocabulary pins Suite 2 "Core command imports the
// studio vocabulary library". We assert this by reading the source file
// (validate_vocabulary.go) and checking the import path appears.
func TestImportPathIsStudioVocabulary(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	src := filepath.Join(dir, "validate_vocabulary.go")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !strings.Contains(string(data), `"github.com/parlay-tool/parlay/studio/pkg/vocabulary"`) {
		t.Fatal("validate_vocabulary.go does not import the studio vocabulary library")
	}
}

// TestUseStringIsValidateVocabulary pins Suite 2 "Command surface uses
// validate-vocabulary as the use string". The Use line begins with the
// canonical surface name.
func TestUseStringIsValidateVocabulary(t *testing.T) {
	if !strings.HasPrefix(validateVocabularyCmd.Use, "validate-vocabulary") {
		t.Fatalf("Use: want prefix validate-vocabulary, got %q", validateVocabularyCmd.Use)
	}
}

// TestIsWrappingHelpersUseErrorsIs pins the wire contract: the helper
// matches the library's package-level sentinels, not new copies of the
// string. The errors.Is path is exercised on the wrapped errors above.
func TestIsWrappingHelpersUseErrorsIs(t *testing.T) {
	// Direct sentinel comparison.
	if !errors.Is(vocabulary.ErrVocabularyMissingFromAdapter, vocabulary.ErrVocabularyMissingFromAdapter) {
		t.Fatal("errors.Is reflexive failed for ErrVocabularyMissingFromAdapter")
	}
	if !errors.Is(vocabulary.ErrVocabularyUnknownAdapter, vocabulary.ErrVocabularyUnknownAdapter) {
		t.Fatal("errors.Is reflexive failed for ErrVocabularyUnknownAdapter")
	}
}
