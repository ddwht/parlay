package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/deployer"
	"github.com/ddwht/parlay/core/internal/feedback"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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

// ambiguityAsSignalFlag is the value of --ambiguity-as-signal. When
// true, PersistentPreRunE emits a structured JSON envelope to stderr
// and exits with AmbiguityExitCode (11) on ambiguity instead of
// prompting interactively or returning a generic error. Used by skill
// wrappers around the CLI.
var ambiguityAsSignalFlag bool

var rootCmd = &cobra.Command{
	Use:               "parlay",
	Short:             "Intent-first toolkit for design-to-specification workflows",
	Long:              "Parlay takes user intents and dialogues and parlays them into prototypes, surfaces, and engineering specifications.",
	PersistentPreRunE: persistentPreRun,
	SilenceUsage:      true,
	// main owns error output. Without this Cobra prints "Error: <msg>" from
	// Execute() and main then prints the bare message again, so every CLI
	// failure appeared twice and read like two distinct problems.
	//
	// It also closes a leak main's own comment claims is already closed: an
	// ExitCodeError is meant to exit silently because the command has already
	// written its user-facing output, but Cobra printed "Error: exit code N"
	// before main ever saw it.
	SilenceErrors: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "parlay %s (%s)\n", appVersion, appCommit)
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
	started := time.Now()
	err := rootCmd.Execute()

	// Bracketing the whole invocation here rather than in a
	// PersistentPostRun: a command that fails in PreRun never reaches
	// PostRun, and a run that could not resolve its root is exactly the
	// kind of failure this log exists to capture.
	if feedback.IsEnabled() {
		feedback.Record(feedback.KindInvocation, invokedCommandPath(), map[string]any{
			"args":        os.Args[1:],
			"exit":        invocationExitCode(err),
			"duration_ms": time.Since(started).Milliseconds(),
			"error":       errorText(err),
		})
		feedback.Stop()
	}
	return err
}

// startFeedback opens the log when the project or the environment asks for
// it. Never fails a command: a project that cannot write its log still runs.
func startFeedback(rootPath string) {
	cfg, err := loadProjectConfigForFeedback(rootPath)
	configValue := false
	if err == nil && cfg != nil {
		configValue = cfg.Feedback
	}
	feedback.Start(rootPath, feedback.Enabled(configValue, os.Getenv), os.Getenv(feedback.RunEnvVar))
}

func loadProjectConfigForFeedback(rootPath string) (*config.ProjectConfig, error) {
	data, err := os.ReadFile(filepath.Join(rootPath, config.ParlayDir, config.ConfigFile))
	if err != nil {
		return nil, err
	}
	var cfg config.ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// invokedCommandPath is the command actually run ("validate", "internal
// diff"), recovered from the arguments rather than from cobra, which has
// already returned by the time Execute records.
func invokedCommandPath() string {
	var parts []string
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "-") {
			break
		}
		parts = append(parts, a)
		if len(parts) == 2 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func invocationExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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

	// `--root <path>` is resolved before the cwd walk-up, not after. The
	// point of naming a project explicitly is that you are not standing in
	// one — resolving cwd first meant the walk-up failed with "no parlay
	// root found" and returned before the flag was ever read, so the path
	// form worked only from inside the very project it was naming.
	if pathRes, resolved, perr := resolveRootFlagAsPath(rootFlag); perr != nil {
		return perr
	} else if resolved {
		return finishResolution(cmd, pathRes, nil)
	} else if unambiguouslyPath(rootFlag) {
		// A value with a separator, or a ~ or ./ prefix, can only have been
		// meant as a path, so failing to resolve it is the error worth
		// reporting. Falling through would run the cwd walk-up and fail
		// with "no parlay root found below cwd" — an accurate sentence
		// about a question nobody asked, which never mentions the flag that
		// was actually wrong.
		return fmt.Errorf("--root %s: not a parlay project directory — no %s/%s there",
			rootFlag, config.ParlayDir, config.ConfigFile)
	}

	res, err := config.ResolveActiveRoot(cwd, envMap())
	if err != nil {
		// On ErrNoRootFound, look for candidates below cwd. If any
		// exist, either emit the structured signal (skill mode) or
		// surface them in the error message (interactive mode).
		if errors.Is(err, config.ErrNoRootFound) {
			candidates := config.DiscoverRootsBelow(cwd, 4)
			if len(candidates) > 0 {
				if ambiguityAsSignalFlag {
					_ = emitAmbiguitySignal(cmd.ErrOrStderr(), AmbiguitySignal{
						Trigger:    TriggerAmbiguousActiveRoot,
						Candidates: candidates,
						Hint:       "re-invoke with --root <name> or set PARLAY_ROOT=<abs-path>",
					})
					return NewExitCodeError(AmbiguityExitCode)
				}
				names := make([]string, 0, len(candidates))
				for _, c := range candidates {
					names = append(names, c.Name)
				}
				return fmt.Errorf("%w; candidate roots discovered below cwd: %v (use --root <name>, set PARLAY_ROOT, or cd into one)",
					err, names)
			}
		}
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

	// --root flag override. Two accepted forms: a registered child-root
	// name, or a path to a parlay project.
	//
	// The path form exists because every skill and agent brief in the tree
	// says `--root <root>`, and an agent reasonably writes a path there.
	// Requiring a registered name made that unusable on a standalone
	// project — the index is empty, so every path lost a lookup against
	// nothing and the caller retried variations of a path that could never
	// have worked. A path that resolves to a real parlay root is a clear,
	// checkable intent, so honor it.
	if rootFlag != "" {
		switch {
		case idx == nil:
			return fmt.Errorf("--root %s: not a parlay project directory (no %s/%s), and there is no roots index at %s to look up a child-root name in",
				rootFlag, config.ParlayDir, config.ConfigFile, res.ActiveRoot.Path)

		default:
			child, ok := idx.Lookup(rootFlag)
			if !ok {
				// Path-shaped and not a parlay root: the caller meant a
				// directory, so say what was wrong with that directory
				// rather than listing child-root names they never wanted.
				if looksLikePath(rootFlag) {
					return fmt.Errorf("--root %s: not a parlay project directory — no %s/%s there. "+
						"Pass a path to a parlay project, or one of the registered child-root names: %v",
						rootFlag, config.ParlayDir, config.ConfigFile, idx.Names())
				}
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
	}

	return finishResolution(cmd, res, idx)

}

// emitStudioVersionWarningOnceFromCtx is a thin wrapper around
// config.emitStudioVersionWarningOnce that pulls the StudioDetection
// off the *Context. Defined here in package commands so tests of the
// detection routine itself can call config.EmitStudioVersionWarningOnce
// directly without dragging in cobra.
func emitStudioVersionWarningOnceFromCtx(c *config.Context) {
	if c == nil {
		return
	}
	config.EmitStudioVersionWarningOnce(c.StudioDetection())
}

// mustContext extracts the resolved *config.Context from the command's
// context.Context. Returns a clear error when none is set — usually
// because the command's PreRunE was skipped (e.g. via the
// parlay-skip-resolution annotation) but the handler then forgot it
// can't rely on a Context.
//
// When cmd.Context() panics (because cmd is fresh and was never given a
// context), mustContext recovers and returns an error — a defensive
// shape so unit tests that drive runXxx directly without going through
// Cobra's full lifecycle get a clear error message instead of a panic.
func mustContext(cmd *cobra.Command) (cfg *config.Context, err error) {
	defer func() {
		if r := recover(); r != nil {
			cfg = nil
			err = fmt.Errorf("no parlay project found (cmd.Context() unavailable; PersistentPreRunE may not have run)")
		}
	}()
	cfg = config.FromCtx(cmd.Context())
	if cfg == nil {
		return nil, fmt.Errorf("no parlay project found (run parlay init first, or check that PersistentPreRunE was not skipped)")
	}
	return cfg, nil
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
	rootCmd.PersistentFlags().BoolVar(&ambiguityAsSignalFlag, "ambiguity-as-signal", false, "Emit a structured JSON envelope on stderr and exit non-zero on ambiguity (used by skill wrappers)")

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
	rootCmd.AddCommand(createDomainModelCmdImpl)
	rootCmd.AddCommand(loadDomainModelCmdImpl)
	// parlay-feature: studio-support/domain-model-yaml-migration
	// parlay-component: migrate-domain-model-command
	rootCmd.AddCommand(migrateDomainModelCmd)
	rootCmd.AddCommand(createArtifactsCmdImpl)
	rootCmd.AddCommand(newInitiativeCmd)
	rootCmd.AddCommand(simplifyCmd)
	rootCmd.AddCommand(moveFeatureCmd)
	rootCmd.AddCommand(repairCmd)
	rootCmd.AddCommand(loopCmd)

	// Utility commands for agent consumption
	rootCmd.AddCommand(validateCmd)
	// parlay-feature: design-loop/vocabulary-validation
	// parlay-component: cross-cutting/core-cli-wiring
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)

	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: cli-and-deployer-registration
	rootCmd.AddCommand(migrateConfigCmd)
	rootCmd.AddCommand(migrateSpecCmd)
	rootCmd.AddCommand(migrateCapabilitiesCmd)
	rootCmd.AddCommand(migrateDomainOperationsCmd)
	rootCmd.AddCommand(reviewCoverageCmd)

	// Multi-root commands.
	rootCmd.AddCommand(addRootCmd)
	rootCmd.AddCommand(promoteRootCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(domainEditCmd)

	// Agent-facing probes live under `parlay internal`; the AI-agent stubs
	// stay runnable but out of the help listing. See internal_group.go.
	registerInternalCommands()
	hideAgentOnlyStubs()
}

// looksLikePath reports whether s is shaped like a filesystem path rather
// than a root name. Used only to pick a clearer error message.
func looksLikePath(s string) bool {
	if s == "." || s == ".." {
		return true
	}
	if strings.ContainsAny(s, "/\\") {
		return true
	}
	if strings.HasPrefix(s, "~") {
		return true
	}
	if info, err := os.Stat(s); err == nil && info.IsDir() {
		return true
	}
	return false
}

// resolveRootFlagAsPath handles the `--root <path>` form. It reports
// resolved=true only when rootFlag is path-shaped AND the directory is a
// real parlay project (has .parlay/config.yaml).
//
// The two conditions are deliberately both required. Path-shaped alone
// would swallow a mistyped child-root name that happens to match a
// directory; a config.yaml check alone would try to stat every name. When
// either fails, the caller falls through to the child-root-name lookup, so
// the name form keeps working unchanged.
func resolveRootFlagAsPath(rootFlag string) (*config.ResolutionResult, bool, error) {
	if !looksLikePath(rootFlag) {
		return nil, false, nil
	}
	abs, err := filepath.Abs(expandHome(rootFlag))
	if err != nil {
		return nil, false, nil
	}
	if _, err := os.Stat(filepath.Join(abs, config.ParlayDir, config.ConfigFile)); err != nil {
		return nil, false, nil
	}
	res, err := config.ResolveActiveRoot(abs, nil)
	if err != nil {
		return nil, false, fmt.Errorf("--root %s: %w", rootFlag, err)
	}
	res.Source = config.SourceRootFlag
	res.AnnouncementRequired = true
	return res, true, nil
}

// expandHome resolves a leading ~ so `--root ~/projects/app` behaves the
// way it does in every other tool. Without this the path form works
// everywhere except the one place users most often type by hand.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// finishResolution runs the checks that apply however the active root was
// chosen — cwd walk-up, PARLAY_ROOT, `--root <name>`, or `--root <path>` —
// and installs the resulting context. Shared so the path form cannot
// accidentally skip the orphan and forbidden-directory checks the other
// paths perform.
func finishResolution(cmd *cobra.Command, res *config.ResolutionResult, idx *config.RootsIndex) error {
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

	// Feedback mode, opened as early as the root is known and closed by
	// Execute. Reading the config here rather than in Start keeps the
	// feedback package free of any dependency on config, which is what
	// lets the agent-facing record command reuse it unchanged.
	startFeedback(res.ActiveRoot.Path)

	// parlay-extends: studio-support/studio-cli-hooks/runtime-studio-detection
	// Use the studio-detection-aware constructor so every command
	// handler sees the per-process record of parlay-studio's
	// availability without re-checking PATH or env. Detection is
	// read-only — we never invoke Studio just to confirm.
	pctx := config.NewContextWithStudioDetection(res, idx)
	// One-line stderr warning at first successful detection when the
	// reported Studio version is outside Core's expected range. The
	// helper is sync.Once-guarded — every subsequent invocation in the
	// same process is a no-op, so concurrent or repeated PreRun calls
	// stay quiet.
	emitStudioVersionWarningOnceFromCtx(pctx)
	cmd.SetContext(config.WithCtx(cmd.Context(), pctx))
	return nil
}

// unambiguouslyPath reports whether s could only have been meant as a
// filesystem path. Narrower than looksLikePath, which also treats a bare
// name that happens to match a directory as path-shaped — that case stays
// ambiguous with a child-root name and must fall through to the name
// lookup rather than hard-erroring.
func unambiguouslyPath(s string) bool {
	return strings.ContainsAny(s, "/\\") || strings.HasPrefix(s, "~") || s == "." || s == ".."
}
