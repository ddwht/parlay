// parlay-feature: parlay-tool/multi-root
// parlay-component: upgrade-refresh-confirmation
// parlay-extends: parlay-tool/multi-root/missing-agent-on-upgrade-error
// parlay-extends: parlay-tool/multi-root/bare-parent-upgrade-error
// parlay-cross-cutting: bare-parent-fallback-removal-in-deployToRoot

package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/deployer"
	"github.com/ddwht/parlay/core/internal/embedded"
	"github.com/ddwht/parlay/core/internal/feedback"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Re-deploy schemas, skills, and agent config to match the current parlay version",
	Long: `Upgrade the project's schemas, skills, and agent configuration files
to match the version of the parlay binary. This is safe to run at any
time — it only overwrites tool-managed files and never touches project
state (config, intents, dialogs, surfaces, adapters, buildfiles, or
baselines).

Run this after updating the parlay binary (e.g., brew upgrade parlay).`,
	RunE: runUpgrade,
}

// upgradeResult is the outcome of one deployToRoot call, used by
// runUpgrade to render its summary line.
type upgradeResult struct {
	SchemaCount  int
	ModuleCount  int
	SkillCount   int
	PruneCount   int
	DeployerName string
}

// ErrBareParentTopology is returned when parlay upgrade runs against a
// parent that has roots.yaml but no config.yaml. The previous code path
// silently proceeded with empty config and skipped skills; that branch
// has been removed in favour of an atomic hard-error.
var ErrBareParentTopology = errors.New("bare-parent topology: missing config.yaml")

// deployToRoot redeploys schemas, skills, and agent config at the named
// root path. The path is treated as the directory containing .parlay/.
// Used by both `parlay upgrade` and the auto-refresh hook on `parlay
// add-root`. Silent on success — callers print their own summary.
//
// Strict topology contract (multi-root v2): there is exactly one
// success path (config.yaml exists with ai-agent) and two distinct
// hard-error paths:
//   - Bare-parent (roots.yaml present, config.yaml missing) → ErrBareParentTopology
//   - Uninitialized project (neither file present) → "(run parlay init first)"
//
// The previously-tolerant fallback that proceeded with an empty config
// no longer exists — there is one path for correct topology and one
// path for hard-error. After parlay repair migrates a bare-parent
// project, the next parlay upgrade runs cleanly.
func deployToRoot(rootPath string) (upgradeResult, error) {
	cfgPath := filepath.Join(rootPath, config.ParlayDir, config.ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			rootsPath := filepath.Join(rootPath, config.ParlayDir, config.RootsIndexFile)
			if _, statErr := os.Stat(rootsPath); statErr == nil {
				// Bare-parent topology: roots.yaml present but no
				// config.yaml. Hard-error — no partial deploys, no
				// schemas, no skills.
				return upgradeResult{}, fmt.Errorf("[ERR] %w: %s is missing — run `parlay repair` to create it",
					ErrBareParentTopology, cfgPath)
			}
			// Uninitialized project: distinct from bare-parent.
			return upgradeResult{}, fmt.Errorf("read %s: %w (run parlay init first)", cfgPath, err)
		}
		return upgradeResult{}, fmt.Errorf("read %s: %w", cfgPath, err)
	}

	var cfg config.ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return upgradeResult{}, fmt.Errorf("parse %s: %w", cfgPath, err)
	}

	// Multi-root parent must declare ai-agent. There is no longer a
	// silent skip-skills fallback — see Agent-Identity Single-Source
	// Validation invariant.
	if cfg.AIAgent == "" {
		return upgradeResult{}, fmt.Errorf("[ERR] no agent identity declared at parent root.\n  - %s has no `ai-agent` field.\nAdd `ai-agent: <Claude Code|Cursor|Generic CLI>` and re-run, or run `parlay repair`",
			cfgPath)
	}

	// Re-deploy schemas.
	schemasPath := filepath.Join(rootPath, config.ParlayDir, config.SchemasDir)
	schemaCount, err := embedded.WriteSchemas(schemasPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("write schemas: %w", err)
	}
	// Retired schemas must actually leave existing projects. Without this,
	// deleting one from the embedded set changed nothing on disk.
	prunedSchemas, err := embedded.PruneStaleSchemas(schemasPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("prune schemas: %w", err)
	}

	// The digest counts as a schema write: it is deployed beside them, from the
	// same sources, and a run that refreshed it did change the schema surface.
	digestWrote, err := embedded.WriteSchemaDigest(schemasPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("write schema digest: %w", err)
	}
	if digestWrote {
		schemaCount++
	}

	// Per-schema authoring digests, deployed and pruned on the same terms as
	// the schemas they derive from: a digest whose schema was retired is read
	// as authoritative exactly like a stale schema would be.
	digestsWritten, err := embedded.WriteAuthoringDigests(schemasPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("write authoring digests: %w", err)
	}
	schemaCount += digestsWritten
	prunedDigests, err := embedded.PruneStaleAuthoringDigests(schemasPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("prune authoring digests: %w", err)
	}
	prunedSchemas += prunedDigests

	// Re-deploy the phase modules. These are skill sources that no longer
	// appear on the agent's menu — the driver and the phase subagents load
	// them by path. They land beside the schemas because the content is
	// adapter-independent.
	modulesPath := filepath.Join(rootPath, config.ParlayDir, config.ModulesDir)
	moduleCount, err := embedded.WriteModules(modulesPath)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("write modules: %w", err)
	}
	if err := embedded.PruneStaleModules(modulesPath); err != nil {
		return upgradeResult{}, fmt.Errorf("prune modules: %w", err)
	}

	// Re-deploy skills and agent config. Only command-surface skills reach
	// the menu; module-surface ones were written above.
	allSkills, _ := embedded.ReadAllSkills()
	skills := embedded.CommandSkills(allSkills)
	dep, err := deployer.Get(cfg.AIAgent)
	if err != nil {
		dep, _ = deployer.Get("generic")
	}
	if dep == nil {
		return upgradeResult{SchemaCount: schemaCount, ModuleCount: moduleCount, PruneCount: prunedSchemas}, nil
	}
	skillWrites, err := dep.Deploy(rootPath, skills)
	if err != nil {
		return upgradeResult{}, fmt.Errorf("deploy skills: %w", err)
	}
	return upgradeResult{
		SchemaCount:  schemaCount,
		ModuleCount:  moduleCount,
		SkillCount:   skillWrites,
		PruneCount:   prunedSchemas,
		DeployerName: dep.Name(),
	}, nil
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	pctx := config.FromCtx(cmd.Context())
	rootPath := "."
	if pctx != nil {
		// Always upgrade at the repo-level root (parent if active is child).
		rootPath = pctx.RepoRoot()
	}

	result, err := deployToRoot(rootPath)
	if err != nil {
		return err
	}

	// Retroactive containment for projects that enabled feedback mode
	// before the log directory was gitignored. Cheap, idempotent, and it
	// has to happen on upgrade because those projects already have a
	// directory that init will never revisit.
	feedback.EnsureContained(rootPath)

	fmt.Fprintf(cmd.OutOrStdout(), "Upgraded to parlay %s:\n", appVersion)
	// The schema and module counts are files actually rewritten, not files
	// considered: a re-run over unchanged sources reports zero because the
	// content-hash skip suppressed every write. Reporting the considered count
	// would describe work that did not happen.
	if result.SchemaCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  schemas — %d updated\n", result.SchemaCount)
	}
	if result.ModuleCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  modules — %d written\n", result.ModuleCount)
	}
	if result.SkillCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  skills  — %d written for %s\n", result.SkillCount, result.DeployerName)
	}
	// Removals are not writes, but they are changes: a run that pruned a retired
	// schema did alter the project, and reporting "nothing rewritten" alone would
	// be true and misleading at once.
	if result.PruneCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  schemas — %d retired schema(s) removed\n", result.PruneCount)
	}
	if result.SchemaCount == 0 && result.ModuleCount == 0 && result.SkillCount == 0 && result.PruneCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  already up to date — nothing rewritten")
	}

	// After the summary, not inside it: this is a separate concern from
	// what the upgrade rewrote, and interleaving it with the counts made
	// both harder to read.
	warnLegacyFeedbackLogs(cmd, rootPath)

	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: adapter-kind-field-opt-in-prompt
	//
	// After re-deploying skills + schemas, offer the kind: opt-in for any
	// adapter file that predates the multi-adapter feature. The validator
	// already treats absent kind: as the legacy presentation default; this
	// prompt makes the default explicit.
	if err := offerAdapterKindOptIn(cmd, rootPath); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Note: adapter kind opt-in skipped (%v)\n", err)
	}

	// parlay-feature: parlay-tool/ledger-and-contract
	// parlay-component: ledger-migration
	//
	// After deploying skills + schemas, scan for features whose founding
	// docs drifted before the single-regime switch and offer to freeze
	// them. Upgrade never migrates silently — freezing grandfathers edits
	// into founding history, and a person should see the list.
	if err := offerLedgerMigration(cmd, rootPath); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Note: ledger migration scan skipped (%v)\n", err)
	}

	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: adapter-emit-base-resolution
	if err := reportLegacySourceRootAdapters(cmd, adapterSearchRoots(rootPath)); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Note: source-root split check skipped (%v)\n", err)
	}

	return nil
}

// offerLedgerMigration runs the migrate-ledger dry-run scan over the repo
// root and every registered child root. If any feature needs freezing it
// prompts interactively to run the migration; in non-interactive mode it
// prints a note naming the command instead. Refusal states (leftover
// surface.md, drifted docs alongside existing amendments) are reported and
// never auto-fixed.
func offerLedgerMigration(cmd *cobra.Command, rootPath string) error {
	roots := []string{rootPath}
	if idx, err := config.LoadRootsIndex(rootPath); err == nil {
		for _, child := range idx.Children {
			roots = append(roots, child.Path)
		}
	}

	type rootScan struct {
		path     string
		toFreeze []ledgerMigrationVerdict
		blocked  []ledgerMigrationVerdict
	}
	var scans []rootScan
	needFreeze, blocked := 0, 0
	for _, root := range roots {
		verdicts, err := scanLedgerMigration(root)
		if err != nil {
			return err
		}
		s := rootScan{path: root}
		for _, v := range verdicts {
			switch v.State {
			case ledgerNeedsFreeze:
				s.toFreeze = append(s.toFreeze, v)
			case ledgerRefuseAmendments, ledgerRefuseSurfaceMD, ledgerError:
				s.blocked = append(s.blocked, v)
			}
		}
		needFreeze += len(s.toFreeze)
		blocked += len(s.blocked)
		scans = append(scans, s)
	}
	if needFreeze == 0 && blocked == 0 {
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Founding docs are frozen at first build in v0.4+. Some features have founding docs that were edited after their last green build — legal when made, but read as ledger_integrity violations now:")
	for _, s := range scans {
		for _, v := range s.toFreeze {
			fmt.Fprintf(out, "  - %s (%s): %d change(s) to freeze as founding state\n", v.Feature, s.path, len(v.Detail))
		}
		for _, v := range s.blocked {
			fmt.Fprintf(out, "  - %s (%s): needs attention before migration — run `parlay migrate-ledger --dry-run` there for details\n", v.Feature, s.path)
		}
	}

	if needFreeze == 0 {
		return nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
		fmt.Fprintln(out, "  Run `parlay migrate-ledger` (per root) to accept the current text as the founding state.")
		return nil
	}

	fmt.Fprint(out, "Freeze the current text as each feature's founding state now? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		// EOF (e.g. stdin from /dev/null, which stats as a char device
		// and so passes the TTY probe) is a decline, not a failure —
		// upgrade never migrates without an explicit yes.
		line = ""
	}
	if strings.TrimSpace(strings.ToLower(line)) != "y" {
		fmt.Fprintln(out, "  Skipped — run `parlay migrate-ledger` when ready; until then check-drift reports these as ledger_integrity.")
		return nil
	}

	for _, s := range scans {
		if len(s.blocked) > 0 {
			fmt.Fprintf(out, "  %s skipped — %d feature(s) need attention first (see above)\n", s.path, len(s.blocked))
			continue
		}
		for _, v := range s.toFreeze {
			if err := restampFoundingHashes(s.path, v.Feature); err != nil {
				return fmt.Errorf("freeze %s: %w", v.Feature, err)
			}
			fmt.Fprintf(out, "  %s — froze current founding text (%d change(s) grandfathered)\n", v.Feature, len(v.Detail))
		}
	}
	return nil
}

// reportLegacySourceRootAdapters names project adapters that still fold the
// project location and the framework's directory into one source-root field.
//
// Report, never rewrite — unlike the kind: opt-in above, which can infer its
// answer from the presence of a supports: block. There is no such signal here.
// `source-root: "src/"` is a framework directory in a single-package app and a
// project location in a repo whose code genuinely lives at ./src, and the file
// itself does not say which. The kind: prompt already established the rule for
// this situation: when it cannot infer, it skips the file and says what the
// author has to decide (upgrade.go, "add its kind: … by hand"). Guessing would
// relocate generated code on someone's next run, which is precisely the failure
// the split exists to end.
//
// Silence would be worse than a note, though. Legacy files keep their old
// behaviour exactly, so nothing breaks and nothing announces that a better
// shape now exists — and the destructive subset is only reported when a
// validation happens to run.
func reportLegacySourceRootAdapters(cmd *cobra.Command, roots []string) error {
	var legacy []string
	for _, rootPath := range roots {
		adaptersDir := filepath.Join(rootPath, ".parlay", "adapters")
		entries, err := os.ReadDir(adaptersDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".adapter.yaml") {
				continue
			}
			full := filepath.Join(adaptersDir, e.Name())
			content, err := os.ReadFile(full)
			if err != nil {
				continue
			}
			if hasFileConventionKey(content, "source-root") && !hasFileConventionKey(content, "project-root") {
				legacy = append(legacy, full)
			}
		}
	}
	if len(legacy) == 0 {
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%d adapter file(s) predate file-conventions.project-root:\n", len(legacy))
	for _, name := range legacy {
		fmt.Fprintf(out, "  - %s\n", name)
	}
	fmt.Fprint(out, ""+
		"\nThey keep working unchanged: without project-root:, an adapter-set target's root:\n"+
		"replaces source-root exactly as before.\n\n"+
		"Splitting them is worth doing when a target root: names a different directory than\n"+
		"the adapter's source-root — that substitution drops the framework's directory from\n"+
		"every derived path, which is how generated code lands outside the build. Run\n"+
		"`parlay validate --project` to see whether any of yours is in that shape; it reports\n"+
		"adapter-root-override-lossy per slot.\n\n"+
		"  project-root:  the deployable project location (\".\", apps/web) — root: replaces this\n"+
		"  source-root:   the framework's directory inside it (src/, src/app/, cmd/) — never replaced\n\n"+
		"Path templates hang off both; packages: and entry-point: off project-root alone.\n"+
		"Nothing is rewritten for you: whether your source-root names a package or a framework\n"+
		"directory is not derivable from the file, and guessing would move where your code is\n"+
		"generated.\n")
	return nil
}

// adapterSearchRoots lists every root whose .parlay/adapters/ this upgrade
// should look at: the repo-level root plus any registered child roots.
//
// Adapters resolve child-first with parent fallback, so in a multi-root project
// they overwhelmingly live in the children — this repo keeps all of its under
// core/ and studio/, and none at the repo level. Reporting only on rootPath
// would have made the note structurally incapable of firing here.
//
// The sibling kind: opt-in above still scans rootPath alone and therefore has
// the same blind spot. Left as-is deliberately: it REWRITES files, and widening
// what a rewrite touches is not a change to make while passing through.
func adapterSearchRoots(rootPath string) []string {
	roots := []string{rootPath}
	// Same enumeration offerLedgerMigration uses, and for the same reason:
	// reading roots.yaml off the repo root works whichever root is active,
	// where the active context's Index is only populated when the active root
	// IS the parent. The first version of this used the context and therefore
	// found nothing when run from inside a child.
	if idx, err := config.LoadRootsIndex(rootPath); err == nil {
		for _, child := range idx.Children {
			if child.Path != "" {
				roots = append(roots, child.Path)
			}
		}
	}
	return roots
}

// hasFileConventionKey reports whether content declares the given key nested
// one level under file-conventions:. Line-based for the same reason
// hasKindField is: a malformed adapter is validated elsewhere, and an upgrade
// note must not be the thing that fails on it.
func hasFileConventionKey(content []byte, key string) bool {
	inBlock := false
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		if !indented {
			inBlock = strings.HasPrefix(trimmed, "file-conventions:")
			continue
		}
		if inBlock && strings.HasPrefix(trimmed, key+":") {
			return true
		}
	}
	return false
}

// offerAdapterKindOptIn scans .parlay/adapters/ under rootPath for adapter
// files lacking an explicit kind: field, and offers to add `kind: presentation`
// to each. Skipping is non-blocking; files keep working without explicit
// kind: per the legacy default.
func offerAdapterKindOptIn(cmd *cobra.Command, rootPath string) error {
	adaptersDir := filepath.Join(rootPath, ".parlay", "adapters")
	entries, err := os.ReadDir(adaptersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var missing []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".adapter.yaml") {
			continue
		}
		path := filepath.Join(adaptersDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !hasKindField(content) {
			missing = append(missing, path)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
		// Non-TTY upgrade — print informational note, don't prompt.
		fmt.Fprintf(cmd.OutOrStdout(), "  Note: %d adapter file(s) lack explicit kind: presentation; run `parlay upgrade` interactively to opt in.\n", len(missing))
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "These adapter files predate the multi-adapter feature; the validator already treats them as kind: presentation.")
	for _, p := range missing {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s — inferred default: kind: presentation\n", p)
	}
	fmt.Fprint(cmd.OutOrStdout(), "Add explicit `kind: presentation` to all of them? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(strings.ToLower(line)) != "y" {
		return nil
	}

	updated, skipped := 0, 0
	for _, p := range missing {
		content, _ := os.ReadFile(p)
		if _, ok := inferAdapterKind(content); !ok {
			// A backend adapter: it declares supports:, so presentation is the
			// wrong answer and nothing here can name the right one.
			fmt.Fprintf(cmd.OutOrStdout(), "  skipped %s — it declares supports:, so it is a backend adapter; add its kind: (transport|application|persistence) by hand\n", filepath.Base(p))
			skipped++
			continue
		}
		newContent := injectKindPresentation(content)
		if err := os.WriteFile(p, newContent, 0644); err != nil {
			// A partial rewrite reported as success is how a half-migrated
			// adapters directory goes unnoticed.
			return fmt.Errorf("add kind: to %s: %w", p, err)
		}
		updated++
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Updated\n  %d file(s) gained an explicit kind: presentation line\n", updated)
	if skipped > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d backend adapter(s) skipped — see above\n", skipped)
	}
	return nil
}

// hasKindField reports whether the supplied adapter YAML content declares a
// top-level kind: field. The check is line-based rather than YAML-aware so
// it tolerates malformed adapter files (which are validated separately).
func hasKindField(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "kind:") || strings.HasPrefix(t, "kind ") {
			// Top-level only — ignore indented occurrences.
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				return true
			}
		}
	}
	return false
}

// inferAdapterKind reads the kind a kind-less adapter actually is, rather than
// assuming presentation.
//
// The opt-in used to stamp `kind: presentation` on every file lacking the
// field. That is right for the adapters that predate it, but wrong for an
// onboard-drafted backend adapter: stamping presentation onto a file that
// declares `supports:` makes it fail adapter-supports-shape-mismatch and
// collide with its own adapter-set slot. A file declaring supports: is a
// backend adapter whose kind we cannot name from the outside, so it is skipped
// and reported instead of guessed.
func inferAdapterKind(content []byte) (kind string, ok bool) {
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "supports:") {
			return "", false
		}
	}
	return "presentation", true
}

// injectKindPresentation inserts `kind: presentation` after the existing
// top-level name: line (or at the top of the file when no name: is found).
func injectKindPresentation(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "name:") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			out := append([]string{}, lines[:i+1]...)
			out = append(out, "kind: presentation")
			out = append(out, lines[i+1:]...)
			return []byte(strings.Join(out, "\n"))
		}
	}
	return []byte("kind: presentation\n" + string(content))
}

// warnLegacyFeedbackLogs tells a user about feedback logs written before
// the sanitising redesign, and does not delete them.
//
// Those logs contain what the old capture recorded: argv with absolute
// paths, full error strings, and validator messages that interpolate
// paths and quote spec content. They can never be exported — the bundle
// enforces a version floor — but they are sitting in the project, and a
// user who was told the mode was safe deserves to hear that the rules
// changed under them.
//
// Parlay does not remove them. They are the user's file, nobody asked for
// them to be touched, and a delete cannot be undone. The notice names the
// command that does it.
func warnLegacyFeedbackLogs(cmd *cobra.Command, rootPath string) {
	n := CountLegacyFeedbackLogs(rootPath)
	if n == 0 {
		return
	}
	out := cmd.ErrOrStderr()
	fmt.Fprintf(out, "\n[NOTICE] %d feedback log file(s) predate the current format.\n", n)
	fmt.Fprintf(out, "         They may contain file paths and content from your project, and are\n")
	fmt.Fprintf(out, "         never included in `parlay feedback-export`. Remove them with\n")
	fmt.Fprintf(out, "         `parlay feedback-prune --legacy`.\n\n")
}
