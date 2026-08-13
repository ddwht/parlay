// parlay-feature: parlay-tool/multi-adapter
// parlay-component: verify-relocation-migration
//
// Copies each intent's **Verify** bullets onto the contract artifact entries
// that trace back to it via `source:` — operations in capabilities.yaml
// first, surface.yaml fragments for intents no operation covers — so
// testcase derivation can read acceptance criteria from the contract
// artifacts instead of the narrative intents.md.
//
// The rewrite is a textual splice, not a re-marshal: the `verify:` block is
// inserted immediately after the entry's `source:` line and every other byte
// of the file is left alone. Entries that already carry `verify:` are
// skipped, which is what makes a second run a no-op.
//
// --dry-run prints the same per-feature routing with nothing written.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateVerifyDryRun bool

var migrateVerifyCmd = &cobra.Command{
	Use:   "migrate-verify",
	Short: "Copy intent Verify bullets onto the capabilities/surface entries that source them",
	Long: `Copy each intent's **Verify** bullets into a verify: list on the
contract artifact entries whose source: refs trace back to it.

Routing: operations in capabilities.yaml take precedence; an intent covered
by at least one operation contributes nothing to surface fragments. Intents
no operation covers land on every surface.yaml fragment that sources them.
An intent whose bullets match no operation and no fragment is reported as
unrouted and left where it is — intents.md is never modified.

Entries that already have verify: are skipped, so the command is idempotent.
Legacy surface.md files are not rewritten (they are themselves pending
migration to surface.yaml; run that first).

Use --dry-run to preview the routing without writing anything.`,
	Args: cobra.NoArgs,
	RunE: runMigrateVerify,
}

func init() {
	migrateVerifyCmd.Flags().BoolVar(&migrateVerifyDryRun, "dry-run", false,
		"Print the would-be routing for every feature; touch nothing on disk")
}

func runMigrateVerify(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	intentsRoot := filepath.Join(cfg.Root.Path, "spec", "intents")
	featureDirs, err := walkIntentFeatureDirs(intentsRoot)
	if err != nil {
		return fmt.Errorf("walk intents tree: %w", err)
	}

	out := cmd.OutOrStdout()
	if migrateVerifyDryRun {
		fmt.Fprintln(out, "(dry run — no files written)")
	}

	totalOps, totalFrags, totalUnrouted := 0, 0, 0
	for _, featDir := range featureDirs {
		feature, _ := filepath.Rel(intentsRoot, featDir)

		intents, err := parser.ParseIntentsFile(filepath.Join(featDir, "intents.md"))
		if err != nil {
			fmt.Fprintf(out, "  %s — read intents failed: %v\n", feature, err)
			continue
		}
		bullets := map[string][]string{}
		for _, in := range intents {
			if len(in.Verify) > 0 {
				bullets[in.Slug] = in.Verify
			}
		}
		if len(bullets) == 0 {
			continue
		}

		covered := map[string]bool{}
		opsAttached, err := spliceVerifyIntoCapabilities(filepath.Join(featDir, "capabilities.yaml"), bullets, covered)
		if err != nil {
			return fmt.Errorf("%s: %w", feature, err)
		}

		// Only intents no operation covered fall through to the surface.
		remaining := map[string][]string{}
		for slug, b := range bullets {
			if !covered[slug] {
				remaining[slug] = b
			}
		}
		fragsAttached, err := spliceVerifyIntoSurfaceYAML(filepath.Join(featDir, "surface.yaml"), remaining, covered)
		if err != nil {
			return fmt.Errorf("%s: %w", feature, err)
		}

		var unrouted []string
		for slug := range bullets {
			if !covered[slug] {
				unrouted = append(unrouted, slug)
			}
		}

		if opsAttached+fragsAttached == 0 && len(unrouted) == 0 {
			continue
		}
		fmt.Fprintf(out, "  %s\n", feature)
		if opsAttached > 0 {
			fmt.Fprintf(out, "    verify: attached to %d operation(s)\n", opsAttached)
		}
		if fragsAttached > 0 {
			fmt.Fprintf(out, "    verify: attached to %d fragment(s)\n", fragsAttached)
		}
		for _, slug := range unrouted {
			fmt.Fprintf(out, "    unrouted: %s (no operation or fragment sources it)\n", slug)
		}
		totalOps += opsAttached
		totalFrags += fragsAttached
		totalUnrouted += len(unrouted)
	}

	fmt.Fprintf(out, "Operations gaining verify: %d\nFragments gaining verify: %d\nUnrouted intents (bullets left in intents.md only): %d\n",
		totalOps, totalFrags, totalUnrouted)
	return nil
}

// walkIntentFeatureDirs returns every directory under root that contains an
// intents.md, skipping authored units the same way the other migrators do.
func walkIntentFeatureDirs(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if config.IsAuthoredUnit(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(path) == "intents.md" {
			out = append(out, filepath.Dir(path))
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

// spliceVerifyIntoCapabilities inserts verify: blocks into capabilities.yaml
// for operations whose source refs match an intent with pending bullets and
// which do not already carry verify:. Marks matched slugs covered — including
// for operations that already had verify:, since the relocation for those
// evidently happened.
func spliceVerifyIntoCapabilities(path string, bullets map[string][]string, covered map[string]bool) (int, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, nil
	}
	caps, err := parser.ParseCapabilities(path)
	if err != nil {
		return 0, err
	}
	// Occurrence-ordered inserts: entry i corresponds to the i-th `source:`
	// line in the file, because yaml preserves list order, `source:` appears
	// only on operations in this artifact, and operations without the key
	// contribute no line (so they are excluded here too).
	var inserts [][]string
	for _, op := range caps.Operations {
		if op.Source == "" {
			continue
		}
		slugs := slugsFromSourceRefs(op.Source)
		var merged []string
		for _, s := range slugs {
			if b, ok := bullets[s]; ok {
				covered[s] = true
				if len(op.Verify) == 0 {
					merged = append(merged, b...)
				}
			}
		}
		inserts = append(inserts, merged)
	}
	return spliceAfterSourceLines(path, inserts)
}

// spliceVerifyIntoSurfaceYAML does the same for surface.yaml fragments.
// Legacy surface.md is deliberately not handled — it is itself pending
// migration to the YAML form.
func spliceVerifyIntoSurfaceYAML(path string, bullets map[string][]string, covered map[string]bool) (int, error) {
	if len(bullets) == 0 {
		return 0, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, nil
	}
	frags, err := parser.LoadSurfaceYAML(path)
	if err != nil {
		return 0, err
	}
	var inserts [][]string
	for _, f := range frags {
		if f.Source == "" {
			continue
		}
		slugs := slugsFromSourceRefs(f.Source)
		var merged []string
		for _, s := range slugs {
			if b, ok := bullets[s]; ok {
				covered[s] = true
				if len(f.Verify) == 0 {
					merged = append(merged, b...)
				}
			}
		}
		inserts = append(inserts, merged)
	}
	return spliceAfterSourceLines(path, inserts)
}

// spliceAfterSourceLines walks the file, and after the i-th line whose first
// key is `source:`, inserts a verify: block with inserts[i] (skipping empty
// entries). The verify: key sits at the same indent as source:, bullets two
// deeper. Returns how many blocks were inserted.
func spliceAfterSourceLines(path string, inserts [][]string) (int, error) {
	if len(inserts) == 0 {
		return 0, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(content), "\n")
	var outLines []string
	occurrence := 0
	attached := 0
	for _, line := range lines {
		outLines = append(outLines, line)
		trimmed := strings.TrimLeft(line, " ")
		isSource := strings.HasPrefix(trimmed, "source:") || strings.HasPrefix(trimmed, "- source:")
		if !isSource {
			continue
		}
		if occurrence < len(inserts) && len(inserts[occurrence]) > 0 {
			indent := line[:len(line)-len(trimmed)]
			if strings.HasPrefix(trimmed, "- ") {
				// List-item form `- source:`: sibling keys align two deeper.
				indent += "  "
			}
			outLines = append(outLines, indent+"verify:")
			for _, b := range inserts[occurrence] {
				outLines = append(outLines, indent+"  - "+yamlScalar(b))
			}
			attached++
		}
		occurrence++
	}
	if attached == 0 {
		return 0, nil
	}
	if !migrateVerifyDryRun {
		if err := os.WriteFile(path, []byte(strings.Join(outLines, "\n")), 0o644); err != nil {
			return 0, err
		}
	}
	return attached, nil
}

// slugsFromSourceRefs parses a comma-separated `@feature/intent-slug` list
// into intent slugs (the segment after the last slash). Features may contain
// slashes (initiative/feature), slugs may not.
func slugsFromSourceRefs(source string) []string {
	var out []string
	for _, ref := range strings.Split(source, ",") {
		ref = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref), "@"))
		if ref == "" {
			continue
		}
		if i := strings.LastIndex(ref, "/"); i >= 0 {
			ref = ref[i+1:]
		}
		if ref != "" {
			out = append(out, ref)
		}
	}
	return out
}

// yamlScalar renders a string as a safe single-line YAML scalar using the
// yaml library's own quoting rules, so bullets containing quotes, colons, or
// backticks survive the splice. Newlines (which Verify bullets never carry,
// but defensively) are flattened to spaces first.
func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	b, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimRight(string(b), "\n")
}
