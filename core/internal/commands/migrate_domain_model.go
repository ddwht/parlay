// parlay-feature: studio-support/domain-model-yaml-migration
// parlay-component: migrate-domain-model-command
// parlay-extends: studio-support/domain-model-yaml-migration/migrate-domain-model-dry-run
// parlay-extends: studio-support/domain-model-yaml-migration/migrate-domain-model-force
// parlay-extends: studio-support/domain-model-yaml-migration/migrate-domain-model-multi-root
// parlay-extends: studio-support/domain-model-yaml-migration/md-deprecation-header
// parlay-extends: studio-support/domain-model-yaml-migration/migrate-domain-model-cobra-command

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

// DeprecationHeader is the one-line deprecation banner prepended to a
// project's legacy domain-model.md after a successful migrate-domain-model
// run. It is plain markdown (not a code fence) and links to the YAML
// that supersedes it. The exact byte-string is matched by
// PrependDeprecationHeader's idempotency guard.
const DeprecationHeader = "> **Deprecated** — see [`./domain-model.yaml`](./domain-model.yaml).\n> Edits to this markdown have no effect post-migration; the YAML is the live source.\n\n"

// migrateDomainModelCmd is the cobra entry point for
// `parlay migrate-domain-model`. The actual markdown→YAML translation
// lives in the migrate-domain-model AI skill (see
// internal/embedded/skills/migrate-domain-model.skill.md). This command
// is the surface that hosts the skill: it documents the flag set, the
// stdout/stderr discipline, and the idempotency guard, and it prints a
// "use the AI skill" message when invoked from a plain CLI without an
// agent in the loop, matching the existing create-domain-model and
// load-domain-model patterns.
var migrateDomainModelCmd = &cobra.Command{
	Use:   "migrate-domain-model",
	Short: "Convert domain-model.md to domain-model.yaml (use /parlay-migrate-domain-model skill)",
	Long: `Convert a project's legacy domain-model.md into the canonical
domain-model.yaml at the active root. Idempotent: refuses to overwrite an
existing YAML without --force. Ambiguous markdown produces a YAML with
annotation markers and a non-zero exit code.

The actual translation requires an AI agent. Use the
/parlay-migrate-domain-model skill in your AI agent (e.g., Claude Code).`,
	RunE: runMigrateDomainModel,
}

var (
	migrateDomainModelDryRun bool
	migrateDomainModelForce  bool
)

func init() {
	migrateDomainModelCmd.Flags().BoolVar(&migrateDomainModelDryRun, "dry-run", false,
		"Print the planned YAML and a diff against any existing artifact; touch nothing on disk")
	migrateDomainModelCmd.Flags().BoolVar(&migrateDomainModelForce, "force", false,
		"Re-parse the .md and overwrite an existing domain-model.yaml; warns once on stderr")
}

// runMigrateDomainModel resolves the active root, runs the idempotency
// guard, and either prints the AI-skill handoff message (default flow)
// or surfaces the deterministic outcomes (greenfield no-op, already-
// migrated guard) directly.
//
// The full markdown→YAML translation requires an AI agent — see the
// migrate-domain-model skill. The Go side of this command exists to:
//
//   - Resolve the active root via the standard resolver.
//   - Detect greenfield (nothing to migrate) and exit 0.
//   - Detect already-migrated state and exit non-zero unless --force is set.
//   - Emit the --force warning before overwriting.
//   - Hand off to the AI skill for the actual translation.
//
// This matches the load-domain-model and create-domain-model patterns
// in this package — those commands are likewise AI-skill stubs that
// resolve some local state and then invite the user to invoke the
// skill from an AI agent.
func runMigrateDomainModel(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	yamlPath := cfg.DomainModelPath()
	mdPath := filepath.Join(cfg.Root.Path, "domain-model.md")

	yamlExists := fileExistsAt(yamlPath)
	mdExists := fileExistsAt(mdPath)

	// Greenfield: nothing to migrate.
	if !yamlExists && !mdExists {
		fmt.Fprintln(cmd.OutOrStdout(), "nothing to migrate")
		return nil
	}

	// Already-migrated guard.
	if yamlExists && !migrateDomainModelForce {
		fmt.Fprintf(cmd.ErrOrStderr(), "[ERR] already migrated: %s\n", yamlPath)
		os.Exit(1)
		return nil
	}

	// --force warning before overwriting.
	if yamlExists && migrateDomainModelForce {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"[WARN] --force will overwrite existing domain-model.yaml; any hand edits to the YAML will be discarded")
	}

	// AI-skill handoff. The actual markdown→YAML translation lives in
	// the migrate-domain-model skill; the Go binary cannot do it
	// without an agent.
	fmt.Fprintln(cmd.OutOrStdout(), "migrate-domain-model requires an AI agent.")
	fmt.Fprintln(cmd.OutOrStdout(), "Use the /parlay-migrate-domain-model skill in your AI agent (e.g., Claude Code).")
	return nil
}

// PrependDeprecationHeader writes the deprecation header to the top of
// the file at mdPath if it is not already present. The check is a
// substring match against DeprecationHeader's first line; a second run
// (via --force) detects the existing header and leaves the file
// untouched. Returns true if the header was newly added, false if the
// file already carried it.
//
// The function intentionally rewrites the file with the header followed
// by the original content verbatim — no trailing-whitespace fix, no
// line-ending normalization, no re-rendering of the markdown body.
func PrependDeprecationHeader(mdPath string) (added bool, err error) {
	body, err := os.ReadFile(mdPath)
	if err != nil {
		return false, fmt.Errorf("read domain-model.md: %w", err)
	}
	// Idempotency: detect the first line of the header. The header is a
	// blockquote with a stable wording, so a substring check is enough.
	if HasDeprecationHeader(body) {
		return false, nil
	}
	out := append([]byte(DeprecationHeader), body...)
	if err := os.WriteFile(mdPath, out, 0644); err != nil {
		return false, fmt.Errorf("write domain-model.md: %w", err)
	}
	return true, nil
}

// HasDeprecationHeader reports whether the given markdown bytes already
// carry the deprecation header. Used by PrependDeprecationHeader's
// idempotency guard and by tests.
func HasDeprecationHeader(content []byte) bool {
	// Match on the first line of the header — the wording is stable.
	return strings.Contains(string(content), "**Deprecated** — see [`./domain-model.yaml`]")
}

// Compile-time guard: ensure config.Context exposes the methods this
// command depends on. This catches accidental signature drift across
// the cross-cutting context.go change.
var _ interface {
	DomainModelPath() string
} = (*config.Context)(nil)
