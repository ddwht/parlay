// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: amendment-ledger-check
//
// Ledger-level validation for a feature's amendments/ directory, plus the
// declared dirty set. Single-file shape problems are `parlay validate
// --type amendment`'s job; this command checks what only the whole ledger
// and the contract artifacts can answer: sequence integrity, supersedes
// resolution, whether every affects: ref names a contract entry that
// exists, and — as JSON for the skills — which entries the ledger's
// unapplied tail says are dirty.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var checkAmendmentsCmd = &cobra.Command{
	Use:   "check-amendments <@feature>",
	Short: "Validate a feature's amendment ledger and emit the declared dirty set (JSON)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckAmendments,
}

type amendmentIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type amendmentEntry struct {
	Seq        int      `json:"seq"`
	Slug       string   `json:"slug"`
	Date       string   `json:"date,omitempty"`
	Affects    []string `json:"affects"`
	Supersedes []string `json:"supersedes,omitempty"`
}

type checkAmendmentsOutput struct {
	Feature    string           `json:"feature"`
	Amendments []amendmentEntry `json:"amendments"`
	// DirtySet is the resolvable affects: refs of the UNAPPLIED TAIL only —
	// amendments whose Seq exceeds the feature baseline's
	// last-applied-amendment. That is the set a rebuild must actually touch:
	// everything at or below the baseline's last-applied sequence was already
	// folded into the generated code when the baseline was saved. Scoping it
	// this way (L7) is what makes dirty_set agree with what
	// `parlay internal diff` infers by hashing — the two disagreed when
	// dirty_set was the cumulative union, because the union kept naming
	// long-applied refs as dirty forever. Deduplicated in first-seen order.
	DirtySet []string `json:"dirty_set"`
	// AllAffects is the cumulative union of EVERY amendment's resolvable
	// affects: refs, deduplicated in first-seen order — the whole ledger's
	// footprint, regardless of what has been applied. This is the former
	// dirty_set semantics, kept under an honest name for consumers that want
	// the full history (audit, cross-feature pressure surveys) rather than the
	// rebuild-scoping tail.
	AllAffects []string `json:"all_affects"`
	// SupersededBy is the computed reverse of every amendment's supersedes:
	// forward links keyed by the superseded slug, valued by the slugs of the
	// later amendments that supersede it. The amendment files are immutable
	// once written, so a "who replaced me" link cannot live in the earlier
	// file; computing it here gives read-time forward navigation without
	// touching the ledger. Always present (possibly empty) so consumers can
	// index it unconditionally.
	SupersededBy map[string][]string `json:"superseded_by"`
	// SupersededIntents maps a founding intent slug to the amendment that
	// retired it. The forward link cannot live in intents.md — the founding
	// documents are frozen and are never written to — so this is the only
	// place a reader learns that a promise has been replaced. Always present
	// (possibly empty) so consumers can index it unconditionally.
	SupersededIntents map[string]string `json:"superseded_intents"`
	Ready             bool              `json:"ready"`
	Issues            []amendmentIssue  `json:"issues"`
}

func runCheckAmendments(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	out := computeCheckAmendments(cfg, slug)
	return emitCheckAmendmentsJSON(cmd, out)
}

// computeCheckAmendments validates a feature's amendment ledger and returns
// the structured result, performing no I/O to stdout and no process exit.
// Both the cobra command (via emitCheckAmendmentsJSON) and the gate aggregator
// call this, so the ledger-integrity logic has exactly one home.
func computeCheckAmendments(cfg *config.Context, slug string) checkAmendmentsOutput {
	featDir := cfg.FeaturePath(slug)

	out := checkAmendmentsOutput{
		Feature:           slug,
		Amendments:        []amendmentEntry{},
		DirtySet:          []string{},
		AllAffects:        []string{},
		SupersededBy:      map[string][]string{},
		SupersededIntents: map[string]string{},
		Issues:            []amendmentIssue{},
	}

	// The unapplied tail is defined against the feature baseline's
	// last-applied-amendment: any amendment beyond it has not yet been folded
	// into generated code. A missing/unreadable baseline (never built, or
	// pre-v3) reads as 0, so every amendment counts as unapplied — the
	// conservative reading, matching a from-scratch build.
	lastApplied := 0
	if blData, readErr := os.ReadFile(baselinePath(cfg, slug)); readErr == nil {
		var baseline Baseline
		if yaml.Unmarshal(blData, &baseline) == nil {
			lastApplied = baseline.LastAppliedAmendment
		}
	}

	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		out.Issues = append(out.Issues, amendmentIssue{
			Severity: "error", Code: "amendment-not-parseable", Message: err.Error(),
		})
		return out
	}

	// Files in the ledger directory that match no accepted name are worth
	// naming: a mis-numbered file silently absent from the ledger reads as
	// "never happened".
	reportStrayAmendmentFiles(featDir, &out)

	slugs := map[string]bool{}
	seqSeen := map[int]string{}
	prevSeq := 0
	// Accumulated in sequence order for scope-overlap detection: each earlier
	// amendment's file slug plus the canonical set of contract entries it
	// declares in affects:. A later amendment editing an entry an earlier one
	// also edits, without naming the earlier in its supersedes:, is two
	// unordered writers on the same contract entry — the L15/F18 hazard.
	type priorScope struct {
		fileSlug string
		affects  map[string]bool
	}
	var priors []priorScope
	// Supersession claims in sequence order. Resolved after the walk because
	// every check below needs the whole ledger in view: whether an intent
	// exists, whether two amendments claim the same one, and whether retiring
	// them all would leave the feature promising nothing.
	var supersessions []intentClaim
	for _, a := range amendments {
		entry := amendmentEntry{Seq: a.Seq, Slug: a.Slug, Date: a.Date, Affects: a.Affects, Supersedes: a.Supersedes}
		out.Amendments = append(out.Amendments, entry)

		// Single-file shape problems surface here too, so one command
		// answers "is the ledger healthy" without a per-file walk.
		content, readErr := os.ReadFile(a.Path)
		if readErr == nil {
			for _, o := range agent.ValidateAmendment(agent.ModeBuild, a.Path, content) {
				out.Issues = append(out.Issues, amendmentIssue{
					Severity: string(o.Severity), Code: o.Code, Message: fmt.Sprintf("%03d-%s: %s", a.Seq, a.FileSlug, o.Message),
				})
			}
		}

		if a.Slug != "" && a.Slug != a.FileSlug {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-slug-mismatch",
				Message: fmt.Sprintf("%s: frontmatter amendment: %q disagrees with the filename slug %q — the file may not lie about its own identity", filepath.Base(a.Path), a.Slug, a.FileSlug),
			})
		}
		if other, dup := seqSeen[a.Seq]; dup {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-out-of-sequence",
				Message: fmt.Sprintf("sequence %03d used by both %q and %q — renumber the later one", a.Seq, other, a.FileSlug),
			})
		}
		seqSeen[a.Seq] = a.FileSlug
		if prevSeq > 0 && a.Seq > prevSeq+1 {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "warning", Code: "amendment-sequence-gap",
				Message: fmt.Sprintf("sequence jumps %03d -> %03d — expected after compaction, otherwise a numbering mistake", prevSeq, a.Seq),
			})
		}
		prevSeq = a.Seq

		for _, sup := range a.Supersedes {
			if !slugs[sup] {
				out.Issues = append(out.Issues, amendmentIssue{
					Severity: "error", Code: "amendment-supersedes-unknown",
					Message: fmt.Sprintf("%03d-%s supersedes %q, which is no EARLIER amendment in this ledger", a.Seq, a.FileSlug, sup),
				})
			}
		}
		slugs[a.FileSlug] = true

		for _, raw := range a.SupersedesIntents {
			slug := strings.TrimSpace(raw)
			if slug == "" || strings.ContainsAny(slug, "@/") {
				// Shape is ValidateAmendment's to report; it already ran above.
				continue
			}
			supersessions = append(supersessions, intentClaim{
				seq: a.Seq, fileSlug: a.FileSlug, intent: slug,
				supersedes: a.Supersedes, affects: a.Affects,
			})
		}

		// Canonical scope of THIS amendment: every affects: ref that parses,
		// keyed by its normalized @feature/kind:name form so two spellings of
		// the same entry collide. Built regardless of on-disk resolvability —
		// scope overlap is about declared intent, and an unresolvable ref is
		// reported on its own line below.
		affectsCanon := map[string]bool{}
		for _, raw := range a.Affects {
			ref, parseErr := parser.ParseAmendmentRef(raw)
			if parseErr != nil {
				continue // already reported by ValidateAmendment as malformed
			}
			affectsCanon[canonicalAmendmentRef(ref)] = true
			if resolveErr := resolveAmendmentRef(cfg, ref); resolveErr != nil {
				out.Issues = append(out.Issues, amendmentIssue{
					Severity: "error", Code: "amendment-affects-unresolved",
					Message: fmt.Sprintf("%03d-%s: %s", a.Seq, a.FileSlug, resolveErr.Error()),
				})
				continue
			}
			// Every resolvable ref joins the cumulative footprint; only the
			// unapplied tail joins the rebuild-scoping dirty set.
			out.AllAffects = appendUniqueRef(out.AllAffects, raw)
			if a.Seq > lastApplied {
				out.DirtySet = appendUniqueRef(out.DirtySet, raw)
			}
		}

		// Scope overlap against every earlier amendment this one does not
		// supersede. Naming the earlier amendment in supersedes: is exactly the
		// declaration that the later change replaces it, so an overlap there is
		// intended and silent; an overlap without it is two writers with no
		// ordering between them.
		supersedesSet := map[string]bool{}
		for _, sup := range a.Supersedes {
			supersedesSet[sup] = true
		}
		for _, prior := range priors {
			if supersedesSet[prior.fileSlug] {
				continue
			}
			var overlap []string
			for ref := range affectsCanon {
				if prior.affects[ref] {
					overlap = append(overlap, ref)
				}
			}
			if len(overlap) == 0 {
				continue
			}
			sort.Strings(overlap)
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "warning", Code: "amendment-scope-overlap",
				Message: fmt.Sprintf("%03d-%s edits %s, which earlier %q also edits and this amendment does not supersede — two amendments change the same contract entry with no ordering between them", a.Seq, a.FileSlug, strings.Join(overlap, ", "), prior.fileSlug),
			})
		}
		priors = append(priors, priorScope{fileSlug: a.FileSlug, affects: affectsCanon})

		// Forward link: this amendment supersedes each named earlier slug, so
		// record it as the "superseded by" of that slug.
		for _, sup := range a.Supersedes {
			out.SupersededBy[sup] = append(out.SupersededBy[sup], a.FileSlug)
		}
	}

	resolveIntentSupersessions(cfg, slug, featDir, amendments, supersessions, &out)

	return out
}

// intentClaim is one amendment's claim to supersede one founding intent.
type intentClaim struct {
	seq        int
	fileSlug   string
	intent     string
	supersedes []string // amendment slugs this claim's amendment replaces
	affects    []string
}

// resolveIntentSupersessions runs the ledger-level half of intent supersession:
// every check that needs more than one file in view.
//
// Shape — malformed entries, cross-feature refs, missing successor or rationale
// — is ValidateAmendment's, already reported per file. What is left is what only
// the whole feature can answer: does the intent exist, does more than one live
// amendment claim it, would retiring it leave the feature promising nothing, and
// has the scope it produced been accounted for.
func resolveIntentSupersessions(cfg *config.Context, slug, featDir string, amendments []parser.Amendment, claims []intentClaim, out *checkAmendmentsOutput) {
	if len(claims) == 0 {
		return
	}

	intents, err := parser.ParseIntentsFile(filepath.Join(featDir, "intents.md"))
	if err != nil {
		out.Issues = append(out.Issues, amendmentIssue{
			Severity: "error", Code: "amendment-supersedes-intent-unknown",
			Message: fmt.Sprintf("this ledger supersedes founding intents but %s cannot be read: %v", filepath.Join(featDir, "intents.md"), err),
		})
		return
	}
	known := map[string]bool{}
	for _, in := range intents {
		known[in.Slug] = true
	}

	// claimants[intent] = the amendments claiming it, in sequence order.
	claimants := map[string][]intentClaim{}
	for _, c := range claims {
		if !known[c.intent] {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-supersedes-intent-unknown",
				Message: fmt.Sprintf("%03d-%s supersedes intent %q, which this feature's intents.md does not declare — an amendment may only retire a promise its own feature actually made", c.seq, c.fileSlug, c.intent),
			})
			continue
		}
		claimants[c.intent] = append(claimants[c.intent], c)
	}

	// A fork is more than one LIVE head retiring the same promise.
	//
	// "Ordered after some earlier claimant" is not enough, and the difference
	// is not academic: 001 claims X, 002 claims X and supersedes 001, 003
	// claims X and supersedes 001. Every claimant orders after something, yet
	// 002 and 003 are competing decisions and 003 settles nothing by ordering
	// after one 002 already replaced. Conversely a genuine chain — 003 replaces
	// 002 replaces 001 — has exactly one head and must not be reported.
	//
	// So the supersedes graph is walked transitively over the whole ledger,
	// not just over claimants: a claim may be replaced through an intermediate
	// amendment that retires no intent of its own.
	replaces := map[string][]string{}
	for _, a := range amendments {
		replaces[a.FileSlug] = append(replaces[a.FileSlug], a.Supersedes...)
	}
	ancestorsOf := func(start string) map[string]bool {
		seen := map[string]bool{}
		stack := append([]string{}, replaces[start]...)
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			stack = append(stack, replaces[n]...)
		}
		return seen
	}

	retired := map[string]bool{}
	// Only STANDING decisions must account for scope. A superseded amendment
	// is history, not specification, so its affects: cannot go on satisfying a
	// later replacement that omits the disposition.
	liveHeads := map[string][]intentClaim{}
	for intent, cs := range claimants {
		var heads []intentClaim
		for _, c := range cs {
			covered := false
			for _, other := range cs {
				if other.fileSlug == c.fileSlug {
					continue
				}
				if ancestorsOf(other.fileSlug)[c.fileSlug] {
					covered = true
					break
				}
			}
			if !covered {
				heads = append(heads, c)
			}
		}
		if len(heads) > 1 {
			var names []string
			for _, h := range heads {
				names = append(names, fmt.Sprintf("%03d-%s", h.seq, h.fileSlug))
			}
			sort.Strings(names)
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-supersedes-intent-forked",
				Message: fmt.Sprintf("%s all supersede intent %q and none replaces the others — ordering after a decision that has itself been replaced settles nothing; name the standing head in supersedes:", strings.Join(names, " and "), intent),
			})
		}
		retired[intent] = true
		liveHeads[intent] = heads
		if len(heads) > 0 {
			// The standing decision, which is the newest live head.
			head := heads[0]
			for _, h := range heads[1:] {
				if h.seq > head.seq {
					head = h
				}
			}
			out.SupersededIntents[intent] = head.fileSlug
		}
	}

	// A feature that promises nothing is a lifecycle question — whether it
	// still has consumers, whether its generated code should go — with its own
	// dependency checks. It is not something a ledger entry decides.
	live := 0
	for _, in := range intents {
		if !retired[in.Slug] {
			live++
		}
	}
	if live == 0 {
		out.Issues = append(out.Issues, amendmentIssue{
			Severity: "error", Code: "amendment-supersedes-last-intent",
			Message: fmt.Sprintf("this ledger would retire every founding intent of %s, leaving the feature promising nothing — retiring a whole feature is a lifecycle operation with its own dependency checks, not a ledger entry", slug),
		})
	}

	reportGovernanceReplacements(amendments, claimants, out)

	reportUnaccountedScope(cfg, slug, featDir, liveHeads, out)
}

// reportStrayAmendmentFiles names files in amendments/ that the loader
// ignores because their name matches no NNN-<slug>.md shape.
func reportStrayAmendmentFiles(featDir string, out *checkAmendmentsOutput) {
	dir := parser.AmendmentsDir(featDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			// archive/ is the one expected subdirectory (compaction moves
			// the old ledger there); anything else is still not an error —
			// the loader only reads well-named files either way.
			continue
		}
		if !amendmentFileNameOK(e.Name()) {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-out-of-sequence",
				Message: fmt.Sprintf("%s does not match NNN-<slug>.md and is invisible to the ledger — rename it or move it out", e.Name()),
			})
		}
	}
}

func amendmentFileNameOK(name string) bool {
	return parser.AmendmentFileNameValid(name)
}

// resolveAmendmentRef checks that a parsed affects: ref names a contract
// entry that exists on disk. The ref carries its own feature, so an
// amendment in one feature may declare effects on another's contract —
// cross-feature pressure is exactly what trigger:/affects: exist to record.
func resolveAmendmentRef(cfg *config.Context, ref parser.AmendmentRef) error {
	featDir := cfg.FeaturePath(ref.Feature)
	switch ref.Kind {
	case "operation":
		capPath := filepath.Join(featDir, "capabilities.yaml")
		caps, err := parser.ParseCapabilities(capPath)
		if err != nil {
			return fmt.Errorf("affects %s: cannot read %s: %v", ref.Raw, capPath, err)
		}
		for _, op := range caps.Operations {
			if op.ID == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no operation %q in %s", ref.Raw, ref.Name, capPath)
	case "surface":
		surfacePath := parser.ResolveSurfacePath(featDir)
		if surfacePath == "" {
			return fmt.Errorf("affects %s: feature has no surface artifact", ref.Raw)
		}
		frags, err := parser.ParseSurfaceFile(surfacePath)
		if err != nil {
			return fmt.Errorf("affects %s: cannot read %s: %v", ref.Raw, surfacePath, err)
		}
		for _, f := range frags {
			if parser.Slugify(f.Name) == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no fragment slugged %q in %s", ref.Raw, ref.Name, surfacePath)
	case "infrastructure":
		infraPath := filepath.Join(featDir, "infrastructure.md")
		_, _, fragments, err := readFragments(infraPath)
		if err != nil {
			return fmt.Errorf("affects %s: cannot read %s: %v", ref.Raw, infraPath, err)
		}
		for _, f := range fragments {
			if fragmentSlug(f.heading) == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no infrastructure fragment slugged %q in %s", ref.Raw, ref.Name, infraPath)
	case "domain":
		// The domain model is root-scoped, not feature-scoped: the ref's
		// feature part records who is asking, the entity resolves against
		// the active root's model.
		dm, err := cfg.LoadDomainModel()
		if err != nil {
			return fmt.Errorf("affects %s: cannot load domain model: %v", ref.Raw, err)
		}
		for _, e := range dm.Entities {
			if e.Name == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no entity %q in the root domain model", ref.Raw, ref.Name)
	default:
		return fmt.Errorf("affects %s: unknown kind %q", ref.Raw, ref.Kind)
	}
}

// canonicalAmendmentRef normalizes a parsed affects: ref to a stable
// @feature/kind:name key so two spellings of the same contract entry compare
// equal in the scope-overlap check. The raw text can vary (surrounding
// whitespace); the parsed fields cannot.
func canonicalAmendmentRef(ref parser.AmendmentRef) string {
	return fmt.Sprintf("@%s/%s:%s", ref.Feature, ref.Kind, ref.Name)
}

func appendUniqueRef(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

func emitCheckAmendmentsJSON(cmd *cobra.Command, out checkAmendmentsOutput) error {
	hasError := false
	for _, i := range out.Issues {
		if i.Severity == "error" {
			hasError = true
		}
	}
	out.Ready = !hasError
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if hasError {
		return NewExitCodeError(1)
	}
	return nil
}

// reportUnaccountedScope refuses a retirement that would orphan generated work.
//
// A superseded intent usually produced something: operations, fragments,
// infrastructure entries that name it in source:. Retiring the promise without
// saying what becomes of them leaves that scope with no owner — still generated,
// still shipped, and now traceable to a decision the project has withdrawn.
//
// The disposition is deliberately not a new vocabulary. Naming the entry in
// ordinary affects: IS the disposition: the amendment that retires the promise
// also states what happens to the contract entries it produced, in the field
// that already means "this amendment changes these entries". A feature with no
// contract artifact — the case this whole mechanism exists for — satisfies the
// rule with an empty set and is never asked for one.
func reportUnaccountedScope(cfg *config.Context, slug, featDir string, claimants map[string][]intentClaim, out *checkAmendmentsOutput) {
	if len(claimants) == 0 {
		return
	}

	// Accounting is PER STANDING DECISION, not pooled across the ledger.
	//
	// Pooling let two unrelated things satisfy a requirement neither had met.
	// A superseded amendment's affects: went on covering the replacement that
	// omitted the disposition, though the schema says a superseded amendment
	// is history and not specification — so the replacement must restate what
	// becomes of the scope, since the record that once said so no longer
	// speaks. And amendment A's affects: could account for amendment B's
	// retired intent, which A never claimed to touch.
	//
	// One amendment retiring several intents may cover their union with one
	// affects: list, which is why the key is the amendment rather than the
	// intent.
	accountedBy := map[string]map[string]bool{}
	intentsOf := map[string][]string{}
	for intent, heads := range claimants {
		for _, c := range heads {
			if accountedBy[c.fileSlug] == nil {
				accountedBy[c.fileSlug] = map[string]bool{}
			}
			for _, raw := range c.affects {
				if ref, err := parser.ParseAmendmentRef(raw); err == nil {
					accountedBy[c.fileSlug][canonicalAmendmentRef(ref)] = true
				}
			}
			intentsOf[c.fileSlug] = append(intentsOf[c.fileSlug], intent)
		}
	}

	type entry struct{ kind, name, source string }
	var entries []entry

	if caps, err := parser.ParseCapabilities(filepath.Join(featDir, "capabilities.yaml")); err == nil {
		for _, op := range caps.Operations {
			entries = append(entries, entry{"operation", op.ID, op.Source})
		}
	}
	if surfacePath := parser.ResolveSurfacePath(featDir); surfacePath != "" {
		if frags, err := parser.ParseSurfaceFile(surfacePath); err == nil {
			for _, f := range frags {
				entries = append(entries, entry{"surface", parser.Slugify(f.Name), f.Source})
			}
		}
	}
	if _, _, frags, err := readFragments(filepath.Join(featDir, "infrastructure.md")); err == nil {
		for _, f := range frags {
			entries = append(entries, entry{"infrastructure", parser.Slugify(f.heading), infraFragmentSource(f.body)})
		}
	}

	var unaccounted []string
	for amendment, intents := range intentsOf {
		for _, intent := range intents {
			for _, e := range entries {
				if e.source == "" || !sourceNamesIntent(e.source, slug, intent) {
					continue
				}
				ref := fmt.Sprintf("@%s/%s:%s", slug, e.kind, e.name)
				if accountedBy[amendment][ref] {
					continue
				}
				unaccounted = append(unaccounted, fmt.Sprintf("%s (from %s, retired by %s)", ref, intent, amendment))
			}
		}
	}
	if len(unaccounted) == 0 {
		return
	}
	sort.Strings(unaccounted)
	out.Issues = append(out.Issues, amendmentIssue{
		Severity: "error", Code: "intent-supersession-unaccounted-affect",
		Message: fmt.Sprintf("retiring these intents leaves %d contract entr%s with no disposition: %s — name each in affects: to say whether it is replaced, removed or retained, or the generated scope outlives the promise that justified it", len(unaccounted), plural(len(unaccounted), "y", "ies"), strings.Join(unaccounted, ", ")),
	})
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// sourceNamesIntent reports whether a contract entry's source: names this
// feature's intent.
//
// source: is a comma-separated list of refs that appear in three shapes in a
// real tree: bare (`some-intent`), feature-qualified (`@feature/some-intent`)
// and initiative-qualified (`@initiative/feature/some-intent`). Only the last
// segment is ever the intent slug.
//
// The feature half is checked as well as the slug, and that is not pedantry.
// A contract entry may legitimately source another feature's intent — that is
// what cross-feature pressure looks like on disk — so matching the last segment
// alone would let feature B's intent satisfy a lookup for feature A's
// identically-slugged one. The consequence lands on the author as a blocking
// intent-supersession-unaccounted-affect demanding they account for an entry
// that does not derive from the retired promise at all.
func sourceNamesIntent(source, feature, intent string) bool {
	// The feature may be addressed by its full slug (initiative/feature) or by
	// its bare name, depending on how the ref was written.
	featureNames := map[string]bool{feature: true}
	if i := strings.LastIndex(feature, "/"); i >= 0 {
		featureNames[feature[i+1:]] = true
	}

	for _, part := range strings.Split(source, ",") {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "@"))
		if part == "" {
			continue
		}
		i := strings.LastIndex(part, "/")
		if i < 0 {
			// Bare slug: unqualified refs can only mean this feature's own.
			if part == intent {
				return true
			}
			continue
		}
		if part[i+1:] != intent {
			continue
		}
		prefix := part[:i]
		if prefix == feature {
			return true
		}
		// Basename comparison is a concession to refs written before
		// initiatives were part of the address, and it is lossy: it cannot
		// tell @initiative-a/catalog/x from @initiative-b/catalog/x. So fall
		// back to it only when exactly one side is unqualified, where there is
		// no initiative on the other side to disagree with. When both carry an
		// initiative, they must match in full.
		refQualified := strings.Contains(prefix, "/")
		featQualified := strings.Contains(feature, "/")
		if refQualified && featQualified {
			continue
		}
		if featureNames[prefix] {
			return true
		}
		if j := strings.LastIndex(prefix, "/"); j >= 0 && featureNames[prefix[j+1:]] {
			return true
		}
	}
	return false
}

// infraFragmentSource pulls the **Source**: line out of an infrastructure
// fragment body. The fragment type carries the verbatim block rather than
// parsed fields, and this is the only field the scope walk needs.
func infraFragmentSource(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "**Source**:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// reportGovernanceReplacements refuses an ordinary amendment that replaces one
// which retired an intent, without restating the retirement.
//
// The ledger says an amendment named in supersedes: is history, not
// specification. Applied to a governance amendment that leaves the two rules
// disagreeing about who has authority. Say 001 retires intent X and 002
// supersedes 001 while claiming nothing:
//
//   - Read the retirement as standing, and it rests on a decision the ledger
//     has replaced — authority outliving the record that granted it.
//   - Read it as lapsed, and the promise silently comes back, un-retired by an
//     ordinary amendment that never faced the no-safe-default decision gate the
//     retirement itself required. An agent could undo a deliberate withdrawal
//     of scope as a side effect of an unrelated change.
//
// Neither is acceptable, so the third option is enforced here: replacing a
// governance decision means taking it over. The replacement restates
// supersedes_intents:, and with it inherits the Why, the Acceptance and the
// scope accounting that retiring a promise demands. Authority then always sits
// with an amendment that is current AND that faced the gate.
//
// This is also what lets the resolver keep its simpler claimant-only model: the
// ledger guarantees a live claimant exists for every retirement in force.
func reportGovernanceReplacements(amendments []parser.Amendment, claimants map[string][]intentClaim, out *checkAmendmentsOutput) {
	// Which intents each amendment retires.
	claimsOf := map[string]map[string]bool{}
	for intent, cs := range claimants {
		for _, c := range cs {
			if claimsOf[c.fileSlug] == nil {
				claimsOf[c.fileSlug] = map[string]bool{}
			}
			claimsOf[c.fileSlug][intent] = true
		}
	}

	for _, a := range amendments {
		for _, sup := range a.Supersedes {
			retired := claimsOf[sup]
			if len(retired) == 0 {
				continue // replacing an ordinary amendment: nothing owed
			}
			var missing []string
			for intent := range retired {
				if !claimsOf[a.FileSlug][intent] {
					missing = append(missing, intent)
				}
			}
			if len(missing) == 0 {
				continue
			}
			sort.Strings(missing)
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-supersedes-governance-incomplete",
				Message: fmt.Sprintf("%03d-%s supersedes %q, which retired %s, but does not restate %s in supersedes_intents: — replacing a decision that withdrew a promise means taking it over, along with its ## Why, ## Acceptance and scope accounting. Otherwise the retirement either outlives the record that granted it, or is quietly undone by an amendment that never faced the decision gate it required",
					a.Seq, a.FileSlug, sup, strings.Join(missing, ", "), strings.Join(missing, ", ")),
			})
		}
	}
}
