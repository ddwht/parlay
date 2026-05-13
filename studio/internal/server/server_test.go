// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness
// parlay-artifact: test

package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/parlay-tool/parlay/studio/internal/config"
	"github.com/parlay-tool/parlay/studio/internal/server"
)

// TestNewRejectsDuplicateToolNames asserts that server.New refuses to
// construct when two registrations report the same Name().
func TestNewRejectsDuplicateToolNames(t *testing.T) {
	reg1 := server.Registration("domain-model", func(r chi.Router) {})
	reg2 := server.Registration("domain-model", func(r chi.Router) {})

	_, err := server.New(server.Deps{
		Config: config.Config{},
		Tools:  []server.ToolRegistration{reg1, reg2},
	})
	if err == nil {
		t.Fatal("server.New: expected ErrToolNameCollision, got nil")
	}
	if !strings.Contains(err.Error(), "studio-tool-name-collision") {
		t.Fatalf("server.New: error %q does not name studio-tool-name-collision", err)
	}
}

// TestMiddlewareOrder asserts the documented five-stage stack order is the
// one the source file declares. This is a file-content assertion (the
// stack order is part of the harness contract) rather than a behavioural
// one because chi does not expose the middleware chain post-construction.
func TestMiddlewareOrder(t *testing.T) {
	src := readServerGoSource(t)
	// The test asserts on the executable lines, not the prose comments.
	// Find each r.Use(...) call site and confirm the documented order.
	wantOrder := []string{
		"r.Use(chimw.RequestID)",
		"r.Use(PanicRecovery)",
		"r.Use(Logger)",
		"r.Use(ErrorEnvelopeTranslate)",
	}
	lastIdx := -1
	for _, want := range wantOrder {
		idx := strings.Index(src, want)
		if idx < 0 {
			t.Fatalf("server.go: stack call site %q absent", want)
		}
		if idx < lastIdx {
			t.Fatalf("server.go: stack call site %q appears before previous call", want)
		}
		lastIdx = idx
	}
	if !strings.Contains(src, "IdleTimeoutReset(deps.IdleTracker)") {
		t.Fatal("server.go: IdleTimeoutReset call site missing from the middleware mount")
	}
}

// TestHarnessDoesNotImportToolPackages asserts that server.go and its
// siblings do not reach back into any Studio tool package.
func TestHarnessDoesNotImportToolPackages(t *testing.T) {
	for _, name := range []string{"server.go", "boot.go", "middleware.go", "envelopes.go", "registration.go", "health.go", "idle.go"} {
		path := filepath.Join(packageDir(t), name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, forbidden := range []string{
			"studio/internal/domain",
			"studio/internal/designloop",
			"studio/internal/files",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s imports forbidden tool package %q", name, forbidden)
			}
		}
	}
}

// TestHealthHandlerReturnsOK asserts /api/health returns 200 with the
// documented {status: "ok"} envelope.
func TestHealthHandlerReturnsOK(t *testing.T) {
	srv := newTestServer(t, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	srv.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("/api/health: status=%d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("/api/health: parse body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("/api/health: body=%v, want status=ok", body)
	}
}

// TestSPAFallback503WhenBundleAbsent asserts the unmatched non-/api path
// returns 503 with studio-ui-bundle-not-built when no UI bundle is
// embedded.
func TestSPAFallback503WhenBundleAbsent(t *testing.T) {
	srv := newTestServer(t, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some/unknown/path", nil)
	srv.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("SPA fallback: status=%d, want 503", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("SPA fallback: parse body: %v", err)
	}
	if body["code"] != "studio-ui-bundle-not-built" {
		t.Fatalf("SPA fallback: code=%v, want studio-ui-bundle-not-built", body["code"])
	}
}

// fakeUIBundle is a tiny stand-in for the embedded UI bundle.
type fakeUIBundle struct{ body string }

func (b *fakeUIBundle) ServeIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = io.WriteString(w, b.body)
}

// TestSPAFallbackServesIndex asserts that when a UIBundle is supplied,
// unmatched non-/api paths serve its index.html.
func TestSPAFallbackServesIndex(t *testing.T) {
	bundle := &fakeUIBundle{body: "<!doctype html>ui-bundle-index.html"}
	srv := newTestServer(t, nil, bundle)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some/unknown/path", nil)
	srv.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("SPA fallback: status=%d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ui-bundle-index.html") {
		t.Fatalf("SPA fallback body does not contain the index sentinel: %q", rr.Body.String())
	}
}

// TestShutdownHandlerWritesReason asserts POST /api/shutdown writes the
// explicit reason onto the shutdown channel and returns 202.
func TestShutdownHandlerWritesReason(t *testing.T) {
	ch := make(chan string, 1)
	srv := newTestServer(t, ch, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/shutdown", nil)
	srv.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("POST /api/shutdown: status=%d, want 202", rr.Code)
	}
	select {
	case reason := <-ch:
		if reason != "explicit: /api/shutdown" {
			t.Fatalf("shutdown reason=%q, want %q", reason, "explicit: /api/shutdown")
		}
	default:
		t.Fatal("shutdown channel did not receive the explicit reason")
	}
}

// TestMountedToolReceivesRequestIDContext asserts a handler mounted via
// the registration interface sees a request whose context carries the
// chi-assigned X-Request-ID and that the response header matches.
func TestMountedToolReceivesRequestIDContext(t *testing.T) {
	var observed string
	reg := server.Registration("example-tool", func(r chi.Router) {
		r.Get("/api/example-tool/ping", func(w http.ResponseWriter, req *http.Request) {
			observed = chimw.GetReqID(req.Context())
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
	req := httptest.NewRequest(http.MethodGet, "/api/example-tool/ping", nil)
	srv.Router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("mounted ping: status=%d, want 200", rr.Code)
	}
	if observed == "" {
		t.Fatal("handler observed empty request_id; expected chi-assigned value")
	}
	if rr.Header().Get("X-Request-ID") != observed {
		t.Fatalf("X-Request-ID header=%q does not match observed=%q",
			rr.Header().Get("X-Request-ID"), observed)
	}
}

func newTestServer(t *testing.T, shutdownCh chan<- string, bundle server.UIBundle) *server.Server {
	t.Helper()
	srv, err := server.New(server.Deps{
		Config:       config.Config{},
		ShutdownChan: shutdownCh,
		UIBundle:     bundle,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

func readServerGoSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(packageDir(t), "server.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: failed")
	}
	return filepath.Dir(file)
}
