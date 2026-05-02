package commands

import (
	"fmt"
	"os"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/deployer"
	"github.com/spf13/cobra"
)

// annotationSkipResolution is the cobra.Command.Annotations key used to
// opt out of multi-root resolution in PersistentPreRunE. Commands that
// either bootstrap the first root (`init`) or do not depend on root
// state (`version`) should set this to "true".
const annotationSkipResolution = "parlay-skip-resolution"

// annotationAllowOrphan tells PersistentPreRunE to skip the parent-pointer
// validation step for this command. Used by `promote-root`, which exists
// specifically to recover orphaned children — its PreRunE must not
// itself fail with ErrParentRootNotFound.
const annotationAllowOrphan = "parlay-allow-orphan"

// rootFlag holds the value of --root, captured by Cobra into a package
// variable so PersistentPreRunE can read it before any subcommand runs.
var rootFlag string

// verboseFlag is the value of --verbose. When true, PersistentPreRunE
// prints a one-line resolution header to stderr.
var verboseFlag bool

var rootCmd = &cobra.Command{
	Use:               "parlay",
	Short:             "Intent-first toolkit for design-to-specification workflows",
	Long:              "Parlay takes user intents and dialogues and parlays them into prototypes, surfaces, and engineering specifications.",
	PersistentPreRunE: persistentPreRun,
	SilenceUsage:      true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("parlay %s (%s)\n", appVersion, appCommit)
	},
	Annotations: map[string]string{annotationSkipResolution: "true"},
}

var (
	appVersion = "dev"
	appCommit  = "none"
)

// SetVersion is called from main to inject build-time values.
func SetVersion(version, commit string) {
	appVersion = version
	appCommit = commit
}

func Execute() error {
	return rootCmd.Execute()
}

// persistentPreRun runs before every subcommand's RunE. It resolves the
// active root via cwd walk-up (or PARLAY_ROOT), validates the parent
// pointer for child roots, runs the forbidden-directory check, and
// stores the resulting *config.Context on cmd.Context() so handlers can
// opt into multi-root-aware path lookup via config.FromCtx.
//
// Commands annotated with annotationSkipResolution skip the entire
// process — used by `init` (which creates the first .parlay/) and
// `version` (which doesn't depend on any root state).
func persistentPreRun(cmd *cobra.Command, args []string) error {
	if cmd.Annotations[annotationSkipResolution] == "true" {
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}

	res, err := config.ResolveActiveRoot(cwd, envMap())
	if err != nil {
		return err
	}

	// Load the parent's roots index when we're at a parent root.
	var idx *config.RootsIndex
	switch res.ActiveRoot.Kind {
	case config.RootKindStandalone, config.RootKindParent:
		idx, err = config.LoadRootsIndex(res.ActiveRoot.Path)
		if err != nil {
			return fmt.Errorf("load roots index: %w", err)
		}
		if idx != nil && len(idx.Children) > 0 {
			// We have children → reclassify as parent.
			res.ActiveRoot.Kind = config.RootKindParent
		}
	case config.RootKindChild:
		// Children themselves don't load a roots index; they may inherit
		// one via their parent if needed.
	}

	// --root flag override (validated against the index).
	if rootFlag != "" {
		if idx == nil {
			return fmt.Errorf("--root %s: no roots index at %s; --root only applies at a parent root",
				rootFlag, res.ActiveRoot.Path)
		}
		child, ok := idx.Lookup(rootFlag)
		if !ok {
			return fmt.Errorf("--root %s: unknown root; known: %v", rootFlag, idx.Names())
		}
		// Re-resolve the active root to point at the chosen child.
		childRes, err := config.ResolveActiveRoot(child.Path, nil)
		if err != nil {
			return fmt.Errorf("--root %s: %w", rootFlag, err)
		}
		childRes.Source = config.SourceRootFlag
		childRes.AnnouncementRequired = true
		res = childRes
		idx = nil
	}

	// Validate the parent pointer (detects orphaned children).
	// Commands that exist to recover from this state (promote-root) opt
	// out via annotationAllowOrphan.
	if cmd.Annotations[annotationAllowOrphan] != "true" {
		if err := config.ValidateParentPointer(res.ActiveRoot); err != nil {
			return err
		}
	}

	// Forbidden-directory check for child roots — surface paths come
	// from the deployer registry.
	if v := config.ValidateChildRootForbiddenDirectories(res.ActiveRoot, deployer.AllAgentSurfacePaths()); v != nil {
		return v
	}

	if verboseFlag {
		fmt.Fprintf(cmd.ErrOrStderr(), "resolved root: %s (source: %s)\n",
			res.ActiveRoot.Path, res.Source)
	}

	pctx := config.NewContext(res, idx)
	cmd.SetContext(config.WithCtx(cmd.Context(), pctx))
	return nil
}

// envMap snapshots os.Environ() into a map[string]string so the resolver
// receives only the keys it cares about. The resolver currently only
// reads PARLAY_ROOT but the map shape is forward-compatible.
func envMap() map[string]string {
	out := map[string]string{}
	if v, ok := os.LookupEnv("PARLAY_ROOT"); ok {
		out["PARLAY_ROOT"] = v
	}
	return out
}

func init() {
	rootCmd.PersistentFlags().StringVar(&rootFlag, "root", "", "Operate against the named child root (overrides cwd walk-up)")
	rootCmd.PersistentFlags().BoolVar(&verboseFlag, "verbose", false, "Print resolution and resource-load details to stderr")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addFeatureCmd)
	rootCmd.AddCommand(createDialogsCmd)
	rootCmd.AddCommand(viewPageCmdImpl)
	rootCmd.AddCommand(lockPageCmdImpl)
	rootCmd.AddCommand(syncCmdImpl)
	rootCmd.AddCommand(registerAdapterCmd)
	rootCmd.AddCommand(buildFeatureCmdImpl)
	rootCmd.AddCommand(generateCodeCmdImpl)
	rootCmd.AddCommand(generateEnggspecCmdImpl)
	rootCmd.AddCommand(extractDomainModelCmdImpl)
	rootCmd.AddCommand(loadDomainModelCmdImpl)
	rootCmd.AddCommand(createArtifactsCmdImpl)
	rootCmd.AddCommand(newInitiativeCmd)
	rootCmd.AddCommand(simplifyCmd)
	rootCmd.AddCommand(moveFeatureCmd)
	rootCmd.AddCommand(repairCmd)
	rootCmd.AddCommand(loopCmd)

	// Utility commands for agent consumption
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(parseCmd)
	rootCmd.AddCommand(checkCoverageCmd)
	rootCmd.AddCommand(collectQuestionsCmd)
	rootCmd.AddCommand(checkDriftCmd)
	rootCmd.AddCommand(checkReadinessCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(scanGeneratedCmd)
	rootCmd.AddCommand(verifyGeneratedCmd)
	rootCmd.AddCommand(saveBuildStateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)

	// Multi-root commands.
	rootCmd.AddCommand(addRootCmd)
	rootCmd.AddCommand(promoteRootCmd)
	rootCmd.AddCommand(statusCmd)
}
