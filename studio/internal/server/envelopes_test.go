// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness
// parlay-artifact: test

package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/parlay-tool/parlay/studio/internal/config"
	"github.com/parlay-tool/parlay/studio/internal/server"
)

// TestEnvelopeValidationFailed asserts the validation-failed sentinel
// produces 400 with the {code, fields[]} envelope shape.
func TestEnvelopeValidationFailed(t *testing.T) {
	verr := &server.ValidationError{Fields: []server.FieldError{{Field: "name", Message: "required"}}}
	rr := exerciseError(t, "/api/example-tool/invalid", verr)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("validation-failed: status=%d, want 400", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("validation-failed: parse body: %v", err)
	}
	if body["code"] != "validation-failed" {
		t.Fatalf("validation-failed: code=%v, want validation-failed", body["code"])
	}
	fields, ok := body["fields"].([]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("validation-failed: fields=%v, want non-empty array", body["fields"])
	}
}

// TestEnvelopeNotFound asserts the not-found sentinel produces 404 with
// the {code, target} envelope shape.
func TestEnvelopeNotFound(t *testing.T) {
	nfe := &server.NotFoundError{Target: "entity-123"}
	rr := exerciseError(t, "/api/example-tool/missing", nfe)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("not-found: status=%d, want 404", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("not-found: parse body: %v", err)
	}
	if body["code"] != "not-found" || body["target"] != "entity-123" {
		t.Fatalf("not-found: body=%v, want {code:not-found,target:entity-123}", body)
	}
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("not-found: missing X-Request-ID response header")
	}
}

// TestEnvelopeConflict asserts the conflict sentinel produces 409 with the
// {code, current_etag, attempted_etag} envelope shape.
func TestEnvelopeConflict(t *testing.T) {
	ce := &server.ConflictError{CurrentETag: "etag-A", AttemptedETag: "etag-B"}
	rr := exerciseError(t, "/api/example-tool/etag-mismatch", ce)

	if rr.Code != http.StatusConflict {
		t.Fatalf("conflict: status=%d, want 409", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("conflict: parse body: %v", err)
	}
	if body["code"] != "conflict" {
		t.Fatalf("conflict: code=%v, want conflict", body["code"])
	}
	if body["current_etag"] != "etag-A" || body["attempted_etag"] != "etag-B" {
		t.Fatalf("conflict: etag envelope=%v", body)
	}
}

// TestEnvelopeUnmappedFallsThroughToServerError asserts an unmapped error
// produces 500 with the {code, request_id} envelope shape and that the
// internal detail does NOT appear in the response body (it does appear in
// the log).
func TestEnvelopeUnmappedFallsThroughToServerError(t *testing.T) {
	var logBuf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOut)

	internal := errors.New("unmapped-internal-FNORD-detail")
	rr := exerciseError(t, "/api/example-tool/unmapped", internal)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("unmapped: status=%d, want 500", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmapped: parse body: %v", err)
	}
	if body["code"] != "server-error" {
		t.Fatalf("unmapped: code=%v, want server-error", body["code"])
	}
	if rid, ok := body["request_id"].(string); !ok || rid == "" {
		t.Fatalf("unmapped: missing request_id; body=%v", body)
	}
	if strings.Contains(rr.Body.String(), "unmapped-internal-FNORD-detail") {
		t.Fatalf("internal detail leaked into response body: %q", rr.Body.String())
	}
	if !strings.Contains(logBuf.String(), "unmapped-internal-FNORD-detail") {
		t.Fatalf("internal detail did not appear in log; log=%q", logBuf.String())
	}
}

func exerciseError(t *testing.T, path string, target error) *httptest.ResponseRecorder {
	t.Helper()
	reg := server.Registration("example-tool", func(r chi.Router) {
		r.Get(path, func(w http.ResponseWriter, req *http.Request) {
			server.WriteError(w, req, target)
		})
	})
	srv, err := server.New(server.Deps{
		Config: config.Config{},
		Tools:  []server.ToolRegistration{reg},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	srv.Router.ServeHTTP(rr, req)
	return rr
}
