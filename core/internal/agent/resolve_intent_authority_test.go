// parlay-feature: parlay-tool/intent-supersession
// parlay-component: active-specification-resolver
// parlay-artifact: test

package agent

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func twoIntents() []parser.Intent {
	return []parser.Intent{
		{Slug: "create-the-thing", Title: "Create The Thing", Verify: []string{"creating returns an id"}},
		{Slug: "browse-the-things", Title: "Browse The Things", Verify: []string{"the list shows every thing"}},
	}
}

func retire(seq int, fileSlug, intent string) parser.Amendment {
	return parser.Amendment{Seq: seq, FileSlug: fileSlug, SupersedesIntents: []string{intent}}
}

func activeSlugs(r IntentResolution) []string {
	var out []string
	for _, in := range r.Active {
		out = append(out, in.Slug)
	}
	return out
}

func TestResolveIntentAuthority_NoLedgerLeavesEveryPromiseActive(t *testing.T) {
	r := ResolveIntentAuthority(twoIntents(), nil, 0, AppliedAuthority)
	if len(r.Active) != 2 || len(r.Superseded) != 0 || r.HasPending() {
		t.Errorf("a feature with no ledger promises what it always did; got %v", activeSlugs(r))
	}
}

func TestResolveIntentAuthority_UnappliedLeavesThePromiseActiveAndBlocks(t *testing.T) {
	// The core safety rule: until the decision is applied, the artifacts and
	// the generated code still make this promise. Dropping it here would stop
	// anything checking a promise the code still keeps.
	r := ResolveIntentAuthority(twoIntents(), []parser.Amendment{retire(1, "browse-moves", "browse-the-things")}, 0, AppliedAuthority)

	if r.IsSuperseded("browse-the-things") {
		t.Error("an unapplied decision must not retire anything under applied authority")
	}
	if len(r.Active) != 2 {
		t.Errorf("expected both promises still active, got %v", activeSlugs(r))
	}
	if !r.HasPending() {
		t.Fatal("an authored-but-unapplied supersession must block the boundary")
	}
	if r.PendingSummary() == "" {
		t.Error("the boundary needs something to report")
	}
}

func TestResolveIntentAuthority_AppliedRetiresThePromise(t *testing.T) {
	r := ResolveIntentAuthority(twoIntents(), []parser.Amendment{retire(1, "browse-moves", "browse-the-things")}, 1, AppliedAuthority)

	if !r.IsSuperseded("browse-the-things") {
		t.Fatalf("an applied decision retires the promise; active=%v", activeSlugs(r))
	}
	if r.HasPending() {
		t.Errorf("nothing is pending once applied: %+v", r.Pending)
	}
	if len(r.Active) != 1 || r.Active[0].Slug != "create-the-thing" {
		t.Errorf("expected only create-the-thing active, got %v", activeSlugs(r))
	}
	// Carried, not dropped: the frozen document has to stay readable as
	// history, which means naming the decision that replaced it.
	if len(r.Superseded) != 1 || r.Superseded[0].ByAmendment != "browse-moves" || !r.Superseded[0].Applied {
		t.Errorf("expected the retiring amendment carried and marked applied, got %+v", r.Superseded)
	}
	if len(r.Superseded[0].Intent.Verify) == 0 {
		t.Error("a retired promise keeps its Verify bullets as history")
	}
}

// The two modes are a safety property, not a convenience. Prospective exists
// for the apply workflow, which must see the decision it is applying; ordinary
// callers must never get it.
func TestResolveIntentAuthority_ProspectiveSeesTheTailAppliedDoesNot(t *testing.T) {
	ams := []parser.Amendment{retire(1, "browse-moves", "browse-the-things")}

	applied := ResolveIntentAuthority(twoIntents(), ams, 0, AppliedAuthority)
	prospective := ResolveIntentAuthority(twoIntents(), ams, 0, ProspectiveAuthority)

	if applied.IsSuperseded("browse-the-things") {
		t.Error("ordinary reads must not see the unapplied tail")
	}
	if !prospective.IsSuperseded("browse-the-things") {
		t.Error("the apply workflow must see the decision it is applying")
	}
	if prospective.Superseded[0].Applied {
		t.Error("a prospective retirement must be marked not-yet-applied, so it cannot be mistaken for one in force")
	}
}

func TestResolveIntentAuthority_AppliedRetirementSurvivesALaterUnappliedClaim(t *testing.T) {
	ams := []parser.Amendment{
		retire(1, "browse-moves", "browse-the-things"),
		retire(2, "browse-moves-again", "browse-the-things"),
	}
	r := ResolveIntentAuthority(twoIntents(), ams, 1, AppliedAuthority)

	if !r.IsSuperseded("browse-the-things") {
		t.Error("a promise withdrawn by an applied decision stays withdrawn when a newer unapplied one also names it")
	}
	if r.HasPending() {
		t.Errorf("already retired, so nothing pending on it: %+v", r.Pending)
	}
}

func TestResolveIntentAuthority_MalformedClaimRetiresNothing(t *testing.T) {
	// A record already reported as wrong must not be able to drop a promise.
	for _, bad := range []string{"", "  ", "@other-feature/some-intent", "other-feature/some-intent"} {
		ams := []parser.Amendment{{Seq: 1, FileSlug: "bad", SupersedesIntents: []string{bad}}}
		r := ResolveIntentAuthority(twoIntents(), ams, 1, AppliedAuthority)
		if len(r.Active) != 2 || len(r.Superseded) != 0 {
			t.Errorf("ref %q retired something it should not have: active=%v superseded=%+v", bad, activeSlugs(r), r.Superseded)
		}
	}
}

// The negative control: an ordinary ledger with no supersession must be
// indistinguishable from no ledger at all.
func TestResolveIntentAuthority_OrdinaryLedgerChangesNothing(t *testing.T) {
	ams := []parser.Amendment{{Seq: 1, FileSlug: "tighten-create", Affects: []string{"@f/operation:thing.create"}}}
	r := ResolveIntentAuthority(twoIntents(), ams, 1, AppliedAuthority)
	if len(r.Active) != 2 || len(r.Superseded) != 0 || r.HasPending() {
		t.Errorf("an ordinary applied amendment retires nothing; active=%v superseded=%+v pending=%+v",
			activeSlugs(r), r.Superseded, r.Pending)
	}
}

// When an applied decision and a newer unapplied one both claim an intent, the
// two modes should name DIFFERENT amendments — that difference is the whole
// point of having two modes. Applied authority reports the decision in force;
// prospective, which exists for the apply workflow, must report the decision
// being applied, or the workflow sees a stale predecessor as the current one.
func TestResolveIntentAuthority_ProspectiveNamesTheNewestClaim(t *testing.T) {
	ams := []parser.Amendment{
		retire(1, "browse-moves-to-search", "browse-the-things"),
		{Seq: 2, FileSlug: "browse-moves-to-feed", Supersedes: []string{"browse-moves-to-search"},
			SupersedesIntents: []string{"browse-the-things"}},
	}

	applied := ResolveIntentAuthority(twoIntents(), ams, 1, AppliedAuthority)
	if len(applied.Superseded) != 1 || applied.Superseded[0].ByAmendment != "browse-moves-to-search" {
		t.Errorf("applied authority reports the decision in force: %+v", applied.Superseded)
	}

	prospective := ResolveIntentAuthority(twoIntents(), ams, 1, ProspectiveAuthority)
	if len(prospective.Superseded) != 1 {
		t.Fatalf("expected one superseded promise, got %+v", prospective.Superseded)
	}
	if prospective.Superseded[0].ByAmendment != "browse-moves-to-feed" {
		t.Errorf("prospective must report the newest claim so the apply workflow does not act on a superseded predecessor; got %q",
			prospective.Superseded[0].ByAmendment)
	}
}
