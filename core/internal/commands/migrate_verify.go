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
fragments sourcing it as well.

--fragments SEEDS A DRAFT, it does not fix the gap. This command relocates text
and cannot tell a presentation claim ("the list shows each customer's name")
from a contract claim ("rejects a duplicate email"), so it will attach
backend-shaped criteria to UI fragments. Read and rewrite what it produces:
unreviewed, those criteria demand display cases that cannot be written honestly,
and the build phase will write vacuous ones to discharge them. Where the split
needs real design judgement, /parlay-refine is the authoritative route.

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
		"Seed a DRAFT of fragment criteria by copying an operation-covered intent's bullets across; you must review and rewrite them — this cannot tell a UI claim from a contract one")
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

	totalOps, totalFrags, totalUnrouted, totalVacant, totalFlowSkipped := 0, 0, 0, 0, 0
	for _, featDir := range featureDirs {
		feature, _ := filepath.Rel(intentsRoot, featDir)

		// Route bullets only from promises still in force. A withdrawn
		// promise's criteria must not be seeded onto a contract artifact —
		// that would re-import, as current acceptance, exactly the
		// expectations an applied amendment retired.
		res, err := resolveActiveIntents(cfg, parser.FeatureSlug(feature))
		if err != nil {
			fmt.Fprintf(out, "  %s — read intents failed: %v\n", feature, err)
			continue
		}
		intents := res.Active
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

		flowSkipped := append(append([]int{}, opsAttached.FlowSkipped...), fragsAttached.FlowSkipped...)
		if opsAttached.Attached+fragsAttached.Attached == 0 && len(unrouted) == 0 &&
			len(vacant) == 0 && len(flowSkipped) == 0 {
			continue
		}
		fmt.Fprintf(out, "  %s\n", feature)
		if opsAttached.Attached > 0 {
			fmt.Fprintf(out, "    verify: attached to %d operation(s)\n", opsAttached.Attached)
		}
		if fragsAttached.Attached > 0 {
			fmt.Fprintf(out, "    verify: attached to %d fragment(s)\n", fragsAttached.Attached)
		}
		if len(flowSkipped) > 0 {
			fmt.Fprintf(out, "    skipped: %d entry(ies) whose verify: is a flow sequence "+
				"(verify: [a, b]) — bullets cannot be merged into one line-wise; convert it to a block list, or edit it by hand\n",
				len(flowSkipped))
		}
		for _, slug := range unrouted {
			fmt.Fprintf(out, "    unrouted: %s (no operation or fragment sources it)\n", slug)
		}
		for _, name := range vacant {
			fmt.Fprintf(out, "    no criteria: fragment %q still carries no verify:\n", name)
		}
		totalOps += opsAttached.Attached
		totalFrags += fragsAttached.Attached
		totalFlowSkipped += len(flowSkipped)
		totalUnrouted += len(unrouted)
		totalVacant += len(vacant)
	}

	fmt.Fprintf(out, "Operations gaining verify: %d\nFragments gaining verify: %d\nUnrouted intents (bullets left in intents.md only): %d\nFragments still without criteria: %d\n",
		totalOps, totalFrags, totalUnrouted, totalVacant)
	if totalFlowSkipped > 0 {
		fmt.Fprintf(out, "Entries skipped (flow-sequence verify:): %d\n", totalFlowSkipped)
	}
	if totalVacant > 0 && !migrateVerifyFragments {
		fmt.Fprintln(out, "\nA fragment with no verify: has nothing for a presentation case to cite, and")
		fmt.Fprintln(out, "nothing downstream reports it as missing — the coverage walkers ask whether")
		fmt.Fprintln(out, "stated criteria are discharged, and these state none. This routing sends an")
		fmt.Fprintln(out, "intent's bullets to the operations that cover it first, so an operation-covered")
		fmt.Fprintln(out, "intent leaves its fragments empty. Author the presentation claims via")
		fmt.Fprintln(out, "/parlay-refine, or re-run with --fragments to seed a draft you then review —")
		fmt.Fprintln(out, "it copies bullets mechanically and cannot tell a UI claim from a contract one.")
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
func spliceVerifyIntoCapabilities(path string, bullets map[string][]string, covered map[string]bool) (spliceResult, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return spliceResult{}, nil
	}
	caps, err := parser.ParseCapabilities(path)
	if err != nil {
		return spliceResult{}, err
	}
	// Occurrence-ordered inserts: entry i corresponds to the i-th `source:`
	// line in the file, because yaml preserves list order, `source:` appears
	// only on operations in this artifact, and operations without the key
	// contribute no line (so they are excluded here too).
	var inserts []verifyInsert
	var perEntrySlugs [][]string
	for _, op := range caps.Operations {
		if op.Source == "" {
			continue
		}
		slugs := slugsFromSourceRefs(op.Source)
		var merged, claimed []string
		for _, s := range slugs {
			if b, ok := bullets[s]; ok {
				claimed = append(claimed, s)
				if len(op.Verify) == 0 {
					merged = append(merged, b...)
				}
			}
		}
		perEntrySlugs = append(perEntrySlugs, claimed)
		inserts = append(inserts, verifyInsert{Bullets: dedupeBullets(op.Verify, merged)})
	}
	res, err := spliceAfterSourceLines(path, inserts)
	if err != nil {
		// The write did not complete successfully, so the tool cannot claim the
		// intent was routed. Deliberately not "nothing reached disk": os.WriteFile
		// truncates before it writes, so a failure can leave the file empty or
		// partial, and a comment promising the file is untouched would make a
		// reader skip the recovery step they still need.
		//
		// Today every caller aborts on this error and no false output escapes,
		// but that makes the helper's contract true by the caller's behaviour
		// rather than by construction — and a future caller that tolerates the
		// error to continue with the next feature would silently inherit the bug.
		return res, err
	}
	markCovered(covered, perEntrySlugs, res)
	return res, nil
}

// markCovered records an intent as routed only for entries the splice actually
// handled. Callers must not call it when the splice returned an error: the write
// did not complete successfully, so no intent can be claimed as routed (the file
// may also be empty or partial — os.WriteFile truncates before writing).
//
// Accounting used to happen while the inserts were being built, before anything
// was written. An entry the splice then declined — a flow-sequence verify: it
// cannot merge into line-wise — still had its intent marked routed, so the
// bullets went nowhere and the run reported neither an attachment nor an
// unrouted intent. Marking after the fact keeps a declined entry visible as
// work still to do.
func markCovered(covered map[string]bool, perEntrySlugs [][]string, res spliceResult) {
	skipped := make(map[int]bool, len(res.FlowSkipped))
	for _, occ := range res.FlowSkipped {
		skipped[occ] = true
	}
	for occ, slugs := range perEntrySlugs {
		if skipped[occ] {
			continue
		}
		for _, s := range slugs {
			covered[s] = true
		}
	}
}

// spliceVerifyIntoSurfaceYAML does the same for surface.yaml fragments.
// Legacy surface.md is deliberately not handled — it is itself pending
// migration to the YAML form.
func spliceVerifyIntoSurfaceYAML(path string, bullets map[string][]string, covered map[string]bool) (spliceResult, error) {
	if len(bullets) == 0 {
		return spliceResult{}, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return spliceResult{}, nil
	}
	frags, err := parser.LoadSurfaceYAML(path)
	if err != nil {
		return spliceResult{}, err
	}
	var inserts []verifyInsert
	var perEntrySlugs [][]string
	for _, f := range frags {
		if f.Source == "" {
			inserts = append(inserts, verifyInsert{})
			perEntrySlugs = append(perEntrySlugs, nil)
			continue
		}
		slugs := slugsFromSourceRefs(f.Source)
		var merged, claimed []string
		for _, s := range slugs {
			if b, ok := bullets[s]; ok {
				claimed = append(claimed, s)
				merged = append(merged, b...)
			}
		}
		perEntrySlugs = append(perEntrySlugs, claimed)
		// De-duplicated against what the fragment already carries, which is what
		// keeps a second run a no-op now that a non-empty entry is merged into
		// rather than skipped wholesale. Skipping was the old idempotence; with
		// merging, de-duplication has to supply it.
		//
		// Whether these bullets become a new block or join an existing one is
		// the splice's call, made from the parsed document. Deciding it here on
		// len(f.Verify) is what wrote a duplicate `verify:` key into an entry
		// whose key existed but was empty.
		inserts = append(inserts, verifyInsert{Bullets: dedupeBullets(f.Verify, merged)})
	}
	res, err := spliceAfterSourceLines(path, inserts)
	if err != nil {
		// The write did not complete successfully, so the tool cannot claim the
		// intent was routed. Deliberately not "nothing reached disk": os.WriteFile
		// truncates before it writes, so a failure can leave the file empty or
		// partial, and a comment promising the file is untouched would make a
		// reader skip the recovery step they still need.
		//
		// Today every caller aborts on this error and no false output escapes,
		// but that makes the helper's contract true by the caller's behaviour
		// rather than by construction — and a future caller that tolerates the
		// error to continue with the next feature would silently inherit the bug.
		return res, err
	}
	markCovered(covered, perEntrySlugs, res)
	return res, nil
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

// verifyInsert is the bullets one contract entry gains from a migration pass.
//
// It carries bullets only. Whether they become a fresh `verify:` block or are
// merged into an existing one is decided by the splice, from the parsed
// document — not by the caller from len(entry.Verify). That distinction is the
// origin of a real defect: `verify: []` and `verify:` both parse to zero
// bullets while the KEY exists, so a caller deciding on emptiness wrote a
// second `verify:` key into the entry and the file stopped parsing.
type verifyInsert struct {
	Bullets []string
}

func (v verifyInsert) empty() bool { return len(v.Bullets) == 0 }

// spliceResult reports what a splice did, including what it declined to do.
type spliceResult struct {
	Attached int
	// FlowSkipped are occurrence indices whose verify: is a flow sequence
	// (`verify: [a, b]`). Line-wise appending cannot express a merge into one,
	// and rewriting it into block form would reformat bytes the author wrote,
	// so these are reported rather than silently dropped or guessed at.
	FlowSkipped []int
}

// spliceAfterSourceLines adds bullets to the i-th contract entry, keyed on the
// i-th mapping that carries a `source:` key in document order.
//
// Positions come from the parsed document's line coordinates, not from
// inferring YAML's shape out of the raw text. Text inference got three layouts
// wrong, each silently: `verify: []` grew a duplicate key; `verify: [x]` was
// skipped because the scan matched only a line reading exactly `verify:`; and a
// block scalar bullet (`- |` with an indented body) had the addition inserted
// between the marker and its body, folding the original criterion into the new
// one — that last one still parses, which makes it the worst of the three.
//
// The insertion point is one line before the next node that starts outside the
// verify block. That is what makes block scalars work without understanding
// them: their body lines contain no node starts, so the next node is whatever
// follows the whole block.
func spliceAfterSourceLines(path string, inserts []verifyInsert) (spliceResult, error) {
	var res spliceResult
	if len(inserts) == 0 {
		return res, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return res, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return res, fmt.Errorf("parse %s: %w", path, err)
	}
	lines := strings.Split(string(content), "\n")

	entries := entriesWithSource(&doc)
	insertAfter := map[int][]string{} // 0-based line index -> lines emitted after it

	for occ, ins := range inserts {
		if occ >= len(entries) || ins.empty() {
			continue
		}
		entry := entries[occ]
		keyIndent := strings.Repeat(" ", entry.sourceKey.Column-1)

		verifyKey, verifyVal := mappingEntry(entry.node, "verify")
		if verifyKey == nil {
			// No key at all: write the whole block after the source: line.
			block := []string{keyIndent + "verify:"}
			for _, b := range ins.Bullets {
				block = append(block, keyIndent+"  - "+yamlScalar(b))
			}
			insertAfter[entry.sourceKey.Line-1] = block
			res.Attached++
			continue
		}
		if verifyVal != nil && verifyVal.Style&yaml.FlowStyle != 0 {
			res.FlowSkipped = append(res.FlowSkipped, occ)
			continue
		}
		// The key exists, block-styled or empty. Bullets append to it; a second
		// `verify:` key must never be written.
		at := blockEndLine(&doc, verifyKey, lines)
		var block []string
		for _, b := range ins.Bullets {
			block = append(block, keyIndent+"  - "+yamlScalar(b))
		}
		insertAfter[at] = append(insertAfter[at], block...)
		res.Attached++
	}

	if res.Attached == 0 {
		return res, nil
	}
	outLines := make([]string, 0, len(lines)+res.Attached)
	for i, line := range lines {
		outLines = append(outLines, line)
		outLines = append(outLines, insertAfter[i]...)
	}
	if !migrateVerifyDryRun {
		if err := os.WriteFile(path, []byte(strings.Join(outLines, "\n")), 0o644); err != nil {
			return res, err
		}
	}
	return res, nil
}

// sourceEntry is one contract entry: the mapping node, and its source: key.
type sourceEntry struct {
	node      *yaml.Node
	sourceKey *yaml.Node
}

// entriesWithSource returns every mapping carrying a `source:` key, in document
// order. Walking for the key rather than for a named sequence keeps this
// artifact-agnostic: capabilities.yaml holds operations, surface.yaml holds
// fragments, and both are entries with a source.
func entriesWithSource(n *yaml.Node) []sourceEntry {
	var out []sourceEntry
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if n.Kind == yaml.MappingNode {
			if key, _ := mappingEntry(n, "source"); key != nil {
				out = append(out, sourceEntry{node: n, sourceKey: key})
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(n)
	return out
}

// mappingEntry returns the key and value nodes for name in a mapping.
func mappingEntry(n *yaml.Node, name string) (key, value *yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == name {
			return n.Content[i], n.Content[i+1]
		}
	}
	return nil, nil
}

// blockEndLine returns the 0-based index of the last line belonging to the
// block introduced by key — the line an addition should follow.
//
// It is the line before the next node that starts after the key and is not
// inside the key's own value. Trailing blank and comment lines are stepped back
// over so an addition stays tight against the block rather than landing between
// a comment and the key it documents.
func blockEndLine(doc *yaml.Node, key *yaml.Node, lines []string) int {
	inside := map[*yaml.Node]bool{}
	var mark func(*yaml.Node)
	mark = func(n *yaml.Node) {
		if n == nil {
			return
		}
		inside[n] = true
		for _, c := range n.Content {
			mark(c)
		}
	}
	// Everything under the key's value belongs to the block.
	var owner *yaml.Node
	var findOwner func(*yaml.Node)
	findOwner = func(n *yaml.Node) {
		if n == nil || owner != nil {
			return
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i] == key {
					owner = n.Content[i+1]
					return
				}
			}
		}
		for _, c := range n.Content {
			findOwner(c)
		}
	}
	findOwner(doc)
	mark(owner)

	next := len(lines) + 1 // 1-based line of the next node outside the block
	var scan func(*yaml.Node)
	scan = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if !inside[n] && n != key && n.Line > key.Line && n.Line < next {
			next = n.Line
		}
		for _, c := range n.Content {
			scan(c)
		}
	}
	scan(doc)

	end := next - 2 // 0-based index of the line before the next node
	if end >= len(lines) {
		end = len(lines) - 1
	}
	for end > key.Line-1 {
		t := strings.TrimSpace(lines[end])
		if t != "" && !strings.HasPrefix(t, "#") {
			break
		}
		end--
	}
	return end
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
