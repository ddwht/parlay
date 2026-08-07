// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin

// Package ui serves the built editor UI from inside the binary.
//
// The bundle is produced by Vite (`make ui`) into dist/ and pulled in by a
// //go:embed directive — the single integration point between the TypeScript and
// Go halves. A released binary therefore serves the UI with no runtime network
// fetches and no on-disk asset directory.
//
// The embed lives in embed.go behind `//go:build !noui`, with embed_noui.go
// standing in under `-tags noui`. Default builds include the UI; the tag exists
// for builds that want the CLI without a Node toolchain in the picture. This
// file holds what both share, so the not-built response is identical whether the
// bundle is absent because nobody ran the build or because the tag excluded it.
package ui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Bundle is the embedded editor UI. It satisfies the harness's server.UIBundle
// interface structurally (ServeIndex), so callers pass a Bundle{} into
// server.BootDeps.UIBundle without this package importing the harness — keeping
// the dependency one-way (caller -> ui, caller -> server).
type Bundle struct{}

// ServeIndex is the harness SPA-fallback handler. The harness routes every
// unmatched non-/api request here. When the path names a real built asset (the
// hashed JS/CSS Vite emits) it is served with its content type; every other path
// serves index.html so client-side routes render within the shell.
func (Bundle) ServeIndex(w http.ResponseWriter, r *http.Request) {
	if p := strings.TrimPrefix(r.URL.Path, "/"); p != "" && p != "index.html" {
		if data, err := readAsset(p); err == nil {
			w.Header().Set("Content-Type", contentTypeFor(p))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
	}
	data, err := readAsset("index.html")
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
// index.html", 500)` — plain text, no code, no remediation. The harness already
// defined the right answer next door in server.ErrUIBundleNotBuilt
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
// imported: the dependency is deliberately one-way so this package cannot import
// the harness. A test asserts the two strings agree, which is the cheap half of
// what an import would have bought.
func writeBundleNotBuilt(w http.ResponseWriter) {
	body := map[string]any{
		"code":     UIBundleNotBuiltCode,
		"severity": "error",
		// The fix once named `make ui` and `parlay-studio`. The first was
		// aspirational — no such target existed, so the one actionable
		// sentence in the envelope told the operator to run a command that
		// would fail. The second named a binary that has since been deleted.
		// `make ui` is a real target now; parlay-studio is not a real
		// anything, so the fix text names neither it nor a second binary.
		"message": "the editor UI bundle has not been built; no index.html is embedded in this binary",
		"fix":     "run `make ui` to build the bundle and rebuild parlay, or use the parlay CLI for this operation",
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

// errNoBundle is what readAsset returns when there is nothing to read. Declared
// here so both build variants report the same condition.
var errNoBundle = fs.ErrNotExist
