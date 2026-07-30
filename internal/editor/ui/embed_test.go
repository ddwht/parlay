// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
// parlay-artifact: test

//go:build !noui

package ui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/editor/config"
	"github.com/ddwht/parlay/internal/editor/server"
	"github.com/ddwht/parlay/internal/editor/ui"
)

// TestBundleSatisfiesUIBundle is the compile-time-plus-runtime check that the
// embedded bundle plugs into the harness UI-bundle contract.
func TestBundleSatisfiesUIBundle(t *testing.T) {
	var _ server.UIBundle = ui.Bundle{}
}

// newHarness constructs the harness with the embedded bundle wired in, exactly
// as main.go does via BootDeps.UIBundle.
func newHarness(t *testing.T) *server.Server {
	t.Helper()
	srv, err := server.New(server.Deps{
		Config:       config.Config{},
		ShutdownChan: make(chan string, 1),
		UIBundle:     ui.Bundle{},
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

// TestRootServesEditorShellNotPlaceholder asserts that with the bundle embedded,
// fetching / serves the editor shell HTML and the studio-ui-bundle-not-built
// 503 placeholder no longer occurs.
func TestRootServesEditorShellNotPlaceholder(t *testing.T) {
	srv := newHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "studio-ui-bundle-not-built") {
		t.Fatal("root still serves the missing-bundle placeholder")
	}
	if !strings.Contains(body, "<div id=\"root\">") && !strings.Contains(body, "<html") {
		t.Fatalf("root did not serve the editor shell HTML: %s", body)
	}
	if !strings.Contains(body, "/src/main.tsx") && !strings.Contains(body, "/assets/") {
		t.Fatalf("shell HTML missing the app entry script: %s", body)
	}
}

// TestClientRouteServesShell asserts an unmatched client route (e.g.
// /domain-model) renders within the shell via the SPA fallback.
func TestClientRouteServesShell(t *testing.T) {
	srv := newHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/domain-model", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "id=\"root\"") {
		t.Fatalf("client route did not serve the shell: %s", w.Body.String())
	}
}

// TestAPIPathNotServedShell asserts an unknown /api/* path surfaces the harness
// not-found envelope, never index.html.
func TestAPIPathNotServedShell(t *testing.T) {
	srv := newHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/unknown-path", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "id=\"root\"") {
		t.Fatal("an /api path was served the SPA shell")
	}
	if !strings.Contains(body, "not-found") {
		t.Fatalf("expected the not-found envelope for an unknown /api path: %s", body)
	}
}

// TestServeIndexServesBuiltAsset asserts a request naming a real built asset is
// served with a non-HTML content type (the hashed JS/CSS Vite emits).
func TestServeIndexServesBuiltAsset(t *testing.T) {
	// Discover an asset path from the served index.html.
	srv := newHarness(t)
	idxReq := httptest.NewRequest(http.MethodGet, "/", nil)
	idxW := httptest.NewRecorder()
	srv.Router.ServeHTTP(idxW, idxReq)
	assetPath := extractAsset(idxW.Body.String())
	if assetPath == "" {
		t.Skip("no hashed asset referenced in index.html; nothing to assert")
	}

	req := httptest.NewRequest(http.MethodGet, assetPath, nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("asset %s: status = %d, want 200", assetPath, w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		t.Fatalf("asset %s served as HTML (content-type %q) — the SPA JS would not load", assetPath, ct)
	}
	if _, err := io.ReadAll(w.Body); err != nil {
		t.Fatalf("read asset body: %v", err)
	}
}

// extractAsset pulls the first /assets/... reference out of index.html.
func extractAsset(html string) string {
	for _, marker := range []string{"src=\"", "href=\""} {
		idx := strings.Index(html, marker+"/assets/")
		if idx == -1 {
			continue
		}
		start := idx + len(marker)
		end := strings.IndexByte(html[start:], '"')
		if end == -1 {
			continue
		}
		return html[start : start+end]
	}
	return ""
}
