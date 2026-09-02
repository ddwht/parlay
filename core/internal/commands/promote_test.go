// parlay-feature: parlay-tool/backlog-and-activity

package commands

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

func promote(t *testing.T, cfg *config.Context, argv ...string) (string, error) {
	t.Helper()
	promoteAsFeature, promoteAsAmendment, promoteInitiative, promoteBy = "", "", "", "dwht"
	initiativeFlag, authoredFlag = "", false
	for i := 0; i+1 < len(argv); i += 2 {
		switch argv[i] {
		case "--as-feature":
			promoteAsFeature = argv[i+1]
		case "--as-amendment":
			promoteAsAmendment = argv[i+1]
		case "--initiative":
			promoteInitiative = argv[i+1]
		case "--by":
			promoteBy = argv[i+1]
		default:
			t.Fatalf("unknown flag %q", argv[i])
		}
	}
	t.Cleanup(func() {
		promoteAsFeature, promoteAsAmendment, promoteInitiative, promoteBy = "", "", "", ""
	})
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	// Run FIRST, then read. `return out.String(), runPromote(...)`
	// evaluates the operands left to right, so it read an empty buffer
	// before the command had written anything to it.
	err := runPromote(cmd, []string{argv[len(argv)-1]})
	return out.String(), err
}

// The scaffold seeds NO Goal, and the observation lands non-parsing.
//
// A promoted feature that parsed as having an intent would report itself
// further along than it is, and `planned` exists precisely to tell those
// apart. An implementation observation is usually not a user-world
// outcome and has no Persona, so seeding one manufactures exactly the
// malformed intent the scaffold warns against.
func TestPromote_AsFeatureSeedsNoGoalAndStaysPlanned(t *testing.T) {
	cfg, _ := parkFixture(t, "existing")
	id := capture(t, cfg, "defect", "Rename collides with orphan", func() {
		noteBody = "Two features can claim the same build dir."
		notePriority = "P0"
		noteEvidence = []string{"core/x.go:42"}
	})

	out, err := promote(t, cfg, "--as-feature", "Rename Collision", id)
	if err != nil {
		t.Fatal(err)
	}

	featurePath := cfg.FeaturePath("rename-collision")
	intents, err := os.ReadFile(filepath.Join(featurePath, "intents.md"))
	if err != nil {
		t.Fatalf("the scaffold did not land: %v", err)
	}
	body := string(intents)

	// The origin is present and NON-PARSING: inside an HTML comment, so
	// the intents parser cannot see it as a promise.
	if !strings.Contains(body, "backlog-origin: "+id) {
		t.Error("the feature does not record where it came from")
	}
	i := strings.Index(body, "backlog-origin")
	openC, closeC := strings.LastIndex(body[:i], "<!--"), strings.Index(body[i:], "-->")
	if openC < 0 || closeC < 0 {
		t.Error("the backlog-origin link is not inside a comment, so it parses as content")
	}
	if !strings.Contains(body, "Rename collides with orphan") || !strings.Contains(body, "core/x.go:42") {
		t.Error("the observation and its evidence did not travel with the promotion")
	}

	// The feature must still read as `planned` — no intent was seeded.
	if phase := ComputeFeaturePhase(cfg, "rename-collision"); phase != PhasePlanned {
		t.Errorf("the promoted feature is %q, want %q — a seeded Goal would move it", phase, PhasePlanned)
	}

	// The priority travels as a PROPOSAL, never as a decided rank.
	if !strings.Contains(out, "PROPOSED priority P0") {
		t.Errorf("the rank was not marked as a proposal for the intents phase:\n%s", out)
	}
	if !strings.Contains(body, "Proposed priority: P0") {
		t.Error("the scaffold does not say the rank is a proposal")
	}

	// The item is RETAINED as provenance, closed with what it became.
	item := itemByID(t, cfg, id)
	if item.State() != parser.BacklogState(parser.EventPromoted) {
		t.Errorf("the item is %q, want promoted", item.State())
	}
	last := item.History[len(item.History)-1]
	if last.Becomes != "@rename-collision" {
		t.Errorf("the promotion records becomes=%q", last.Becomes)
	}
	if last.By != "dwht" {
		t.Errorf("the promotion is attributed to %q", last.By)
	}
}

// --as-amendment writes NOTHING. An amendment is authored with a person
// in the loop, so a command that wrote one alone would be recording a
// decision nobody made. What it does is close the causal gap the
// amendment schema already describes.
func TestPromote_AsAmendmentWritesNothingAndEmitsTheTrigger(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Filter belongs above the table", nil)
	before := dirSnapshot(t, featurePath)

	out, err := promote(t, cfg, "--as-amendment", "@widget", id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "trigger: backlog:"+id) {
		t.Errorf("the pre-filled trigger is missing — that is the whole point:\n%s", out)
	}
	if !strings.Contains(out, "/parlay-refine") {
		t.Errorf("the output does not route to where the amendment is authored:\n%s", out)
	}
	if after := dirSnapshot(t, featurePath); after != before {
		t.Errorf("--as-amendment wrote to the feature:\n before %v\n after  %v", before, after)
	}
	// The item stays OPEN until the amendment lands, so `becomes:` can
	// name a real amendment rather than an intention.
	if st := itemByID(t, cfg, id).State(); st != parser.StateOpen {
		t.Errorf("the item is %q; it must stay open until the amendment exists", st)
	}
}

// An amendment amends a promise that EXISTS. If the feature does not,
// the act is --as-feature, and saying so beats scaffolding one silently.
func TestPromote_AsAmendmentRefusesAnAbsentFeature(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)

	out, err := promote(t, cfg, "--as-amendment", "@no-such-feature", id)
	if err == nil {
		t.Fatal("promoting into a feature that does not exist must be refused")
	}
	if !strings.Contains(err.Error(), "--as-feature") {
		t.Errorf("the refusal does not name the act this actually is: %v", err)
	}
	if st := itemByID(t, cfg, id).State(); st != parser.StateOpen {
		t.Error("the refused promotion still closed the item")
	}
	_ = out
}

// The two forms are different acts and the tool must not guess between
// them; neither may a promotion say nothing about what the work becomes,
// since `becomes:` is required on the event it records.
func TestPromote_RefusesAmbiguousAndUnattributedRequests(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)

	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"neither form", []string{id}, "becomes"},
		{"both forms", []string{"--as-feature", "X", "--as-amendment", "@widget", id}, "Pick one"},
		{"initiative without a feature", []string{"--as-amendment", "@widget", "--initiative", "auth", id}, "--as-feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := promote(t, cfg, tc.argv...)
			if err == nil {
				t.Fatalf("%s must be refused", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal does not explain: %v", err)
			}
		})
	}

	// Attribution is required: promotion is a decision.
	promoteAsFeature, promoteBy = "X", ""
	t.Cleanup(func() { promoteAsFeature, promoteBy = "", "" })
	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})
	if err := runPromote(cmd, []string{id}); err == nil || !strings.Contains(err.Error(), "--by") {
		t.Errorf("an unattributed promotion was accepted: %v", err)
	}
}

// A closed item cannot be promoted. The schema allows at most one
// terminal event and it must be last, so appending would produce a
// record the validator refuses — better to say why here than to let the
// write fail with a shape error the caller cannot act on.
func TestPromote_RefusesAnAlreadyClosedItem(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)
	if _, err := backlogCLI(t, cfg, "decline", id, "--reason", "no", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}

	_, err := promote(t, cfg, "--as-feature", "Something Else", id)
	if err == nil {
		t.Fatal("a closed item was promoted")
	}
	if !strings.Contains(err.Error(), "declined") {
		t.Errorf("the refusal does not say what state blocked it: %v", err)
	}
	if _, statErr := os.Stat(cfg.FeaturePath("something-else")); statErr == nil {
		t.Error("the refused promotion still scaffolded a feature")
	}
}

// dirSnapshot fingerprints a directory by name and CONTENT, so
// "wrote nothing" means the bytes are unchanged rather than that the
// modification times happened to match at this resolution.
func dirSnapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b.WriteString(e.Name())
		b.WriteString("=")
		if !e.IsDir() {
			content, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
			if readErr != nil {
				t.Fatal(readErr)
			}
			fmt.Fprintf(&b, "%x", sha256.Sum256(content))
		}
		b.WriteString(";")
	}
	return b.String()
}

// THE FULL AMENDMENT LIFECYCLE: trigger emitted -> amendment written ->
// item reaches terminal `amended`.
//
// The emitted close command used to be `backlog fold --into @feature/...`,
// which could not run at all: fold resolves another BACKLOG ITEM id and
// rejects every amendment ref. So the amendment path had no ending, and
// nothing tested that it did.
func TestPromote_AmendmentLifecycleReachesTerminalAmended(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Filter belongs above the table", nil)

	out, err := promote(t, cfg, "--as-amendment", "@widget", id)
	if err != nil {
		t.Fatal(err)
	}

	// The emitted close command must be RUNNABLE, and must not be fold.
	if strings.Contains(out, "backlog fold") {
		t.Error("the amendment path still emits fold, which cannot take an amendment ref")
	}
	if !strings.Contains(out, "backlog amend "+id) {
		t.Errorf("no runnable close command was emitted:\n%s", out)
	}

	// NO CLOSURE BEFORE THE AMENDMENT EXISTS. Closing against an
	// amendment nobody wrote is an observation lost with a receipt.
	if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter-above-table", "--by", "dwht"); err == nil {
		t.Fatal("the item was closed against an amendment that does not exist")
	}
	if st := itemByID(t, cfg, id).State(); st != parser.StateOpen {
		t.Fatalf("the refused close still moved the item to %q", st)
	}

	// The amendment lands.
	amendments := parser.AmendmentsDir(featurePath)
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\namendment: filter-above-table\ndate: 2026-09-01\ntrigger: backlog:" + id + "\n---\n\n## Change\n\nThe filter moves above the table.\n"
	if err := os.WriteFile(filepath.Join(amendments, "001-filter-above-table.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := backlogCLI(t, cfg, "amend", id, "--into", "@widget/001-filter-above-table", "--by", "dwht"); err != nil {
		t.Fatalf("the emitted close command is not runnable once the amendment exists: %v", err)
	}

	item := itemByID(t, cfg, id)
	if item.State() != parser.BacklogState(parser.EventAmended) {
		t.Errorf("the item is %q, want amended — not folded, which means something else", item.State())
	}
	last := item.History[len(item.History)-1]
	if last.Becomes != "@widget/001-filter-above-table" {
		t.Errorf("becomes=%q; it must name the amendment that carries the work", last.Becomes)
	}
	if last.By != "dwht" {
		t.Errorf("the closure is attributed to %q", last.By)
	}
	// The causal link the amendment schema was waiting for.
	written, _ := os.ReadFile(filepath.Join(amendments, "001-filter-above-table.md"))
	if !strings.Contains(string(written), "trigger: backlog:"+id) {
		t.Error("the amendment does not carry the backlog trigger")
	}
}

// --as-amendment names a FEATURE. ValidateAboutRef also accepts a
// contract ref, and BareAboutFeature resolved it to the feature — so a
// syntactically wrong target was silently accepted and echoed back as
// though it were what the caller wrote.
func TestPromote_AsAmendmentRefusesAContractRef(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Something", nil)

	for _, ref := range []string{
		"@widget/operation:rename",
		"@widget/surface:some-fragment",
		"@widget/domain:some-entity",
	} {
		out, err := promote(t, cfg, "--as-amendment", ref, id)
		if err == nil {
			t.Errorf("%q was accepted as a feature:\n%s", ref, out)
			continue
		}
		if !strings.Contains(err.Error(), "not a contract entry") {
			t.Errorf("the refusal for %q does not explain the expected form: %v", ref, err)
		}
	}
	// The bare forms still work.
	if _, err := promote(t, cfg, "--as-amendment", "@widget", id); err != nil {
		t.Errorf("the bare feature form was refused: %v", err)
	}
	if st := itemByID(t, cfg, id).State(); st != parser.StateOpen {
		t.Error("--as-amendment closed the item")
	}
}

// TWO CONCURRENT PROMOTIONS must scaffold ONE feature, not two.
//
// The open-state check that authorises the scaffold used to sit outside
// every lock, so both callers could see an open item, both scaffold a
// different feature tree, and only one terminal event survive — leaving
// an orphan feature nobody asked for, and history that mentions only
// the winner. Re-checking under the item's own lock prevents corrupt
// history, which is a different thing from preventing duplicated work.
func TestPromote_ConcurrentPromotionsScaffoldOnlyOnce(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "One observation", nil)

	names := []string{"First Attempt", "Second Attempt"}
	var wg sync.WaitGroup
	errs := make([]error, len(names))
	start := make(chan struct{})
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			<-start
			cmd := testCommandWithContext(t, cfg)
			cmd.SetOut(&bytes.Buffer{})
			errs[i] = promoteFeature(cmd, cfg, id, name, "", "dwht")
		}(i, name)
	}
	close(start)
	wg.Wait()

	won := 0
	for _, err := range errs {
		if err == nil {
			won++
		}
	}
	if won != 1 {
		t.Errorf("%d of 2 concurrent promotions succeeded, want exactly 1: %v", won, errs)
	}

	scaffolded := 0
	for _, name := range names {
		if _, err := os.Stat(cfg.FeaturePath(parser.Slugify(name))); err == nil {
			scaffolded++
		}
	}
	if scaffolded != 1 {
		t.Errorf("%d features were scaffolded for one observation, want 1", scaffolded)
	}

	item := itemByID(t, cfg, id)
	if item.State() != parser.BacklogState(parser.EventPromoted) {
		t.Errorf("the item is %q, want promoted", item.State())
	}
	terminal := 0
	for _, e := range item.History {
		if e.Event == parser.EventPromoted {
			terminal++
		}
	}
	if terminal != 1 {
		t.Errorf("history records %d promotions, want 1", terminal)
	}
}

// PROMOTE vs DECLINE: a concurrent decline must not leave a scaffolded
// feature beside an item closed as `declined`. Exactly one act wins, and
// the filesystem must agree with the history about which.
func TestPromote_ConcurrentDeclineDoesNotStrandAScaffold(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		cfg, _ := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "One observation", nil)

		var wg sync.WaitGroup
		var promoteErr, declineErr error
		start := make(chan struct{})

		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cmd := testCommandWithContext(t, cfg)
			cmd.SetOut(&bytes.Buffer{})
			promoteErr = promoteFeature(cmd, cfg, id, "Promoted Feature", "", "dwht")
		}()
		go func() {
			defer wg.Done()
			<-start
			cmd := testCommandWithContext(t, cfg)
			cmd.SetOut(&bytes.Buffer{})
			declineErr = appendDispositionWith(cmd, cfg, id, parser.EventDeclined, "", "not doing it", "dwht")
		}()
		close(start)
		wg.Wait()

		if (promoteErr == nil) == (declineErr == nil) {
			t.Fatalf("attempt %d: promote=%v decline=%v — exactly one must win", attempt, promoteErr, declineErr)
		}

		item := itemByID(t, cfg, id)
		_, scaffoldErr := os.Stat(cfg.FeaturePath("promoted-feature"))
		scaffolded := scaffoldErr == nil

		switch item.State() {
		case parser.BacklogState(parser.EventPromoted):
			if !scaffolded {
				t.Fatalf("attempt %d: the item says promoted but no feature was scaffolded", attempt)
			}
		case parser.BacklogState(parser.EventDeclined):
			if scaffolded {
				t.Fatalf("attempt %d: the item was declined but a feature was scaffolded anyway — the promotion did work it was not authorised to do", attempt)
			}
		default:
			t.Fatalf("attempt %d: the item is %q after two terminal attempts", attempt, item.State())
		}
	}
}

// Both goroutines call the parameter-taking cores rather than the
// flag-reading entry points: the package flag globals are cobra's, and
// two goroutines assigning them would measure the test's own data race
// instead of the guard it exists to check.

// The amendment must say it is THIS item's amendment.
//
// Existence alone was the check, so any amendment on the feature could
// close any open item as `amended` — manufacturing exactly the causal
// link the trigger exists to preserve.
func TestBacklogAmend_RequiresTheAmendmentToNameThisItem(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	mine := capture(t, cfg, "gap", "Filter belongs above the table", nil)
	other := capture(t, cfg, "gap", "Something unrelated", nil)

	amendments := parser.AmendmentsDir(featurePath)
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(seq, slug, trigger string) {
		t.Helper()
		fm := "---\namendment: " + slug + "\ndate: 2026-09-01\n"
		if trigger != "" {
			fm += "trigger: " + trigger + "\n"
		}
		fm += "---\n\n## Change\n\nSomething changed.\n"
		if err := os.WriteFile(filepath.Join(amendments, seq+"-"+slug+".md"), []byte(fm), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("001", "no-trigger", "")
	write("002", "someone-elses", "backlog:"+other)
	write("003", "mine", "backlog:"+mine)
	if err := os.WriteFile(filepath.Join(amendments, "004-malformed.md"), []byte("not an amendment at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, into, wantIn string
	}{
		{"no trigger", "@widget/001-no-trigger", "carries no `trigger:`"},
		{"another item's trigger", "@widget/002-someone-elses", "different amendment"},
		{"malformed", "@widget/004-malformed", "will not parse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := backlogCLI(t, cfg, "amend", mine, "--into", tc.into, "--by", "dwht")
			if err == nil {
				t.Fatalf("%s closed the item", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("the refusal does not explain: %v", err)
			}
			if st := itemByID(t, cfg, mine).State(); st != parser.StateOpen {
				t.Errorf("the refused closure moved the item to %q", st)
			}
		})
	}

	// The matching case closes it.
	if _, err := backlogCLI(t, cfg, "amend", mine, "--into", "@widget/003-mine", "--by", "dwht"); err != nil {
		t.Fatalf("the item's own amendment was refused: %v", err)
	}
	item := itemByID(t, cfg, mine)
	if item.State() != parser.BacklogState(parser.EventAmended) {
		t.Errorf("the item is %q, want amended", item.State())
	}
	if got := item.History[len(item.History)-1].Becomes; got != "@widget/003-mine" {
		t.Errorf("becomes=%q", got)
	}
	// And the other item is untouched by any of it.
	if st := itemByID(t, cfg, other).State(); st != parser.StateOpen {
		t.Errorf("the unrelated item was moved to %q", st)
	}
}

// THE SAME ITEM, ADDRESSED TWO WAYS, must take the same guard.
//
// The promotion guard was keyed on the caller's raw ref while every
// other terminal verb locks by canonical id, so a full id and its
// accepted short prefix took two different locks and did not contend at
// all — the guard was present and the race was still open. The existing
// concurrency tests use one spelling and cannot see this.
func TestPromote_GuardIsKeyedByCanonicalIDNotTheSpellingUsed(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		cfg, _ := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "One observation", nil)
		short := shortID(id)
		if short == id {
			t.Fatal("fixture does not produce a distinct short form")
		}

		var wg sync.WaitGroup
		var promoteErr, declineErr error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cmd := testCommandWithContext(t, cfg)
			cmd.SetOut(&bytes.Buffer{})
			// SHORT prefix — the spelling whose raw ref is NOT the
			// canonical id, which is where the mis-keyed guard shows.
			promoteErr = promoteFeature(cmd, cfg, short, "Promoted Feature", "", "dwht")
		}()
		go func() {
			defer wg.Done()
			<-start
			cmd := testCommandWithContext(t, cfg)
			cmd.SetOut(&bytes.Buffer{})
			// FULL id — the same item, the other spelling.
			declineErr = appendDispositionWith(cmd, cfg, id, parser.EventDeclined, "", "not doing it", "dwht")
		}()
		close(start)
		wg.Wait()

		if (promoteErr == nil) == (declineErr == nil) {
			t.Fatalf("attempt %d: promote=%v decline=%v — exactly one must win", attempt, promoteErr, declineErr)
		}
		item := itemByID(t, cfg, id)
		_, statErr := os.Stat(cfg.FeaturePath("promoted-feature"))
		scaffolded := statErr == nil
		if item.State() == parser.BacklogState(parser.EventDeclined) && scaffolded {
			t.Fatalf("attempt %d: declined by full id, but the promotion (addressed by short prefix) scaffolded anyway — the two spellings took different locks", attempt)
		}
		if item.State() == parser.BacklogState(parser.EventPromoted) && !scaffolded {
			t.Fatalf("attempt %d: promoted with no feature", attempt)
		}
	}
}

// TWO DIFFERENT ITEMS promoting to the SAME feature name.
//
// Per-item guards do not coordinate here — the thing being contended is
// the feature directory and the origin file inside it. Both could pass
// add-feature's existence check before either created it, and
// appendBacklogOrigin is a read-then-replace, so one origin could be
// lost. Atomic replacement prevents truncation, not lost updates.
//
// THE GUARANTEE IS EXCLUSIVE CREATION: one wins, the loser stays open.
func TestPromote_ConcurrentPromotionsToTheSameTargetLeaveOneOpen(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		cfg, _ := parkFixture(t, "widget")
		a := capture(t, cfg, "gap", "First observation", nil)
		b := capture(t, cfg, "defect", "Second observation", nil)

		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		for i, id := range []string{a, b} {
			wg.Add(1)
			go func(i int, id string) {
				defer wg.Done()
				<-start
				cmd := testCommandWithContext(t, cfg)
				cmd.SetOut(&bytes.Buffer{})
				errs[i] = promoteFeature(cmd, cfg, id, "Shared Target", "", "dwht")
			}(i, id)
		}
		close(start)
		wg.Wait()

		won := 0
		for _, err := range errs {
			if err == nil {
				won++
			}
		}
		if won != 1 {
			t.Fatalf("attempt %d: %d of 2 promotions to one target succeeded, want exactly 1: %v", attempt, won, errs)
		}

		// The loser stays OPEN — merging two observations into one
		// feature is a judgment for a person, not for whichever
		// goroutine lost.
		open, promoted := 0, 0
		for _, id := range []string{a, b} {
			switch itemByID(t, cfg, id).State() {
			case parser.StateOpen:
				open++
			case parser.BacklogState(parser.EventPromoted):
				promoted++
			}
		}
		if open != 1 || promoted != 1 {
			t.Fatalf("attempt %d: %d open / %d promoted, want 1 and 1", attempt, open, promoted)
		}

		// Exactly one origin link, and it is the winner's — no lost
		// update, and no origin for an item that is still open.
		intents, err := os.ReadFile(filepath.Join(cfg.FeaturePath("shared-target"), "intents.md"))
		if err != nil {
			t.Fatalf("attempt %d: the winner scaffolded nothing: %v", attempt, err)
		}
		links := strings.Count(string(intents), "backlog-origin:")
		if links != 1 {
			t.Fatalf("attempt %d: %d origin links, want exactly 1:\n%s", attempt, links, intents)
		}
		for _, id := range []string{a, b} {
			named := strings.Contains(string(intents), id)
			isPromoted := itemByID(t, cfg, id).State() == parser.BacklogState(parser.EventPromoted)
			if named != isPromoted {
				t.Fatalf("attempt %d: the feature names %s=%v but its state is promoted=%v", attempt, shortID(id), named, isPromoted)
			}
		}
	}
}

// AN INTERRUPTED PROMOTION MUST RESUME, not be misclassified.
//
// Exclusive target creation, done naively, destroys the repair/retry
// property the proposal states: the scaffold is a sequence of writes
// rather than a transaction, so crashing between creating the feature
// and appending the terminal event is ORDINARY, not exotic. Existence
// alone cannot tell "our interrupted scaffold" from "somebody else's
// feature", so a durable reservation records the claim before the claim
// is acted on.
//
// The end state after any resume must be: one feature, one origin link,
// one terminal event.
func TestPromote_ResumesAfterInterruptionAtEveryStage(t *testing.T) {
	assertSettled := func(t *testing.T, cfg *config.Context, id string) {
		t.Helper()
		featurePath := cfg.FeaturePath("resumed-feature")
		intents, err := os.ReadFile(filepath.Join(featurePath, "intents.md"))
		if err != nil {
			t.Fatalf("no feature after resume: %v", err)
		}
		if n := strings.Count(string(intents), "backlog-origin:"); n != 1 {
			t.Errorf("%d origin links, want exactly 1:\n%s", n, intents)
		}
		if !strings.Contains(string(intents), id) {
			t.Error("the origin does not name this item")
		}
		item := itemByID(t, cfg, id)
		if item.State() != parser.BacklogState(parser.EventPromoted) {
			t.Errorf("the item is %q, want promoted", item.State())
		}
		terminal := 0
		for _, e := range item.History {
			if e.Event == parser.EventPromoted {
				terminal++
			}
		}
		if terminal != 1 {
			t.Errorf("%d terminal events, want 1", terminal)
		}
	}

	// (a) Interrupted DURING a real promotion, after the scaffold. The
	// reservation under test is the one the promotion wrote itself.
	t.Run("after the scaffold, interrupted mid-run", func(t *testing.T) {
		cfg, _ := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "Interrupted here", nil)

		promotionFailAfterScaffold = func() error { return fmt.Errorf("process died") }
		t.Cleanup(func() { promotionFailAfterScaffold = nil })

		cmd := testCommandWithContext(t, cfg)
		cmd.SetOut(&bytes.Buffer{})
		if err := promoteFeature(cmd, cfg, id, "Resumed Feature", "", "dwht"); err == nil {
			t.Fatal("the injected failure did not stop the promotion")
		}
		if st := itemByID(t, cfg, id).State(); st != parser.StateOpen {
			t.Fatalf("the interrupted promotion closed the item anyway: %q", st)
		}

		// The re-run must resume, not refuse.
		promotionFailAfterScaffold = nil
		if err := promoteFeature(cmd, cfg, id, "Resumed Feature", "", "dwht"); err != nil {
			t.Fatalf("the interrupted promotion could not be resumed: %v", err)
		}
		assertSettled(t, cfg, id)
	})

	// (a2) Interrupted INSIDE the scaffold, between creating the
	// directories and writing intents.md. This is the only interruption
	// that leaves a feature directory carrying nothing to identify it —
	// no intents.md, so no origin comment — and it is therefore the case
	// the reservation alone can recover. Without it, the re-run sees a
	// directory it cannot attribute and refuses as "another promotion
	// created it", stranding the item permanently.
	t.Run("interrupted inside the scaffold, before intents.md", func(t *testing.T) {
		cfg, _ := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "Interrupted mid-scaffold", nil)

		scaffoldFailAfterDirs = func() error { return fmt.Errorf("disk full") }
		t.Cleanup(func() { scaffoldFailAfterDirs = nil })

		cmd := testCommandWithContext(t, cfg)
		cmd.SetOut(&bytes.Buffer{})
		if err := promoteFeature(cmd, cfg, id, "Resumed Feature", "", "dwht"); err == nil {
			t.Fatal("the injected failure did not stop the scaffold")
		}
		featurePath := cfg.FeaturePath("resumed-feature")
		if _, err := os.Stat(featurePath); err != nil {
			t.Fatalf("the fixture did not produce a partial scaffold: %v", err)
		}
		if _, err := os.Stat(filepath.Join(featurePath, "intents.md")); err == nil {
			t.Fatal("the fixture wrote intents.md, so there is nothing unidentifiable to recover")
		}

		scaffoldFailAfterDirs = nil
		if err := promoteFeature(cmd, cfg, id, "Resumed Feature", "", "dwht"); err != nil {
			t.Fatalf("an unidentifiable partial scaffold could not be resumed: %v", err)
		}
		assertSettled(t, cfg, id)
	})

	// (b) A PARTIAL scaffold: the directory exists, intents.md does not,
	// and a file a person may have started editing is already there.
	// Resuming must complete it without clobbering that file.
	t.Run("after a PARTIAL scaffold with no origin", func(t *testing.T) {
		cfg, _ := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "Interrupted here", nil)
		featurePath := cfg.FeaturePath("resumed-feature")

		if err := os.MkdirAll(featurePath, 0o755); err != nil {
			t.Fatal(err)
		}
		const notes = "# Resumed Feature — Dialogs\n\nSomebody already started writing here.\n"
		if err := os.WriteFile(filepath.Join(featurePath, "dialogs.md"), []byte(notes), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writePromotionReservation(cfg, "@resumed-feature", id); err != nil {
			t.Fatal(err)
		}

		cmd := testCommandWithContext(t, cfg)
		cmd.SetOut(&bytes.Buffer{})
		if err := promoteFeature(cmd, cfg, id, "Resumed Feature", "", "dwht"); err != nil {
			t.Fatalf("the interrupted promotion could not be resumed: %v", err)
		}
		assertSettled(t, cfg, id)

		// A repair must not destroy what it was repairing around.
		got, err := os.ReadFile(filepath.Join(featurePath, "dialogs.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != notes {
			t.Errorf("resuming overwrote an existing file:\n want %q\n got  %q", notes, got)
		}
	})

	t.Run("with NO reservation, recovered from the origin alone", func(t *testing.T) {
		cfg, _ := parkFixture(t, "widget")
		id := capture(t, cfg, "gap", "Interrupted, reservation lost", nil)

		cmd := testCommandWithContext(t, cfg)
		cmd.SetOut(&bytes.Buffer{})
		item := itemByID(t, cfg, id)
		if err := scaffoldFeature(cmd, cfg, "Resumed Feature", "", false, false); err != nil {
			t.Fatal(err)
		}
		if err := appendBacklogOrigin(cfg.FeaturePath("resumed-feature"), item); err != nil {
			t.Fatal(err)
		}
		// No reservation at all — the feature's own origin comment is
		// the only evidence, and it is enough.
		if err := promoteFeature(cmd, cfg, id, "Resumed Feature", "", "dwht"); err != nil {
			t.Fatalf("a feature carrying this item's own origin was refused: %v", err)
		}
		assertSettled(t, cfg, id)
	})
}

// Resumability must NOT become a way to take over somebody else's
// feature. A scaffold carrying another item's origin is refused, and so
// is a live reservation held by another open item.
func TestPromote_ResumeDoesNotAdoptAnotherItemsFeature(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	mine := capture(t, cfg, "gap", "Mine", nil)
	theirs := capture(t, cfg, "gap", "Theirs", nil)

	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})
	if err := promoteFeature(cmd, cfg, theirs, "Their Feature", "", "dwht"); err != nil {
		t.Fatal(err)
	}

	// A completed promotion: the feature carries their origin, and the
	// reservation is gone. The ONLY evidence is the origin comment.
	if held, _ := readPromotionReservation(cfg, "@their-feature"); held != "" {
		t.Fatalf("a completed promotion left a reservation behind: %q", held)
	}
	err := promoteFeature(cmd, cfg, mine, "Their Feature", "", "dwht")
	if err == nil {
		t.Fatal("promotion adopted a feature belonging to another item")
	}
	// It must be refused for the RIGHT reason — that the feature is
	// somebody else's — not incidentally by a later check.
	if !strings.Contains(err.Error(), shortID(theirs)) {
		t.Errorf("the refusal does not identify whose feature it is: %v", err)
	}
	// Refused at the OWNERSHIP check, with the guidance a person needs —
	// not incidentally by a later consistency check whose message is
	// about writing a second origin comment.
	if !strings.Contains(err.Error(), "judgment for a person") {
		t.Errorf("the refusal does not tell the user what to do instead: %v", err)
	}
	// And nothing may be written into it.
	intents, _ := os.ReadFile(filepath.Join(cfg.FeaturePath("their-feature"), "intents.md"))
	if strings.Contains(string(intents), mine) {
		t.Error("the refused promotion still wrote its origin into another item's feature")
	}
	if st := itemByID(t, cfg, mine).State(); st != parser.StateOpen {
		t.Errorf("the refused promotion moved the item to %q", st)
	}

	// A LIVE reservation held by an open item.
	held := capture(t, cfg, "gap", "Holding a claim", nil)
	if err := writePromotionReservation(cfg, "@contested", held); err != nil {
		t.Fatal(err)
	}
	if err := promoteFeature(cmd, cfg, mine, "Contested", "", "dwht"); err == nil {
		t.Fatal("promotion took a target reserved by another open item")
	}

	// A STALE reservation — the holder has since closed — must not
	// block forever.
	if _, err := backlogCLI(t, cfg, "decline", held, "--reason", "no", "--by", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := promoteFeature(cmd, cfg, mine, "Contested", "", "dwht"); err != nil {
		t.Fatalf("a reservation held by a closed item blocked the target: %v", err)
	}
	if st := itemByID(t, cfg, mine).State(); st != parser.BacklogState(parser.EventPromoted) {
		t.Errorf("the item is %q, want promoted", st)
	}
}

// intents.md IS NOT A COMPLETION SENTINEL.
//
// The scaffold creates three tree directories and two files, so a crash
// after intents.md and before dialogs.md — or before the handoff and
// build trees exist — leaves a tree the retry used to skip entirely,
// then record `promoted` against. No single file marks the scaffold
// complete, so the conditional scaffold has to run every time.
func TestPromote_ResumeCompletesAPartialTreeEvenWhenIntentsExists(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Interrupted between files", nil)
	featurePath := cfg.FeaturePath("resumed-feature")

	// intents.md present, dialogs.md absent, and only the intents tree
	// created — the exact shape of a scaffold cut off mid-sequence.
	if err := os.MkdirAll(featurePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featurePath, "intents.md"), []byte("# Resumed Feature\n\n> \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePromotionReservation(cfg, "@resumed-feature", id); err != nil {
		t.Fatal(err)
	}

	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})
	if err := promoteFeature(cmd, cfg, id, "Resumed Feature", "", "dwht"); err != nil {
		t.Fatalf("the partial tree could not be completed: %v", err)
	}

	// Every piece a healthy feature has, present after the resume.
	if _, err := os.Stat(filepath.Join(featurePath, "dialogs.md")); err != nil {
		t.Errorf("dialogs.md was never written — the resume skipped the scaffold: %v", err)
	}
	for _, root := range threeTreeRoots(cfg) {
		if _, err := os.Stat(filepath.Join(root, "resumed-feature")); err != nil {
			t.Errorf("tree %s has no directory for the feature: %v", root, err)
		}
	}
	if st := itemByID(t, cfg, id).State(); st != parser.BacklogState(parser.EventPromoted) {
		t.Errorf("the item is %q, want promoted", st)
	}
}

// An UNVERIFIABLE holder must not lose its claim.
//
// reservationIsStale treated every resolve failure as "the item is
// gone", including an unreadable store — so another item could take a
// live claim at exactly the moment nothing could confirm it was dead.
// Only an exact not-found means stale.
func TestPromote_UnverifiableReservationHolderIsNotTreatedAsStale(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	mine := capture(t, cfg, "gap", "Mine", nil)
	holder := capture(t, cfg, "gap", "Holding a claim", nil)
	if err := writePromotionReservation(cfg, "@contested", holder); err != nil {
		t.Fatal(err)
	}

	// The holder's own record becomes unreadable. It has NOT gone away;
	// nothing can say whether its claim is live.
	holderPath := filepath.Join(parser.BacklogRoot(cfg.Root.Path), holder+".yaml")
	if err := os.WriteFile(holderPath, []byte("{{{ not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})
	err := promoteFeature(cmd, cfg, mine, "Contested", "", "dwht")
	if err == nil {
		t.Fatal("a target was taken from a holder whose state could not be verified")
	}
	if !strings.Contains(err.Error(), "cannot verify") {
		t.Errorf("the refusal does not say ownership is unverifiable: %v", err)
	}
	if _, statErr := os.Stat(cfg.FeaturePath("contested")); statErr == nil {
		t.Error("the refused promotion scaffolded anyway")
	}
	if got, _ := readPromotionReservation(cfg, "@contested"); got != holder {
		t.Errorf("the reservation was lost: %q", got)
	}
	if st := itemByID(t, cfg, mine).State(); st != parser.StateOpen {
		t.Errorf("the refused promotion moved the item to %q", st)
	}

	// A holder that is genuinely GONE is still stale.
	if err := os.Remove(holderPath); err != nil {
		t.Fatal(err)
	}
	if err := promoteFeature(cmd, cfg, mine, "Contested", "", "dwht"); err != nil {
		t.Fatalf("a reservation held by a vanished item blocked the target: %v", err)
	}
}

// The INITIATIVE path gets the same guarantees. It kept reading the
// global authoredFlag and writing unconditionally, so an initiative
// promotion could still race the global and a retry could clobber files.
func TestPromote_InitiativeResumePreservesExistingFiles(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id := capture(t, cfg, "gap", "Under an initiative", nil)

	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&bytes.Buffer{})

	// Interrupt inside the scaffold, then leave a file behind.
	scaffoldFailAfterDirs = func() error { return fmt.Errorf("disk full") }
	if err := promoteFeature(cmd, cfg, id, "Reset Password", "auth", "dwht"); err == nil {
		t.Fatal("the injected failure did not stop the scaffold")
	}
	scaffoldFailAfterDirs = nil
	t.Cleanup(func() { scaffoldFailAfterDirs = nil })

	featurePath := cfg.FeaturePath("auth/reset-password")
	const notes = "# Reset Password — Dialogs\n\nSomebody already started writing here.\n"
	if err := os.WriteFile(filepath.Join(featurePath, "dialogs.md"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := promoteFeature(cmd, cfg, id, "Reset Password", "auth", "dwht"); err != nil {
		t.Fatalf("the initiative promotion could not be resumed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(featurePath, "dialogs.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != notes {
		t.Errorf("resuming an initiative promotion overwrote an existing file:\n want %q\n got  %q", notes, got)
	}
	intents, err := os.ReadFile(filepath.Join(featurePath, "intents.md"))
	if err != nil {
		t.Fatalf("the initiative scaffold was never completed: %v", err)
	}
	if n := strings.Count(string(intents), "backlog-origin:"); n != 1 {
		t.Errorf("%d origin links, want exactly 1", n)
	}
	item := itemByID(t, cfg, id)
	if item.State() != parser.BacklogState(parser.EventPromoted) {
		t.Errorf("the item is %q, want promoted", item.State())
	}
	if got := item.History[len(item.History)-1].Becomes; got != "@auth/reset-password" {
		t.Errorf("becomes=%q, want the initiative-qualified ref", got)
	}
}

// Different items, DIFFERENT targets, concurrently. Their per-item and
// per-target guards do not contend at all, so nothing may be shared
// between them — which is why the scaffold takes its inputs as
// arguments instead of swapping package globals.
func TestPromote_ConcurrentDifferentTargetsDoNotInterfere(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	plain := capture(t, cfg, "gap", "Plain target", nil)
	nested := capture(t, cfg, "gap", "Nested target", nil)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	specs := []struct{ id, name, initiative string }{
		{plain, "Plain Feature", ""},
		{nested, "Nested Feature", "auth"},
	}
	for i, spec := range specs {
		wg.Add(1)
		go func(i int, id, name, initiative string) {
			defer wg.Done()
			<-start
			cmd := testCommandWithContext(t, cfg)
			cmd.SetOut(&bytes.Buffer{})
			errs[i] = promoteFeature(cmd, cfg, id, name, initiative, "dwht")
		}(i, spec.id, spec.name, spec.initiative)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("%s failed: %v", specs[i].name, err)
		}
	}
	// Each landed where it was told, not where the other was told.
	if _, err := os.Stat(cfg.FeaturePath("plain-feature")); err != nil {
		t.Errorf("the plain feature is missing: %v", err)
	}
	if _, err := os.Stat(cfg.FeaturePath("auth/nested-feature")); err != nil {
		t.Errorf("the initiative-nested feature is missing: %v", err)
	}
	if _, err := os.Stat(cfg.FeaturePath("nested-feature")); err == nil {
		t.Error("the nested feature was created at the top level — the initiative was lost across goroutines")
	}
	if _, err := os.Stat(cfg.FeaturePath("auth/plain-feature")); err == nil {
		t.Error("the plain feature was nested under an initiative it never asked for")
	}
}
