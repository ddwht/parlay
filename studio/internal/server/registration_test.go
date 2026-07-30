// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness
// parlay-artifact: test

package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ddwht/parlay/studio/internal/config"
	"github.com/ddwht/parlay/studio/internal/server"
)

// TestRegistrationMountsHandler confirms a Registration constructor wires
// its mount closure onto the router.
func TestRegistrationMountsHandler(t *testing.T) {
	hit := false
	reg := server.Registration("domain-model", func(r chi.Router) {
		r.Get("/api/domain-model/ping", func(w http.ResponseWriter, req *http.Request) {
			hit = true
			w.WriteHeader(http.StatusOK)
		})
	})
	if reg.Name() != "domain-model" {
		t.Fatalf("Registration.Name()=%q, want domain-model", reg.Name())
	}

	srv, err := server.New(server.Deps{
		Config: config.Config{},
		Tools:  []server.ToolRegistration{reg},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/domain-model/ping", nil)
	srv.Router.ServeHTTP(rr, req)

	if !hit {
		t.Fatal("registered handler did not fire")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("registered handler: status=%d, want 200", rr.Code)
	}
}

// TestUnknownAPIRouteReturnsNotFoundEnvelope asserts an /api/* path with
// no registered handler returns the not-found envelope (not the SPA
// fallback's index.html).
func TestUnknownAPIRouteReturnsNotFoundEnvelope(t *testing.T) {
	srv, err := server.New(server.Deps{Config: config.Config{}})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nope/missing", nil)
	srv.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("/api/nope/missing: status=%d, want 404", rr.Code)
	}
}
