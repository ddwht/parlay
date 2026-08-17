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
