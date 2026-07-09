// parlay-feature: parlay-tool/cross-cutting-target-paths
// parlay-component: validate (extends project-pass-validation-and-cli-flag)
// parlay-artifact: test
//
// Tests for the `parlay validate --project` flag introduced by the
// cross-cutting-target-paths feature. The flag composes with the existing
// validate command surface; these tests cover the Args reconciliation and
// the project-pass JSON envelope.

package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestValidate_ProjectFlag_RejectsPositionalPath(t *testing.T) {
	// A bare cobra Args invocation: --project plus a positional arg
	// should be rejected with the validate-project-takes-no-path error.
	// The Args validator runs BEFORE RunE, so we test it directly.
	cmd := validateCmd
	resetFlagsAfterTest(t, validateCmd.Flags())
	if err := cmd.Flags().Set("project", "true"); err != nil {
		t.Fatalf("failed to set project flag: %s", err)
	}

	err := validateArgs(cmd, []string{"some/path.yaml"})
	if err == nil {
		t.Fatal("expected validateArgs to reject positional path with --project")
	}
	if msg := err.Error(); !contains(msg, "validate-project-takes-no-path") {
		t.Errorf("expected error message to contain validate-project-takes-no-path code, got: %s", msg)
	}
}

func TestValidate_ProjectFlag_AcceptsZeroArgs(t *testing.T) {
	cmd := validateCmd
	resetFlagsAfterTest(t, validateCmd.Flags())
	if err := cmd.Flags().Set("project", "true"); err != nil {
		t.Fatalf("failed to set project flag: %s", err)
	}

	err := validateArgs(cmd, []string{})
	if err != nil {
		t.Fatalf("expected validateArgs to accept zero args with --project, got: %s", err)
	}
}

func TestValidate_NoProjectFlag_RequiresOnePositional(t *testing.T) {
	cmd := validateCmd
	// Default (project unset) should require exactly one arg.
	err := validateArgs(cmd, []string{})
	if err == nil {
		t.Fatal("expected validateArgs to reject zero args without --project")
	}
}

// contains is a tiny helper to keep this file's import surface minimal.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestValidate_ProjectPass_EmptyProjectIsOK drives runValidateProject
// directly, in-process, against a temp dir with no .parlay/build/ at
// all. This used to be untestable without a subprocess (the old
// os.Exit(1) call would have killed the test binary on any failure);
// now that failures return an *ExitCodeError instead, the full path
// runs in-process.
func TestValidate_ProjectPass_EmptyProjectIsOK(t *testing.T) {
	root := t.TempDir()
	if _, err := os.Stat(filepath.Join(root, ".parlay", "build")); err == nil {
		t.Fatal("expected empty .parlay/build for this test setup")
	}

	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Path: root, Kind: config.RootKindStandalone},
	}, nil)
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runValidateProject(cmd); err != nil {
		t.Fatalf("expected OK for a project with no .parlay/build/, got: %v", err)
	}
	if !strings.Contains(buf.String(), "OK") {
		t.Errorf("expected OK output, got: %s", buf.String())
	}
}

// TestValidate_ProjectPass_ResolvesRootViaContext confirms
// runValidateProject resolves its root from mustContext(cmd) — the
// standard active-root resolver — rather than falling back to
// PARLAY_ROOT or cwd. Regression test for the --root shadowing bug:
// this command used to register its own local --root flag with
// different semantics than the persistent one, and its root-resolution
// fell back to an ad hoc PARLAY_ROOT-or-cwd check that never consulted
// the resolved *config.Context at all. A buildfile placed under the
// context's root must be found even when cwd points somewhere else
// entirely.
func TestValidate_ProjectPass_ResolvesRootViaContext(t *testing.T) {
	contextRoot := t.TempDir()
	contextRoot, _ = filepath.EvalSymlinks(contextRoot)
	buildDir := filepath.Join(contextRoot, config.ParlayDir, config.BuildDir, "my-feature")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `feature: my-feature
adapter-set: presentation-only
targets:
  presentation:
    adapter: go-cli
components: {}
plan: {}
`
	if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	// cwd points somewhere unrelated — a PARLAY_ROOT-or-cwd fallback
	// would resolve here and never find the buildfile above.
	elsewhere := t.TempDir()
	prevWd, _ := os.Getwd()
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prevWd) })
	t.Setenv("PARLAY_ROOT", "")

	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Path: contextRoot, Kind: config.RootKindStandalone},
	}, nil)
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	// Root resolution is what this test targets, not deep buildfile
	// validation correctness (that's internal/agent's contract, tested
	// there) — so the assertion only needs to prove the buildfile was
	// discovered under the context root, whether or not it validates
	// clean. An *ExitCodeError from validation failures is an expected,
	// acceptable outcome here; only a resolution failure is not.
	err := runValidateProject(cmd)
	if err != nil {
		var exitErr *ExitCodeError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected either success or an *ExitCodeError from validation issues, got: %v", err)
		}
	}
	if strings.Contains(buf.String(), "no buildfiles") {
		t.Errorf("expected the buildfile under the context root to be discovered, got: %s", buf.String())
	}
	// The clean-validation path only prints a count ("OK (1 feature(s)
	// validated)"), not per-feature names — those only appear when a
	// feature has errors. Either shape proves discovery; only the
	// "no buildfiles" note (checked above) would prove resolution failed.
	if !strings.Contains(buf.String(), "1 feature(s) validated") && !strings.Contains(buf.String(), "my-feature") {
		t.Errorf("expected the discovered buildfile to be reflected in the output one way or another, got: %s", buf.String())
	}
}
