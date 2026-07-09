// parlay-feature: parlay-tool/multi-root
// parlay-component: add-child-root-result
// parlay-extends: parlay-tool/multi-root/add-root-refusal-without-parent-agent
// parlay-cross-cutting: parlay-add-root-command
// parlay-cross-cutting: auto-refresh-hook-on-add-and-remove-root
// parlay-cross-cutting: add-root-parent-agent-precondition

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var addRootCmd = &cobra.Command{
	Use:   "add-root <subdir>",
	Short: "Register a subfolder as a child parlay root",
	Long: `Create a new child parlay root inside the active root and register it
in the parent's roots index. After this command:

  - <subdir>/.parlay/config.yaml carries a 'parent:' pointer back to the
    repo-level root.
  - <subdir>/spec/{intents,handoff,pages}/ exist as empty designer trees.
  - The parent's .parlay/roots.yaml lists the new child by short name.
  - The deployer is re-run so the agent surface (CLAUDE.md or
    equivalent) reflects the new root immediately.

Refusal cases (printed to stderr, exit non-zero, no work performed):
  - Parent root is missing ai-agent (run 'parlay init' at parent first).
  - <subdir> already contains a .parlay/ directory.
  - The active root is itself a child (no nested children).
  - <subdir> is outside the active root's directory tree.
  - A child with the same short name or path is already registered.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddRoot,
}

// ErrParentMissingAIAgent is returned by parlay add-root when the parent
// root has no config.yaml or has a config.yaml without an ai-agent field.
// The precondition is enforced before any other validation so the user
// sees the structural problem first when multiple errors apply.
var ErrParentMissingAIAgent = errors.New("parent is missing ai-agent — run `parlay init` at the parent first")

// validateParentHasAIAgent enforces the add-root parent-agent precondition.
// Returns nil when the parent's .parlay/config.yaml exists and declares a
// non-empty ai-agent. Returns ErrParentMissingAIAgent (wrapped with the
// resolved parent path) otherwise. This is a behavior change from the
// previous flow that would happily create children against bare-parent
// projects.
func validateParentHasAIAgent(parentRoot string) error {
	cfgPath := filepath.Join(parentRoot, config.ParlayDir, config.ConfigFile)
	if !config.HasAIAgentField(cfgPath) {
		return fmt.Errorf("[ERR] %w\n  parent path: %s", ErrParentMissingAIAgent, parentRoot)
	}
	return nil
}

func runAddRoot(cmd *cobra.Command, args []string) error {
	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		return fmt.Errorf("no active parlay root; run from inside a parlay project (or run `parlay init`)")
	}

	// Precondition: parent must have ai-agent. Enforced FIRST so the
	// structural problem surfaces ahead of any subdir-collision or
	// nesting check.
	if err := validateParentHasAIAgent(pctx.Root.Path); err != nil {
		return err
	}

	if pctx.Root.Kind == config.RootKindChild {
		return fmt.Errorf("nested children are not supported: active root is a child at %s", pctx.Root.Path)
	}

	subdirArg := args[0]
	subdirAbs := subdirArg
	if !filepath.IsAbs(subdirAbs) {
		subdirAbs = filepath.Join(pctx.Root.Path, subdirArg)
	}
	subdirAbs = filepath.Clean(subdirAbs)

	// The subdir must live inside the active root.
	rel, err := filepath.Rel(pctx.Root.Path, subdirAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("subdir %q is not inside the active root %s", subdirArg, pctx.Root.Path)
	}

	// Refuse if subdir already contains a .parlay/.
	if _, err := os.Stat(filepath.Join(subdirAbs, config.ParlayDir)); err == nil {
		return fmt.Errorf("%s already contains a %s directory", subdirAbs, config.ParlayDir)
	}

	// Load existing index (or start an empty one).
	idx := pctx.Index
	if idx == nil {
		var err error
		idx, err = config.LoadRootsIndex(pctx.Root.Path)
		if err != nil {
			return fmt.Errorf("load roots index: %w", err)
		}
	}

	// Refusal: same path already registered.
	for _, c := range idx.Children {
		registeredAbs := filepath.Clean(filepath.Join(pctx.Root.Path, c.RelativePath))
		if registeredAbs == subdirAbs {
			return fmt.Errorf("subdir %s is already registered as child %q", subdirAbs, c.Name)
		}
	}

	// Compute the new child's short name from the leaf path component.
	name := parser.Slugify(filepath.Base(subdirAbs))
	if _, exists := idx.Lookup(name); exists {
		return fmt.Errorf("a child root named %q is already registered", name)
	}

	// Create the child's parlay and spec directories.
	if err := os.MkdirAll(filepath.Join(subdirAbs, config.ParlayDir), 0755); err != nil {
		return fmt.Errorf("create %s/.parlay/: %w", subdirAbs, err)
	}
	for _, dir := range []string{
		filepath.Join(subdirAbs, config.SpecDir, config.IntentsDir),
		filepath.Join(subdirAbs, config.SpecDir, config.HandoffDir),
		filepath.Join(subdirAbs, config.SpecDir, config.PagesDir),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	// Write the child's parent pointer.
	if err := config.WriteParentPointer(subdirAbs, pctx.Root.Path); err != nil {
		return fmt.Errorf("write parent pointer: %w", err)
	}

	// Append to the parent's roots index.
	child := config.Root{
		Name:         name,
		RelativePath: rel,
		Path:         subdirAbs,
		ParentPath:   pctx.Root.Path,
		Kind:         config.RootKindChild,
	}
	if _, err := config.AppendRootToIndex(idx, child); err != nil {
		return fmt.Errorf("append to roots index: %w", err)
	}

	// Auto-refresh the agent surface at the parent root so CLAUDE.md
	// (or the adapter equivalent) lists the new child immediately.
	refreshFailed := false
	if _, err := deployToRoot(pctx.Root.Path); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: agent-surface refresh failed: %v (re-run `parlay upgrade` to retry)\n", err)
		refreshFailed = true
	}

	fmt.Fprintf(cmd.OutOrStdout(), "[OK] Created child root at %s/.\n", rel)
	fmt.Fprintf(cmd.OutOrStdout(), "Registered in parent's roots index as %s.\n", name)
	if !refreshFailed {
		fmt.Fprintf(cmd.OutOrStdout(), "Agent surface refreshed (CLAUDE.md now lists %s).\n", name)
	}
	return nil
}
