// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: backlog-item

package commands

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/parser"
)

// next-backlog-review hands a person ONE item to decide about.
//
// A backlog that only grows is a graveyard, and a graveyard is worse than
// nothing because the absence of tracking looks like tracking. The shape
// is lifted from next-legacy-review for the reason that command exists:
// a wall of items converts into decisions at roughly the rate of zero,
// while one question with its evidence attached converts at the rate
// people actually answer questions.
//
// READ-ONLY. It selects and presents; the decision is made by the triage
// verbs, which carry the attribution.
var nextBacklogReviewCmd = &cobra.Command{
	Use:   "next-backlog-review",
	Short: "Emit the next backlog item for a person to decide",
	Long: `Select one open item and emit what is needed to decide it.

Nothing is written. The answer is given with ` + "`parlay backlog defer`" + `,
` + "`decline`" + `, ` + "`obsolete`" + ` or ` + "`fold`" + `, which record who
decided and why.

PRIOR DEFERRALS TRAVEL WITH THE ITEM. Somebody already looked at this and could
not say; the next reviewer starts from that rather than from nothing, which is
the whole reason a deferral is recorded rather than the item simply being
skipped.

Order: untriaged first, because an unranked item is one nobody has considered,
and that is the work triage exists to do. Pass --exclude for each item handled
in this sitting; nothing is persisted, so without exclusions a run would hand
back an item somebody has just looked at and chosen not to decide.`,
	Args: cobra.NoArgs,
	RunE: runNextBacklogReview,
}

var nextBacklogExclude []string

func init() {
	nextBacklogReviewCmd.Flags().StringArrayVar(&nextBacklogExclude, "exclude", nil,
		"item already handled this sitting (repeatable)")
}

type backlogReviewSubject struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Priority string   `json:"priority,omitempty"`
	Title    string   `json:"title"`
	Body     string   `json:"body,omitempty"`
	About    []string `json:"about,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
	Captured string   `json:"captured"`
	// Root is which root holds this item, and Exclude is the exact token
	// to pass back. Root-qualified, because two roots may hold items and
	// an unqualified id is only unique by construction rather than by
	// guarantee.
	Root    string `json:"root"`
	Exclude string `json:"exclude"`
	// PriorDeferrals is why this is not just "the next item": somebody
	// may already have looked and failed to decide, and starting the next
	// reviewer from nothing wastes that.
	PriorDeferrals []string               `json:"prior_deferrals,omitempty"`
	Options        []activityReviewOption `json:"options"`
}

type backlogReviewOutput struct {
	Root string `json:"root"`
	// RootsExamined names every root walked, so a reader can tell an
	// empty review from one that never looked at the children. Same
	// discipline as next-activity-review, reusing its walk rather than
	// inventing a second pattern for the same question.
	RootsExamined []string `json:"roots_examined"`
	// RootErrors names roots that could not be enumerated. "No open
	// backlog items" while a child holds unreviewed observations is
	// false, and the falsehood is invisible without this.
	RootErrors []string              `json:"root_errors,omitempty"`
	Summary    backlogReviewSummary  `json:"summary"`
	Subject    *backlogReviewSubject `json:"subject,omitempty"`
	// Unreadable BACKLOG ITEMS are named rather than skipped: an item
	// nobody can read still needs a person, and a review that omitted it
	// would report a smaller backlog than the project has.
	//
	// Backlog items only — unlike the listing's field of the same name,
	// which also carries unusable activity declarations. This command
	// loads no activity, so the narrower meaning is accurate here; if it
	// ever does, this field is renamed rather than quietly widened.
	Unreadable []string `json:"unreadable,omitempty"`
	// Findings are cross-file problems on the items this sitting covers
	// — a `becomes:` that stopped resolving, an item open past the age
	// bucket. Carried here because a reviewer deciding about an item
	// should see that its recorded outcome no longer exists.
	Findings []backlogFinding `json:"findings,omitempty"`
	Note     string           `json:"note"`
}

type backlogReviewSummary struct {
	Total     int `json:"total"`
	Open      int `json:"open"`
	Untriaged int `json:"untriaged"`
	Deferred  int `json:"deferred"`
	Closed    int `json:"closed"`
	Remaining int `json:"remaining"`
}

func runNextBacklogReview(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	excluded := map[string]bool{}
	for _, e := range nextBacklogExclude {
		excluded[strings.TrimSpace(e)] = true
	}

	out := backlogReviewOutput{Root: cfg.Root.Path}
	var pending []*backlogReviewCandidate

	for _, target := range activityReviewRoots(cfg) {
		// Appended BEFORE the error check: a root that failed must still
		// appear as examined, or a reader cannot tell a child that was
		// skipped from one that does not exist.
		out.RootsExamined = append(out.RootsExamined, target.name)
		if target.err != nil {
			out.RootErrors = append(out.RootErrors, fmt.Sprintf("%s: %v", target.name, target.err))
			continue
		}

		items, broken, err := loadBacklogAt(target.ctx.Root.Path)
		if err != nil {
			out.RootErrors = append(out.RootErrors, fmt.Sprintf("%s: %v", target.name, err))
			continue
		}
		for _, b := range broken {
			out.Unreadable = append(out.Unreadable, target.name+"/"+b)
		}
		out.Findings = append(out.Findings, crossFileFindings(target.ctx, target.name, items, broken, time.Now().UTC())...)

		for _, it := range items {
			out.Summary.Total++
			if it.State() != parser.StateOpen {
				out.Summary.Closed++
				continue
			}
			out.Summary.Open++
			if it.Priority == "" {
				out.Summary.Untriaged++
			}
			if len(it.Deferrals()) > 0 {
				out.Summary.Deferred++
			}
			out.Summary.Remaining++

			token := backlogExcludeToken(target.name, it.ID)
			if excluded[token] || excluded[it.ID] || excluded[shortID(it.ID)] {
				continue
			}
			pending = append(pending, &backlogReviewCandidate{
				item: it, root: target.name, exclude: token,
				rootName: target.rootName,
			})
		}
	}

	// Untriaged first across ALL roots, not per root: an unranked item in
	// a child is no less unconsidered than one in the parent, and
	// draining the parent before starting the child would be an ordering
	// nobody asked for.
	sort.SliceStable(pending, func(i, j int) bool {
		pi, pj := pending[i].item.Priority, pending[j].item.Priority
		if (pi == "") != (pj == "") {
			return pi == ""
		}
		return pending[i].item.ID < pending[j].item.ID
	})

	if len(pending) > 0 {
		out.Subject = describeBacklogSubject(pending[0])
		out.Note = "Nothing has been written. Answer with one of the commands in options, then re-run with --exclude " + pending[0].exclude + " to move on."
	} else if out.Summary.Remaining > 0 {
		out.Note = "Every remaining item was excluded this sitting. Re-run without --exclude to revisit them."
	} else {
		out.Note = "No open backlog items. Nothing left to review."
	}
	// A root that could not be enumerated OUTRANKS the clean note. It
	// was populating RootErrors and then still saying "Nothing left to
	// review", which is the exact falsehood the field exists to prevent:
	// a child that failed might hold anything, and doctor invokes this
	// from the parent.
	if len(out.RootErrors) > 0 && out.Subject == nil {
		out.Note = fmt.Sprintf("%d root(s) could not be enumerated — see root_errors. Their items have NOT been reviewed, so this is not 'nothing left'.", len(out.RootErrors))
	}
	if len(out.Unreadable) > 0 && out.Subject == nil {
		// Never "nothing left" while an item could not be read.
		out.Note = fmt.Sprintf("No decidable items, but %d could not be read — see unreadable. They have not been reviewed.", len(out.Unreadable))
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// backlogReviewCandidate carries the item with the root it came from.
// Without the root, an emitted command would run against whichever root
// the shell happens to resolve — which is the active one, not
// necessarily the one holding the item.
type backlogReviewCandidate struct {
	item    *parser.BacklogItem
	root    string
	exclude string
	// rootName is empty for the active root and the bare name for a
	// child. Both the pasteable Command and the structured Argv are
	// built from it, so they cannot disagree about which root they
	// target.
	rootName string
}

// backlogExcludeToken is the exact --exclude value for one subject.
// Root-qualified for the same reason activityExcludeToken is: two roots
// may hold items, and an unqualified exclusion could skip the wrong one.
func backlogExcludeToken(root, id string) string {
	return root + ":" + id
}

func describeBacklogSubject(c *backlogReviewCandidate) *backlogReviewSubject {
	it := c.item
	s := &backlogReviewSubject{
		Root:    c.root,
		Exclude: c.exclude,
		ID:      it.ID, Kind: string(it.Kind), Priority: it.Priority,
		Title: it.Title, Body: it.Body, About: it.About,
		Captured: fmt.Sprintf("%s by %s", it.Captured.At, it.Captured.By),
	}
	if it.Captured.Feature != "" {
		s.Captured += " while working on " + it.Captured.Feature
		if it.Captured.Phase != "" {
			s.Captured += " (" + it.Captured.Phase + ")"
		}
	}
	for _, e := range it.Evidence {
		if e.Line > 0 {
			s.Evidence = append(s.Evidence, fmt.Sprintf("%s:%d", e.Path, e.Line))
			continue
		}
		s.Evidence = append(s.Evidence, e.Path)
	}
	for _, d := range it.Deferrals() {
		s.PriorDeferrals = append(s.PriorDeferrals, fmt.Sprintf("%s (%s): %s", d.By, d.At, d.Reason))
	}
	s.Options = backlogReviewOptions(c)
	return s
}

// backlogReviewOptions emits only commands the tool will accept.
//
// No "skip": skipping is --exclude, which is the caller's business and
// leaves no record. No default either — a default would let an unattended
// run close somebody's observation, which is precisely the judgment this
// command exists to route to a person.
func backlogReviewOptions(c *backlogReviewCandidate) []activityReviewOption {
	it := c.item
	// The id is emitted in full, and --root is carried, because these
	// commands are meant to be run as written. A short id plus no root
	// is a command that resolves against the wrong project as readily as
	// the right one.
	// The id is emitted IN FULL and --root is carried, because these
	// commands are meant to be run as written. A short id with no root
	// resolves against whichever project the shell happens to be in,
	// which is as likely to be the wrong one as the right one.
	// Argv is the STRUCTURED form, Command the pasteable one. Both,
	// because round-tripping shell text back into arguments is exactly
	// what a caller must not have to do: a root name or a reason
	// carrying a space or a quote survives Argv and does not survive
	// splitting Command on whitespace.
	build := func(verb string, tail ...string) ([]string, string) {
		argv := []string{"parlay"}
		human := "parlay "
		// ONE value behind both forms, same as activity's builder.
		if c.rootName != "" {
			argv = append(argv, "--root", c.rootName)
			human += "--root " + shellQuote(c.rootName) + " "
		}
		argv = append(argv, "backlog", verb, it.ID)
		argv = append(argv, tail...)
		human += "backlog " + verb + " " + it.ID + " " + strings.Join(quoteAll(tail), " ")
		return argv, strings.TrimSpace(human)
	}
	why := []string{"--reason", "<why>", "--by", "<who>"}

	deferArgv, deferCmd := build("defer", why...)
	declineArgv, declineCmd := build("decline", why...)
	fixArgv, fixCmd := build("fix", why...)
	obsoleteArgv, obsoleteCmd := build("obsolete", why...)
	foldArgv, foldCmd := build("fold", "--into", "<other-id>", "--by", "<who>")

	opts := []activityReviewOption{
		{ID: "defer", Label: "Looked, cannot decide — record the attempt", Command: deferCmd, Argv: deferArgv},
		{ID: "decline", Label: "Deliberately not doing this", Command: declineCmd, Argv: declineArgv},
		{ID: "fix", Label: "The work was done directly — no feature, no amendment", Command: fixCmd, Argv: fixArgv},
		{ID: "obsolete", Label: "The condition that produced it is gone", Command: obsoleteCmd, Argv: obsoleteArgv},
		{ID: "fold", Label: "Absorb into another item", Command: foldCmd, Argv: foldArgv},
	}
	// Ranking is offered only where it is missing. Offering it on a
	// ranked item would suggest re-ranking is part of triage, which it is
	// not — that is an edit, and it has its own command.
	if it.Priority == "" {
		rankArgv, rankCmd := build("edit", "--priority", "P1")
		opts = append(opts, activityReviewOption{
			ID: "rank", Label: "Not a disposition — rank it and leave it open",
			Command: rankCmd, Argv: rankArgv})
	}
	return opts
}

// quoteAll makes the pasteable Command safe without mangling it.
//
// A PLACEHOLDER keeps double quotes — `--reason "<why>"` — because that
// is the quoting hint the person needs: they replace the placeholder
// with prose, and an unquoted multi-word reason becomes several
// arguments. Running shellQuote over it instead produced `'<why>'`,
// which is correct shell and the wrong hint, and it silently changed
// text that was already deployed.
//
// Everything else goes through shellQuote. Argv, not this, is the form a
// caller should actually use.
func quoteAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "<") && strings.HasSuffix(a, ">") {
			out[i] = `"` + a + `"`
			continue
		}
		out[i] = shellQuote(a)
	}
	return out
}
