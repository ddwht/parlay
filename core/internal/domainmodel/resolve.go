// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/domain-model-read-path-resolution

// Package domainmodel is the project domain model as a document: the
// load/save persistence path (loader reuse, compare-and-swap, deterministic
// serialization, deprecated-operations passthrough), the read-path resolver,
// the validation seam, and feature contributions.
//
// It was the browser editor's domain-model subsystem, and the route group,
// handlers, and wire envelopes went with that editor. What survived is
// everything that was never about HTTP — a document, a token, and
// the rules for replacing one safely — so the package now depends on no
// transport at all.
package domainmodel

import "path/filepath"

// modelFileName is the single canonical on-disk file this package reads and
// writes. The legacy domain-model.md is never consulted — see resolveModelPath.
const modelFileName = "domain-model.yaml"

// resolveModelPath returns the sole file this package targets for a given
// resolved project root: <root>/domain-model.yaml, and nothing else.
//
// This encodes the domain-model artifact's read-path precedence rule: all
// reads and writes target the resolved root's domain-model.yaml. The legacy
// domain-model.md is never parsed, never merged, and never consulted as a
// fallback under any code path. In a multi-root project, v1 offers no root
// selector and no configuration override — the resolved root's file is the
// sole target.
func resolveModelPath(root string) string {
	return filepath.Join(root, modelFileName)
}
