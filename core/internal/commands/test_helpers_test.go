package commands

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlagsAfterTest snapshots the current value of every flag in fs
// and registers a t.Cleanup that restores each one. This package
// registers command flags as package-level vars bound via
// StringVar/BoolVar/etc at init() time (e.g. moveToFlag, moveOutFlag),
// so a flag set by one test via its package var stays set for every
// later test in the same process unless something resets it — pflag
// only overwrites a flag's value when that flag is actually passed on
// the command line; it does not reset unset flags back to their
// default before each Execute().
//
// Before this helper, tests worked around the leak with hand-written
// per-flag defer blocks (move_feature_test.go, disambig_signal_test.go)
// or a bespoke resetXxxFlags() function per command
// (migrate_domain_operations_test.go). Both forms silently go stale the
// moment a new flag is added and nobody remembers to list it. This
// helper walks the FlagSet instead of a fixed list of variable names,
// so newly-added flags are covered automatically.
//
// Pass the real, package-level singleton command's FlagSet — e.g.
// resetFlagsAfterTest(t, moveFeatureCmd.Flags()) or
// resetFlagsAfterTest(t, rootCmd.PersistentFlags()) — not a throwaway
// *cobra.Command built for context injection (testCommandWithContext),
// which has no flags registered and so nothing to snapshot. The flag
// values live on the package-level vars regardless of which
// *cobra.Command object a test hands to the runXxx function under test.
func resetFlagsAfterTest(t *testing.T, fs *pflag.FlagSet) {
	t.Helper()
	type snapshot struct {
		flag  *pflag.Flag
		value string
	}
	var snaps []snapshot
	fs.VisitAll(func(f *pflag.Flag) {
		snaps = append(snaps, snapshot{flag: f, value: f.Value.String()})
	})
	t.Cleanup(func() {
		for _, s := range snaps {
			_ = s.flag.Value.Set(s.value)
		}
	})
}

// captureStdout temporarily redirects the real process os.Stdout, runs
// fn, and returns whatever was written. Only for tests that need to
// prove a function has NO stdout side effects at all — most tests that
// exercise a command handler should prefer cmd.SetOut(&buf) instead,
// since handlers write through cmd.OutOrStdout() rather than the
// process-global stdout. This helper stays for callers like a pure
// helper function with no *cobra.Command to hand a buffer to.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	w.Close()
	os.Stdout = prev
	return <-done
}

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
