// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness
// parlay-artifact: test

package server_test

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ddwht/parlay/internal/editor/config"
	"github.com/ddwht/parlay/internal/editor/server"
)

// TestPanicRecoveryReturnsServerErrorEnvelope asserts that a handler panic
// is recovered, the response is 500 with the server-error envelope, and
// the panic detail is logged (with the request_id) rather than returned
// in the body.
func TestPanicRecoveryReturnsServerErrorEnvelope(t *testing.T) {
	var logBuf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOut)

	reg := server.Registration("example-tool", func(r chi.Router) {
		r.Get("/api/example-tool/panic", func(w http.ResponseWriter, req *http.Request) {
			panic("synthetic-panic-FNORD")
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
	req := httptest.NewRequest(http.MethodGet, "/api/example-tool/panic", nil)
	srv.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("panic recovery: status=%d, want 500", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("panic recovery: parse body: %v", err)
	}
	if body["code"] != "server-error" {
		t.Fatalf("panic recovery: code=%v, want server-error", body["code"])
	}
	if rid, ok := body["request_id"].(string); !ok || rid == "" {
		t.Fatalf("panic recovery: missing request_id; body=%v", body)
	}
	// The panic detail must NOT leak into the response body.
	if strings.Contains(rr.Body.String(), "synthetic-panic-FNORD") {
		t.Fatalf("panic detail leaked into response body: %q", rr.Body.String())
	}
	// The panic detail SHOULD appear in the log line.
	if !strings.Contains(logBuf.String(), "synthetic-panic-FNORD") {
		t.Fatalf("panic detail did not appear in log; log=%q", logBuf.String())
	}
}

// TestLoggerEmitsRequestIDHeader asserts the Logger middleware writes the
// X-Request-ID response header. This is the load-bearing assertion for
// the "every HTTP response carries X-Request-ID" invariant.
func TestLoggerEmitsRequestIDHeader(t *testing.T) {
	reg := server.Registration("example-tool", func(r chi.Router) {
		r.Get("/api/example-tool/ok", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
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
	req := httptest.NewRequest(http.MethodGet, "/api/example-tool/ok", nil)
	srv.Router.ServeHTTP(rr, req)

	if rid := rr.Header().Get("X-Request-ID"); rid == "" {
		t.Fatal("X-Request-ID header missing on response")
	}
}
