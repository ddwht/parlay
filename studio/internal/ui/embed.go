// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin

// Package ui embeds the built Studio UI bundle into the binary. The Vite build
// (`npm run build` in this directory) writes dist/ before `go build`; the
// //go:embed directive below is the single integration point between the
// TypeScript and Go halves of the binary. A released binary therefore serves
// the UI with no runtime network fetches and no on-disk asset directory.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// distFS holds the built UI. The `all:` prefix embeds files whose names begin
// with `.` or `_` too, so nothing Vite emits is silently dropped.
//
//go:embed all:dist
var distFS embed.FS

// dist is distFS rooted at the dist/ directory so lookups use bundle-relative
// paths ("index.html", "assets/index-*.js").
var dist = mustSub(distFS, "dist")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic("ui: embed dist/: " + err.Error())
	}
	return sub
}

// Bundle is the embedded Studio UI. It satisfies the harness's
// server.UIBundle interface structurally (ServeIndex), so main.go passes a
// Bundle{} into server.BootDeps.UIBundle without this package importing the
// harness — keeping the dependency one-way (main -> ui, main -> server).
type Bundle struct{}

// ServeIndex is the harness SPA-fallback handler. The harness routes every
// unmatched non-/api request here. When the path names a real built asset
// (the hashed JS/CSS Vite emits) it is served with its content type; every
// other path serves index.html so client-side routes render within the shell.
func (Bundle) ServeIndex(w http.ResponseWriter, r *http.Request) {
	if p := strings.TrimPrefix(r.URL.Path, "/"); p != "" && p != "index.html" {
		if data, err := fs.ReadFile(dist, p); err == nil {
			w.Header().Set("Content-Type", contentTypeFor(p))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
	}
	data, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "studio ui bundle missing index.html", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// contentTypeFor maps a bundle-relative asset path to a response content type.
// The set covers what the Vite build emits; anything else falls back to a
// generic binary type.
func contentTypeFor(p string) string {
	switch path.Ext(p) {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".map":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
