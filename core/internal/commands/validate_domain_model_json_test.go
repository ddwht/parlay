// parlay-feature: studio-support/structured-domain-model-validation
// parlay-component: cross-cutting/json-validation-mode
// parlay-artifact: test
//
// CLI-level tests for `parlay validate --type domain-model --json`:
// per-violation findings from stdin and from a path, the bare-array shape,
// exit-0-as-query semantics, stdin/path equivalence, the preserved human
// path, unparseable-input handling, and authoring-mode severity.

package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/spf13/cobra"
)

const dmvCleanModel = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
`

const dmvTwoViolations = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
      - name: amount
        type: decimal
        required: true
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
relationships:
  - name: order-customer
    from: Order
    to: Customer
    cardinality: sideways
`

const dmvWithOperations = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
operations:
  - name: cancel-order
    input:
      - Order.id
    effects:
      - "set Order.status to cancelled"
`

// newDMVCmd builds a cobra command wired with stdin/stdout buffers.
func newDMVCmd(stdin string) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetIn(strings.NewReader(stdin))
	return cmd, out
}

// clean model piped on stdin prints [] and exits 0.
func TestValidateDMVJSON_CleanStdinEmptyArray(t *testing.T) {
	cmd, out := newDMVCmd(dmvCleanModel)
	err := runValidateDomainModelJSON(cmd, "-")
	if err != nil {
		t.Fatalf("expected exit 0 (nil error), got: %v", err)
	}
	var findings []agent.ValidationError
	if jerr := json.Unmarshal(out.Bytes(), &findings); jerr != nil {
		t.Fatalf("stdout is not a valid JSON array: %v\n%s", jerr, out.String())
	}
	if len(findings) != 0 {
		t.Errorf("expected empty finding array for a clean model, got: %s", out.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("expected literal [] for a clean model, got: %q", strings.TrimSpace(out.String()))
	}
}

// model with two violations prints a two-element array and exits 0.
func TestValidateDMVJSON_TwoViolationsArrayExit0(t *testing.T) {
	cmd, out := newDMVCmd(dmvTwoViolations)
	err := runValidateDomainModelJSON(cmd, "-")
	if err != nil {
		t.Fatalf("expected exit 0 (a finding list is a query result), got: %v", err)
	}
	var findings []agent.ValidationError
	if jerr := json.Unmarshal(out.Bytes(), &findings); jerr != nil {
		t.Fatalf("stdout is not valid JSON: %v", jerr)
	}
	if len(findings) != 2 {
		t.Errorf("expected 2 findings (never collapsed), got %d: %s", len(findings), out.String())
	}
}

// path input and piped bytes produce the same finding set.
func TestValidateDMVJSON_StdinPathEquivalence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "domain-model.yaml")
	if werr := os.WriteFile(p, []byte(dmvTwoViolations), 0644); werr != nil {
		t.Fatalf("write fixture: %v", werr)
	}

	cmdPath, outPath := newDMVCmd("")
	if err := runValidateDomainModelJSON(cmdPath, p); err != nil {
		t.Fatalf("path run errored: %v", err)
	}
	cmdStdin, outStdin := newDMVCmd(dmvTwoViolations)
	if err := runValidateDomainModelJSON(cmdStdin, "-"); err != nil {
		t.Fatalf("stdin run errored: %v", err)
	}
	if outPath.String() != outStdin.String() {
		t.Errorf("path and stdin runs produced different findings for identical bytes:\npath:  %s\nstdin: %s", outPath.String(), outStdin.String())
	}
}

// without --json the human path is preserved with a non-zero exit.
func TestValidateDMVJSON_HumanPathPreservedNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "domain-model.yaml")
	if werr := os.WriteFile(p, []byte(dmvTwoViolations), 0644); werr != nil {
		t.Fatalf("write fixture: %v", werr)
	}

	// Drive the full runValidate with --json unset.
	savedType, savedJSON := validateType, validateJSON
	defer func() { validateType, validateJSON = savedType, savedJSON }()
	validateType, validateJSON = "domain-model", false

	cmd, out := newDMVCmd("")
	err := runValidate(cmd, []string{p})
	if err == nil {
		t.Fatalf("expected a non-zero exit for an invalid model on the human path")
	}
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Errorf("expected an ExitCodeError, got %T: %v", err, err)
	}
	// The human path collapses to a single schema-validation-failed error,
	// not a JSON array.
	if strings.TrimSpace(out.String()) == "" {
		t.Errorf("expected human-readable output on the collapsed path")
	}
	var arr []agent.ValidationError
	if json.Unmarshal(out.Bytes(), &arr) == nil {
		t.Errorf("human path must not emit a JSON array; got: %s", out.String())
	}
}

// unparseable model under --json emits a stable parse-failure code as valid JSON.
func TestValidateDMVJSON_UnparseableStableCode(t *testing.T) {
	cmd, out := newDMVCmd("schema_version: 1\nentities: [oops")
	err := runValidateDomainModelJSON(cmd, "-")
	if err != nil {
		t.Fatalf("unparseable input must still exit 0 with valid JSON, got: %v", err)
	}
	var findings []agent.ValidationError
	if jerr := json.Unmarshal(out.Bytes(), &findings); jerr != nil {
		t.Fatalf("stdout must remain valid JSON for unparseable input: %v\n%s", jerr, out.String())
	}
	if len(findings) == 0 || findings[0].Code != "invalid-yaml" {
		t.Errorf("expected findings[0].code == invalid-yaml, got: %s", out.String())
	}
}

// findings under --json carry authoring-mode severity.
func TestValidateDMVJSON_AuthoringSeverity(t *testing.T) {
	cmd, out := newDMVCmd(dmvWithOperations)
	if err := runValidateDomainModelJSON(cmd, "-"); err != nil {
		t.Fatalf("run errored: %v", err)
	}
	var findings []agent.ValidationError
	if jerr := json.Unmarshal(out.Bytes(), &findings); jerr != nil {
		t.Fatalf("stdout not valid JSON: %v", jerr)
	}
	var found bool
	for _, f := range findings {
		if f.Code == "domain-operations-deprecated" {
			found = true
			if f.Severity != "warning" {
				t.Errorf("expected authoring-mode warning severity, got %q", f.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected a domain-operations-deprecated finding, got: %s", out.String())
	}
}
