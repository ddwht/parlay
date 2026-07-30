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
	"encoding/json"
	"fmt"
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
		writeBundleNotBuilt(w)
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

// writeBundleNotBuilt reports an unbuilt UI bundle as the documented error
// rather than as a bare 500.
//
// This path previously emitted `http.Error(w, "studio ui bundle missing
// index.html", 500)` — plain text, no code, no remediation. The harness
// already defined the right answer next door in server.ErrUIBundleNotBuilt
// ("studio-ui-bundle-not-built"), and the reachable path did not use it. A
// regression run hit this and got a third, undocumented state: neither the UI
// shell nor the documented 503, with nothing in the response telling the
// operator that a build step had been skipped.
//
// 503 rather than 500 because the condition is a missing build artifact, not a
// server fault: the service is correctly configured and temporarily unable to
// serve this route, and the fix is a command the operator can run.
//
// The code string is duplicated from server.ErrUIBundleNotBuilt rather than
// imported: the dependency is deliberately one-way (main -> ui, main -> server)
// so this package cannot import the harness. A test asserts the two strings
// agree, which is the cheap half of what an import would have bought.
func writeBundleNotBuilt(w http.ResponseWriter) {
	body := map[string]any{
		"code":     UIBundleNotBuiltCode,
		"severity": "error",
		"message":  "the Studio UI bundle has not been built; no index.html is embedded in this binary",
		"fix":      "build the UI bundle (make ui) and rebuild parlay-studio, or use the parlay CLI for this operation",
	}
	buf, err := json.Marshal(body)
	if err != nil {
		// Never loop back through the JSON path on a marshal failure.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, UIBundleNotBuiltCode)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write(buf)
}

// UIBundleNotBuiltCode is the stable error code for an unbuilt UI bundle. It
// must equal server.ErrUIBundleNotBuilt's message; see writeBundleNotBuilt.
const UIBundleNotBuiltCode = "studio-ui-bundle-not-built"
