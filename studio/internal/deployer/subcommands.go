// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/embedded-source-and-deployer-subcommands

// subcommands.go owns the public Init/Upgrade entry points. Studio's
// main.go dispatches via os.Args to these functions; cobra is NOT in
// Studio's go.mod and is not introduced here. The flag-parsing surface
// is intentionally tiny: --project <path>, --help.
//
// Per Q1.1 (strict), both subcommands REFUSE to operate when the project
// root does not contain a .parlay/ subdirectory, surfacing the stable
// code studio-deployer-parlay-not-initialized. There is NO automatic
// chain to `parlay init`.
package deployer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/parlay-tool/parlay/studio/internal/config"
	"github.com/parlay-tool/parlay/studio/internal/embedded"
)

// ErrParlayNotInitialized is the stable sentinel returned when Init or
// Upgrade is invoked against a directory whose .parlay/ subdirectory does
// not exist. The wrapped message names the missing marker so the operator
// can act on it.
var ErrParlayNotInitialized = errors.New("studio-deployer-parlay-not-initialized")

// Stdout/Stderr are package-level seams to make the help-text test
// deterministic without spinning up a subprocess. Production code keeps
// these as the real os.Stdout/os.Stderr.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// Init runs the deployer in "first-time bootstrap" framing. The on-disk
// behavior is identical to Upgrade — both produce the same deployed file
// set against the same embedded source — but the help text and the
// one-time banner differ.
func Init(ctx context.Context, args []string) error {
	return runSubcommand(ctx, args, "init")
}

// Upgrade runs the deployer in "idempotent re-deploy" framing. Same
// on-disk behavior as Init; different help text; no first-run banner.
func Upgrade(ctx context.Context, args []string) error {
	return runSubcommand(ctx, args, "upgrade")
}

// runSubcommand is the shared implementation behind Init and Upgrade. The
// subcommandName drives the help text and the optional first-run banner.
func runSubcommand(ctx context.Context, args []string, subcommandName string) error {
	if showHelp(args) {
		fmt.Fprint(stdout, helpText(subcommandName))
		return nil
	}
	root, err := resolveRoot(args)
	if err != nil {
		return err
	}
	if err := requireParlayMarker(root); err != nil {
		return err
	}
	agents, err := DetectAgentSurfaces(root)
	if err != nil {
		return err
	}
	d := &Deployer{
		ProjectRoot: root,
		Agents:      agents,
		SkillReader: embedded.ReadSkill,
		SkillLister: embedded.ListSkills,
	}
	if subcommandName == "init" {
		fmt.Fprintln(stdout, "parlay-studio init: deploying embedded source to detected agent surfaces")
	}
	res, err := d.Run(ctx)
	if err != nil {
		return err
	}
	if printErr := PrintSummary(stdout, res.Entries); printErr != nil {
		return printErr
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("parlay-studio %s: one or more files failed to deploy (see summary above)", subcommandName)
	}
	return nil
}

// resolveRoot wraps config.ResolveProjectRoot to translate Studio's
// existing resolver into the deployer's argument shape. Relative paths
// are resolved against cwd; STUDIO_PROJECT_ROOT is honored; absent both,
// the cwd walk-up looks for the nearest ancestor with a .parlay/.
func resolveRoot(args []string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve project root: getwd: %w", err)
	}
	envMap := osEnvMap()
	home, _ := os.UserHomeDir()
	root, _, err := config.ResolveProjectRoot(args, envMap, cwd, home)
	if err != nil {
		return "", err
	}
	return root, nil
}

// requireParlayMarker enforces the strict-mode rule from Q1.1: the
// resolved project root MUST directly contain a .parlay/ subdirectory.
// No automatic chain to `parlay init`.
//
// In practice config.ResolveProjectRoot already enforces this for the
// explicit-override branches (--project, STUDIO_PROJECT_ROOT) and for
// the walk-up branch. This function is a defense-in-depth check so the
// stable error code studio-deployer-parlay-not-initialized surfaces
// uniformly regardless of how the root was resolved.
func requireParlayMarker(root string) error {
	parlay := filepath.Join(root, ".parlay")
	info, err := os.Stat(parlay)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: %s does not contain a .parlay/ subdirectory. Run `parlay init` first; parlay-studio does NOT initialize parlay automatically",
			ErrParlayNotInitialized, root)
	}
	return nil
}

// showHelp returns true when --help (or -h, --help=true) appears in args.
func showHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "--help=true" {
			return true
		}
	}
	return false
}

// helpText returns the per-subcommand help block. Both blocks mention
// the per-agent fan-out (the file-set the deployer writes) and the
// file-ownership rules (manifest-based, leaves user files alone) — the
// testcase suite asserts these two phrases verbatim.
func helpText(subcommand string) string {
	base := strings.Join([]string{
		"  --project <path>   Explicit project root (override). Must directly contain .parlay/.",
		"  --help, -h          Print this help and exit.",
		"",
		"File ownership: the deployer reads its manifest from the embedded source surface on every run.",
		"User-authored files outside the manifest (including parlay-prefixed files not on the manifest)",
		"are never touched. Orphan files from prior Studio versions are reported and left on disk.",
		"",
		"Per-agent fan-out: the deployer detects Claude Code (.claude/), Cursor (.cursor/), and",
		"Generic CLI (.parlay/cli/) surfaces and writes one per-skill target per detected surface.",
	}, "\n")
	switch subcommand {
	case "init":
		return strings.Join([]string{
			"Usage: parlay-studio init [flags]",
			"",
			"First-time bootstrap: deploy the embedded Parlay Studio source surface into a parlay",
			"project's agent surfaces. After the first successful init, subsequent runs use `upgrade`.",
			"",
			base,
			"",
		}, "\n")
	case "upgrade":
		return strings.Join([]string{
			"Usage: parlay-studio upgrade [flags]",
			"",
			"Idempotent re-deploy: refresh the embedded Parlay Studio source surface in a parlay",
			"project's agent surfaces. Files whose content hash matches the embedded source are",
			"skipped; files that drifted (or are missing) are rewritten via atomic write.",
			"",
			base,
			"",
		}, "\n")
	default:
		return strings.Join([]string{"Usage: parlay-studio " + subcommand, "", base, ""}, "\n")
	}
}

// osEnvMap captures os.Environ as a map so config.ResolveProjectRoot can
// look up STUDIO_PROJECT_ROOT through its existing signature.
func osEnvMap() map[string]string {
	env := os.Environ()
	out := make(map[string]string, len(env))
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		out[kv[:i]] = kv[i+1:]
	}
	return out
}
