// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// backlogCLI drives the REAL command with real argv.
//
// The other helper here sets package globals and calls runBacklogEdit
// with a bare cobra.Command, which means it never went through the flag
// layer at all — and that is how a completely unattached command tree
// passed a green suite. Anything whose behaviour depends on whether a
// flag was PASSED (as `edit` now does, so an empty --body can clear a
// body) has to be tested this way or it is not being tested.
func backlogCLI(t *testing.T, cfg *config.Context, argv ...string) (string, error) {
	t.Helper()
	// Accept the invocation exactly as an option emits it — leading
	// "parlay", an optional global "--root <name>", then "backlog" —
	// so a test can hand over what a user would paste rather than a
	// hand-assembled variant that proves nothing about the emitted form.
	argv = append([]string(nil), argv...)
	if len(argv) > 0 && argv[0] == "parlay" {
		argv = argv[1:]
	}
	if len(argv) > 1 && argv[0] == "--root" {
		if got := argv[1]; got != activeRootLabel(cfg) {
			t.Fatalf("the emitted command targets root %q, but this test runs against %q", got, activeRootLabel(cfg))
		}
		argv = argv[2:]
	}
	if len(argv) > 0 && argv[0] == "backlog" {
		argv = argv[1:]
	}
	sub, _, err := backlogCmd.Find(argv)
	if err != nil {
		t.Fatalf("backlog %v is not reachable: %v", argv, err)
	}
	// Reset at the START, and reset the parsed state too — cobra keeps
	// Changed() sticky across Execute calls on the same command object.
	sub.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		}
	})
	backlogBy, backlogReason, backlogInto = "dwht", "", ""
	backlogAbout, backlogEvidence = nil, nil

	var out bytes.Buffer
	sub.SetOut(&out)
	sub.SetErr(&out)
	sub.SetContext(config.WithCtx(context.Background(), cfg))

	// ParseFlags + RunE rather than Execute(): cobra's Execute on a
	// subcommand re-roots to rootCmd, which then parses `go test`'s own
	// argv and runs nothing. The first version of this helper did that
	// and every assertion in it passed against a command that never ran.
	if err := sub.ParseFlags(argv[1:]); err != nil {
		return out.String(), err
	}
	positional := sub.Flags().Args()
	if sub.Args != nil {
		if err := sub.Args(sub, positional); err != nil {
			return out.String(), err
		}
	}
	err = sub.RunE(sub, positional)
	return out.String(), err
}

func capture(t *testing.T, cfg *config.Context, kind, title string, mut func()) string {
	t.Helper()
	id, err := note(t, cfg, kind, title, "claude", mut)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func triage(t *testing.T, cfg *config.Context, run func(*cobra.Command, []string) error, id string, mut func()) error {
	t.Helper()
	// Same discipline as note(): reset every flag at the START, because
	// a test that triages twice would otherwise inherit the first call's
	// --reason or --into.
	backlogBy, backlogReason, backlogInto = "dwht", "", ""
	backlogTitle, backlogBody, backlogPriority = "", "", ""
	backlogClearPri, backlogAll, backlogKind = false, false, ""
	backlogAbout, backlogEvidence = nil, nil
	if mut != nil {
		mut()
	}
	t.Cleanup(func() {
		backlogBy, backlogReason, backlogInto = "", "", ""
		backlogTitle, backlogBody, backlogPriority = "", "", ""
		backlogClearPri, backlogAll, backlogKind = false, false, ""
		backlogAbout, backlogEvidence = nil, nil
	})
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	return run(cmd, []string{id})
}

func itemByID(t *testing.T, cfg *config.Context, id string) *parser.BacklogItem {
	t.Helper()
	it, err := parser.ParseBacklogFile(filepath.Join(parser.BacklogRoot(cfg.Root.Path), id+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return it
}

// A DEFERRAL IS NOT AN ANSWER. The item stays open and keeps being
// reported; attempts accumulate, because two people independently unable
// to decide is a different fact from one attempt overwritten twice.
func TestBacklogDefer_AccumulatesAndDoesNotClose(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something unclear", nil)

	for _, who := range []string{"alice", "bob"} {
		if err := triage(t, cfg, runBacklogDefer, id, func() {
			backlogBy, backlogReason = who, who+" could not tell"
		}); err != nil {
			t.Fatal(err)
		}
	}

	it := itemByID(t, cfg, id)
	if it.State() != parser.StateOpen {
		t.Errorf("deferral must not close the item, got %q", it.State())
	}
	if d := it.Deferrals(); len(d) != 2 || d[0].By != "alice" || d[1].By != "bob" {
		t.Fatalf("both attempts must survive: %+v", d)
	}
}

// declined and obsolete are distinct dispositions, and neither may name
// something the work became — nothing did.
func TestBacklogDecline_AndObsolete_CloseWithoutABecomes(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*cobra.Command, []string) error
		want parser.BacklogEventKind
	}{
		{"decline", runBacklogDecline, parser.EventDeclined},
		{"obsolete", runBacklogObsolete, parser.EventObsolete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := parkFixture(t, "widget")
			id := capture(t, cfg, "idea", "An idea", nil)
			if err := triage(t, cfg, tc.run, id, func() { backlogReason = "because" }); err != nil {
				t.Fatal(err)
			}
			it := itemByID(t, cfg, id)
			if parser.BacklogEventKind(it.State()) != tc.want {
				t.Errorf("want state %q, got %q", tc.want, it.State())
			}
			if it.History[0].Becomes != "" {
				t.Errorf("%s must not name a destination: %q", tc.name, it.History[0].Becomes)
			}
		})
	}
}

func TestBacklogTriage_RequiresReasonAndAttribution(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)

	if err := triage(t, cfg, runBacklogDecline, id, func() { backlogReason = "" }); err == nil {
		t.Error("a disposition with no reason must be refused")
	}
	if err := triage(t, cfg, runBacklogDecline, id, func() { backlogBy, backlogReason = "", "r" }); err == nil {
		t.Error("a disposition with no attribution must be refused")
	}
	// Neither refusal may have written anything.
	if len(itemByID(t, cfg, id).History) != 0 {
		t.Error("a refused disposition was recorded")
	}
}

// A closed item is closed. At most one terminal event, and no event may
// follow it.
func TestBacklogTriage_RefusesASecondDisposition(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)
	if err := triage(t, cfg, runBacklogDecline, id, func() { backlogReason = "no" }); err != nil {
		t.Fatal(err)
	}

	err := triage(t, cfg, runBacklogObsolete, id, func() { backlogReason = "gone" })
	if !errors.Is(err, errBacklogClosed) {
		t.Fatalf("want errBacklogClosed, got %v", err)
	}
	if len(itemByID(t, cfg, id).History) != 1 {
		t.Error("a refused second disposition was appended anyway")
	}
}

// Folding must name somewhere real. A dangling becomes: is a reader
// following a reference to nothing.
func TestBacklogFold_RequiresAResolvableDestination(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	a := capture(t, cfg, "gap", "First", nil)
	b := capture(t, cfg, "gap", "Second", nil)

	if err := triage(t, cfg, runBacklogFold, a, func() { backlogInto = "" }); err == nil {
		t.Error("a fold naming nowhere must be refused")
	}
	if err := triage(t, cfg, runBacklogFold, a, func() { backlogInto = "no-such-item" }); err == nil {
		t.Error("a fold into a nonexistent item must be refused")
	}
	if err := triage(t, cfg, runBacklogFold, a, func() { backlogInto = a }); !errors.Is(err, errBacklogFoldIntoSelf) {
		t.Errorf("folding into itself must be refused, got %v", err)
	}

	if err := triage(t, cfg, runBacklogFold, a, func() { backlogInto = b }); err != nil {
		t.Fatal(err)
	}
	it := itemByID(t, cfg, a)
	if it.History[0].Becomes != b {
		t.Errorf("the fold must record where the work went: %q", it.History[0].Becomes)
	}
}

// edit reaches ONLY the mutable fields. captured and history are not
// expressible from it, which is the guarantee rather than a rule somebody
// has to remember.
func TestBacklogEdit_TouchesOnlyMutableFields(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Original title", nil)
	before := itemByID(t, cfg, id)

	if _, err := backlogCLI(t, cfg, "edit", id, "--title", "Corrected title", "--priority", "P1"); err != nil {
		t.Fatal(err)
	}
	after := itemByID(t, cfg, id)
	if after.Title != "Corrected title" || after.Priority != "P1" {
		t.Errorf("edit did not apply: %+v", after)
	}
	if after.Captured != before.Captured {
		t.Errorf("edit changed the immutable capture block:\n before %+v\n after  %+v", before.Captured, after.Captured)
	}
	if len(after.History) != len(before.History) {
		t.Errorf("edit touched history, which is append-only dispositions and not this")
	}
}

// An empty --body CLEARS the body. Testing the string for emptiness could
// not tell "clear this" apart from "flag not passed", so the one command
// that exists to correct a record silently refused to remove anything
// from it. This is why edit reads Changed() rather than the value.
func TestBacklogEdit_EmptyValueClearsRatherThanBeingIgnored(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Has a body", func() { noteBody = "some observation" })
	if itemByID(t, cfg, id).Body == "" {
		t.Fatal("fixture did not capture a body")
	}

	if _, err := backlogCLI(t, cfg, "edit", id, "--body", ""); err != nil {
		t.Fatal(err)
	}
	if got := itemByID(t, cfg, id).Body; got != "" {
		t.Errorf("--body \"\" must clear the body, got %q", got)
	}
}

// A no-op edit is refused rather than reported as success. `edited <id>`
// on a command that changed nothing is a lie a script would believe.
func TestBacklogEdit_RefusesANoOp(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Untouched", nil)

	_, err := backlogCLI(t, cfg, "edit", id)
	if err == nil {
		t.Fatal("an edit that passes no field must be refused, not reported as an edit")
	}
	if !strings.Contains(err.Error(), "nothing to edit") {
		t.Errorf("the refusal must say what to pass instead: %v", err)
	}
}

// Title is required by the schema, so clearing it is refused HERE with a
// reason, rather than by the post-mutation validator with a shape error
// the caller cannot act on.
func TestBacklogEdit_RefusesToClearTheTitle(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Keeps its title", nil)

	if _, err := backlogCLI(t, cfg, "edit", id, "--title", ""); err == nil {
		t.Fatal("clearing a required field must be refused")
	}
	if got := itemByID(t, cfg, id).Title; got != "Keeps its title" {
		t.Errorf("the refused edit still changed the item: %q", got)
	}
}

// Absent priority is a meaningful state, so a mistaken rank must be
// reversible to it rather than only replaceable by another rank.
func TestBacklogEdit_ClearPriorityReturnsToUntriaged(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", func() { notePriority = "P0" })

	if err := triage(t, cfg, runBacklogEdit, id, func() { backlogClearPri = true }); err != nil {
		t.Fatal(err)
	}
	if p := itemByID(t, cfg, id).Priority; p != "" {
		t.Errorf("want untriaged, got %q", p)
	}
	// And the contradiction is refused rather than silently resolved.
	if err := triage(t, cfg, runBacklogEdit, id, func() {
		backlogClearPri, backlogPriority = true, "P1"
	}); err == nil {
		t.Error("--priority with --clear-priority must be refused")
	}
}

// An ambiguous prefix is refused rather than resolved to the first match:
// a triage command acting on the wrong item would be silent about it.
func TestResolveBacklogItem_AmbiguousPrefixIsRefused(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	capture(t, cfg, "gap", "First", nil)
	capture(t, cfg, "gap", "Second", nil)

	// The shared date prefix matches both.
	_, _, err := resolveBacklogItem(cfg, "2026")
	if !errors.Is(err, errBacklogAmbiguous) {
		t.Fatalf("want errBacklogAmbiguous, got %v", err)
	}
}

// ---------------------------------------------------------------------
// The review.
// ---------------------------------------------------------------------

func reviewBacklog(t *testing.T, cfg *config.Context, exclude ...string) backlogReviewOutput {
	t.Helper()
	prev := nextBacklogExclude
	nextBacklogExclude = exclude
	t.Cleanup(func() { nextBacklogExclude = prev })
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runNextBacklogReview(cmd, nil); err != nil {
		t.Fatalf("review failed: %v", err)
	}
	var out backlogReviewOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output did not parse: %v\n%s", err, buf.String())
	}
	return out
}

// PRIOR DEFERRALS TRAVEL WITH THE ITEM. That is the whole reason a
// deferral is recorded rather than the item simply being skipped: the
// next reviewer starts from what the last one could not resolve.
func TestNextBacklogReview_CarriesPriorDeferrals(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something unclear", nil)
	if err := triage(t, cfg, runBacklogDefer, id, func() {
		backlogBy, backlogReason = "alice", "cannot tell if still reachable"
	}); err != nil {
		t.Fatal(err)
	}

	got := reviewBacklog(t, cfg)
	if got.Subject == nil {
		t.Fatal("a deferred item is still open and must be offered")
	}
	if len(got.Subject.PriorDeferrals) != 1 {
		t.Fatalf("the prior attempt must travel with the item: %+v", got.Subject.PriorDeferrals)
	}
	if !strings.Contains(got.Subject.PriorDeferrals[0], "alice") ||
		!strings.Contains(got.Subject.PriorDeferrals[0], "cannot tell") {
		t.Errorf("the deferral must carry who and why: %q", got.Subject.PriorDeferrals[0])
	}
	if got.Summary.Deferred != 1 {
		t.Errorf("the summary must count deferred items: %+v", got.Summary)
	}
}

// Untriaged first: an unranked item is one nobody has considered.
func TestNextBacklogReview_UntriagedComeFirst(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	capture(t, cfg, "debt", "Ranked already", func() { notePriority = "P0" })
	untriaged := capture(t, cfg, "gap", "Nobody has looked", nil)

	got := reviewBacklog(t, cfg)
	if got.Subject == nil || got.Subject.ID != untriaged {
		t.Fatalf("want the untriaged item first, got %+v", got.Subject)
	}
}

// Every option must be a command the tool accepts.
// The emitted commands must actually RUN, as written.
//
// This used to assert that the option ids were present and then close
// the item by calling runBacklogDecline through package globals — which
// tested a path no user takes and would have passed just as happily
// while the whole command tree was unattached and every emitted string
// was unrunnable. It now parses each command out of the option and
// executes it through the real subcommand.
func TestNextBacklogReview_OptionsAreExecutable(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)
	got := reviewBacklog(t, cfg)

	byID := map[string]string{}
	for _, o := range got.Subject.Options {
		byID[o.ID] = o.Command
	}
	for _, want := range []string{"defer", "decline", "obsolete", "fold", "rank", "fix"} {
		if byID[want] == "" {
			t.Errorf("option %q missing", want)
		}
	}

	// Every option, run verbatim, each against its own item so they do
	// not contend for one.
	destination := capture(t, cfg, "gap", "Fold destination", nil)
	subjects := map[string]string{"defer": id}
	for _, verb := range []string{"decline", "obsolete", "fold", "rank", "fix"} {
		subjects[verb] = capture(t, cfg, "gap", "Subject for "+verb, nil)
	}
	byVerb := map[string]activityReviewOption{}
	for _, o := range got.Subject.Options {
		byVerb[o.ID] = o
	}
	for verb, subject := range subjects {
		argv := reviewArgv(t, byVerb[verb], subject, destination)
		if _, err := backlogCLI(t, cfg, argv...); err != nil {
			t.Errorf("the offered %q command is not runnable as emitted (%v): %v", verb, byVerb[verb].Argv, err)
		}
	}

	// Each verb's emitted command must have had the effect its label
	// promises — running without error is not the same as doing the
	// thing, and only the second is what the reviewer was offered.
	if st := itemByID(t, cfg, subjects["defer"]).State(); st != parser.StateOpen {
		t.Errorf("defer closed the item; a deferral is not an answer (state %q)", st)
	}
	if n := len(itemByID(t, cfg, subjects["defer"]).Deferrals()); n != 1 {
		t.Errorf("defer recorded %d attempts, want 1", n)
	}
	for verb, want := range map[string]parser.BacklogState{
		"decline":  parser.BacklogState(parser.EventDeclined),
		"obsolete": parser.BacklogState(parser.EventObsolete),
		"fold":     parser.BacklogState(parser.EventFolded),
		"fix":      parser.BacklogState(parser.EventFixed),
	} {
		if st := itemByID(t, cfg, subjects[verb]).State(); st != want {
			t.Errorf("the emitted %q command left the item %q, want %q", verb, st, want)
		}
	}
	// rank is explicitly NOT a disposition: it sets a priority and
	// leaves the item open for somebody to actually decide.
	ranked := itemByID(t, cfg, subjects["rank"])
	if ranked.State() != parser.StateOpen {
		t.Errorf("rank closed the item; it is not a disposition (state %q)", ranked.State())
	}
	if ranked.Priority == "" {
		t.Error("the emitted rank command set no priority")
	}
}

// reviewArgv takes the STRUCTURED argv off the option and fills the
// placeholders, rather than splitting the pasteable Command on
// whitespace.
//
// The previous version did split Command, and it could not parse the
// options whose correctness matters most: a child item emits
// `parlay --root 'child' backlog ...`, so the check that fields[1] was
// "backlog" rejected it, and strings.Fields would have left the quotes
// on the root name anyway. That is why no test ran a child command.
func reviewArgv(t *testing.T, opt activityReviewOption, subject, foldInto string) []string {
	t.Helper()
	if len(opt.Argv) == 0 {
		t.Fatalf("option %q emits no argv; a caller would have to re-split the shell text", opt.ID)
	}
	if opt.Argv[0] != "parlay" {
		t.Fatalf("option %q argv does not start with the binary: %v", opt.ID, opt.Argv)
	}
	out := make([]string, 0, len(opt.Argv))
	for _, a := range opt.Argv {
		// The placeholder substitution and the subject substitution are
		// exclusive. They were not, and the filled <other-id> — an id,
		// so it matched the id test — was immediately overwritten with
		// the subject, producing a fold into itself.
		switch {
		case a == "<why>":
			a = "cannot say — needs the owner"
		case a == "<who>":
			a = "dwht"
		case a == "<other-id>":
			a = foldInto
		case strings.HasPrefix(a, "2026"):
			a = subject
		}
		out = append(out, a)
	}
	for _, a := range out {
		if strings.ContainsAny(a, "<>") {
			t.Fatalf("option %q left an unfilled placeholder: %v", opt.ID, out)
		}
	}
	return out
}

// An unreadable item is named, never skipped — and never covered by
// "nothing left to review".
func TestNextBacklogReview_UnreadableItemsAreReported(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	dir := parser.BacklogRoot(cfg.Root.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("schema_version: 1\nknid: gap\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := reviewBacklog(t, cfg)
	if len(got.Unreadable) == 0 {
		t.Fatal("an unreadable item must be reported")
	}
	if strings.Contains(got.Note, "Nothing left to review") {
		t.Errorf("the note must not claim completeness while an item is unreadable: %q", got.Note)
	}
}

// The review writes nothing. The decision is made by the triage verbs,
// which carry the attribution.
func TestNextBacklogReview_WritesNothing(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)
	before := itemByID(t, cfg, id)
	_ = reviewBacklog(t, cfg)
	after := itemByID(t, cfg, id)
	if len(before.History) != len(after.History) {
		t.Error("the review recorded a disposition")
	}
}

// THE RECIPROCAL FOLD RACE.
//
// A fold touches TWO items and the per-item lock covers one, so A→B and
// B→A take different locks: without a shared scope both read the other as
// open and both publish, leaving a two-node cycle in which neither item
// survives and each names the other as where the work went. A sequential
// test proves only the easy half — this drives both directions at once
// and requires exactly one winner and exactly one edge.
func TestBacklogFold_ReciprocalConcurrentFoldsLeaveOneEdge(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	a := capture(t, cfg, "gap", "First", nil)
	b := capture(t, cfg, "gap", "Second", nil)

	// The command layer cannot be driven concurrently in-process (cobra
	// flags are package vars), so this drives the same guarded path the
	// command does, with per-goroutine arguments.
	fold := func(src, dst string) error {
		return withBacklogFoldLock(cfg, func() error {
			target, _, err := resolveBacklogItem(cfg, dst)
			if err != nil {
				return err
			}
			if target.State() != parser.StateOpen {
				return errBacklogFoldClosedTarget
			}
			_, path, err := resolveBacklogItem(cfg, src)
			if err != nil {
				return err
			}
			return mutateBacklogItem(cfg, path, func(item *parser.BacklogItem) error {
				if item.State() != parser.StateOpen {
					return errBacklogClosed
				}
				item.History = append(item.History, parser.BacklogEvent{
					Event: parser.EventFolded, Becomes: target.ID,
					At: "2026-09-01T12:00:00Z", By: "dwht",
				})
				return nil
			})
		})
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		wg.Add(1)
		go func(src, dst string) {
			defer wg.Done()
			results <- fold(src, dst)
		}(pair[0], pair[1])
	}
	wg.Wait()
	close(results)

	var won, refused int
	for err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, errBacklogFoldClosedTarget), errors.Is(err, errBacklogClosed):
			refused++
		default:
			t.Fatalf("unexpected failure: %v", err)
		}
	}
	if won != 1 || refused != 1 {
		t.Fatalf("want exactly one winner and one refusal, got %d/%d", won, refused)
	}

	// Exactly one edge, and the item it points at must still be open —
	// otherwise the work was recorded as moving somewhere that is gone.
	edges := 0
	open := 0
	for _, id := range []string{a, b} {
		it := itemByID(t, cfg, id)
		if it.State() == parser.StateOpen {
			open++
			continue
		}
		edges++
		if it.History[len(it.History)-1].Becomes == "" {
			t.Error("a fold recorded no destination")
		}
	}
	if edges != 1 || open != 1 {
		t.Fatalf("want one folded and one surviving item, got %d folded / %d open", edges, open)
	}
}

// Folding into a closed item is refused sequentially too — the work would
// be recorded as moving somewhere that had already stopped.
func TestBacklogFold_RefusesAClosedDestination(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	a := capture(t, cfg, "gap", "First", nil)
	b := capture(t, cfg, "gap", "Second", nil)
	if err := triage(t, cfg, runBacklogDecline, b, func() { backlogReason = "no" }); err != nil {
		t.Fatal(err)
	}

	err := triage(t, cfg, runBacklogFold, a, func() { backlogInto = b })
	if !errors.Is(err, errBacklogFoldClosedTarget) {
		t.Fatalf("want errBacklogFoldClosedTarget, got %v", err)
	}
	if len(itemByID(t, cfg, a).History) != 0 {
		t.Error("a refused fold was recorded anyway")
	}
}

// The scoped read must not silently miss a ref category.
//
// This used to match four kind prefixes by substring, so a fifth added to
// the schema would have been unmatched with nothing reporting it. It now
// routes through the canonical parser; this pins every allowed kind plus
// the bare and initiative-qualified forms.
func TestSameBacklogFeature_HandlesEveryRefForm(t *testing.T) {
	cases := []struct {
		ref, feature string
		want         bool
	}{
		// Every kind the canonical parser admits.
		{"@widget/operation:rename", "@widget", true},
		{"@widget/surface:some-fragment", "@widget", true},
		{"@widget/infrastructure:some-concept", "@widget", true},
		{"@widget/domain:some-entity", "@widget", true},
		// Bare, both sides.
		{"@widget", "@widget", true},
		{"widget", "@widget", true},
		{"@widget", "widget", true},
		// Initiative-qualified, which contains a slash of its own — the
		// case a naive split on "/" gets wrong.
		{"@auth/reset-password", "@auth/reset-password", true},
		{"@auth/reset-password/operation:send", "@auth/reset-password", true},
		// Non-matches.
		{"@widget", "@gadget", false},
		{"@auth/reset-password", "@auth/change-password", false},
		{"", "@widget", false},
		{"@widget", "", false},
	}
	for _, tc := range cases {
		if got := sameBacklogFeature(tc.ref, tc.feature); got != tc.want {
			t.Errorf("sameBacklogFeature(%q, %q) = %v, want %v", tc.ref, tc.feature, got, tc.want)
		}
	}
}

// The subtree must actually be reachable from the root command.
//
// It was not: every subcommand was defined, none was attached, and
// `parlay backlog list` did not exist at runtime. The tests all called
// the runBacklog* functions directly, so a completely unrunnable command
// tree passed a green suite. This asserts through cobra's own lookup,
// which is the thing a user's shell goes through.
func TestBacklogCommandTreeIsReachable(t *testing.T) {
	want := map[string][]string{
		"list":     {"all", "open", "untriaged", "kind", "priority", "about", "related", "json"},
		"show":     {"json"},
		"edit":     {"title", "body", "priority", "clear-priority", "about", "evidence"},
		"defer":    {"reason", "by"},
		"decline":  {"reason", "by"},
		"obsolete": {"reason", "by"},
		"fold":     {"into", "by"},
		"amend":    {"into", "by"},
		"fix":      {"reason", "by"},
	}
	for verb, flags := range want {
		sub, _, err := backlogCmd.Find([]string{verb})
		if err != nil || sub == nil || sub.Name() != verb {
			t.Errorf("`parlay backlog %s` is not reachable: %v", verb, err)
			continue
		}
		if sub.RunE == nil {
			t.Errorf("`parlay backlog %s` has no RunE", verb)
		}
		for _, fl := range flags {
			if sub.Flags().Lookup(fl) == nil {
				t.Errorf("`parlay backlog %s` has no --%s", verb, fl)
			}
		}
	}
	// And the parent must hang off the root, or none of the above is
	// reachable from a shell either.
	if sub, _, err := rootCmd.Find([]string{"backlog", "list"}); err != nil || sub.Name() != "list" {
		t.Errorf("`parlay backlog list` not reachable from root: %v", err)
	}
	if sub, _, err := rootCmd.Find([]string{"note"}); err != nil || sub.Name() != "note" {
		t.Errorf("`parlay note` not reachable from root: %v", err)
	}

	// fold must NOT take --reason: the schema forbids a reason on a
	// folded item, and offering the flag would invite a value the
	// validator then refuses.
	fold, _, _ := backlogCmd.Find([]string{"fold"})
	if fold.Flags().Lookup("reason") != nil {
		t.Error("`backlog fold` offers --reason, which the schema forbids on a folded item")
	}
	// amend must NOT take --reason: the amendment it names is the
	// reason, and the schema forbids a reason on an `amended` item.
	amend, _, _ := backlogCmd.Find([]string{"amend"})
	if amend.Flags().Lookup("reason") != nil {
		t.Error("`backlog amend` offers --reason, which the schema forbids on an amended item")
	}
	// fix must NOT take --into: nothing became of a fix, and `becomes:`
	// is a typed lifecycle edge rather than a place to put evidence.
	fix, _, _ := backlogCmd.Find([]string{"fix"})
	if fix.Flags().Lookup("into") != nil {
		t.Error("`backlog fix` offers --into; the schema forbids becomes: on a fixed item")
	}
	// edit must NOT take --by: it has nowhere to record one.
	edit, _, _ := backlogCmd.Find([]string{"edit"})
	if edit.Flags().Lookup("by") != nil {
		t.Error("`backlog edit` still demands --by and has nowhere to record it")
	}
}

// list is the SINGLE inventory: items and parked features together, in
// every root. Two commands for "what are we not doing?" is a question a
// reader has to remember the answer to, and the parked-feature half was
// the part with no listing at all.
func TestBacklogList_IsOneInventoryAcrossRoots(t *testing.T) {
	cfg, parent := multiRootBacklogFixture(t)

	inv := collectBacklogInventory(cfg, backlogListFilters{openOnly: true})

	if len(inv.Roots) < 2 {
		t.Fatalf("the walk did not reach the child root: %v", inv.Roots)
	}
	var roots []string
	for _, it := range inv.Items {
		roots = append(roots, it.Root)
	}
	if !containsRoot(roots, "child") {
		t.Errorf("a child root's items are missing from the inventory: %v", roots)
	}
	if !containsRoot(roots, activeRootLabel(cfg)) {
		t.Errorf("the active root's items are missing from the inventory: %v", roots)
	}
	if len(inv.Parked) == 0 {
		t.Error("parked features are missing: the inventory is still only half the answer")
	}
	for _, p := range inv.Parked {
		if p.Root == "" {
			t.Error("a parked feature is unattributed to a root; two roots may hold the same slug")
		}
	}
	_ = parent
}

// A root that cannot be enumerated is REPORTED, never dropped. A shorter
// listing that says nothing about why is indistinguishable from a
// project with less outstanding work.
func TestBacklogList_NamesRootsItCouldNotRead(t *testing.T) {
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	if err := os.MkdirAll(filepath.Join(parent, config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Registered, but the path does not exist — the same shape
	// next-activity-review uses, because it is the failure that actually
	// happens: a child moved or not checked out.
	cfg := parentWithChild(t, parent, "ghost", filepath.Join(parent, "no-such-child"))

	inv := collectBacklogInventory(cfg, backlogListFilters{openOnly: true})

	if !containsRoot(inv.Roots, "ghost") {
		t.Errorf("the unreachable child vanished from roots_examined: %v", inv.Roots)
	}
	if len(inv.RootErrors) == 0 {
		t.Fatal("a root that could not be enumerated must be named, not silently omitted")
	}
	var out bytes.Buffer
	writeBacklogInventory(&out, inv)
	got := out.String()
	if !strings.Contains(got, "could not be enumerated") || !strings.Contains(got, "ghost") {
		t.Errorf("the human listing hides the root error:\n%s", got)
	}
	// And it must not read as an empty, healthy backlog.
	if strings.Contains(got, "Nothing outstanding") {
		t.Error("a listing that could not read a root reported the project as clear")
	}
}

func rootsOf(items []backlogListEntry) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Root)
	}
	return out
}

func containsRoot(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// multiRootBacklogFixture builds a parent with one registered child,
// each holding a backlog item and a parked feature. Two roots, because
// the failure this guards against — a listing that reports only the
// active root — is invisible in a single-root project.
func multiRootBacklogFixture(t *testing.T) (*config.Context, string) {
	t.Helper()
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	childPath := filepath.Join(parent, "child")

	for _, root := range []string{parent, childPath} {
		if err := os.MkdirAll(filepath.Join(root, config.SpecDir, config.IntentsDir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := parentWithChild(t, parent, "child", childPath)

	seed := func(root, slug string) {
		ctx := rootCtxAt(root)
		if err := runAddFeature(testCommandWithContext(t, ctx), []string{slug}); err != nil {
			t.Fatal(err)
		}
		if err := park(t, ctx, "@"+slug, "waiting on the upstream decision", "", "dwht"); err != nil {
			t.Fatal(err)
		}
		if _, err := note(t, ctx, "gap", "found in "+slug, "claude", nil); err != nil {
			t.Fatal(err)
		}
	}
	seed(parent, "widget")
	seed(childPath, "gadget")

	return cfg, parent
}

// counts describe THIS listing; project_totals describe the project.
//
// They used to be one field, accumulated before filtering while the rows
// were filtered — so `--related @widget --json` returned one item beside
// a count for the whole project. A consumer reading both as one answer
// got a number describing a different question, which is exactly the
// trap the designer scoped read would have walked into.
func TestBacklogList_FilteredCountsDescribeTheListingNotTheProject(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	capture(t, cfg, "gap", "About widget", func() { noteAbout = []string{"@widget"} })
	capture(t, cfg, "defect", "About something else", func() { noteAbout = []string{"@gadget"} })
	capture(t, cfg, "debt", "About nothing in particular", nil)

	scoped := backlogListFilters{openOnly: true, related: "@widget"}
	inv := collectBacklogInventory(cfg, scoped)
	inv.Filtered = scoped.isNarrowing()

	if len(inv.Items) != 1 {
		t.Fatalf("the scoped read returned %d items, want 1: %+v", len(inv.Items), inv.Items)
	}
	if inv.Counts.Items != 1 {
		t.Errorf("counts must describe the listing: got %d, want 1", inv.Counts.Items)
	}
	if inv.ProjectTotals.Items != 3 {
		t.Errorf("project_totals must describe the project: got %d, want 3", inv.ProjectTotals.Items)
	}
	if !inv.Filtered {
		t.Error("a narrowing read must declare itself filtered rather than making a consumer compare the two counts")
	}

	// And the whole-project read must have the two agreeing, or the
	// distinction is noise.
	all := collectBacklogInventory(cfg, backlogListFilters{})
	if all.Counts.Items != all.ProjectTotals.Items {
		t.Errorf("unfiltered, the two counts must agree: %d vs %d", all.Counts.Items, all.ProjectTotals.Items)
	}
}

// The HUMAN listing must carry the root, not just the JSON.
//
// Two roots may hold the same feature slug and the same id prefix. A row
// without its root leaves the reader with no way to know which --root to
// pass, and the command they then run silently targets the wrong project.
func TestBacklogList_HumanListingCarriesTheRoot(t *testing.T) {
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	childPath := filepath.Join(parent, "child")
	for _, root := range []string{parent, childPath} {
		if err := os.MkdirAll(filepath.Join(root, config.SpecDir, config.IntentsDir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := parentWithChild(t, parent, "child", childPath)

	// THE SAME SLUG in both roots — the case a bare row cannot express.
	for _, root := range []string{parent, childPath} {
		ctx := rootCtxAt(root)
		if err := runAddFeature(testCommandWithContext(t, ctx), []string{"widget"}); err != nil {
			t.Fatal(err)
		}
		if err := park(t, ctx, "@widget", "waiting on the upstream decision", "", "dwht"); err != nil {
			t.Fatal(err)
		}
		if _, err := note(t, ctx, "gap", "ambiguous without a root", "claude", nil); err != nil {
			t.Fatal(err)
		}
	}

	inv := collectBacklogInventory(cfg, backlogListFilters{openOnly: true})
	var out bytes.Buffer
	writeBacklogInventory(&out, inv)
	got := out.String()

	if !strings.Contains(got, "child") {
		t.Errorf("the child root is not named anywhere in the human listing:\n%s", got)
	}
	// Both parked features have the slug "widget"; the listing must
	// distinguish them, which it can only do by root.
	parkedRows := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "widget") && strings.Contains(line, "waiting on the upstream") {
			parkedRows++
			if !strings.Contains(line, "child") && !strings.Contains(line, activeRootLabel(cfg)) {
				t.Errorf("a parked-feature row carries no root: %q", line)
			}
		}
	}
	if parkedRows != 2 {
		t.Errorf("want both roots' parked widgets listed, got %d rows:\n%s", parkedRows, got)
	}
}
