// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness

package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountFunc is a thin adapter for tools that want to register without
// declaring a named type. Callers wrap an inline mount closure with the
// constructor below.
type MountFunc func(r chi.Router)

// Registration constructs a ToolRegistration from a name and a mount
// closure. Useful in tests and in tools that don't want a dedicated
// registration type.
func Registration(name string, mount MountFunc) ToolRegistration {
	return &funcRegistration{name: name, mount: mount}
}

type funcRegistration struct {
	name  string
	mount MountFunc
}

func (f *funcRegistration) Name() string         { return f.name }
func (f *funcRegistration) Mount(r chi.Router)   { f.mount(r) }

// spaFallback returns the handler the harness mounts at chi.Router.NotFound.
// When the bundle is non-nil, unmatched non-/api paths serve the UI's
// index.html. When it is nil, the handler returns 503 with the
// studio-ui-bundle-not-built envelope.
func spaFallback(bundle UIBundle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// /api/* paths that fall through to NotFound are real 404s — they
		// should not be intercepted by the SPA fallback.
		if isAPIPath(r.URL.Path) {
			writeEnvelope(w, http.StatusNotFound, map[string]any{
				"code":   "not-found",
				"target": r.URL.Path,
			})
			return
		}
		if bundle == nil {
			writeEnvelope(w, http.StatusServiceUnavailable, map[string]any{
				"code":    "studio-ui-bundle-not-built",
				"message": "Studio UI bundle is not packaged into this binary; rebuild with `npm run build` in studio/internal/ui/ before `go build`.",
			})
			return
		}
		bundle.ServeIndex(w, r)
	}
}

// isAPIPath reports whether the request path is rooted at /api so the SPA
// fallback can refuse to serve index.html for API paths.
func isAPIPath(path string) bool {
	const prefix = "/api"
	if len(path) < len(prefix) {
		return false
	}
	if path[:len(prefix)] != prefix {
		return false
	}
	if len(path) == len(prefix) {
		return true
	}
	return path[len(prefix)] == '/'
}
