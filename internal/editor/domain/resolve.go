// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-read-path-resolution

// Package domain is the Studio domain-model editor's tool subsystem: the
// /api/domain-model route group, the load/save persistence path (loader
// reuse, compare-and-swap, deterministic serialization, deprecated-operations
// passthrough), and the read-path resolver.
//
// The package depends on the web-server harness (internal/server) ONLY for
// the tool-registration mechanism and the closed error-envelope kinds
// (validation-failed, not-found, conflict, server-error). The harness never
// imports this package: the dependency is one-way and acyclic.
package domain

import "path/filepath"

// modelFileName is the single canonical on-disk file the editor reads and
// writes. The legacy domain-model.md is never consulted — see resolveModelPath.
const modelFileName = "domain-model.yaml"

// resolveModelPath returns the sole file the editor targets for a given
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
