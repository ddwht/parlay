// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validItem = `schema_version: 1
id: 01K4M2QX7N8PZR-rename-collides-with-orphan
kind: gap
priority: P1
title: Initiative rename does not check orphan-feature collision
body: |
  Renaming an initiative does not check for a colliding orphan feature.
captured:
  at: 2026-09-01T10:45:00Z
  by: claude
  run: 20260901T104500Z-4412
  feature: "@parlay-tool/multi-root"
  phase: code
  origin_root: core
about:
  - "@features-and-initiatives-renaming/operation:rename-initiative"
evidence:
  - path: core/internal/commands/move_feature.go
    line: 118
    detail: no namespace check before rename
`

func TestParseBacklog_RoundTrips(t *testing.T) {
	item, err := ParseBacklogBytes("item.yaml", []byte(validItem))
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != KindGap || item.Priority != "P1" {
		t.Errorf("kind/priority lost: %q %q", item.Kind, item.Priority)
	}
	if item.Captured.Run != "20260901T104500Z-4412" || item.Captured.OriginRoot != "core" {
		t.Errorf("capture provenance lost: %+v", item.Captured)
	}
	if len(item.Evidence) != 1 || item.Evidence[0].Line != 118 {
		t.Errorf("evidence lost: %+v", item.Evidence)
	}
	if item.State() != StateOpen {
		t.Errorf("a fresh item is open, got %q", item.State())
	}
	if p := ValidateBacklogShape(item); len(p) != 0 {
		t.Errorf("a valid item produced problems: %+v", p)
	}
}

// Absent priority means UNTRIAGED, never a default. Absence is a fact
// about the record, and it is what lets a listing surface the pile that
// actually needs a person.
func TestParseBacklog_AbsentPriorityIsUntriaged(t *testing.T) {
	item, err := ParseBacklogBytes("item.yaml", []byte(strings.Replace(validItem, "priority: P1\n", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if item.Priority != "" {
		t.Errorf("want empty priority, got %q", item.Priority)
	}
	if p := ValidateBacklogShape(item); len(p) != 0 {
		t.Errorf("an untriaged item is valid: %+v", p)
	}
}

func TestParseBacklog_Refusals(t *testing.T) {
	cases := map[string]string{
		"unknown field":   strings.Replace(validItem, "kind: gap", "knid: gap", 1),
		"unknown kind":    strings.Replace(validItem, "kind: gap", "kind: friction", 1),
		"unknown version": strings.Replace(validItem, "schema_version: 1", "schema_version: 3", 1),
		"unknown event":   validItem + "history:\n  - event: mothballed\n    at: 2026-09-02T00:00:00Z\n    by: x\n",
		"two documents":   validItem + "---\n" + validItem,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseBacklogBytes("item.yaml", []byte(content)); err == nil {
				t.Fatal("expected a refusal")
			}
		})
	}
}

// State is derived, never stored, so it cannot disagree with the events
// that produced it.
func TestBacklogState_DerivesFromHistory(t *testing.T) {
	at := func(k BacklogEventKind, becomes string) BacklogEvent {
		return BacklogEvent{Event: k, Becomes: becomes, Reason: "because", At: "2026-09-02T00:00:00Z", By: "dwht"}
	}
	cases := []struct {
		name    string
		history []BacklogEvent
		want    BacklogState
	}{
		{"no events", nil, StateOpen},
		// A deferral is NOT an answer: the item stays open and the tool
		// keeps reporting it.
		{"deferred only", []BacklogEvent{at(EventDeferred, "")}, StateOpen},
		{"deferred twice", []BacklogEvent{at(EventDeferred, ""), at(EventDeferred, "")}, StateOpen},
		{"promoted", []BacklogEvent{at(EventPromoted, "@new-feature")}, BacklogState(EventPromoted)},
		{"deferred then declined", []BacklogEvent{at(EventDeferred, ""), at(EventDeclined, "")}, BacklogState(EventDeclined)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := &BacklogItem{SchemaVersion: 1, History: tc.history}
			if got := i.State(); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// Deferral attempts accumulate. Two people independently unable to decide
// is a different fact from one attempt overwritten twice.
func TestBacklogDeferrals_Accumulate(t *testing.T) {
	i := &BacklogItem{SchemaVersion: 1, History: []BacklogEvent{
		{Event: EventDeferred, Reason: "first", At: "2026-09-02T00:00:00Z", By: "a"},
		{Event: EventDeferred, Reason: "second", At: "2026-09-03T00:00:00Z", By: "b"},
	}}
	d := i.Deferrals()
	if len(d) != 2 || d[0].By != "a" || d[1].By != "b" {
		t.Fatalf("both attempts must survive: %+v", d)
	}
	if i.State() != StateOpen {
		t.Error("deferrals never close an item")
	}
}

func TestValidateBacklogShape(t *testing.T) {
	base := func() *BacklogItem {
		return &BacklogItem{
			SchemaVersion: 1, ID: "x", Kind: KindGap, Title: "t",
			Captured: BacklogCapture{At: "2026-09-01T10:45:00Z", By: "claude"},
		}
	}
	ev := func(k BacklogEventKind, becomes, reason string) BacklogEvent {
		return BacklogEvent{Event: k, Becomes: becomes, Reason: reason, At: "2026-09-02T00:00:00Z", By: "dwht"}
	}
	cases := []struct {
		name string
		mut  func(*BacklogItem)
		want BacklogProblemKind
	}{
		{"missing title", func(i *BacklogItem) { i.Title = "" }, BacklogProblemMissingTitle},
		{"missing capture", func(i *BacklogItem) { i.Captured.By = "" }, BacklogProblemCaptureMissing},
		{"capture timestamp", func(i *BacklogItem) { i.Captured.At = "last tuesday" }, BacklogProblemTimestamp},
		{"bad priority", func(i *BacklogItem) { i.Priority = "urgent" }, BacklogProblemPriority},
		// `about` is semantic parlay refs and used to accept any string,
		// so a misspelled ref became durable state that every scoped
		// read silently missed.
		{"about is free text", func(i *BacklogItem) { i.About = []string{"the widget thing"} }, BacklogProblemAboutRef},
		{"about has an unknown kind", func(i *BacklogItem) { i.About = []string{"@widget/newkind:x"} }, BacklogProblemAboutRef},
		{"about is too deep", func(i *BacklogItem) { i.About = []string{"@a/b/c"} }, BacklogProblemAboutRef},
		{"about is empty", func(i *BacklogItem) { i.About = []string{""} }, BacklogProblemAboutRef},
		// promoted/amended/folded must say what the work became...
		{"promoted with no becomes", func(i *BacklogItem) {
			i.History = []BacklogEvent{ev(EventPromoted, "", "")}
		}, BacklogProblemBecomesMissing},
		// ...and declined/obsolete must not, because nothing did.
		{"declined with becomes", func(i *BacklogItem) {
			i.History = []BacklogEvent{ev(EventDeclined, "@somewhere", "because")}
		}, BacklogProblemBecomesUnwanted},
		{"declined with no reason", func(i *BacklogItem) {
			i.History = []BacklogEvent{ev(EventDeclined, "", "")}
		}, BacklogProblemReasonMissing},
		{"event attribution", func(i *BacklogItem) {
			i.History = []BacklogEvent{{Event: EventDeferred, Reason: "r", At: "2026-09-02T00:00:00Z"}}
		}, BacklogProblemEventAttribution},
		// At most one terminal, and it must be last: deriving from "the
		// latest terminal" would admit a deferral recorded after a
		// promotion.
		{"event after terminal", func(i *BacklogItem) {
			i.History = []BacklogEvent{ev(EventPromoted, "@f", ""), ev(EventDeferred, "", "r")}
		}, BacklogProblemTerminalNotLast},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := base()
			tc.mut(i)
			var kinds []BacklogProblemKind
			var found bool
			for _, p := range ValidateBacklogShape(i) {
				kinds = append(kinds, p.Kind)
				if p.Kind == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("want %q, got %v", tc.want, kinds)
			}
		})
	}
}

// The registry must agree with the producer, both directions.
func TestBacklogProblems_EmittedKindsMatchTheRegistry(t *testing.T) {
	fixtures := []*BacklogItem{
		{SchemaVersion: 99},
		{SchemaVersion: 1, ID: "x", Kind: KindGap, Title: "t", Priority: "urgent",
			About:    []string{"@widget/newkind:x"},
			Captured: BacklogCapture{At: "nope", By: "c"}},
		{SchemaVersion: 1, ID: "x", Kind: KindGap, Title: "t",
			Captured: BacklogCapture{At: "2026-09-01T00:00:00Z", By: "c"},
			History: []BacklogEvent{
				{Event: EventPromoted, At: "2026-09-02T00:00:00Z", By: "d"},
				{Event: EventDeclined, Becomes: "@x", At: "2026-09-03T00:00:00Z"},
			}},
	}
	emitted := map[BacklogProblemKind]bool{}
	for _, f := range fixtures {
		for _, p := range ValidateBacklogShape(f) {
			emitted[p.Kind] = true
		}
	}
	registered := map[BacklogProblemKind]bool{}
	for _, k := range KnownBacklogProblemKinds() {
		registered[k] = true
	}
	for k := range emitted {
		if !registered[k] {
			t.Errorf("kind %q is emitted but not registered", k)
		}
	}
	for k := range registered {
		if !emitted[k] {
			t.Errorf("kind %q is registered but no fixture produces it", k)
		}
	}
}

// The two shapes `about:` legitimately holds must both survive
// validation, or the check that closes the silent-miss hole opens a
// worse one by refusing correct refs.
func TestValidateAboutRef_AcceptsBothLegitimateShapes(t *testing.T) {
	ok := []string{
		"@widget",
		"@auth/reset-password",
		"@widget/operation:rename",
		"@widget/surface:some-fragment",
		"@widget/infrastructure:some-concept",
		"@widget/domain:some-entity",
		"@auth/reset-password/operation:send",
	}
	for _, ref := range ok {
		if err := ValidateAboutRef(ref); err != nil {
			t.Errorf("ValidateAboutRef(%q) refused a legitimate ref: %v", ref, err)
		}
	}
	bad := []string{"", "widget", "the widget thing", "@widget/newkind:x", "@a/b/c", "@Widget", "@widget/"}
	for _, ref := range bad {
		if err := ValidateAboutRef(ref); err == nil {
			t.Errorf("ValidateAboutRef(%q) accepted an invalid ref", ref)
		}
	}
}

// BareAboutFeature is only claimed to be correct on validated input, and
// this pins that pairing: for everything ValidateAboutRef accepts, the
// feature it yields is the feature a reader would name.
func TestBareAboutFeature_IsCorrectOnValidatedRefs(t *testing.T) {
	cases := map[string]string{
		"@widget":                             "widget",
		"@auth/reset-password":                "auth/reset-password",
		"@widget/operation:rename":            "widget",
		"@widget/domain:some-entity":          "widget",
		"@auth/reset-password/operation:send": "auth/reset-password",
	}
	for ref, want := range cases {
		if err := ValidateAboutRef(ref); err != nil {
			t.Fatalf("fixture %q is not a valid ref: %v", ref, err)
		}
		if got := BareAboutFeature(ref); got != want {
			t.Errorf("BareAboutFeature(%q) = %q, want %q", ref, got, want)
		}
	}
}

// THE v1→v2 BOUNDARY, which exists because v2 admits `fixed`.
//
// A v1 parser rejects unknown event kinds, so a v1 file containing
// `fixed` is not an additive-compatible document — it is a file whose
// declared version contradicts its own contents. The five cases below
// are the whole contract.
func TestBacklogSchema_V1ToV2Boundary(t *testing.T) {
	v1 := "schema_version: 1\nid: x\nkind: gap\ntitle: t\ncaptured:\n  at: 2026-09-01T00:00:00Z\n  by: c\n"
	fixedEvent := "history:\n  - event: fixed\n    reason: corrected in place\n    at: 2026-09-02T00:00:00Z\n    by: d\n"

	t.Run("a v1 file reads, and reading does not rewrite it", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "item.yaml")
		if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(path)

		item, err := ParseBacklogFile(path)
		if err != nil {
			t.Fatalf("an ordinary v1 item was refused: %v", err)
		}
		// IN MEMORY the item is migrated to current — that is what the
		// chain is for, and it is why every consumer can assume one
		// shape. ON DISK nothing changes: a v1 file is upgraded on its
		// next explicit WRITE, never on a read.
		if item.SchemaVersion != BacklogSchemaVersion {
			t.Errorf("the in-memory item is at %d; the chain did not run", item.SchemaVersion)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) {
			t.Errorf("reading rewrote the file:\n before %q\n after  %q", before, after)
		}
	})

	t.Run("v1 plus fixed is refused", func(t *testing.T) {
		_, err := ParseBacklogBytes("item.yaml", []byte(v1+fixedEvent))
		if err == nil {
			t.Fatal("a v1 file carrying a v2-only event was accepted")
		}
		if !strings.Contains(err.Error(), "introduced at schema_version 2") {
			t.Errorf("the refusal does not say why: %v", err)
		}
	})

	t.Run("v2 plus fixed is accepted", func(t *testing.T) {
		v2 := strings.Replace(v1, "schema_version: 1", "schema_version: 2", 1)
		item, err := ParseBacklogBytes("item.yaml", []byte(v2+fixedEvent))
		if err != nil {
			t.Fatalf("a v2 item carrying fixed was refused: %v", err)
		}
		if item.State() != BacklogState(EventFixed) {
			t.Errorf("state = %q, want fixed", item.State())
		}
	})

	t.Run("the chain has no hole", func(t *testing.T) {
		link, ok := backlogMigrations[1]
		if !ok {
			t.Fatal("no v1 link registered; the walk would refuse every v1 file")
		}
		if link.to != 2 {
			t.Errorf("the v1 link advances to %d, want 2", link.to)
		}
	})

	t.Run("every terminal event still requires what it should", func(t *testing.T) {
		if !EventFixed.IsTerminal() {
			t.Error("fixed must close the item")
		}
		if EventFixed.RequiresBecomes() {
			t.Error("fixed must not require becomes: nothing became of it in parlay's terms")
		}
		// Seven values: deferred nonterminal, six terminal.
		all := []BacklogEventKind{EventDeferred, EventPromoted, EventAmended, EventFolded, EventDeclined, EventObsolete, EventFixed}
		terminal := 0
		for _, k := range all {
			if !KnownBacklogEvent(k) {
				t.Errorf("%q is not a known event", k)
			}
			if k.IsTerminal() {
				terminal++
			}
		}
		if len(all) != 7 || terminal != 6 {
			t.Errorf("want seven values with six terminal, got %d with %d terminal", len(all), terminal)
		}
	})
}

// `fixed` obeys the same shape rules as the other reason-carrying
// terminals: a reason is required, and `becomes:` is forbidden because
// nothing became of it.
func TestBacklogShape_FixedRequiresReasonAndForbidsBecomes(t *testing.T) {
	base := func(mut func(*BacklogItem)) *BacklogItem {
		i := &BacklogItem{SchemaVersion: 2, ID: "x", Kind: KindDefect, Title: "t",
			Captured: BacklogCapture{At: "2026-09-01T00:00:00Z", By: "c"}}
		mut(i)
		return i
	}
	ev := func(reason, becomes string) BacklogEvent {
		return BacklogEvent{Event: EventFixed, Reason: reason, Becomes: becomes,
			At: "2026-09-02T00:00:00Z", By: "d"}
	}

	if p := ValidateBacklogShape(base(func(i *BacklogItem) {
		i.History = []BacklogEvent{ev("corrected in place", "")}
	})); len(p) != 0 {
		t.Errorf("a well-formed fixed event was refused: %v", p)
	}
	if p := ValidateBacklogShape(base(func(i *BacklogItem) {
		i.History = []BacklogEvent{ev("", "")}
	})); len(p) == 0 {
		t.Error("fixed with no reason was accepted; a disposition nobody can review later is not one")
	}
	if p := ValidateBacklogShape(base(func(i *BacklogItem) {
		i.History = []BacklogEvent{ev("corrected", "@some-feature")}
	})); len(p) == 0 {
		t.Error("fixed carrying becomes: was accepted; becomes is a typed lifecycle edge and nothing became of a fix")
	}
}
