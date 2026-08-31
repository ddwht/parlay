// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/out-of-process-validate-endpoint
// parlay-artifact: test

package domainmodel

import (
	"context"
	"strings"
	"testing"
)

// fakeValidator returns a ValidatorFunc that yields a fixed finding set,
// ignoring the draft. The mapping layer is what these tests exercise: given
// this finding from Core, the caller must receive exactly this Finding.
// Driving Core's real rules instead would mean hand-picking a model that
// provokes each code, and would fail for reasons belonging to Core.
func fakeValidator(findings ...CoreFinding) ValidatorFunc {
	return func(context.Context, []byte) []CoreFinding { return findings }
}

// capturingValidator returns a ValidatorFunc that records the draft YAML it was
// handed and reports nothing, for the tests that assert on what gets serialized.
func capturingValidator(into *[]byte) ValidatorFunc {
	return func(_ context.Context, draft []byte) []CoreFinding {
		*into = draft
		return nil
	}
}

// The endpoint-level half of this suite is gone with the HTTP layer it drove:
// eight tests over POST /api/domain-model/validate, asserting status codes,
// the JSON envelope, and malformed-request handling. What they covered that
// still exists — the mapping from a Core finding to a Finding, and the draft
// serialization handed to the validator — is exactly what the wrapper-level
// tests below cover, over the same function the handler called.
//
// The file-exists suite is gone too. TestValidationSourceFilesExist asserted
// that validate.go and parity_test.go were present in this directory — a
// testcases-derived check from when the file layout was itself the
// deliverable — which after the parity suite moved left a test asserting its
// own existence.

// --- wrapper-level mapping (Validate over an injected validator) ------------

// TestValidateMapsFieldTypeViolation asserts a field-type violation from Core
// is mapped verbatim into a Finding — closed code, anchored element path, and
// error severity unchanged.
func TestValidateMapsFieldTypeViolation(t *testing.T) {
	v := fakeValidator(CoreFinding{Code: "field-type-outside-closed-set", Message: "unknown type", Context: "entities.Order.fields.qty", Fix: "use a closed-set type", Severity: "error"})
	findings, err := Validate(context.Background(), v, Model{})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Code != "field-type-outside-closed-set" || f.Field != "entities.Order.fields.qty" || f.Severity != "error" {
		t.Fatalf("finding not mapped verbatim: %+v", f)
	}
	if !f.IsError() {
		t.Fatalf("field-type violation should be error-severity: %+v", f)
	}
}

// TestValidateCleanIsEmptyList asserts a clean draft (Core prints []) maps to
// an empty finding list.
func TestValidateCleanIsEmptyList(t *testing.T) {
	findings, err := Validate(context.Background(), fakeValidator(), Model{SchemaVersion: 1})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("want empty finding list, got %+v", findings)
	}
}

// TestValidateWarningSeverityPassedThrough asserts domain-operations-deprecated
// arrives at warning severity (verbatim from Core's authoring-mode table) and
// is NOT reclassified — it is not an error finding.
func TestValidateWarningSeverityPassedThrough(t *testing.T) {
	v := fakeValidator(CoreFinding{Code: "domain-operations-deprecated", Message: "deprecated", Context: "operations", Fix: "migrate", Severity: "warning"})
	findings, err := Validate(context.Background(), v, Model{})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(findings) != 1 || findings[0].Severity != "warning" {
		t.Fatalf("want a single warning finding, got %+v", findings)
	}
	if findings[0].IsError() {
		t.Fatalf("domain-operations-deprecated must not be an error finding: %+v", findings[0])
	}
	if HasErrorFinding(findings) {
		t.Fatalf("a warning-only set must not report an error finding")
	}
}

// TestValidateWholeModelPathPreserved asserts a whole-model finding carries the
// distinguished top-level token, not a blank or fabricated element path.
func TestValidateWholeModelPathPreserved(t *testing.T) {
	v := fakeValidator(CoreFinding{Code: "missing-schema-version", Message: "no schema_version", Context: "<domain-model>", Fix: "add schema_version", Severity: "error"})
	findings, err := Validate(context.Background(), v, Model{})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(findings) != 1 || findings[0].Field != WholeModelPath {
		t.Fatalf("want the whole-model token %q, got %+v", WholeModelPath, findings)
	}
}

// TestValidateSerializesDeprecatedOperations asserts the draft's parsed
// operations block is rendered into the YAML piped to the validator, so a
// wire draft's deprecated operations are actually seen (not dropped as the
// persistence serializer's byte-passthrough would).
func TestValidateSerializesDeprecatedOperations(t *testing.T) {
	var captured []byte
	v := capturingValidator(&captured)
	model := Model{
		SchemaVersion: 1,
		Operations:    []map[string]any{{"name": "cancel-order", "kind": "command"}},
	}
	if _, err := Validate(context.Background(), v, model); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.Contains(string(captured), "operations:") || !strings.Contains(string(captured), "cancel-order") {
		t.Fatalf("draft YAML did not carry the deprecated operations block:\n%s", captured)
	}
}

// TestValidateOmitsAbsentSchemaVersion asserts a draft with no schema_version
// (zero value) reproduces the missing-schema-version condition — the marshaled
// YAML omits schema_version rather than emitting a fabricated `0`.
func TestValidateOmitsAbsentSchemaVersion(t *testing.T) {
	var captured []byte
	v := capturingValidator(&captured)
	if _, err := Validate(context.Background(), v, Model{ /* SchemaVersion: 0 */ }); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if strings.Contains(string(captured), "schema_version") {
		t.Fatalf("absent schema_version should be omitted, not emitted:\n%s", captured)
	}
}

// TestBinaryLocatedOnceAtConstruction is gone with the thing it described. It
// asserted New resolved a `parlay` executable exactly once and reused the stored
// path on every request — a real property while validation was a subprocess, and
// the reason PARLAY_BIN existed at all. There is no binary to locate now, so
// there is no "once" to assert.
//
// Its replacement cannot live here. Proving validation runs with no binary
// available requires running Core's real rules, and this package cannot import
// them — core/internal/agent imports this package, so the edge back would close
// a cycle. It is TestValidatesWithNoBinary in core/internal/commands, beside
// the validator Core wires in, which is also where the one-engine suite lives
// for the same reason.
