package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/internal/config"
	"github.com/spf13/cobra"
)

// testContext returns a *config.Context whose active root is the
// current working directory. Tests typically chdir into a temp dir via
// setupTestDir(t) before calling testContext(t).
func testContext(t *testing.T) *config.Context {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cwd, _ = filepath.EvalSymlinks(cwd)
	return config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{
			Name: filepath.Base(cwd),
			Path: cwd,
			Kind: config.RootKindStandalone,
		},
		Source: config.SourceCwdWalkUp,
	}, nil)
}

// testCommandWithContext returns a cobra.Command whose context.Context
// carries the given parlay *config.Context. Used by handler tests that
// invoke runXxx directly (bypassing Cobra's PersistentPreRunE).
func testCommandWithContext(t *testing.T, cfg *config.Context) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "fake"}
	cmd.SetContext(config.WithCtx(context.Background(), cfg))
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	return cmd
}
