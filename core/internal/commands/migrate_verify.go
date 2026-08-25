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

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateVerifyDryRun bool

// migrateVerifyFragments opts into duplicating an operation-covered intent's
// bullets onto the fragments that source it.
//
// Off by default, and it must stay off by default: the migrator is a textual
// relocator and cannot tell a presentation claim from a contract claim, so a
// contract-shaped bullet copied onto a fragment demands a display case that
// cannot be written honestly — and the build phase will write a vacuous one to
// discharge it. The flag is for a project that would rather review duplicated
// criteria than author them from scratch; the accurate fix is /parlay-refine.
var migrateVerifyFragments bool

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

Because operations are routed first, a feature whose every intent produces an
operation ends the run with every fragment still empty. Those fragments are
reported as "no criteria": a fragment with no verify: has nothing for a
presentation case to cite, and no downstream check reports it, since the
coverage walkers ask whether stated criteria are discharged and these state
none. --fragments copies an operation-covered intent's bullets onto the
fragments sourcing it as well — duplicating rather than routing, so review the
result; this command cannot tell a presentation claim from a contract claim.
Authoring them properly is /parlay-refine's job.

Bullets already present on an entry are never added twice, so the command is
idempotent with or without --fragments. Legacy surface.md files are not
rewritten (they are themselves pending migration to surface.yaml; run that
first).

Use --dry-run to preview the routing without writing anything.`,
	Args: cobra.NoArgs,
	RunE: runMigrateVerify,
}

func init() {
	migrateVerifyCmd.Flags().BoolVar(&migrateVerifyDryRun, "dry-run", false,
		"Print the would-be routing for every feature; touch nothing on disk")
	migrateVerifyCmd.Flags().BoolVar(&migrateVerifyFragments, "fragments", false,
		"Also copy an operation-covered intent's bullets onto the fragments sourcing it (duplicates rather than routes; review the result)")
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

	totalOps, totalFrags, totalUnrouted, totalVacant := 0, 0, 0, 0
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

		// Only intents no operation covered fall through to the surface —
		// unless --fragments, which sends every intent's bullets to both sides.
		//
		// This routing rule is why a full-stack feature could end up with no
		// presentation criteria at all: every intent produced an operation, so
		// every intent was "covered", so nothing reached the fragments. The
		// authoring fix is in /parlay-create-artifacts, which now routes by what
		// a claim asserts; this flag is the retrofit for artifacts already
		// written under the old rule.
		remaining := map[string][]string{}
		for slug, b := range bullets {
			if migrateVerifyFragments || !covered[slug] {
				remaining[slug] = b
			}
		}
		surfacePath := filepath.Join(featDir, "surface.yaml")
		fragsAttached, err := spliceVerifyIntoSurfaceYAML(surfacePath, remaining, covered)
		if err != nil {
			return fmt.Errorf("%s: %w", feature, err)
		}

		// Projected vacancy: fragments that will still carry no verify: when
		// this run finishes. Computed from the same in-memory routing the
		// splice used, NOT by re-reading the file — under --dry-run nothing is
		// written, so a re-read returns pre-splice state and would name every
		// fragment the real run would have filled.
		vacant := projectedVacantFragments(surfacePath, remaining)

		var unrouted []string
		for slug := range bullets {
			if !covered[slug] {
				unrouted = append(unrouted, slug)
			}
		}

		if opsAttached+fragsAttached == 0 && len(unrouted) == 0 && len(vacant) == 0 {
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
		for _, name := range vacant {
			fmt.Fprintf(out, "    no criteria: fragment %q still carries no verify:\n", name)
		}
		totalOps += opsAttached
		totalFrags += fragsAttached
		totalUnrouted += len(unrouted)
		totalVacant += len(vacant)
	}

	fmt.Fprintf(out, "Operations gaining verify: %d\nFragments gaining verify: %d\nUnrouted intents (bullets left in intents.md only): %d\nFragments still without criteria: %d\n",
		totalOps, totalFrags, totalUnrouted, totalVacant)
	if totalVacant > 0 && !migrateVerifyFragments {
		fmt.Fprintln(out, "\nA fragment with no verify: has nothing for a presentation case to cite, and")
		fmt.Fprintln(out, "nothing downstream reports it as missing — the coverage walkers ask whether")
		fmt.Fprintln(out, "stated criteria are discharged, and these state none. This routing sends an")
		fmt.Fprintln(out, "intent's bullets to the operations that cover it first, so an operation-covered")
		fmt.Fprintln(out, "intent leaves its fragments empty. Author the presentation claims via")
		fmt.Fprintln(out, "/parlay-refine, or re-run with --fragments to copy the bullets across and review them.")
	}
	return nil
}

// projectedVacantFragments names the fragments that will carry no verify: once
// this run's routing is applied — the fragment names, in file order.
//
// It takes the routed bullets rather than reading the post-splice file for the
// reason --dry-run makes unavoidable: with nothing written, the file still
// shows the pre-splice state.
func projectedVacantFragments(path string, routed map[string][]string) []string {
	frags, err := parser.LoadSurfaceYAML(path)
	if err != nil {
		return nil
	}
	var vacant []string
	for _, f := range frags {
		if len(f.Verify) > 0 {
			continue
		}
		gains := false
		for _, slug := range slugsFromSourceRefs(f.Source) {
			if len(routed[slug]) > 0 {
				gains = true
				break
			}
		}
		if !gains {
			vacant = append(vacant, f.Name)
		}
	}
	return vacant
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
	var inserts []verifyInsert
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
		inserts = append(inserts, verifyInsert{NewBlock: dedupeBullets(nil, merged)})
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
	var inserts []verifyInsert
	for _, f := range frags {
		if f.Source == "" {
			inserts = append(inserts, verifyInsert{})
			continue
		}
		slugs := slugsFromSourceRefs(f.Source)
		var merged []string
		for _, s := range slugs {
			if b, ok := bullets[s]; ok {
				covered[s] = true
				merged = append(merged, b...)
			}
		}
		// De-duplicated against what the fragment already carries, which is what
		// keeps a second run a no-op now that a non-empty entry is merged into
		// rather than skipped wholesale. Skipping was the old idempotence; with
		// merging, de-duplication has to supply it.
		merged = dedupeBullets(f.Verify, merged)
		if len(f.Verify) == 0 {
			inserts = append(inserts, verifyInsert{NewBlock: merged})
			continue
		}
		inserts = append(inserts, verifyInsert{Append: merged})
	}
	return spliceAfterSourceLines(path, inserts)
}

// dedupeBullets drops bullets already present in `existing`, and any repeated
// within `add` itself. Comparison uses the same canonicalization criterion
// identity uses, so the migrator and the coverage walker agree about when two
// bullets are the same bullet.
func dedupeBullets(existing, add []string) []string {
	seen := make(map[string]bool, len(existing))
	for _, e := range existing {
		seen[agent.CanonicalCriterionText(e)] = true
	}
	var out []string
	for _, b := range add {
		key := agent.CanonicalCriterionText(b)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, b)
	}
	return out
}

// verifyInsert is what one contract entry gets from a migration pass.
//
// Two kinds, because an entry with no verify: needs a whole block written after
// its source: line, while an entry that already has one needs its missing
// bullets merged into the block it has. The original migrator only had the
// first: it skipped an entry carrying any verify: wholesale, which is what made
// it idempotent and also what made `--fragments` impossible to express — a
// fragment with one criterion could never gain a second.
type verifyInsert struct {
	// NewBlock writes a fresh verify: block after the entry's source: line.
	NewBlock []string
	// Append merges bullets into the entry's existing verify: block.
	Append []string
}

func (v verifyInsert) empty() bool { return len(v.NewBlock) == 0 && len(v.Append) == 0 }

// spliceAfterSourceLines applies inserts[i] to the i-th contract entry, keyed
// on the i-th line whose first key is `source:`. A new block's verify: key sits
// at the same indent as source:, bullets two deeper; an append lands after the
// last bullet of the entry's existing verify: block. Returns how many entries
// were touched.
//
// Two passes, because one is not enough. A single forward scan from the source:
// line finds a verify: block only when it comes AFTER source: — and YAML key
// order is arbitrary, so a hand-authored fragment listing verify: first had its
// merged bullets silently dropped while the run still counted the entry as
// touched. Locating each entry's line range first makes the position of
// verify: within it irrelevant.
func spliceAfterSourceLines(path string, inserts []verifyInsert) (int, error) {
	if len(inserts) == 0 {
		return 0, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(content), "\n")

	// Pass 1: for each source: occurrence, where the entry's keys are indented,
	// and where an append would land.
	type site struct {
		sourceLine  int // index of the source: line
		keyIndent   string
		appendAfter int // index of the last bullet of an existing verify: block, or -1
	}
	var sites []site
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if !strings.HasPrefix(trimmed, "source:") && !strings.HasPrefix(trimmed, "- source:") {
			continue
		}
		rawIndent := line[:len(line)-len(trimmed)]
		keyIndent := rawIndent
		if strings.HasPrefix(trimmed, "- ") {
			// List-item form `- source:`: sibling keys align two deeper.
			keyIndent += "  "
		}
		sites = append(sites, site{sourceLine: i, keyIndent: keyIndent, appendAfter: findVerifyBulletEnd(lines, entryBounds(lines, i, keyIndent), keyIndent)})
	}

	// Pass 2: emit.
	insertAfter := map[int][]string{} // line index -> lines to emit after it
	attached := 0
	for occ, ins := range inserts {
		if occ >= len(sites) || ins.empty() {
			continue
		}
		st := sites[occ]
		if len(ins.NewBlock) > 0 {
			block := []string{st.keyIndent + "verify:"}
			for _, b := range ins.NewBlock {
				block = append(block, st.keyIndent+"  - "+yamlScalar(b))
			}
			insertAfter[st.sourceLine] = block
			attached++
			continue
		}
		// An append with no verify: block to append to would be a lie about
		// what happened, so it is skipped and not counted rather than written
		// somewhere arbitrary.
		if st.appendAfter < 0 {
			continue
		}
		var block []string
		for _, b := range ins.Append {
			block = append(block, st.keyIndent+"  - "+yamlScalar(b))
		}
		insertAfter[st.appendAfter] = block
		attached++
	}
	if attached == 0 {
		return 0, nil
	}

	outLines := make([]string, 0, len(lines)+attached)
	for i, line := range lines {
		outLines = append(outLines, line)
		outLines = append(outLines, insertAfter[i]...)
	}
	if !migrateVerifyDryRun {
		if err := os.WriteFile(path, []byte(strings.Join(outLines, "\n")), 0o644); err != nil {
			return 0, err
		}
	}
	return attached, nil
}

// entryBounds returns the line index one past the end of the contract entry
// owning the key at line `from`, indented at keyIndent.
//
// An entry ends at the next list item shallower than its own keys: for both the
// `- name:` / `  source:` layout and the `- source:` layout, the next entry's
// dash sits at less than keyIndent, while a verify: bullet sits deeper.
func entryBounds(lines []string, from int, keyIndent string) int {
	for i := from + 1; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		if len(lines[i])-len(trimmed) < len(keyIndent) {
			return i
		}
	}
	return len(lines)
}

// findVerifyBulletEnd locates the last bullet of the verify: block belonging to
// the entry spanning [from, to), or -1 when the entry has none. The block is
// found anywhere in the entry rather than only after source:.
func findVerifyBulletEnd(lines []string, to int, keyIndent string) int {
	// Walk the whole entry; the caller passes the end and the entry's start is
	// implied by the previous entry's end, so scan from the first line that
	// belongs to this indent level.
	start := to - 1
	for start > 0 {
		trimmed := strings.TrimLeft(lines[start-1], " ")
		if strings.HasPrefix(trimmed, "- ") && len(lines[start-1])-len(trimmed) < len(keyIndent) {
			break
		}
		start--
	}
	verifyAt := -1
	for i := start; i < to; i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if trimmed == "verify:" && len(lines[i])-len(trimmed) == len(keyIndent) {
			verifyAt = i
			break
		}
	}
	if verifyAt < 0 {
		return -1
	}
	last := verifyAt
	for i := verifyAt + 1; i < to; i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if strings.HasPrefix(trimmed, "- ") && len(lines[i])-len(trimmed) > len(keyIndent) {
			last = i
			continue
		}
		break
	}
	return last
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
