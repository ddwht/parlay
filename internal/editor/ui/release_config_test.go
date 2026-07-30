// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
// parlay-artifact: test

// The release path must build this bundle. Nothing in Go can notice that it
// doesn't: the embed succeeds against an empty dist/, the binary links, the suite
// passes, and the failure appears only when someone opens the editor from an
// installed copy. That is precisely what happened — P9-2, where every released
// binary served studio-ui-bundle-not-built because .goreleaser.yaml had no npm
// step and the tracked dist/ holds only .gitkeep.
//
// So the guard is on the release configuration itself. It reads structure rather
// than matching sentences: a reworded comment or a renamed flag should not fail
// the build, but removing the UI build from the release path should.

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/internal/testsupport"
)

// uiDirFromRoot is this package's path relative to the module root — the path a
// release hook has to name to build the bundle.
const uiDirFromRoot = "internal/editor/ui"

func TestGoreleaserBuildsTheUIBundle(t *testing.T) {
	root, err := testsupport.ModuleRoot(".")
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}

	var cfg struct {
		Before struct {
			Hooks []string `yaml:"hooks"`
		} `yaml:"before"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}
	if len(cfg.Before.Hooks) == 0 {
		t.Fatal("no before.hooks in .goreleaser.yaml; the release cannot be building the UI bundle")
	}

	// Two separate claims, because either alone is satisfiable without building
	// anything: a hook that installs dependencies in this directory, and a hook
	// that runs a build in it.
	var installs, builds bool
	for _, h := range cfg.Before.Hooks {
		if !strings.Contains(h, uiDirFromRoot) {
			continue
		}
		if strings.Contains(h, "ci") || strings.Contains(h, "install") {
			installs = true
		}
		if strings.Contains(h, "build") {
			builds = true
		}
	}
	if !installs {
		t.Errorf("no before.hook installs UI dependencies in %s; hooks were %v", uiDirFromRoot, cfg.Before.Hooks)
	}
	if !builds {
		t.Errorf("no before.hook builds the UI in %s — a release built from this config would embed an empty dist/ and serve %s; hooks were %v",
			uiDirFromRoot, UIBundleNotBuiltCode, cfg.Before.Hooks)
	}
}

// TestMakeBuildDependsOnTheBundle covers the local half. `make build` has to
// produce the bundle before compiling, or a fresh clone yields a binary that
// cannot serve the editor — the same defect as the release one, reached by a
// different route.
func TestMakeBuildDependsOnTheBundle(t *testing.T) {
	root, err := testsupport.ModuleRoot(".")
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)

	// The build target must carry a prerequisite, whatever it is named. Matching
	// the variable rather than a literal path keeps this from breaking when the
	// path moves — which it already did once, studio/internal/ui ->
	// internal/editor/ui.
	var buildLine string
	for _, line := range strings.Split(makefile, "\n") {
		if strings.HasPrefix(line, "build:") {
			buildLine = line
			break
		}
	}
	if buildLine == "" {
		t.Fatal("no `build:` target in the Makefile")
	}
	deps := strings.TrimSpace(strings.TrimPrefix(buildLine, "build:"))
	if deps == "" {
		t.Fatal("`make build` has no prerequisites; it cannot be building the UI bundle first")
	}
	if !strings.Contains(makefile, "UI_BUNDLE") {
		t.Error("the Makefile does not define UI_BUNDLE; `make build` has nothing to depend on")
	}

	// And a `ui` target has to exist, because the not-built envelope tells the
	// operator to run exactly that. A fix hint naming a target that does not
	// exist is worse than no hint — it was the previous state of this string.
	if !strings.Contains(makefile, "\nui:") {
		t.Error("no `ui:` target in the Makefile, but the not-built envelope tells operators to run `make ui`")
	}
}

// TestNotBuiltFixNamesARealTarget closes the loop between the two: the
// remediation the 503 offers must be a command the Makefile actually defines.
func TestNotBuiltFixNamesARealTarget(t *testing.T) {
	root, err := testsupport.ModuleRoot(".")
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	// writeBundleNotBuilt's fix string names `make ui`; assert the target is
	// real rather than trusting the two to stay in agreement.
	if !strings.Contains(string(data), "\nui:") {
		t.Fatal("the not-built fix hint names `make ui`, which the Makefile does not define")
	}
}
