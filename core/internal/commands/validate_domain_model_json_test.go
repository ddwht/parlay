// parlay-feature: studio-support/structured-domain-model-validation
// parlay-component: cross-cutting/json-validation-mode
// parlay-artifact: test
//
// CLI-level tests for `parlay validate --type domain-model --json`:
// per-violation findings from stdin and from a path, the bare-array shape,
// the exit code agreeing with the findings, stdin/path equivalence, the
// preserved human path, unparseable-input handling, and authoring-mode
// severity.

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

// model with two violations prints a two-element array AND exits non-zero.
//
// R4-22. This asserted exit 0 until the contract question behind it was
// settled: `--json` chooses how the result is rendered, not whether the
// command has a verdict. A surface that prints two blocking problems and
// reports success is the ambiguity the whole consolidation exists to remove,
// and it contradicted parlay's own instruction that the exit code is the one
// thing a CI script may trust.
//
// The output half of this test is unchanged, and that is the point: nothing
// that parses stdout notices this change. Only a caller that trusted the exit
// code does, and it stops being misled.
func TestValidateDMVJSON_TwoViolationsArrayExitsNonZero(t *testing.T) {
	cmd, out := newDMVCmd(dmvTwoViolations)
	err := runValidateDomainModelJSON(cmd, "-")
	if err == nil {
		t.Fatalf("expected a non-zero exit for a model with blocking findings, got nil: %s", out.String())
	}
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Errorf("expected an ExitCodeError, got %T: %v", err, err)
	}
	var findings []agent.ValidationError
	if jerr := json.Unmarshal(out.Bytes(), &findings); jerr != nil {
		t.Fatalf("stdout is not valid JSON: %v", jerr)
	}
	if len(findings) != 2 {
		t.Errorf("expected 2 findings (never collapsed), got %d: %s", len(findings), out.String())
	}
}

// path input and piped bytes produce the same finding set and the same exit.
// Rendering is identical either way, so the verdict has to be too.
func TestValidateDMVJSON_StdinPathEquivalence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "domain-model.yaml")
	if werr := os.WriteFile(p, []byte(dmvTwoViolations), 0644); werr != nil {
		t.Fatalf("write fixture: %v", werr)
	}

	cmdPath, outPath := newDMVCmd("")
	errPath := runValidateDomainModelJSON(cmdPath, p)
	cmdStdin, outStdin := newDMVCmd(dmvTwoViolations)
	errStdin := runValidateDomainModelJSON(cmdStdin, "-")

	if outPath.String() != outStdin.String() {
		t.Errorf("path and stdin runs produced different findings for identical bytes:\npath:  %s\nstdin: %s", outPath.String(), outStdin.String())
	}
	if (errPath == nil) != (errStdin == nil) {
		t.Errorf("path and stdin runs disagreed on the exit code: path=%v stdin=%v", errPath, errStdin)
	}
	if errPath == nil {
		t.Errorf("both runs carry blocking findings and must exit non-zero")
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

// unparseable model under --json emits a stable parse-failure code as valid
// JSON, and exits non-zero. The invariant being defended here is that the
// command never crashes and never emits non-JSON on stdout — not that it
// pretends an unreadable model is fine.
func TestValidateDMVJSON_UnparseableStableCode(t *testing.T) {
	cmd, out := newDMVCmd("schema_version: 1\nentities: [oops")
	err := runValidateDomainModelJSON(cmd, "-")
	if err == nil {
		t.Errorf("a model that could not be parsed must not report success")
	}
	var findings []agent.ValidationError
	if jerr := json.Unmarshal(out.Bytes(), &findings); jerr != nil {
		t.Fatalf("stdout must remain valid JSON for unparseable input: %v\n%s", jerr, out.String())
	}
	if len(findings) == 0 || findings[0].Code != "invalid-yaml" {
		t.Errorf("expected findings[0].code == invalid-yaml, got: %s", out.String())
	}
}

// findings under --json carry authoring-mode severity. Since v0.3 the
// operations: block is a removal error in every mode, so the model that
// used to be the warning-only example now correctly exits non-zero — the
// exit code still follows blocking findings, exactly as the human path does.
func TestValidateDMVJSON_AuthoringSeverity(t *testing.T) {
	cmd, out := newDMVCmd(dmvWithOperations)
	if err := runValidateDomainModelJSON(cmd, "-"); err == nil {
		t.Fatalf("a model carrying the removed operations: block must exit non-zero")
	}
	var findings []agent.ValidationError
	if jerr := json.Unmarshal(out.Bytes(), &findings); jerr != nil {
		t.Fatalf("stdout not valid JSON: %v", jerr)
	}
	var found bool
	for _, f := range findings {
		if f.Code == "domain-operations-unsupported" {
			found = true
			if f.Severity != "error" {
				t.Errorf("expected error severity (field removed in v0.3), got %q", f.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected a domain-operations-unsupported finding, got: %s", out.String())
	}
}
