// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: cross-cutting/out-of-process-validate-endpoint
// parlay-artifact: test

package domain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeValidator returns a ValidatorFunc that yields a fixed finding set,
// ignoring the draft. The mapping layer is what these tests exercise: given
// this finding from Core, the browser must receive exactly this Finding.
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

// mountValidateRouter mounts the subsystem with an injected validator and
// returns the router.
func mountValidateRouter(root string, validate func(context.Context, Model) ([]Finding, error)) http.Handler {
	return mountTestRouterWithValidator(root, validate)
}

func postValidate(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/domain-model/validate", strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// The file-exists suite is gone. TestValidationSourceFilesExist asserted that
// validate.go and parity_test.go were present in this directory — a testcases-
// derived check from when the file layout was itself the deliverable. Once
// parity_test.go moved to core/internal/commands, repointing it at this file left
// a test asserting its own existence, which cannot fail for any reason worth
// knowing about. Deleted rather than repaired.
//
// no_core_import_test.go went earlier and for a related reason: it walked the
// studio module and failed on any import of Core, enforcing a boundary that
// existed only because Studio was a separate Go module. With one module that
// guard would fail on every legitimate shared import.

// --- wrapper-level mapping (Validate over a faked subprocess) ---------------

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

// --- endpoint-level (validateHandler via the router) ------------------------

// TestValidateEndpointFieldTypeViolation asserts a field-type violation returns
// 200 with the closed code and an anchored path — a finding list is a query
// result, never a 4xx.
func TestValidateEndpointFieldTypeViolation(t *testing.T) {
	router := mountValidateRouter(t.TempDir(), func(context.Context, Model) ([]Finding, error) {
		return []Finding{{Field: "entities.Order.fields.qty", Code: "field-type-outside-closed-set", Severity: "error", Message: "unknown type"}}, nil
	})
	w := postValidate(t, router, `{"model":{"schema_version":1,"entities":[]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp validateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, w.Body.String())
	}
	if len(resp.Fields) != 1 || resp.Fields[0].Code != "field-type-outside-closed-set" ||
		resp.Fields[0].Field != "entities.Order.fields.qty" || resp.Fields[0].Severity != "error" {
		t.Fatalf("unexpected fields payload: %+v", resp.Fields)
	}
}

// TestValidateEndpointCleanEmptyList asserts a clean draft returns 200 with an
// empty fields[] — clean is an explicit empty list, not an error.
func TestValidateEndpointCleanEmptyList(t *testing.T) {
	router := mountValidateRouter(t.TempDir(), cleanValidator)
	w := postValidate(t, router, `{"model":{"schema_version":1,"entities":[]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := strings.TrimSpace(w.Body.String()); !strings.Contains(got, `"fields":[]`) {
		t.Fatalf("clean draft should return an empty fields list, got: %s", got)
	}
}

// TestValidateEndpointDeprecatedWarning asserts a deprecated-operations draft
// returns the warning-severity finding while the endpoint still answers 200.
func TestValidateEndpointDeprecatedWarning(t *testing.T) {
	router := mountValidateRouter(t.TempDir(), func(context.Context, Model) ([]Finding, error) {
		return warningFinding(), nil
	})
	w := postValidate(t, router, `{"model":{"schema_version":1,"operations":[{"name":"x"}]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp validateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Fields) != 1 || resp.Fields[0].Code != "domain-operations-deprecated" || resp.Fields[0].Severity != "warning" {
		t.Fatalf("want a single domain-operations-deprecated warning, got %+v", resp.Fields)
	}
}

// TestValidateEndpointWholeModelPath asserts a whole-model violation carries
// the distinguished top-level path.
func TestValidateEndpointWholeModelPath(t *testing.T) {
	router := mountValidateRouter(t.TempDir(), func(context.Context, Model) ([]Finding, error) {
		return []Finding{{Field: WholeModelPath, Code: "missing-schema-version", Severity: "error", Message: "no schema_version"}}, nil
	})
	w := postValidate(t, router, `{"model":{"entities":[]}}`)
	var resp validateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Fields) != 1 || resp.Fields[0].Field != WholeModelPath {
		t.Fatalf("want the whole-model token, got %+v", resp.Fields)
	}
}

// TestValidateEndpointMalformedRequest asserts unparseable bytes are a
// validation-failed HTTP error at 400 — distinct from a finding list — and the
// subprocess/validator is NEVER run.
func TestValidateEndpointMalformedRequest(t *testing.T) {
	var called bool
	router := mountValidateRouter(t.TempDir(), func(context.Context, Model) ([]Finding, error) {
		called = true
		return nil, nil
	})
	w := postValidate(t, router, `{not json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "validation-failed") {
		t.Fatalf("body missing validation-failed: %s", w.Body.String())
	}
	if called {
		t.Fatalf("the validator was run for a malformed request; it must be rejected first")
	}
}

// TestValidateEndpointSideEffectFree asserts the endpoint validates the
// submitted bytes alone: with a clean on-disk model but an error-carrying
// draft, the response reflects the draft and nothing is written to disk.
func TestValidateEndpointSideEffectFree(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	router := mountValidateRouter(root, func(context.Context, Model) ([]Finding, error) {
		return []Finding{{Field: "entities.Order.fields.status", Code: "undeclared-entity-reference", Severity: "error", Message: "OrderStatus not declared"}}, nil
	})
	w := postValidate(t, router, `{"model":{"schema_version":1,"entities":[]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp validateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Fields) != 1 || resp.Fields[0].Field != "entities.Order.fields.status" {
		t.Fatalf("response should reflect the submitted draft, got %+v", resp.Fields)
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != testPopulatedYAML {
		t.Fatalf("validate wrote to disk; it must be side-effect-free:\n%s", after)
	}
}

// TestBinaryLocatedOnceAtConstruction is gone with the thing it described. It
// asserted New resolved a `parlay` executable exactly once and reused the stored
// path on every request — a real property while validation was a subprocess, and
// the reason PARLAY_BIN existed at all. There is no binary to locate now, so
// there is no "once" to assert.
//
// Its replacement cannot live here. Proving the editor validates with no binary
// available requires running Core's real rules, and this package cannot import
// them (Go's internal rule). It is TestEditorValidatesWithNoBinary in
// core/internal/commands, beside the validator Core wires in — which is also
// where the parity suite moved, for the same reason.
