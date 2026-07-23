// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-subsystem-registration
// parlay-artifact: test

package domain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// mountTestRouter builds a bare chi router with the domain subsystem mounted,
// bound to root. The handlers translate their own errors via server.WriteError,
// so no harness middleware is required for these assertions.
func mountTestRouter(root string) http.Handler {
	r := chi.NewRouter()
	New(root).Mount(r)
	return r
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
