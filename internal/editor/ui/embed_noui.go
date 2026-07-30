// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin

//go:build noui

// The lean build: no Vite bundle, and therefore no Node toolchain needed to
// produce a working `parlay`.
//
// Everything except the UI routes behaves identically — the CLI is the whole
// product for most invocations, and `parlay domain-edit` is the only surface
// that needs the bundle. Asking for it here answers the same documented 503 an
// unbuilt bundle does, because from the operator's side the situation is the
// same: this binary cannot serve the editor, and the response says so with a
// code and a remediation rather than failing at boot.

package ui

// readAsset always reports no bundle. The `noui` tag excluded the embed, so
// there is nothing to read and no dist/ directory required at build time.
func readAsset(string) ([]byte, error) {
	return nil, errNoBundle
}
