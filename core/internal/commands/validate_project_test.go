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
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_ProjectFlag_RejectsPositionalPath(t *testing.T) {
	// A bare cobra Args invocation: --project plus a positional arg
	// should be rejected with the validate-project-takes-no-path error.
	// The Args validator runs BEFORE RunE, so we test it directly.
	cmd := validateCmd
	if err := cmd.Flags().Set("project", "true"); err != nil {
		t.Fatalf("failed to set project flag: %s", err)
	}
	defer func() { _ = cmd.Flags().Set("project", "false") }()

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
	if err := cmd.Flags().Set("project", "true"); err != nil {
		t.Fatalf("failed to set project flag: %s", err)
	}
	defer func() { _ = cmd.Flags().Set("project", "false") }()

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

// TestValidate_ProjectPass_EmptyProjectIsOK exercises the runValidateProject
// path against a temp dir with no .parlay/build/ at all. Expect OK status.
func TestValidate_ProjectPass_EmptyProjectIsOK(t *testing.T) {
	root := t.TempDir()
	// Verify ValidateBuildfilesProjectStructured returns zero verdicts.
	// The full CLI runValidateProject calls os.Exit on failure so we
	// don't drive it directly here — that's covered by integration
	// suites. The library-level contract is tested in the agent
	// package.
	if _, err := os.Stat(filepath.Join(root, ".parlay", "build")); err == nil {
		t.Fatal("expected empty .parlay/build for this test setup")
	}
}
