// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin

//go:build !noui

// The default build: the Vite bundle is compiled into the binary.
//
// dist/ must contain a build before `go build` runs — `make build` depends on it,
// and `make ui` produces it. Only dist/.gitkeep is tracked, so a checkout that
// skips the UI build embeds a directory with no index.html and every UI route
// answers the documented 503. That was the actual cause of P9-2: goreleaser had
// no UI step, so every released binary shipped that empty directory.

package ui

import (
	"embed"
	"io/fs"
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

// readAsset reads a bundle-relative path out of the embedded build.
func readAsset(p string) ([]byte, error) {
	return fs.ReadFile(dist, p)
}
