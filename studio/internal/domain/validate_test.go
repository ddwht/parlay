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

// withFakeExec swaps the subprocess seam for a fake so the wrapper's parsing
// and mapping can be exercised without a live `parlay` on PATH.
func withFakeExec(t *testing.T, fn func(ctx context.Context, bin string, stdin []byte) ([]byte, error)) {
	t.Helper()
	prev := execValidateJSON
	execValidateJSON = fn
	t.Cleanup(func() { execValidateJSON = prev })
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

// --- file-exists cases ------------------------------------------------------

// TestValidationSourceFilesExist mirrors the file-exists testcases: the
// subprocess wrapper, the parity suite, and the no-Core-import guard all live
// in this package.
func TestValidationSourceFilesExist(t *testing.T) {
	for _, f := range []string{"validate.go", "parity_test.go", "no_core_import_test.go"} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected source file %s to exist: %v", f, err)
		}
	}
}

// --- wrapper-level mapping (Validate over a faked subprocess) ---------------

// TestValidateMapsFieldTypeViolation asserts a field-type violation from Core
// is mapped verbatim into a Finding — closed code, anchored element path, and
// error severity unchanged.
func TestValidateMapsFieldTypeViolation(t *testing.T) {
	withFakeExec(t, func(context.Context, string, []byte) ([]byte, error) {
		return []byte(`[{"code":"field-type-outside-closed-set","message":"unknown type","context":"entities.Order.fields.qty","fix":"use a closed-set type","severity":"error"}]`), nil
	})
	findings, err := Validate(context.Background(), "parlay", Model{})
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
	withFakeExec(t, func(context.Context, string, []byte) ([]byte, error) {
		return []byte(`[]`), nil
	})
	findings, err := Validate(context.Background(), "parlay", Model{SchemaVersion: 1})
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
	withFakeExec(t, func(context.Context, string, []byte) ([]byte, error) {
		return []byte(`[{"code":"domain-operations-deprecated","message":"deprecated","context":"operations","fix":"migrate","severity":"warning"}]`), nil
	})
	findings, err := Validate(context.Background(), "parlay", Model{})
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
	withFakeExec(t, func(context.Context, string, []byte) ([]byte, error) {
		return []byte(`[{"code":"missing-schema-version","message":"no schema_version","context":"<domain-model>","fix":"add schema_version","severity":"error"}]`), nil
	})
	findings, err := Validate(context.Background(), "parlay", Model{})
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
	withFakeExec(t, func(_ context.Context, _ string, stdin []byte) ([]byte, error) {
		captured = stdin
		return []byte(`[]`), nil
	})
	model := Model{
		SchemaVersion: 1,
		Operations:    []map[string]any{{"name": "cancel-order", "kind": "command"}},
	}
	if _, err := Validate(context.Background(), "parlay", model); err != nil {
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
	withFakeExec(t, func(_ context.Context, _ string, stdin []byte) ([]byte, error) {
		captured = stdin
		return []byte(`[]`), nil
	})
	if _, err := Validate(context.Background(), "parlay", Model{ /* SchemaVersion: 0 */ }); err != nil {
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

// TestBinaryLocatedOnceAtConstruction asserts New resolves the parlay binary
// once and stores it on the subsystem; the validate path reuses the stored
// value rather than re-resolving per request.
func TestBinaryLocatedOnceAtConstruction(t *testing.T) {
	// Point PARLAY_BIN at a runnable temp file so resolution is deterministic
	// and needs no real `parlay` on PATH.
	bin := filepath.Join(t.TempDir(), "parlay")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	t.Setenv("PARLAY_BIN", bin)

	s := New("/project")
	if s.parlayBin != bin {
		t.Fatalf("parlayBin = %q, want the resolved path %q (located once at construction)", s.parlayBin, bin)
	}
	// The stored path is a field; a second validate call cannot re-resolve it.
	// Confirm it stays stable after use.
	withFakeExec(t, func(_ context.Context, gotBin string, _ []byte) ([]byte, error) {
		if gotBin != bin {
			t.Fatalf("validate used bin %q, want the once-resolved %q", gotBin, bin)
		}
		return []byte(`[]`), nil
	})
	if _, err := s.validate(context.Background(), Model{SchemaVersion: 1}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if s.parlayBin != bin {
		t.Fatalf("parlayBin changed after a request: %q", s.parlayBin)
	}
}
