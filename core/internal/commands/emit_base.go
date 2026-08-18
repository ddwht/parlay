// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-emit-base-resolution
//
// One answer to "where does this adapter's generated code go", used by every
// site that needs it.
//
// There were three, and they disagreed. Plan derivation replaced the adapter's
// source-root with the target root; the toolchain write-set check prefixed one
// with the other; the composition store path ignored the target root entirely.
// So for angular-nest-prisma the plan wrote apps/web/core/state/domain.store.ts,
// the write-set authorized apps/web/src/app/**, and composition looked for
// src/app/core/state/domain.store.ts — three answers, no agreement, and no
// single place to correct.

package commands

import "path"

// emitBase resolves the directory that an adapter's path templates,
// packages and entry-point are relative to.
//
// Two fields carry the answer, because one could not:
//
//   - project-root is the deployable project location (apps/web, apps/api, or
//     "." for a single-package repo). This is what an adapter-set's
//     targets.<kind>.root replaces — the topology decides where a slot lands.
//   - source-root is the framework's conventional inner directory (src/,
//     src/app/, cmd/). The topology has no opinion about this and must not
//     replace it: it is a property of the framework, not of the project layout.
//
// Before the split, source-root meant both. Backend adapters read it as a
// project location (nestjs: apps/api, with src/ inside its templates) and
// presentation adapters as a framework directory (react-antd: src/, with
// templates starting at features/). Replacing it is lossless under the first
// reading and destructive under the second, so react-antd pinned to
// root: apps/web derived apps/web/features/… while the app built from
// apps/web/src/ — outside tsconfig's include, therefore not type-checked, not
// bundled, and invisible to a green build. Three of the four bundled presets
// were wrong; the fourth was right only because its root happened to equal its
// source-root.
//
// LEGACY MODE. An adapter that declares no project-root gets exactly the old
// behaviour — targetRoot replaces sourceRoot — bugs included. Third-party and
// onboarded adapters therefore keep working unchanged, and nothing starts
// emitting to a new location because parlay was upgraded. adapter-root-override-lossy
// reports the shapes where that old behaviour is destructive, so the silent
// failure is loud while the file is still legacy.
func emitBase(projectRoot, sourceRoot, targetRoot string) string {
	if projectRoot == "" {
		// Legacy: one field, replaced wholesale.
		if targetRoot != "" {
			return path.Clean(targetRoot)
		}
		return cleanOrEmpty(sourceRoot)
	}

	base := targetRoot
	if base == "" {
		base = projectRoot
	}
	joined := path.Join(base, sourceRoot)
	if joined == "." {
		return ""
	}
	return joined
}

// cleanOrEmpty normalizes a root while preserving "no root at all" as the
// empty string. path.Clean("") is ".", which would turn an unset source-root
// into a "./" prefix on every derived path.
func cleanOrEmpty(p string) string {
	if p == "" {
		return ""
	}
	c := path.Clean(p)
	if c == "." {
		return ""
	}
	return c
}
