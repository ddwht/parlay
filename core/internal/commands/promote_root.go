package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

var promoteRootCmd = &cobra.Command{
	Use:   "promote-root",
	Short: "Promote a child root to standalone (recovery for orphaned children)",
	Long: `Strip the parent: pointer from the active root's .parlay/config.yaml
so the root no longer participates in a parent-child topology. Used to
recover from a deleted or moved parent root: cd into the orphaned child
and run promote-root.

Refuses to run when the parent path still resolves — promote-root is
explicitly an orphan-recovery escape hatch, not a way to silently sever
a working parent-child link.`,
	Args:        cobra.NoArgs,
	RunE:        runPromoteRoot,
	Annotations: map[string]string{annotationAllowOrphan: "true"},
}

func runPromoteRoot(cmd *cobra.Command, args []string) error {
	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		return fmt.Errorf("no active parlay root")
	}
	if pctx.Root.Kind != config.RootKindChild {
		return fmt.Errorf("active root at %s is not a child (no parent: pointer in config); promote-root is only for orphaned children",
			pctx.Root.Path)
	}
	// Refuse if the parent still resolves — promote-root is for orphans only.
	if err := config.ValidateParentPointer(pctx.Root); err == nil {
		return fmt.Errorf("parent root at %s still resolves; promote-root is only for orphaned children. Delete the parent first if you really want to sever the link",
			pctx.Root.ParentPath)
	}

	if err := config.RemoveParentPointer(pctx.Root.Path); err != nil {
		return fmt.Errorf("remove parent pointer: %w", err)
	}
	fmt.Printf("Promoted %s to standalone root.\n", pctx.Root.Path)
	return nil
}
