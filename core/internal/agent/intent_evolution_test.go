package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// Stage 1 — a promise can change without dying.
//
// Every test here establishes its precondition and asserts the identity of the
// guard it names, per ground rule 6 of the applied-authority plan.

// validVersion is a snapshot that satisfies the same minimum a founding intent
// must, so a fixture testing something else is not also testing version shape.
func validVersion(goal string) *parser.IntentVersion {
	return &parser.IntentVersion{Title: "A Promise", Goal: goal, Persona: "User"}
}

func foundingIntent() parser.Intent {
	return parser.Intent{
		Slug:   "users-receive-an-email-when-shared",
		Title:  "Users Receive An Email When Shared",
		Goal:   "Users receive an email when a document is shared with them.",
		Verify: []string{"An email arrives when a document is shared."},
	}
}

func revisionAmendment(seq int, slug string, mode parser.IntentMode, goal string) parser.Amendment {
	return parser.Amendment{
		Seq: seq, FileSlug: slug,
		AmendsIntents: []parser.IntentAmendment{{
			Intent: "users-receive-an-email-when-shared",
			Mode:   mode,
			Version: &parser.IntentVersion{
				Title:   "Users Receive A Notification When Shared",
				Goal:    goal,
				Persona: "Recipient",
				Verify:  []string{"A notification arrives on the user's chosen channel."},
			},
		}},
	}
}

// The headline: a revised promise stays ACTIVE and reads differently. Under the
// old vocabulary the only way to express this was to retire the promise, which
// orphaned everything it justified.
func TestStage1_RevisedPromiseStaysActiveWithNewText(t *testing.T) {
	const revised = "Users receive a notification on their chosen channel when a document is shared."
	res := ResolveIntentAuthority(
		[]parser.Intent{foundingIntent()},
		[]parser.Amendment{revisionAmendment(1, "channel-choice", parser.IntentRevise, revised)},
		1, AppliedAuthority)

	if len(res.Superseded) != 0 {
		t.Fatalf("a revision must not retire the lineage; superseded = %+v", res.Superseded)
	}
	if len(res.Active) != 1 {
		t.Fatalf("the promise must still be in force; active = %+v", res.Active)
	}
	if got := res.Active[0].Goal; got != revised {
		t.Errorf("goal = %q, want the revised text — the projection must answer with the CURRENT "+
			"version of the proposition, not the founding one", got)
	}
	if len(res.Active[0].Verify) != 1 || !strings.Contains(res.Active[0].Verify[0], "chosen channel") {
		t.Errorf("the revision's verify bullets must replace the founding ones; got %v",
			res.Active[0].Verify)
	}
	// Identity is the lineage, not the text.
	if res.Active[0].Slug != foundingIntent().Slug {
		t.Error("a revision must not change the lineage slug — attribution binds to it")
	}
}

// An unapplied revision must not change what the feature promises. The
// artifacts and the generated code still make the old promise.
func TestStage1_UnappliedRevisionLeavesTheFoundingPromiseStanding(t *testing.T) {
	res := ResolveIntentAuthority(
		[]parser.Intent{foundingIntent()},
		[]parser.Amendment{revisionAmendment(1, "channel-choice", parser.IntentRevise, "something new")},
		0, AppliedAuthority)

	if len(res.Active) != 1 || res.Active[0].Goal != foundingIntent().Goal {
		t.Errorf("an unapplied revision changed the promise in force; goal = %q",
			res.Active[0].Goal)
	}
	if len(res.Pending) != 1 || res.Pending[0].Intent != foundingIntent().Slug {
		t.Errorf("an unapplied revision must be reported pending so a boundary refuses to "+
			"advance past it; pending = %+v", res.Pending)
	}
}

// The apply workflow must see the decision it is applying.
func TestStage1_ProspectiveAuthoritySeesTheUnappliedRevision(t *testing.T) {
	const revised = "the new promise"
	res := ResolveIntentAuthority(
		[]parser.Intent{foundingIntent()},
		[]parser.Amendment{revisionAmendment(1, "channel-choice", parser.IntentRevise, revised)},
		0, ProspectiveAuthority)

	if len(res.Active) != 1 || res.Active[0].Goal != revised {
		t.Errorf("prospective authority must report the decision being applied; goal = %q",
			res.Active[0].Goal)
	}
}

// Later applied revisions win, and the winner is decided by sequence rather
// than by the order the caller happened to pass them.
func TestStage1_LatestAppliedRevisionWinsBySequence(t *testing.T) {
	res := ResolveIntentAuthority(
		[]parser.Intent{foundingIntent()},
		[]parser.Amendment{
			revisionAmendment(2, "second", parser.IntentRevise, "second text"),
			revisionAmendment(1, "first", parser.IntentRevise, "first text"),
		},
		2, AppliedAuthority)

	if res.Active[0].Goal != "second text" {
		t.Errorf("goal = %q, want the highest-sequence applied revision regardless of input order",
			res.Active[0].Goal)
	}
}

// A retirement still ends the lineage, and a revision does not resurrect one.
func TestStage1_RetirementStillEndsTheLineage(t *testing.T) {
	retire := parser.Amendment{
		Seq: 2, FileSlug: "closed",
		AmendsIntents: []parser.IntentAmendment{{
			Intent: "users-receive-an-email-when-shared", Mode: parser.IntentRetire,
		}},
	}
	res := ResolveIntentAuthority(
		[]parser.Intent{foundingIntent()},
		[]parser.Amendment{revisionAmendment(1, "first", parser.IntentRevise, "revised"), retire},
		2, AppliedAuthority)

	if len(res.Superseded) != 1 {
		t.Fatalf("retire must end the lineage; superseded = %+v", res.Superseded)
	}
	if len(res.Active) != 0 {
		t.Errorf("a retired promise must not remain active; active = %+v", res.Active)
	}
}

// The legacy spelling keeps working and keeps its old meaning, without being
// relabelled as a known retirement.
func TestStage1_LegacySupersessionStillRetiresAndIsNotRelabelled(t *testing.T) {
	legacy := parser.Amendment{
		Seq: 1, FileSlug: "old-style",
		SupersedesIntents: []string{"users-receive-an-email-when-shared"},
	}
	res := ResolveIntentAuthority([]parser.Intent{foundingIntent()},
		[]parser.Amendment{legacy}, 1, AppliedAuthority)
	if len(res.Superseded) != 1 {
		t.Fatalf("the legacy spelling must still retire; superseded = %+v", res.Superseded)
	}

	trs := legacy.IntentTransitions()
	if len(trs) != 1 {
		t.Fatalf("transitions = %+v", trs)
	}
	if trs[0].Mode != parser.IntentLegacySupersession {
		t.Errorf("mode = %q — a pre-vocabulary record must read as legacy_supersession, never as "+
			"a known retire: retirement was the only available spelling, so an author intending a "+
			"revision had no way to say so", trs[0].Mode)
	}
	if !trs[0].IsLegacy() {
		t.Error("the transition must read as legacy so nothing reports its semantics as known")
	}
	if res.Superseded[0].Mode != parser.IntentLegacySupersession {
		t.Errorf("the resolution must PRESERVE the uncertainty it normalises — a consumer has to "+
			"be able to tell a known retire from a legacy supersession; mode = %q",
			res.Superseded[0].Mode)
	}
}

// Shape validation. The vocabulary is closed and its modes have obligations.
func TestStage1_TransitionShapeIsValidated(t *testing.T) {
	cases := []struct {
		name string
		a    parser.Amendment
		want string
	}{
		{
			name: "unknown mode",
			a:    parser.Amendment{AmendsIntents: []parser.IntentAmendment{{Intent: "x", Mode: "widen"}}},
			want: "the vocabulary is closed",
		},
		{
			name: "legacy mode may not be authored",
			a: parser.Amendment{AmendsIntents: []parser.IntentAmendment{
				{Intent: "x", Mode: parser.IntentLegacySupersession}}},
			want: "may not be authored",
		},
		{
			name: "revise with no new text",
			a:    parser.Amendment{AmendsIntents: []parser.IntentAmendment{{Intent: "x", Mode: parser.IntentRevise}}},
			want: "supplies no version:",
		},
		{
			name: "retire carrying new text",
			a: parser.Amendment{AmendsIntents: []parser.IntentAmendment{
				{Intent: "x", Mode: parser.IntentRetire, Version: &parser.IntentVersion{Goal: "still promising something"}}}},
			want: "does not also read differently",
		},
		{
			name: "same lineage twice",
			a: parser.Amendment{AmendsIntents: []parser.IntentAmendment{
				{Intent: "x", Mode: parser.IntentExtend, Version: validVersion("a")},
				{Intent: "x", Mode: parser.IntentNarrow, Version: validVersion("b")},
			}},
			want: "one record states one transition per lineage",
		},
		{
			name: "both vocabularies for one lineage",
			a: parser.Amendment{
				AmendsIntents:     []parser.IntentAmendment{{Intent: "x", Mode: parser.IntentRevise, Version: validVersion("a")}},
				SupersedesIntents: []string{"x"},
			},
			want: "in one vocabulary",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := tc.a.ValidateIntentTransitions()
			if len(problems) == 0 {
				t.Fatal("this shape must be reported")
			}
			var joined string
			for _, p := range problems {
				joined += p + "\n"
			}
			if !strings.Contains(joined, tc.want) {
				t.Errorf("the finding must say what is wrong; got %q", joined)
			}
		})
	}

	// And a well-formed record produces nothing.
	ok := revisionAmendment(1, "fine", parser.IntentRevise, "a new promise")
	if problems := ok.ValidateIntentTransitions(); len(problems) != 0 {
		t.Errorf("a well-formed transition must validate cleanly; got %v", problems)
	}
}

// BLOCKER (Codex): an applied revision plus a PENDING retirement must expose
// the revised text, not roll back to the founding promise.
//
// seq 1 revises email to notification and is applied; seq 2 retires and is
// not. What the system currently promises is the seq-1 text, with a retirement
// pending. Answering with founding text describes a system nobody is running.
func TestStage1_PendingRetirementDoesNotRollBackAnAppliedRevision(t *testing.T) {
	const revised = "Users receive a notification on their chosen channel when a document is shared."
	retire := parser.Amendment{
		Seq: 2, FileSlug: "closed",
		AmendsIntents: []parser.IntentAmendment{{
			Intent: "users-receive-an-email-when-shared", Mode: parser.IntentRetire,
		}},
	}
	res := ResolveIntentAuthority(
		[]parser.Intent{foundingIntent()},
		[]parser.Amendment{revisionAmendment(1, "channel-choice", parser.IntentRevise, revised), retire},
		1, AppliedAuthority)

	if len(res.Active) != 1 {
		t.Fatalf("the promise is still in force until the retirement applies; active = %+v", res.Active)
	}
	if got := res.Active[0].Goal; got != revised {
		t.Errorf("goal = %q, want the applied revision. A pending retirement says the promise is "+
			"about to end; it does not un-apply a revision that already happened", got)
	}
	if len(res.Pending) != 1 || res.Pending[0].Mode != parser.IntentRetire {
		t.Errorf("the pending retirement must be reported with its mode; pending = %+v", res.Pending)
	}
}

// BLOCKER (Codex): a pending transition must be summarised by its real mode.
// Reporting an unapplied revision as a retirement tells an operator a promise
// is about to be withdrawn when it is about to be reworded.
func TestStage1_PendingSummaryNamesTheRealMode(t *testing.T) {
	cases := []struct {
		mode parser.IntentMode
		want string
	}{
		{parser.IntentRevise, "revises"},
		{parser.IntentExtend, "extends"},
		{parser.IntentNarrow, "narrows"},
		{parser.IntentRetire, "retires"},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			a := revisionAmendment(1, "pending-one", tc.mode, "new text")
			if tc.mode.EndsLineage() {
				a.AmendsIntents[0].Version = nil
			}
			res := ResolveIntentAuthority([]parser.Intent{foundingIntent()},
				[]parser.Amendment{a}, 0, AppliedAuthority)
			got := res.PendingSummary()
			if !strings.Contains(got, tc.want) {
				t.Errorf("summary = %q, want it to say %q — a pending %s is not a pending "+
					"retirement", got, tc.want, tc.mode)
			}
		})
	}
}

// A version is a SNAPSHOT: every field comes from it, so an omitted field is
// cleared rather than inherited, and fields other than goal really do change.
func TestStage1_VersionIsASnapshotNotAPatch(t *testing.T) {
	founding := foundingIntent()
	founding.Constraints = []string{"Email only."}
	founding.Persona = "Recipient"

	a := parser.Amendment{
		Seq: 1, FileSlug: "channel-choice",
		AmendsIntents: []parser.IntentAmendment{{
			Intent: founding.Slug, Mode: parser.IntentRevise,
			Version: &parser.IntentVersion{
				Title:   "Users Choose Their Channel",
				Goal:    "Users receive a notification on their chosen channel.",
				Persona: "Account holder",
				// Constraints and Verify deliberately omitted.
			},
		}},
	}
	res := ResolveIntentAuthority([]parser.Intent{founding}, []parser.Amendment{a}, 1, AppliedAuthority)
	got := res.Active[0]

	if got.Title != "Users Choose Their Channel" {
		t.Errorf("title = %q — a versioned title is what lets a human-facing name change while "+
			"the lineage slug stays stable", got.Title)
	}
	if got.Persona != "Account holder" {
		t.Errorf("persona = %q, want the revised value — a revision changes more than the goal",
			got.Persona)
	}
	if len(got.Constraints) != 0 {
		t.Errorf("constraints = %v, want none. Omission in a snapshot means ABSENT; inheriting "+
			"the founding value would make this a patch with undefined semantics", got.Constraints)
	}
	if len(got.Verify) != 0 {
		t.Errorf("verify = %v, want none — an omitted field must be clearable", got.Verify)
	}
	if got.Slug != founding.Slug {
		t.Error("the lineage slug is the one field a transition may never change")
	}
}

// Obligations attach to lineage-ENDING behaviour, but only the SAFETY ones.
//
// An earlier version of this test demanded that every code the legacy form
// emits also appear for a known retire. That pinned a false equivalence:
// legacy supersession assumed something always replaces a withdrawn promise
// because withdrawal was the only verb available, while `mode: retire` means
// the opposite by definition. Carrying that wording into the new mode would
// rebuild inside the validator the exact conflation this stage removes.
//
// So the property is: both forms require Why and Acceptance, and the known
// retire's message never claims a successor.
func TestStage1_LineageEndingObligationsAreSafetyNotSemanticFiction(t *testing.T) {
	body := func(frontmatter string) []byte {
		return []byte("---\n" + frontmatter + "---\n\n## Change\nThe promise ends.\n")
	}
	legacy := ValidateAmendment(ModeBuild, "001-x.md", body(
		"amendment: x\ndate: 2026-09-01\nsupersedes_intents:\n  - a-promise\n"))
	modern := ValidateAmendment(ModeBuild, "001-x.md", body(
		"amendment: x\ndate: 2026-09-01\namends_intents:\n  - intent: a-promise\n    mode: retire\n"))

	codes := func(os []ValidationOutcome) map[string]string {
		m := map[string]string{}
		for _, o := range os {
			m[o.Code] = o.Message
		}
		return m
	}
	lc, mc := codes(legacy), codes(modern)

	// Both owe the same safety obligations.
	for _, code := range []string{"amendment-supersession-no-rationale", "amendment-supersession-no-successor"} {
		if _, ok := lc[code]; !ok {
			t.Fatalf("fixture: the legacy form must report %s, or this proves nothing", code)
		}
		if _, ok := mc[code]; !ok {
			t.Errorf("a known retire must owe %s too — saying why, and saying what is observably "+
				"true afterwards, are safety obligations of ENDING a promise", code)
		}
	}

	// But a known retire must not be told something replaces the promise.
	msg := strings.ToLower(mc["amendment-supersession-no-successor"])
	for _, wrong := range []string{"successor", "what replaces it", "is deletion, not supersession"} {
		if strings.Contains(msg, wrong) {
			t.Errorf("the retire diagnostic says %q, contradicting the mode the author explicitly "+
				"chose: retire MEANS the lineage ends with nothing taking it over. Message: %q",
				wrong, mc["amendment-supersession-no-successor"])
		}
	}
	// The legacy wording is preserved, because its author's intent was never
	// recorded and the compatibility message is the honest one for an unknown.
	if !strings.Contains(strings.ToLower(lc["amendment-supersession-no-successor"]), "replaces it") {
		t.Errorf("the legacy diagnostic should keep its wording; got %q",
			lc["amendment-supersession-no-successor"])
	}
}

// A terminal record naming every lineage through the new vocabulary must not be
// rejected as naming none.
func TestStage1_FeatureRetirementAcceptsTheNewVocabulary(t *testing.T) {
	fm := "amendment: closed\ndate: 2026-09-01\nretires_feature: true\noutcome: obsolete\n" +
		"amends_intents:\n  - intent: a-promise\n    mode: retire\n"
	outcomes := ValidateAmendment(ModeBuild, "001-closed.md",
		[]byte("---\n"+fm+"---\n\n## Change\nClosed.\n\n## Why\nDone.\n\n## Acceptance\n- Gone.\n"))
	for _, o := range outcomes {
		if o.Code == "amendment-retirement-no-intents" {
			t.Errorf("a retirement naming its promises in amends_intents was rejected as naming "+
				"none: %s", o.Message)
		}
	}

	// And one that truly ends nothing still fails, by identity.
	none := ValidateAmendment(ModeBuild, "001-closed.md",
		[]byte("---\namendment: closed\ndate: 2026-09-01\nretires_feature: true\noutcome: obsolete\naffects: [\"@f/operation:x\"]\n---\n\n## Change\nClosed.\n\n## Why\nDone.\n\n## Acceptance\n- Gone.\n"))
	var found bool
	for _, o := range none {
		if o.Code == "amendment-retirement-no-intents" {
			found = true
		}
	}
	if !found {
		t.Error("a retirement that ends no promise must still be reported")
	}
}

// A version snapshot is held to the same minimum as the founding intent it
// replaces — while the list fields stay clearable, which is the point.
func TestStage1_VersionSnapshotHasFoundingMinimumValidity(t *testing.T) {
	full := func(mut func(*parser.IntentVersion)) *parser.IntentVersion {
		v := &parser.IntentVersion{Title: "A Promise", Goal: "It does something.", Persona: "User"}
		mut(v)
		return v
	}
	cases := []struct {
		name, want string
		v          *parser.IntentVersion
	}{
		{"no title", "no title", full(func(v *parser.IntentVersion) { v.Title = "" })},
		{"no goal", "no goal", full(func(v *parser.IntentVersion) { v.Goal = "" })},
		{"no persona", "no persona", full(func(v *parser.IntentVersion) { v.Persona = "" })},
		{"bad priority", "must be P0, P1 or P2", full(func(v *parser.IntentVersion) { v.Priority = "urgent" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := parser.Amendment{AmendsIntents: []parser.IntentAmendment{
				{Intent: "a-promise", Mode: parser.IntentRevise, Version: tc.v}}}
			problems := a.ValidateIntentTransitions()
			var joined string
			for _, p := range problems {
				joined += p + "\n"
			}
			if !strings.Contains(joined, tc.want) {
				t.Errorf("a version weaker than the founding intent it replaces must be "+
					"reported; got %q", joined)
			}
		})
	}

	// Clearing the list fields stays legal — removing an answered question or
	// a dropped constraint is what a snapshot is for.
	ok := parser.Amendment{AmendsIntents: []parser.IntentAmendment{{
		Intent: "a-promise", Mode: parser.IntentRevise,
		Version: full(func(v *parser.IntentVersion) {
			v.Priority = "P2"
			v.Verify, v.Constraints, v.Objects, v.Questions = nil, nil, nil, nil
		}),
	}}}
	if problems := ok.ValidateIntentTransitions(); len(problems) != 0 {
		t.Errorf("clearing the list fields must stay legal; got %v", problems)
	}
}

// The "same founding minimum" claim rests on two validators agreeing about
// priority — one in the parser for versions, one here for founding intents.
// This repository has repeatedly suffered from duplicated validators, so pin
// the parity rather than trusting that they stay in step.
func TestStage1_VersionAndFoundingPriorityRulesAgree(t *testing.T) {
	for _, p := range []string{"P0", "P1", "P2", "", "P3", "urgent", "p1"} {
		v := &parser.IntentVersion{Title: "A Promise", Goal: "It does something.", Persona: "User", Priority: p}
		a := parser.Amendment{AmendsIntents: []parser.IntentAmendment{
			{Intent: "a-promise", Mode: parser.IntentRevise, Version: v}}}
		versionRejects := false
		for _, prob := range a.ValidateIntentTransitions() {
			if strings.Contains(prob, "P0, P1 or P2") {
				versionRejects = true
			}
		}

		// ValidateIntentsDeep reads from the PATH and ignores its content
		// argument, so the fixture has to be on disk or the validator never
		// sees it — an in-memory fixture silently produced intents-not-readable
		// and made this parity check pass for the wrong reason.
		md := "## A Promise\n\n**Goal**: It does something.\n**Persona**: User\n"
		if p != "" {
			md += "**Priority**: " + p + "\n"
		}
		path := filepath.Join(t.TempDir(), "intents.md")
		if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		foundingRejects := false
		sawIntent := false
		for _, o := range ValidateIntentsDeep(ModeBuild, path, nil) {
			if o.Code == "invalid-priority" {
				foundingRejects = true
			}
			if o.Code == "intents-not-readable" || o.Code == "no-intents" {
				t.Fatalf("fixture: the founding validator never saw an intent (%s)", o.Code)
			}
			sawIntent = true
		}
		_ = sawIntent

		if versionRejects != foundingRejects {
			t.Errorf("priority %q: version validator rejects=%v, founding validator rejects=%v — "+
				"the two rules must agree or the same-minimum claim drifts", p, versionRejects, foundingRejects)
		}
	}
}
