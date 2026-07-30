// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-subsystem-registration
// parlay-artifact: test
// parlay-extends: domain-model-editor/domain-model-editor-validation/cross-cutting/save-validation-gate-before-cas

package domain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/editor/server"
	"github.com/go-chi/chi/v5"
)

// cleanValidator is a validator that reports no findings. Save tests that
// exercise the persistence path inject it so the suite runs without a live
// `parlay` binary and the save-validation gate passes through to the
// compare-and-swap unchanged.
func cleanValidator(context.Context, Model) ([]Finding, error) { return nil, nil }

// mountTestRouterWithValidator builds a bare chi router with the domain
// subsystem mounted, bound to root, with the injected validator wired into
// both the validate endpoint and the save gate. The handlers translate their
// own errors via server.WriteError, so no harness middleware is required.
func mountTestRouterWithValidator(root string, validate func(context.Context, Model) ([]Finding, error)) http.Handler {
	r := chi.NewRouter()
	// nil validator: every caller of this helper replaces s.validate below.
	s := New(root, nil)
	s.validate = validate
	s.Mount(r)
	return r
}

// mountTestRouter mounts the subsystem with a clean validator, so a save
// reaches the compare-and-swap exactly as it did before the gate existed.
func mountTestRouter(root string) http.Handler {
	return mountTestRouterWithValidator(root, cleanValidator)
}

// TestLoadHandlerEmptyProjectReturnsEmptyEtag asserts a GET against a project
// with no model file returns 200 with the empty-model bootstrap etag, never a
// not-found.
func TestLoadHandlerEmptyProjectReturnsEmptyEtag(t *testing.T) {
	router := mountTestRouter(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/domain-model/model", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp modelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	if resp.Etag != string(SentinelEmpty) {
		t.Fatalf("etag = %q, want %q", resp.Etag, SentinelEmpty)
	}
}

// TestLoadHandlerMissingSchemaVersionIsValidationFailed asserts a load against a
// file with no schema_version maps to the validation-failed envelope (400), not
// a generic server-error.
func TestLoadHandlerMissingSchemaVersionIsValidationFailed(t *testing.T) {
	root := writeTempModel(t, "entities: []\n")
	router := mountTestRouter(root)
	req := httptest.NewRequest(http.MethodGet, "/api/domain-model/model", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "validation-failed") {
		t.Fatalf("body missing validation-failed code: %s", w.Body.String())
	}
}

// TestSaveHandlerRejectsBadJSON asserts a malformed body is a validation-failed
// envelope, never a panic or a 500.
func TestSaveHandlerRejectsBadJSON(t *testing.T) {
	router := mountTestRouter(writeTempModel(t, testPopulatedYAML))
	req := httptest.NewRequest(http.MethodPut, "/api/domain-model/model", strings.NewReader("{not json"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "validation-failed") {
		t.Fatalf("body missing validation-failed code: %s", w.Body.String())
	}
}

// TestSaveHandlerStaleEtagIsConflict asserts a save presenting a stale etag
// returns the 409 conflict envelope carrying both etags.
func TestSaveHandlerStaleEtagIsConflict(t *testing.T) {
	router := mountTestRouter(writeTempModel(t, testPopulatedYAML))
	body := `{"model":{"schema_version":1,"enums":[],"entities":[]},"etag":"sha256:stale"}`
	req := httptest.NewRequest(http.MethodPut, "/api/domain-model/model", strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env["code"] != "conflict" {
		t.Fatalf("code = %v, want conflict", env["code"])
	}
	if env["attempted_etag"] != "sha256:stale" {
		t.Fatalf("attempted_etag = %v, want sha256:stale", env["attempted_etag"])
	}
	if env["current_etag"] == "" || env["current_etag"] == nil {
		t.Fatalf("conflict envelope missing current_etag: %v", env)
	}
}

// TestSaveHandlerHappyPath asserts a matching-etag save round-trips through the
// handler and returns the new etag.
func TestSaveHandlerHappyPath(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	router := mountTestRouter(root)

	// Load the current etag through the handler first.
	loadReq := httptest.NewRequest(http.MethodGet, "/api/domain-model/model", nil)
	loadW := httptest.NewRecorder()
	router.ServeHTTP(loadW, loadReq)
	var loaded modelResponse
	if err := json.Unmarshal(loadW.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("decode load: %v", err)
	}

	body, err := json.Marshal(saveRequest{Model: loaded.Model, Etag: loaded.Etag})
	if err != nil {
		t.Fatalf("marshal save: %v", err)
	}
	saveReq := httptest.NewRequest(http.MethodPut, "/api/domain-model/model", strings.NewReader(string(body)))
	saveW := httptest.NewRecorder()
	router.ServeHTTP(saveW, saveReq)

	if saveW.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", saveW.Code, saveW.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Save-validation gate (cross-cutting/save-validation-gate-before-cas)
// ---------------------------------------------------------------------------

// errorFinding is a single error-severity finding — the field-type violation
// used across the gate cases.
func errorFinding() []Finding {
	return []Finding{{
		Field:    "entities.Order.fields.qty",
		Code:     "field-type-outside-closed-set",
		Severity: "error",
		Message:  "field type \"quantity\" is outside the closed set",
	}}
}

// warningFinding is a single warning-severity finding — the sole warning in
// the closed vocabulary. It must never gate a save.
func warningFinding() []Finding {
	return []Finding{{
		Field:    "operations",
		Code:     "domain-operations-deprecated",
		Severity: "warning",
		Message:  "top-level operations are deprecated",
	}}
}

func putSave(t *testing.T, router http.Handler, etag string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":{"schema_version":1,"enums":[],"entities":[]},"etag":"` + etag + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/domain-model/model", strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestSaveGateRejectsErrorFindingWritesNothing asserts a save whose draft
// carries a field-type-outside-closed-set error returns validation-failed with
// the finding listed and leaves the on-disk file untouched — nothing written.
func TestSaveGateRejectsErrorFindingWritesNothing(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	router := mountTestRouterWithValidator(root, func(context.Context, Model) ([]Finding, error) {
		return errorFinding(), nil
	})

	w := putSave(t, router, "sha256:whatever")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 validation-failed; body=%s", w.Code, w.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env["code"] != "validation-failed" {
		t.Fatalf("code = %v, want validation-failed; body=%s", env["code"], w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "field-type-outside-closed-set") &&
		!strings.Contains(w.Body.String(), "entities.Order.fields.qty") {
		t.Fatalf("validation-failed body does not name the finding: %s", w.Body.String())
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != testPopulatedYAML {
		t.Fatalf("on-disk file changed despite a blocked save:\n%s", after)
	}
}

// TestSaveGateWarningOnlySucceeds asserts a save whose only finding is
// domain-operations-deprecated passes the gate and writes through the
// compare-and-swap.
func TestSaveGateWarningOnlySucceeds(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	router := mountTestRouterWithValidator(root, func(context.Context, Model) ([]Finding, error) {
		return warningFinding(), nil
	})

	// Load the current etag through the handler first so the CAS matches.
	loadReq := httptest.NewRequest(http.MethodGet, "/api/domain-model/model", nil)
	loadW := httptest.NewRecorder()
	router.ServeHTTP(loadW, loadReq)
	var loaded modelResponse
	if err := json.Unmarshal(loadW.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("decode load: %v", err)
	}

	body, _ := json.Marshal(saveRequest{Model: loaded.Model, Etag: loaded.Etag})
	saveReq := httptest.NewRequest(http.MethodPut, "/api/domain-model/model", strings.NewReader(string(body)))
	saveW := httptest.NewRecorder()
	router.ServeHTTP(saveW, saveReq)

	if saveW.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (warnings never gate); body=%s", saveW.Code, saveW.Body.String())
	}
}

// TestSaveGateOrdersBeforeCAS asserts the gate runs before the etag
// compare-and-swap: an invalid draft with a stale etag returns validation-
// failed (the gate), not conflict (the etag); once the draft is fixed with the
// same still-stale etag it then returns conflict.
func TestSaveGateOrdersBeforeCAS(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)

	// Invalid draft + stale etag → the gate wins: validation-failed, not conflict.
	blocked := mountTestRouterWithValidator(root, func(context.Context, Model) ([]Finding, error) {
		return errorFinding(), nil
	})
	w1 := putSave(t, blocked, "sha256:stale")
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("stage 1 status = %d, want 400 validation-failed (gate precedes CAS); body=%s", w1.Code, w1.Body.String())
	}
	if !strings.Contains(w1.Body.String(), "validation-failed") {
		t.Fatalf("stage 1 body missing validation-failed: %s", w1.Body.String())
	}

	// Draft fixed (clean validator) but the SAME etag is still stale → now the
	// CAS fires: conflict.
	fixed := mountTestRouterWithValidator(root, cleanValidator)
	w2 := putSave(t, fixed, "sha256:stale")
	if w2.Code != http.StatusConflict {
		t.Fatalf("stage 2 status = %d, want 409 conflict once the gate passes; body=%s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "conflict") {
		t.Fatalf("stage 2 body missing conflict: %s", w2.Body.String())
	}
}

// TestSaveGateDirectAPIGatedIdentically asserts a save submitted directly to
// the PUT endpoint (bypassing the UI's blocked-save affordance) is rejected by
// the authoritative server-side gate identically; there is no force-save path.
func TestSaveGateDirectAPIGatedIdentically(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	router := mountTestRouterWithValidator(root, func(context.Context, Model) ([]Finding, error) {
		return errorFinding(), nil
	})

	// A direct API save with the CORRECT (matching) etag is still rejected —
	// the gate is authoritative regardless of the client and the CAS token.
	w := putSave(t, router, "sha256:stale")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 validation-failed for a direct API save; body=%s", w.Code, w.Body.String())
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != testPopulatedYAML {
		t.Fatalf("direct API save wrote to disk despite the gate:\n%s", after)
	}
}

// TestLoadHandlerNeverEmitsNullCollections asserts the two collections that
// carry no json:",omitempty" are emitted as [] and never as null, for a model
// file that legitimately omits them.
//
// This asserts on the RAW BODY on purpose. Decoding into modelResponse — as
// every other handler test here does — unmarshals JSON null straight back
// into a nil slice, so a struct-level assertion passes on exactly the bytes
// that break the client and cannot catch this class of defect at all. The
// same reasoning is recorded at core/internal/commands/domain_parity_test.go,
// which re-declares its wire structs locally rather than sharing them with
// the code under test.
//
// The bug: enums: and entities: are optional in the schema, so a file that
// omits either is valid. Left nil they marshalled to `null`, the UI types
// them as non-optional arrays and calls .length on both, and the page threw
// during render and unmounted — a blank screen with the error only in the
// browser console. The empty-model bootstrap built real empty slices, so a
// new project was fine and only a real file was broken.
func TestLoadHandlerNeverEmitsNullCollections(t *testing.T) {
	cases := []struct {
		name  string
		model string
	}{
		{
			name: "entities only, no enums key",
			model: "schema_version: 1\n" +
				"entities:\n  - name: Widget\n    fields:\n      - name: id\n        type: uuid\n",
		},
		{
			name:  "enums only, no entities key",
			model: "schema_version: 1\nenums:\n  - name: Role\n    values:\n      - value: admin\n",
		},
		{
			name:  "neither key present",
			model: "schema_version: 1\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := mountTestRouter(writeTempModel(t, tc.model))
			req := httptest.NewRequest(http.MethodGet, "/api/domain-model/model", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, forbidden := range []string{`"enums":null`, `"entities":null`} {
				if strings.Contains(body, forbidden) {
					t.Errorf("body contains %s — the UI dereferences .length on it and unmounts; body=%s", forbidden, body)
				}
			}
			for _, want := range []string{`"enums":`, `"entities":`} {
				if !strings.Contains(body, want) {
					t.Errorf("body is missing %s entirely; body=%s", want, body)
				}
			}
		})
	}
}

// A hand-broken domain-model.yaml must come back actionable, not as a bare
// server-error.
//
// It used to produce a 500 whose body was a request id and nothing else, while
// `parlay validate` on the same file named the offending line. The user best
// placed to fix the file was the one told least about it — and Phase 13's
// "clean in the editor and passes the build are the same statement" stopped
// holding for the most ordinary kind of breakage there is.
func TestLoadHandlerReportsUnparseableYAMLActionably(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "domain-model.yaml") // Load takes the root, not this
	// A duplicate mapping key — what a careless hand-edit actually produces.
	if err := os.WriteFile(path, []byte("entities:\n  - name: A\nentities:\n  - name: B\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := Load(context.Background(), dir)
	if err == nil {
		t.Fatal("expected a load failure")
	}
	if !errors.Is(err, ErrInvalidYAML) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidYAML", err)
	}

	mapped := mapLoadError(err)
	var ve *server.ValidationError
	if !errors.As(mapped, &ve) {
		t.Fatalf("mapped = %T (%v), want a *server.ValidationError so the client gets validation-failed, not server-error", mapped, mapped)
	}
	if len(ve.Fields) != 1 {
		t.Fatalf("fields = %+v", ve.Fields)
	}
	// The parser's own message must survive — the line number is the whole
	// point of surfacing this at all.
	if !strings.Contains(ve.Fields[0].Message, "line") {
		t.Errorf("message should carry the parser's line reference: %q", ve.Fields[0].Message)
	}
}
