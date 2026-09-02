// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: backlog-item

package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/ledger"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The subtree was defined and never attached. `parlay backlog list` did
// not exist at runtime — the tests called the runBacklog* functions
// directly, so every one of them passed against a command nobody could
// run. TestBacklogCommandTreeIsReachable pins it.
func init() {
	backlogCmd.AddCommand(
		backlogListCmd, backlogShowCmd, backlogEditCmd,
		backlogDeferCmd, backlogDeclineCmd, backlogObsoleteCmd, backlogFoldCmd,
		backlogAmendCmd, backlogFixCmd,
	)

	lf := backlogListCmd.Flags()
	lf.BoolVar(&backlogAll, "all", false, "include closed items")
	lf.BoolVar(&backlogOpen, "open", false, "open items only (the default; explicit for scripts)")
	lf.BoolVar(&backlogUntriaged, "untriaged", false, "only items nobody has ranked")
	lf.StringVar(&backlogKind, "kind", "", "one of defect, gap, debt, idea")
	lf.StringVar(&backlogListPriority, "priority", "", "P0, P1 or P2")
	lf.StringVar(&backlogAboutFilter, "about", "", "items that DECLARE they concern this ref")
	lf.StringVar(&backlogRelated, "related", "", "items about this ref OR captured while working on it")
	lf.BoolVar(&backlogJSON, "json", false, "machine-readable inventory")

	ef := backlogEditCmd.Flags()
	ef.StringVar(&backlogTitle, "title", "", "replace the title")
	ef.StringVar(&backlogBody, "body", "", "replace the body (empty clears it)")
	ef.StringVar(&backlogPriority, "priority", "", "P0, P1 or P2")
	ef.BoolVar(&backlogClearPri, "clear-priority", false, "return the item to untriaged")
	ef.StringArrayVar(&backlogAbout, "about", nil, "replace the about refs (repeatable)")
	ef.StringArrayVar(&backlogEvidence, "evidence", nil, "replace the evidence, path[:line] (repeatable)")

	// Reason and attribution on the closing verbs, and on defer, which
	// is the record of an attempt rather than a closure.
	for _, c := range []*cobra.Command{backlogDeferCmd, backlogDeclineCmd, backlogObsoleteCmd} {
		c.Flags().StringVar(&backlogReason, "reason", "", "why (required)")
		c.Flags().StringVar(&backlogBy, "by", "", "who decided (required)")
	}
	// Fold takes no --reason: the destination IS the reason, and the
	// schema forbids a reason on `folded` anyway. The parent Long said
	// every closing action requires one, which was wrong about this verb.
	backlogFoldCmd.Flags().StringVar(&backlogInto, "into", "", "the id to fold this into (required)")
	backlogFoldCmd.Flags().StringVar(&backlogBy, "by", "", "who decided (required)")

	// amend takes an AMENDMENT ref, not an item id — which is exactly
	// why it is a separate verb. `fold --into` resolves through
	// resolveBacklogItem and would reject every amendment ref, and
	// relabelling amendment provenance as `folded` would erase a
	// distinction the schema keeps on purpose: folded means the work
	// merged into another observation, amended means it landed as a
	// change to a promise the project had already made.
	backlogAmendCmd.Flags().StringVar(&backlogInto, "into", "", "the amendment that carries it: @feature/NNN-slug (required)")
	backlogAmendCmd.Flags().StringVar(&backlogBy, "by", "", "who applied it (required)")

	// A reason, and no --into: nothing became of a fix in parlay's
	// terms. `becomes:` is a typed lifecycle edge to a parlay object,
	// and overloading it with a commit or a file would weaken every
	// cross-file check that resolves it.
	backlogFixCmd.Flags().StringVar(&backlogReason, "reason", "", "what was done (required)")
	backlogFixCmd.Flags().StringVar(&backlogBy, "by", "", "who did it (required)")

	backlogShowCmd.Flags().BoolVar(&backlogJSON, "json", false, "machine-readable item")
}

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "List and triage recorded observations",
	Long: `Work through what was noticed and not done.

Capture is ` + "`parlay note`" + `, and it is deliberately cheap. This is the other
half: the decisions, which are deliberately not. Closing an item carries
a closed vocabulary and an attribution, because closing is where things get
silently lost. Beyond that the requirement splits: ` + "`decline`" + `, ` + "`obsolete`" + `
and ` + "`fix`" + ` each carry a reason, while ` + "`fold`" + `, ` + "`promote`" + ` and
` + "`amend`" + ` each carry a typed destination instead — what the work became IS
the record, and a reason would only restate it.`,
}

// Triage verbs. One subcommand per disposition rather than a --disposition
// flag, because the required arguments genuinely differ — folding needs
// somewhere to fold INTO, declining needs a reason and must NOT name a
// destination — and a single verb would have to validate that by hand and
// explain it in prose.
var (
	backlogListCmd     = &cobra.Command{Use: "list", Short: "Show open items, untriaged first", Args: cobra.NoArgs, RunE: runBacklogList}
	backlogShowCmd     = &cobra.Command{Use: "show <id>", Short: "Show one item in full, history and all", Args: cobra.ExactArgs(1), RunE: runBacklogShow}
	backlogEditCmd     = &cobra.Command{Use: "edit <id>", Short: "Correct or enrich an item's mutable fields", Args: cobra.ExactArgs(1), RunE: runBacklogEdit}
	backlogDeferCmd    = &cobra.Command{Use: "defer <id>", Short: "Record that somebody looked and could not decide", Args: cobra.ExactArgs(1), RunE: runBacklogDefer}
	backlogDeclineCmd  = &cobra.Command{Use: "decline <id>", Short: "Deliberately not doing this", Args: cobra.ExactArgs(1), RunE: runBacklogDecline}
	backlogObsoleteCmd = &cobra.Command{Use: "obsolete <id>", Short: "The condition that produced this is gone", Args: cobra.ExactArgs(1), RunE: runBacklogObsolete}
	backlogFoldCmd     = &cobra.Command{Use: "fold <id>", Short: "Absorb this into another item", Args: cobra.ExactArgs(1), RunE: runBacklogFold}
	backlogAmendCmd    = &cobra.Command{Use: "amend <id>", Short: "An amendment landed and carries this work", Args: cobra.ExactArgs(1), RunE: runBacklogAmend}
	// `fix`, the verb; `fixed`, the state it records. The command is
	// what somebody does, the event is what the record says happened.
	backlogFixCmd = &cobra.Command{Use: "fix <id>", Short: "The work was done directly — no feature, no amendment", Args: cobra.ExactArgs(1), RunE: runBacklogFix}
)

var (
	backlogBy           string
	backlogReason       string
	backlogInto         string
	backlogTitle        string
	backlogBody         string
	backlogPriority     string
	backlogClearPri     bool
	backlogAbout        []string
	backlogEvidence     []string
	backlogAll          bool
	backlogOpen         bool
	backlogUntriaged    bool
	backlogKind         string
	backlogListPriority string
	backlogAboutFilter  string
	backlogRelated      string
	backlogJSON         bool
)

// backlogListFilters is the validated filter set.
//
// Validated rather than matched loosely, because an unknown --kind used
// to produce "No open backlog items" — a false EMPTY inventory, which is
// the worst possible answer: it says the project has nothing outstanding
// when it says nothing of the kind.
type backlogListFilters struct {
	openOnly  bool
	untriaged bool
	kind      string
	priority  string
	about     string
	related   string
}

func resolveListFilters() (backlogListFilters, error) {
	f := backlogListFilters{
		openOnly:  !backlogAll,
		untriaged: backlogUntriaged,
		kind:      strings.TrimSpace(backlogKind),
		priority:  strings.TrimSpace(backlogListPriority),
		about:     strings.TrimSpace(backlogAboutFilter),
		related:   strings.TrimSpace(backlogRelated),
	}
	if backlogOpen && backlogAll {
		return f, fmt.Errorf("--open and --all contradict each other")
	}
	if f.kind != "" && !parser.KnownBacklogKind(parser.BacklogKind(f.kind)) {
		return f, fmt.Errorf("--kind %q is not one of defect, gap, debt, idea", f.kind)
	}
	if f.priority != "" && !parser.KnownBacklogPriority(f.priority) {
		return f, fmt.Errorf("--priority %q is not one of P0, P1, P2", f.priority)
	}
	if f.untriaged && f.priority != "" {
		return f, fmt.Errorf("--untriaged and --priority contradict each other: untriaged means no priority at all")
	}
	return f, nil
}

// matches decides whether one item is in scope.
//
// `about` is the narrow filter: items that DECLARE they concern the
// feature. `related` is the union of that and where the discovery
// happened — because about is optional and, when present, may point
// somewhere other than the feature somebody was working in. An item
// captured in @multi-root and about @renaming is invisible to --about
// @multi-root, which is exactly the feature whose designer is about to
// reopen that ground.
// isNarrowing reports whether this filter set can exclude anything, so
// the payload can say plainly whether counts and project totals may
// differ rather than making a consumer compare them.
func (f backlogListFilters) isNarrowing() bool {
	return f.openOnly || f.untriaged || f.kind != "" || f.priority != "" || f.about != "" || f.related != ""
}

func (f backlogListFilters) matches(it *parser.BacklogItem) bool {
	if f.openOnly && it.State() != parser.StateOpen {
		return false
	}
	if f.untriaged && it.Priority != "" {
		return false
	}
	if f.kind != "" && string(it.Kind) != f.kind {
		return false
	}
	if f.priority != "" && it.Priority != f.priority {
		return false
	}
	if f.about != "" && !itemAbout(it, f.about) {
		return false
	}
	if f.related != "" && !itemAbout(it, f.related) && !sameBacklogFeature(it.Captured.Feature, f.related) {
		return false
	}
	return true
}

func itemAbout(it *parser.BacklogItem, feature string) bool {
	for _, a := range it.About {
		if sameBacklogFeature(a, feature) {
			return true
		}
	}
	return false
}

// backlogRefFeature extracts the feature a ref names.
//
// Delegated to the parser, which owns the ref grammar. Correct because
// stored refs are now VALIDATED at write time by ValidateAboutRef, so
// the only shapes reaching here are a canonical contract ref (handled by
// the canonical parser) and a bare feature ref (the @ strip).
//
// The previous version claimed an unknown kind "still resolves to its
// feature". It did not: `@widget/newkind:x` came back as the literal
// `widget/newkind:x`, matching no feature, so the scoped read missed the
// item and reported nothing. Validation is what makes the claim true
// rather than a comment asserting it.
func backlogRefFeature(ref string) string {
	return parser.BareAboutFeature(ref)
}

// sameBacklogFeature reports whether two refs name the same feature,
// tolerantly of the leading @ and of any `/kind:name` qualifier — so
// `--related @widget` matches `@widget/operation:rename`, and an
// initiative-qualified `@auth/reset-password` matches itself.
func sameBacklogFeature(ref, feature string) bool {
	a, b := backlogRefFeature(ref), backlogRefFeature(feature)
	return a != "" && a == b
}

// loadBacklog reads every item in a root, skipping nothing silently.
//
// A file that will not parse is carried as an error rather than dropped:
// an item nobody can read is a finding, and a listing that quietly
// omitted it would report a smaller backlog than the project has.
func loadBacklog(cfg *config.Context) ([]*parser.BacklogItem, []string, error) {
	return loadBacklogAt(cfg.Root.Path)
}

// loadBacklogAt is the path-level form, so the multi-root walk can read a
// child without constructing a second Context for it.
func loadBacklogAt(rootPath string) ([]*parser.BacklogItem, []string, error) {
	dir := parser.BacklogRoot(rootPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var items []*parser.BacklogItem
	var broken []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		item, perr := parser.ParseBacklogFile(filepath.Join(dir, e.Name()))
		if perr != nil {
			broken = append(broken, fmt.Sprintf("%s: %v", e.Name(), perr))
			continue
		}
		items = append(items, item)
	}
	sortBacklog(items)
	return items, broken, nil
}

// sortBacklog puts the pile a person is needed for first.
//
// Untriaged before ranked, because an unranked item is one nobody has
// looked at, and that is the work triage exists to do. Within a group,
// id order — which is approximately capture order.
func sortBacklog(items []*parser.BacklogItem) {
	rank := map[string]int{"": 0, "P0": 1, "P1": 2, "P2": 3}
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := rank[items[i].Priority], rank[items[j].Priority]
		if ri != rj {
			return ri < rj
		}
		return items[i].ID < items[j].ID
	})
}

// backlogInventory is what `list` answers: the SINGLE inventory the
// proposal promises — backlog items and parked features together.
//
// One listing because a reader asking "what are we not doing?" should not
// have to remember the answer lives in two shapes. The two are different
// records — an item has no home yet, a parked feature has one — and that
// difference is preserved in the output rather than in which command you
// have to run.
type backlogInventory struct {
	Root       string               `json:"root"`
	Roots      []string             `json:"roots_examined"`
	RootErrors []string             `json:"root_errors,omitempty"`
	Items      []backlogListEntry   `json:"items"`
	Parked     []parkedFeatureEntry `json:"parked_features"`
	Unreadable []string             `json:"unreadable,omitempty"`
	// Counts describes THIS listing — the rows above, after filtering.
	// It used to be accumulated before filters while Items and Parked
	// were filtered, so `--related @widget --json` returned one item
	// beside counts for the whole project. A consumer reading both as
	// one answer got a number describing a different question, and
	// nothing in the payload said so.
	Counts backlogCounts `json:"counts"`
	// ProjectTotals describes the project regardless of filter, so a
	// scoped read can say "3 of 41 open items concern this feature"
	// without a second call. Separately named because the two answer
	// different questions and collapsing them is the trap above.
	ProjectTotals backlogCounts `json:"project_totals"`
	// Filtered is true when the two can differ, so a consumer never has
	// to compare them to find out.
	Filtered bool `json:"filtered"`
	// Findings are the CROSS-FILE problems — refs that stopped
	// resolving, items that have been open too long. Reported on the
	// whole root regardless of filters: a dangling ref does not become
	// less wrong because the reader narrowed the listing, and hiding it
	// behind a filter is how it stays unnoticed.
	Findings []backlogFinding `json:"findings,omitempty"`
}

type backlogListEntry struct {
	ID        string   `json:"id"`
	Root      string   `json:"root"`
	Kind      string   `json:"kind"`
	Priority  string   `json:"priority,omitempty"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	About     []string `json:"about,omitempty"`
	Feature   string   `json:"captured_feature,omitempty"`
	Deferrals int      `json:"deferrals,omitempty"`
}

type parkedFeatureEntry struct {
	Feature string `json:"feature"`
	Root    string `json:"root"`
	Phase   string `json:"phase"`
	Reason  string `json:"reason,omitempty"`
	Stale   bool   `json:"stale,omitempty"`
}

type backlogCounts struct {
	Items     int `json:"items"`
	Open      int `json:"open"`
	Untriaged int `json:"untriaged"`
	Parked    int `json:"parked_features"`
}

// collectBacklogInventory walks the active root and every child.
//
// Same walk `gate --all` and next-activity-review use, and reusing it is
// the point: a second copy is a second place for the child-error handling
// to be got wrong, which it already was once when failed children were
// dropped before they could be reported.
func collectBacklogInventory(cfg *config.Context, f backlogListFilters) backlogInventory {
	return collectBacklogInventoryAt(cfg, f, time.Now().UTC())
}

// collectBacklogInventoryAt takes the instant, so the age rule is
// testable rather than only observable in a project old enough to
// trigger it.
func collectBacklogInventoryAt(cfg *config.Context, f backlogListFilters, now time.Time) backlogInventory {
	inv := backlogInventory{Root: cfg.Root.Path, Items: []backlogListEntry{}, Parked: []parkedFeatureEntry{}}

	for _, target := range activityReviewRoots(cfg) {
		inv.Roots = append(inv.Roots, target.name)
		if target.err != nil {
			inv.RootErrors = append(inv.RootErrors, fmt.Sprintf("%s: %v", target.name, target.err))
			continue
		}

		items, broken, err := loadBacklogAt(target.ctx.Root.Path)
		if err != nil {
			inv.RootErrors = append(inv.RootErrors, fmt.Sprintf("%s: %v", target.name, err))
		}
		for _, b := range broken {
			inv.Unreadable = append(inv.Unreadable, target.name+"/"+b)
		}
		inv.Findings = append(inv.Findings, crossFileFindings(target.ctx, target.name, items, broken, now)...)

		for _, it := range items {
			open := it.State() == parser.StateOpen
			tally := func(c *backlogCounts) {
				c.Items++
				if open {
					c.Open++
					if it.Priority == "" {
						c.Untriaged++
					}
				}
			}
			tally(&inv.ProjectTotals)
			if !f.matches(it) {
				continue
			}
			tally(&inv.Counts)
			inv.Items = append(inv.Items, backlogListEntry{
				ID: it.ID, Root: target.name, Kind: string(it.Kind),
				Priority: it.Priority, Title: it.Title, State: string(it.State()),
				About: it.About, Feature: it.Captured.Feature,
				Deferrals: len(it.Deferrals()),
			})
		}

		// Parked features, from the same walk. A feature filter applies
		// to them too: asking what is outstanding "about @widget" should
		// not hide that @widget itself is parked.
		for _, slug := range target.features {
			reading := readActivity(target.ctx.FeaturePath(slug))
			phase := ComputeFeaturePhase(target.ctx, slug)
			observed := HasObservedPipelineActivity(phase)
			state := reading.Resolve(observed)
			if state == ActivityUnavailable {
				// A declaration that exists and cannot be read is not a
				// feature that is unparked — it is a feature whose
				// disposition nobody can determine. Named, because an
				// inventory that quietly treated it as active would
				// under-report what is outstanding, which is precisely
				// the failure this whole command exists to prevent.
				if why, unusable := reading.Unusable(); unusable {
					inv.Unreadable = append(inv.Unreadable,
						fmt.Sprintf("%s/%s: activity declaration unreadable (%s)", target.name, slug, why))
				}
				continue
			}
			if state != string(parser.ActivityParked) {
				continue
			}
			inv.ProjectTotals.Parked++
			if f.about != "" && !sameBacklogFeature(slug, f.about) {
				continue
			}
			if f.related != "" && !sameBacklogFeature(slug, f.related) {
				continue
			}
			if f.kind != "" || f.priority != "" || f.untriaged {
				// Kind and priority are item vocabulary; a parked feature
				// has neither, so a filter on them is a question about
				// items and this is not one.
				continue
			}
			inv.Counts.Parked++
			entry := parkedFeatureEntry{Feature: slug, Root: target.name, Phase: string(phase), Stale: reading.ParkingIsStale(observed)}
			if p, ok := reading.activity.LatestParking(); ok {
				entry.Reason = p.Reason
			}
			inv.Parked = append(inv.Parked, entry)
		}
	}
	return inv
}

func runBacklogList(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	f, err := resolveListFilters()
	if err != nil {
		return err
	}
	inv := collectBacklogInventory(cfg, f)
	inv.Filtered = f.isNarrowing()

	if backlogJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(inv)
	}
	writeBacklogInventory(cmd.OutOrStdout(), inv)
	return nil
}

func writeBacklogInventory(w io.Writer, inv backlogInventory) {
	groups := map[string][]backlogListEntry{}
	var order []string
	for _, it := range inv.Items {
		label := it.Priority
		if label == "" {
			label = "UNTRIAGED"
		}
		if _, seen := groups[label]; !seen {
			order = append(order, label)
		}
		groups[label] = append(groups[label], it)
	}
	// UNTRIAGED first: that is the pile a person is needed for.
	sort.SliceStable(order, func(i, j int) bool {
		return order[i] == "UNTRIAGED" && order[j] != "UNTRIAGED"
	})

	if len(inv.Items) == 0 && len(inv.Parked) == 0 && len(inv.Unreadable) == 0 &&
		len(inv.RootErrors) == 0 && len(inv.Findings) == 0 {
		fmt.Fprintln(w, "Nothing outstanding: no matching backlog items and no parked features.")
		return
	}
	for _, label := range order {
		g := groups[label]
		fmt.Fprintf(w, "\n%s (%d)\n", label, len(g))
		for _, it := range g {
			suffix := ""
			if it.Deferrals > 0 {
				suffix = fmt.Sprintf("  [deferred x%d]", it.Deferrals)
			}
			// Root-qualified, always. Two roots may hold the same id
			// prefix or the same feature slug, and a bare row leaves
			// the reader with no way to know which --root to pass.
			fmt.Fprintf(w, "  %-14s %s  %-7s %s%s\n", it.Root, shortID(it.ID), it.Kind, it.Title, suffix)
		}
	}
	if len(inv.Parked) > 0 {
		fmt.Fprintf(w, "\nPARKED FEATURES (%d)\n", len(inv.Parked))
		for _, p := range inv.Parked {
			stale := ""
			if p.Stale {
				stale = "  [parking is stale]"
			}
			fmt.Fprintf(w, "  %-14s %-38s %-9s %s%s\n", p.Root, p.Feature, p.Phase, p.Reason, stale)
		}
	}
	// Named, never omitted, for the same reason everywhere else in this
	// package: a listing that silently dropped them would report a
	// smaller backlog than the project has.
	if len(inv.Unreadable) > 0 {
		// "record(s)", not "item(s)": this now carries unusable activity
		// declarations as well as unparseable backlog items, and calling
		// a parked feature's broken declaration an item is wrong.
		fmt.Fprintf(w, "\n!! %d record(s) could not be read:\n", len(inv.Unreadable))
		for _, b := range inv.Unreadable {
			fmt.Fprintf(w, "   - %s\n", b)
		}
	}
	if inv.Filtered && inv.Counts.Items != inv.ProjectTotals.Items {
		fmt.Fprintf(w, "\nShowing %d of %d item(s) — filters are in effect.\n",
			inv.Counts.Items, inv.ProjectTotals.Items)
	}
	if len(inv.Findings) > 0 {
		fmt.Fprintf(w, "\n!! %d finding(s):\n", len(inv.Findings))
		for _, f := range inv.Findings {
			fmt.Fprintf(w, "   - [%s] %s %s\n", f.Code, shortID(f.Item), f.Message)
			if f.Fix != "" {
				fmt.Fprintf(w, "       %s\n", f.Fix)
			}
		}
	}
	if len(inv.RootErrors) > 0 {
		fmt.Fprintf(w, "\n!! %d root(s) could not be enumerated — their items are NOT in this listing:\n", len(inv.RootErrors))
		for _, e := range inv.RootErrors {
			fmt.Fprintf(w, "   - %s\n", e)
		}
	}
}

// shortID is the id's leading timestamp plus suffix — enough to address
// an item without pasting a slug a reader can already see in the title.
func shortID(id string) string {
	parts := strings.SplitN(id, "-", 3)
	if len(parts) < 2 {
		return id
	}
	return parts[0] + "-" + parts[1]
}

func runBacklogShow(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	item, _, err := resolveBacklogItem(cfg, args[0])
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(item)
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.OutOrStdout(), string(out))
	return nil
}

var errBacklogAmbiguous = errors.New("ambiguous item reference")

// errBacklogNotFound is the EXACT not-found result, typed so a caller can
// tell "this item is definitely gone" from "the store could not be read".
// Those must never collapse: a promotion that treats an unreadable store
// as proof the holder vanished will steal a live claim at precisely the
// moment ownership cannot be verified.
var errBacklogNotFound = errors.New("no such backlog item")

// resolveBacklogItem accepts a full id or any unambiguous prefix.
//
// A prefix that matches more than one item is refused rather than
// resolved to the first: a triage command that acted on the wrong item
// because two ids shared a timestamp would be silent about it.
func resolveBacklogItem(cfg *config.Context, ref string) (*parser.BacklogItem, string, error) {
	items, _, err := loadBacklog(cfg)
	if err != nil {
		return nil, "", err
	}
	ref = strings.TrimSuffix(strings.TrimSpace(ref), ".yaml")
	var matches []*parser.BacklogItem
	for _, it := range items {
		if it.ID == ref || strings.HasPrefix(it.ID, ref) {
			matches = append(matches, it)
		}
	}
	switch len(matches) {
	case 0:
		return nil, "", fmt.Errorf("%w: no backlog item matching %q", errBacklogNotFound, ref)
	case 1:
		return matches[0], filepath.Join(parser.BacklogRoot(cfg.Root.Path), matches[0].ID+".yaml"), nil
	default:
		var ids []string
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return nil, "", fmt.Errorf("%w: %q matches %d items — %s", errBacklogAmbiguous, ref, len(matches), strings.Join(ids, ", "))
	}
}

// The refusals a disposition can raise.
var (
	errBacklogClosed           = errors.New("item is already closed")
	errBacklogUnusable         = errors.New("item cannot be used")
	errBacklogFoldIntoSelf     = errors.New("an item cannot be folded into itself")
	errBacklogFoldClosedTarget = errors.New("cannot fold into a closed item")
)

func runBacklogDefer(cmd *cobra.Command, args []string) error {
	return appendDisposition(cmd, args[0], parser.EventDeferred, "")
}
func runBacklogDecline(cmd *cobra.Command, args []string) error {
	return appendDisposition(cmd, args[0], parser.EventDeclined, "")
}
func runBacklogObsolete(cmd *cobra.Command, args []string) error {
	return appendDisposition(cmd, args[0], parser.EventObsolete, "")
}

func runBacklogFold(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(backlogInto) == "" {
		return fmt.Errorf("--into is required: folding names where the work went, and a fold that names nowhere is a decline with the reason left out")
	}
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	// EVERY fold serialises against every other fold in this root.
	//
	// A fold touches TWO items and the per-item lock only covers one, so
	// two reciprocal folds take different locks and neither sees the
	// other: A→B locks A, B→A locks B, both read the other as open, and
	// both publish — leaving a two-node cycle in which neither item
	// survives, each pointing at the other as where the work went.
	// Re-reading the destination inside the source's callback does not
	// close it either, because that read is still outside the
	// destination's lock.
	//
	// One shared scope for the whole graph mutation is the cheap fix, and
	// contention is negligible: folds are interactive and rare. Lock
	// ORDER is fold-lock then item-lock, and it must stay that way
	// everywhere — the reverse in one place is a deadlock.
	return withBacklogFoldLock(cfg, func() error {
		// Resolved INSIDE the lock, both of them. Outside it, either
		// could close between the check and the write.
		target, _, terr := resolveBacklogItem(cfg, backlogInto)
		if terr != nil {
			return fmt.Errorf("--into %q: %w", backlogInto, terr)
		}
		source, _, serr := resolveBacklogItem(cfg, args[0])
		if serr != nil {
			return serr
		}
		if source.ID == target.ID {
			return errBacklogFoldIntoSelf
		}
		// The destination must still be OPEN. This is what prevents the
		// cycle rather than detecting it afterwards: once A folds into B,
		// A is closed, so a later B→A finds a closed destination and is
		// refused. It also rules out folding into something already
		// declined, which is separately wrong — the work would be
		// recorded as moving somewhere that had already stopped.
		if target.State() != parser.StateOpen {
			return fmt.Errorf("%w: %s is %s, so folding into it would record the work as moving somewhere already closed",
				errBacklogFoldClosedTarget, shortID(target.ID), target.State())
		}
		return appendDisposition(cmd, args[0], parser.EventFolded, target.ID)
	})
}

// withBacklogFoldLock serialises fold graph mutations within one root.
// withBacklogDecisionLock serialises EVERY terminal decision on one item.
//
// The per-item ledger lock is not enough, because it only covers the
// moment of the write. A promotion does WORK before that write — it
// scaffolds a feature tree — and the open-state check that authorised
// the work sat outside any lock, so two concurrent promotions could
// both see an open item, both scaffold a different feature, and only
// one terminal event survive; a concurrent decline could leave a
// scaffolded feature beside an item closed as `declined`. Re-checking
// under mutateBacklogItem prevents corrupt history, which is a
// different thing from preventing duplicated work.
//
// So the guard is held across the WHOLE decision — deciding, doing, and
// recording — by every verb that can produce a terminal event. Losing
// the race means never starting the work, rather than discovering
// afterwards that it was wasted.
//
// LOCK ORDER, everywhere, without exception: fold-guard, then
// decision-guard, then the item's own ledger lock. The reverse in one
// place is a deadlock.
//
// Named per item, so decisions about different items do not contend.
func withBacklogDecisionLock(cfg *config.Context, itemID string, fn func() error) error {
	name := ".decision-" + strings.NewReplacer("/", "_", string(os.PathSeparator), "_").Replace(itemID)
	guard := ledger.New(cfg.Root.Path, filepath.Join(parser.BacklogRoot(cfg.Root.Path), name))
	var inner error
	if err := guard.Update(func([]byte, bool) ([]byte, bool, error) {
		inner = fn()
		// Never write: the guard file is a lock name, not a record.
		return nil, false, nil
	}); err != nil {
		return err
	}
	return inner
}

func withBacklogFoldLock(cfg *config.Context, fn func() error) error {
	guard := ledger.New(cfg.Root.Path, filepath.Join(parser.BacklogRoot(cfg.Root.Path), ".fold-guard"))
	var inner error
	if err := guard.Update(func([]byte, bool) ([]byte, bool, error) {
		inner = fn()
		// Never write. The guard file is a lock name, not a record —
		// creating it would put a dotfile in a directory a person reads
		// for no benefit.
		return nil, false, nil
	}); err != nil {
		return err
	}
	return inner
}

// appendDisposition is the one write path for every triage verb.
func appendDisposition(cmd *cobra.Command, ref string, event parser.BacklogEventKind, becomes string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	return appendDispositionWith(cmd, cfg, ref, event, becomes,
		strings.TrimSpace(backlogReason), strings.TrimSpace(backlogBy))
}

// appendDispositionWith takes the reason and attribution as ARGUMENTS.
//
// Same reason promoteFeature does: the flags stay where cobra needs
// them, and the decision itself can be exercised concurrently without
// two callers racing on globals that one real invocation never
// contends for.
func appendDispositionWith(cmd *cobra.Command, cfg *config.Context, ref string, event parser.BacklogEventKind, becomes, reason, by string) error {
	if by == "" {
		return fmt.Errorf("--by is required: a disposition nobody can attribute tells the next reader nothing they did not already know")
	}
	needsReason := event == parser.EventDeferred || event == parser.EventDeclined || event == parser.EventObsolete
	if needsReason && reason == "" {
		return fmt.Errorf("--reason is required: a decision nobody can review later is not one")
	}

	_, path, err := resolveBacklogItem(cfg, ref)
	if err != nil {
		return err
	}

	ev := parser.BacklogEvent{
		Event:   event,
		Reason:  reason,
		Becomes: becomes,
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		By:      by,
	}

	// A deferral is NOT terminal, so it does not contend for the
	// decision guard — two people recording independent failed attempts
	// is a fact worth having twice, not a race.
	apply := func() error {
		return mutateBacklogItem(cfg, path, func(item *parser.BacklogItem) error {
			// Judged against the LOCKED item, not the one read before.
			if item.State() != parser.StateOpen {
				return fmt.Errorf("%w: it is %s", errBacklogClosed, item.State())
			}
			item.History = append(item.History, ev)
			return nil
		})
	}
	if event != parser.EventDeferred {
		id := filepath.Base(strings.TrimSuffix(path, ".yaml"))
		if err := withBacklogDecisionLock(cfg, id, apply); err != nil {
			return err
		}
	} else if err := apply(); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", event, shortID(filepath.Base(strings.TrimSuffix(path, ".yaml"))))
	if ev.Reason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  reason: %s\n", ev.Reason)
	}
	if ev.Becomes != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  becomes: %s\n", ev.Becomes)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  by:     %s\n", ev.By)
	return nil
}

func runBacklogEdit(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	// No --by here, deliberately. It used to be demanded and then
	// discarded — the edit recorded no attribution anywhere, so the flag
	// bought a false impression of accountability at the cost of a
	// required argument. `captured` is immutable and `history` is
	// append-only dispositions; a correction is neither, and the schema's
	// mutability section is explicit that hand edits are not detectable
	// here at all. Demanding a name for a record that cannot hold it is
	// worse than not asking.
	if backlogClearPri && strings.TrimSpace(backlogPriority) != "" {
		return fmt.Errorf("--priority and --clear-priority contradict each other")
	}
	if p := strings.TrimSpace(backlogPriority); p != "" && !parser.KnownBacklogPriority(p) {
		return fmt.Errorf("--priority %q is not one of P0, P1, P2", backlogPriority)
	}
	_, path, err := resolveBacklogItem(cfg, args[0])
	if err != nil {
		return err
	}

	evidence, err := parseEvidenceFlags(backlogEvidence)
	if err != nil {
		return err
	}

	// Changed(), not emptiness. An empty --body is a request to CLEAR
	// the body, and testing the string could not tell that apart from
	// not passing the flag at all — so the one command that exists to
	// correct a record silently refused to remove anything from it.
	f := cmd.Flags()
	set := func(name string) bool { return f.Changed(name) }

	if !set("title") && !set("body") && !set("priority") && !backlogClearPri &&
		!set("about") && !set("evidence") {
		return fmt.Errorf("nothing to edit: pass at least one of --title, --body, --priority, --clear-priority, --about, --evidence")
	}

	err = mutateBacklogItem(cfg, path, func(item *parser.BacklogItem) error {
		// ONLY the mutable fields. captured and history are not
		// reachable from here, which is the guarantee rather than a rule
		// somebody has to remember: this function cannot express a
		// change to them.
		if set("title") {
			t := strings.TrimSpace(backlogTitle)
			if t == "" {
				// Title is required by the schema, so clearing it would
				// write a record the validator refuses. Say that here
				// rather than let the post-mutation validation report it
				// as a mysterious invalid item.
				return fmt.Errorf("--title cannot be empty: an item with no title is one nobody can recognise in a listing")
			}
			item.Title = t
		}
		if set("body") {
			item.Body = strings.TrimSpace(backlogBody)
		}
		if backlogClearPri {
			// Absent is a meaningful state, so a mistaken rank must be
			// reversible to it rather than only replaceable by another.
			item.Priority = ""
		} else if set("priority") {
			p := strings.TrimSpace(backlogPriority)
			if p == "" {
				return fmt.Errorf("--priority cannot be empty: to unrank an item pass --clear-priority, which says so")
			}
			item.Priority = p
		}
		if set("about") {
			item.About = backlogAbout
		}
		if set("evidence") {
			item.Evidence = evidence
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "edited %s\n", shortID(filepath.Base(strings.TrimSuffix(path, ".yaml"))))
	return nil
}

// mutateBacklogItem is the locked read-modify-write every mutation runs
// through.
//
// Same discipline as activity's append: the item is re-read and
// re-validated INSIDE the lock, the mutation is applied to those bytes,
// and the result is validated before it is published — so a bug here
// cannot write a record the tool would refuse to read back.
func mutateBacklogItem(cfg *config.Context, path string, mutate func(*parser.BacklogItem) error) error {
	store := ledger.New(cfg.Root.Path, path)
	return store.Update(func(current []byte, exists bool) ([]byte, bool, error) {
		if !exists {
			return nil, false, fmt.Errorf("%s vanished before it could be written", filepath.Base(path))
		}
		item, perr := parser.ParseBacklogBytes(path, current)
		if perr != nil {
			return nil, false, fmt.Errorf("%w: %v", errBacklogUnusable, perr)
		}
		if problems := parser.ValidateBacklogShape(item); len(problems) > 0 {
			return nil, false, fmt.Errorf("%w: %s", errBacklogUnusable, problems[0].Message)
		}
		// Snapshot the immutable parts BEFORE the callback runs, so the
		// guarantee is ENFORCED rather than merely unexpressible.
		//
		// Until now the argument was that `edit` has no line that could
		// reach `captured` or rewrite `history`. True, and worth
		// nothing the moment somebody adds one — it made the invariant
		// depend on every future author noticing it. Comparing before
		// and after is what turns "no caller does this" into "no caller
		// can".
		wasCaptured := item.Captured
		wasHistory := append([]parser.BacklogEvent(nil), item.History...)

		if err := mutate(item); err != nil {
			return nil, false, err
		}

		if item.Captured != wasCaptured {
			return nil, false, fmt.Errorf("%s: `captured` is immutable — it records who observed what, and when, and a record whose provenance can be rewritten is not provenance",
				agent.CodeBacklogCapturedUpdateForbidden)
		}
		if err := historyOnlyAppended(wasHistory, item.History); err != nil {
			return nil, false, fmt.Errorf("%s: %w", agent.CodeBacklogHistoryUpdateForbidden, err)
		}

		if problems := parser.ValidateBacklogShape(item); len(problems) > 0 {
			return nil, false, fmt.Errorf("refusing to write an item that would not validate: %s", problems[0].Message)
		}
		out, merr := yaml.Marshal(item)
		if merr != nil {
			return nil, false, fmt.Errorf("serialise item: %w", merr)
		}
		return out, true, nil
	})
}

// runBacklogAmend closes an item because an amendment now carries it.
//
// This is the other half of `parlay promote --as-amendment`, which
// deliberately writes nothing and leaves the item OPEN: an amendment is
// authored with a person in the loop, so nothing can be closed against
// it until it actually exists. This is the command that runs afterwards,
// and it refuses unless the amendment is on disk — an item closed
// against an amendment nobody wrote is an observation lost with a
// receipt saying it was handled.
// runBacklogFix records that somebody changed the system so the
// condition no longer holds.
//
// The third ending that produces no parlay object, beside decline and
// obsolete — and distinct from both, because "we chose not to", "it
// stopped mattering" and "somebody fixed it" are three different facts
// a reader should not have to reconstruct from prose.
func runBacklogFix(cmd *cobra.Command, args []string) error {
	return appendDisposition(cmd, args[0], parser.EventFixed, "")
}

func runBacklogAmend(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	into := strings.TrimSpace(backlogInto)
	if into == "" {
		return fmt.Errorf("--into is required: it names the amendment that carries the work, as @feature/NNN-slug")
	}
	// Resolve the ITEM first, so the trigger is checked against the
	// canonical id rather than whatever spelling the caller used.
	item, _, err := resolveBacklogItem(cfg, args[0])
	if err != nil {
		return err
	}
	ref, err := resolveAmendmentTarget(cfg, into, item.ID)
	if err != nil {
		return err
	}
	return appendDisposition(cmd, item.ID, parser.EventAmended, ref)
}

// resolveAmendmentTarget checks the amendment is real AND that it is
// THIS item's amendment.
//
// Existence alone is not enough, and that was the hole: any amendment on
// the feature could close any open item as `amended`, manufacturing
// exactly the causal link this is supposed to preserve. The amendment
// has to say so itself, in the `trigger:` the schema already describes
// as "the causal link that previously lived nowhere". So the target is
// parsed with the real amendment parser and its trigger must equal
// `backlog:<id>` exactly.
//
// THE BOUNDARY IS AUTHORED-ON-DISK, not applied. An amendment exists as
// a decision the moment it is written — it is append-only and never
// edited — whereas application is a separate, later act with its own
// gate, and an item that stayed open until then would keep being handed
// to reviewers who have nothing left to decide about it. This is stated
// here, in the schema and in the skill so nobody later assumes the
// stronger meaning.
func resolveAmendmentTarget(cfg *config.Context, raw, itemID string) (string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "@")
	idx := strings.LastIndex(trimmed, "/")
	if idx <= 0 || idx == len(trimmed)-1 {
		return "", fmt.Errorf("--into %q is not an amendment ref: write @feature/NNN-slug, or @initiative/feature/NNN-slug", raw)
	}
	featureSlug, slug := trimmed[:idx], trimmed[idx+1:]
	slug = strings.TrimSuffix(slug, ".md")

	dir := parser.AmendmentsDir(cfg.FeaturePath(featureSlug))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("@%s has no amendments directory at %s — nothing has been amended there yet, so the item cannot be closed against one", featureSlug, dir)
		}
		return "", fmt.Errorf("read %s: %w", dir, err)
	}
	var available []string
	var target string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		available = append(available, name)
		if name == slug {
			target = filepath.Join(dir, e.Name())
		}
	}
	if target == "" {
		return "", fmt.Errorf("no amendment %q on @%s. The item stays open until the amendment exists. Available: %s",
			slug, featureSlug, strings.Join(available, ", "))
	}

	content, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", target, err)
	}
	amendment, err := parser.ParseAmendmentBytes(target, content)
	if err != nil {
		// Refused rather than accepted on the strength of the filename.
		// An amendment that will not parse is not a decision anybody can
		// read, so it cannot be the thing an item became.
		return "", fmt.Errorf("%s will not parse, so it cannot be what this item became: %w", slug, err)
	}

	want := "backlog:" + itemID
	switch got := strings.TrimSpace(amendment.Trigger); {
	case got == want:
		return "@" + featureSlug + "/" + slug, nil
	case got == "":
		return "", fmt.Errorf("%s carries no `trigger:`, so nothing connects it to %s. Add `trigger: %s` to the amendment — that link is the whole reason this closure is recorded rather than assumed",
			slug, shortID(itemID), want)
	default:
		return "", fmt.Errorf("%s carries `trigger: %s`, not `%s` — it is a different amendment for a different reason. Closing this item against it would manufacture a causal link that does not exist",
			slug, got, want)
	}
}

// historyOnlyAppended enforces append-only on a disposition history.
//
// Every existing event must survive byte-identically and in order; the
// only legal change is new events at the end. A history somebody can
// edit is not a record of what was decided, it is a record of what
// somebody currently says was decided, and those are different
// documents.
func historyOnlyAppended(before, after []parser.BacklogEvent) error {
	if len(after) < len(before) {
		return fmt.Errorf("history is append-only: %d event(s) were removed", len(before)-len(after))
	}
	for i, was := range before {
		if after[i] != was {
			return fmt.Errorf("history is append-only: event %d (%s, recorded %s by %s) was rewritten",
				i, was.Event, was.At, was.By)
		}
	}
	return nil
}
