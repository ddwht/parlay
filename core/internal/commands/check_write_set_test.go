// parlay-feature: parlay-tool/multi-adapter
// parlay-component: codegen-read-set-and-layer-pipeline
// parlay-artifact: test

// The plan: allowlist was prose-only: generate-code.skill.md told the agent to
// write only declared paths, and nothing ever checked. Interception is not
// available — parlay does not perform codegen's writes — so this is an audit
// against the record codegen leaves in .code-hashes.yaml.
//
// These tests use controlled fixtures rather than a real project, because the
// real one cannot isolate a single violation: a cross-cutting path is typically
// declared several times over (once in plan.creates, once in a cross-cutting
// target-files, and again by another feature), so removing one declaration proves
// nothing about the check.

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// writeSetFixture builds a temp root with one buildfile and one code-hashes
// sidecar, and returns the context to check against.
func writeSetFixture(t *testing.T, buildfile string, hashes string) *config.Context {
	t.Helper()
	root := t.TempDir()
	bfDir := filepath.Join(root, config.ParlayDir, config.BuildDir, "feat")
	if err := os.MkdirAll(bfDir, 0o755); err != nil {
		t.Fatalf("mkdir buildfile dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bfDir, "buildfile.yaml"), []byte(buildfile), 0o644); err != nil {
		t.Fatalf("write buildfile: %v", err)
	}
	projDir := filepath.Join(root, config.ParlayDir, config.BuildDir, "_project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir _project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".code-hashes.yaml"), []byte(hashes), 0o644); err != nil {
		t.Fatalf("write code-hashes: %v", err)
	}
	return &config.Context{Root: config.Root{Name: filepath.Base(root), Path: root}}
}

const wsBuildfile = `feature: feat
adapter: angular-clarity
plan:
  creates:
    - path: src/app/declared.ts
      sources: [component/thing]
`

func TestWriteSet_DeclaredFileIsClean(t *testing.T) {
	cfg := writeSetFixture(t, wsBuildfile, `files:
    src/app/declared.ts:
        component: thing
        hash: aaa
`)
	declared, features, err := declaredPlanPaths(cfg, nil)
	if err != nil {
		t.Fatalf("declaredPlanPaths: %v", err)
	}
	if len(features) != 1 {
		t.Fatalf("want 1 feature contributing declarations, got %d", len(features))
	}
	if !declared["src/app/declared.ts"] {
		t.Fatalf("declared set missing the planned path: %v", declared)
	}
}

// The finding this whole command exists for: a component-attributed
// implementation file that no plan declares.
func TestWriteSet_UndeclaredComponentFileIsReported(t *testing.T) {
	cfg := writeSetFixture(t, wsBuildfile, `files:
    src/app/declared.ts:
        component: thing
        hash: aaa
    src/app/sneaked-in.ts:
        component: thing
        hash: bbb
`)
	declared, _, err := declaredPlanPaths(cfg, nil)
	if err != nil {
		t.Fatalf("declaredPlanPaths: %v", err)
	}
	if declared["src/app/sneaked-in.ts"] {
		t.Fatal("fixture is wrong: the undeclared path appears in the declared set")
	}
	if reason := exemptFromPlan(cfg, "src/app/sneaked-in.ts", "thing"); reason != "" {
		t.Fatalf("an ordinary component file must not be exempt, got %q", reason)
	}
}

// Exemption 1: test files come from the test-generation step, not the plan.
// Identified by the documented marker rather than a filename suffix, which would
// be a guess about one framework's convention.
func TestWriteSet_TestMarkerFileIsExempt(t *testing.T) {
	cfg := writeSetFixture(t, wsBuildfile, "files: {}\n")
	specPath := "src/app/thing.component.spec.ts"
	full := filepath.Join(cfg.Root.Path, specPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("// parlay-artifact: test\ndescribe('x', () => {})\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	reason := exemptFromPlan(cfg, specPath, "thing")
	if reason == "" {
		t.Fatal("a file carrying `parlay-artifact: test` must be exempt from the plan allowlist")
	}
	if !strings.Contains(reason, "test") {
		t.Errorf("exemption reason should name the test-file rule, got %q", reason)
	}
}

// A file with the same name but no marker is NOT exempt — the marker is the rule,
// not the suffix.
func TestWriteSet_SpecSuffixWithoutMarkerIsNotExempt(t *testing.T) {
	cfg := writeSetFixture(t, wsBuildfile, "files: {}\n")
	specPath := "src/app/unmarked.component.spec.ts"
	full := filepath.Join(cfg.Root.Path, specPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("describe('x', () => {})\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if reason := exemptFromPlan(cfg, specPath, "thing"); reason != "" {
		t.Errorf("exemption must key on the marker, not the .spec.ts suffix; got %q", reason)
	}
}

// Exemption 2: project scaffold belongs to no component, so no feature's plan can
// declare it.
func TestWriteSet_UnattributedScaffoldIsExempt(t *testing.T) {
	cfg := writeSetFixture(t, wsBuildfile, "files: {}\n")
	reason := exemptFromPlan(cfg, "src/app/app.config.ts", "")
	if reason == "" {
		t.Fatal("a file with no component attribution must be exempt")
	}
	if !strings.Contains(reason, "scaffold") {
		t.Errorf("exemption reason should name the scaffold rule, got %q", reason)
	}
}

// A malformed buildfile must refuse rather than contribute zero declared paths.
// Skipping quietly is what the first version did, and breaking one buildfile's
// YAML turned eleven correctly-declared files into violations — one real problem
// surfacing as many false ones is how an audit gets switched off.
func TestWriteSet_MalformedBuildfileRefusesInsteadOfFlooding(t *testing.T) {
	cfg := writeSetFixture(t, "feature: feat\nplan:\n  creates:\n    - path: [unclosed\n", "files: {}\n")
	_, _, err := declaredPlanPaths(cfg, nil)
	if err == nil {
		t.Fatal("a buildfile that will not parse must be an error, not an empty allowlist")
	}
	if !strings.Contains(err.Error(), "will not load") {
		t.Errorf("error should explain why judging is impossible, got %v", err)
	}
}

// Cross-cutting emissions are declared, just in a different section. Treating
// them as undeclared would report the tool's own documented mechanism.
func TestWriteSet_CrossCuttingTargetsCountAsDeclared(t *testing.T) {
	cfg := writeSetFixture(t, `feature: feat
cross-cutting:
  - id: shared-thing
    target-files:
      - src/app/shared/existing.ts
    target-creates:
      - src/app/shared/new.ts
`, "files: {}\n")
	declared, _, err := declaredPlanPaths(cfg, nil)
	if err != nil {
		t.Fatalf("declaredPlanPaths: %v", err)
	}
	for _, p := range []string{"src/app/shared/existing.ts", "src/app/shared/new.ts"} {
		if !declared[p] {
			t.Errorf("cross-cutting path %q is not in the declared set", p)
		}
	}
}
