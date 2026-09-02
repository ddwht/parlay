// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// parkFixture creates a feature and returns the context and its path.
func parkFixture(t *testing.T, slug string) (*config.Context, string) {
	t.Helper()
	setupTestDir(t)
	if err := os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testContext(t)
	if err := runAddFeature(testCommandWithContext(t, cfg), []string{slug}); err != nil {
		t.Fatal(err)
	}
	return cfg, cfg.FeaturePath(slug)
}

func park(t *testing.T, cfg *config.Context, ref, reason, until, by string) error {
	t.Helper()
	parkReason, parkUntil, parkBy = reason, until, by
	t.Cleanup(func() { parkReason, parkUntil, parkBy = "", "", "" })
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	return runPark(cmd, []string{ref})
}

func unpark(t *testing.T, cfg *config.Context, ref, by string) error {
	t.Helper()
	unparkBy = by
	t.Cleanup(func() { unparkBy = "" })
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	return runUnpark(cmd, []string{ref})
}

// ---------------------------------------------------------------------
// Attribution is mandatory.
// ---------------------------------------------------------------------

func TestPark_RequiresReasonAndBy(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")

	if err := park(t, cfg, "widget", "", "", "dwht"); err == nil {
		t.Error("a park with no reason must be refused")
	}
	if err := park(t, cfg, "widget", "superseded", "", ""); err == nil {
		t.Error("a park with no attribution must be refused")
	}
	if err := unpark(t, cfg, "widget", ""); err == nil {
		t.Error("an unpark with no attribution must be refused")
	}
}

// A refused command must leave no file behind. A validation error that
// still writes is worse than one that does not, because the next read
// finds a declaration nobody meant to make.
func TestPark_RefusedCommandWritesNothing(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	_ = park(t, cfg, "widget", "", "", "dwht")
	if _, err := os.Stat(parser.ActivityPath(featurePath)); !os.IsNotExist(err) {
		t.Error("a refused park created a declaration")
	}
}

// ---------------------------------------------------------------------
// Append semantics.
// ---------------------------------------------------------------------

func TestPark_WritesAValidDeclaration(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded by the shipped impl", "after v2 lands", "dwht"); err != nil {
		t.Fatal(err)
	}

	r := readActivity(featurePath)
	if got := r.Resolve(false); got != string(parser.ActivityParked) {
		t.Fatalf("want parked, got %q", got)
	}
	if !strings.Contains(r.Detail(), "superseded") || !strings.Contains(r.Detail(), "until after v2") {
		t.Errorf("detail lost the reason or the until: %q", r.Detail())
	}

	// What was written must satisfy the published validator, not merely
	// round-trip through our own reader.
	content, err := os.ReadFile(parser.ActivityPath(featurePath))
	if err != nil {
		t.Fatal(err)
	}
	if out := validateActivityContent(parser.ActivityPath(featurePath), content); len(out) != 0 {
		t.Errorf("park wrote a declaration its own validator rejects: %+v", out)
	}
}

// Unparking records the reversal; it does not erase the parking. What a
// reader wants months later is that the pause happened and ended.
func TestUnpark_AppendsRatherThanErasing(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := unpark(t, cfg, "widget", "dwht"); err != nil {
		t.Fatal(err)
	}

	activity, declared, err := parser.ParseActivityFile(parser.ActivityPath(featurePath))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if len(activity.History) != 2 {
		t.Fatalf("want 2 events, got %d", len(activity.History))
	}
	if activity.History[0].Event != parser.EventParked || activity.History[0].Reason != "superseded" {
		t.Error("the original parking was not preserved")
	}
	if readActivity(featurePath).Resolve(false) != string(parser.ActivityActive) {
		t.Error("after unparking the feature should read active")
	}
}

// Repeated park/unpark cycles accumulate, each one a fact.
func TestPark_RepeatedCyclesAccumulate(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	for i, step := range []struct {
		park   bool
		reason string
	}{
		{true, "first pause"},
		{false, ""},
		{true, "second pause, different cause"},
		{false, ""},
	} {
		var err error
		if step.park {
			err = park(t, cfg, "widget", step.reason, "", "dwht")
		} else {
			err = unpark(t, cfg, "widget", "dwht")
		}
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	activity, _, err := parser.ParseActivityFile(parser.ActivityPath(featurePath))
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.History) != 4 {
		t.Fatalf("want 4 events, got %d", len(activity.History))
	}
	if activity.History[2].Reason != "second pause, different cause" {
		t.Errorf("the second parking's reason was lost: %q", activity.History[2].Reason)
	}
}

// ---------------------------------------------------------------------
// Transitions that would not transition.
// ---------------------------------------------------------------------

func TestPark_RefusesToParkAnAlreadyParkedFeature(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(parser.ActivityPath(featurePath))

	err := park(t, cfg, "widget", "a different reason", "", "dwht")
	if err == nil {
		t.Fatal("parking an already-parked feature must be refused")
	}
	if !strings.Contains(err.Error(), "already parked") || !strings.Contains(err.Error(), "superseded") {
		t.Errorf("the refusal should name the state and the reason in force: %v", err)
	}
	after, _ := os.ReadFile(parser.ActivityPath(featurePath))
	if !bytes.Equal(before, after) {
		t.Error("a refused park modified the declaration")
	}
}

func TestUnpark_RefusesWhenNotParked(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")

	if err := unpark(t, cfg, "widget", "dwht"); err == nil {
		t.Error("unparking an undeclared feature must be refused")
	}
	if _, err := os.Stat(parser.ActivityPath(featurePath)); !os.IsNotExist(err) {
		t.Error("a refused unpark created a declaration")
	}

	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := unpark(t, cfg, "widget", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := unpark(t, cfg, "widget", "dwht"); err == nil {
		t.Error("unparking an already-active feature must be refused")
	}
}

// ---------------------------------------------------------------------
// An unusable declaration is refused, and left exactly as it was.
// ---------------------------------------------------------------------

func TestPark_RefusesAnUnusableDeclarationWithoutRewritingIt(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"unparseable", "schema_version: 1\nhistroy: [oops]\n"},
		{"newer schema", "schema_version: 3\nhistory: []\n"},
		{"empty history", "schema_version: 1\nhistory: []\n"},
		{"parked with no reason", "schema_version: 1\nhistory:\n  - event: parked\n    at: 2026-01-01T00:00:00Z\n    by: x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, featurePath := parkFixture(t, "widget")
			path := parser.ActivityPath(featurePath)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}

			err := park(t, cfg, "widget", "superseded", "", "dwht")
			if err == nil {
				t.Fatal("park must refuse an unusable declaration")
			}
			if !strings.Contains(err.Error(), "refusing to append") {
				t.Errorf("the refusal should say it is not touching the file: %v", err)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("the file should still exist: %v", readErr)
			}
			if string(after) != tc.content {
				t.Errorf("a refused park rewrote the file:\n want %q\n got  %q", tc.content, after)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Scope and lifecycle.
// ---------------------------------------------------------------------

func TestPark_RefusesAFeatureThatDoesNotExist(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	err := park(t, cfg, "no-such-feature", "superseded", "", "dwht")
	if err == nil {
		t.Fatal("parking a nonexistent feature must be refused")
	}
	if !strings.Contains(err.Error(), "no feature") {
		t.Errorf("the error should say the feature was not found: %v", err)
	}
}

// Parking is a pre-build act. A built feature's promises are frozen and
// its contract is live, so "not now" is no longer something anybody can
// truthfully say about it.
func TestPark_RefusesABuiltFeature(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	buildDir := cfg.BuildPath("widget")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := park(t, cfg, "widget", "superseded", "", "dwht")
	if err == nil {
		t.Fatal("parking a built feature must be refused")
	}
	if !strings.Contains(err.Error(), "pre-build") || !strings.Contains(err.Error(), "retires_feature") {
		t.Errorf("the refusal should name the right alternative: %v", err)
	}
}

// Unparking a built feature is still allowed: a feature parked before the
// build and built since is exactly the case where somebody needs to say
// the pause is over.
func TestUnpark_AllowedOnABuiltFeature(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	buildDir := cfg.BuildPath("widget")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := unpark(t, cfg, "widget", "dwht"); err != nil {
		t.Fatalf("unparking a built feature should be allowed: %v", err)
	}
}

// ---------------------------------------------------------------------
// Concurrency — the reason the ledger store exists.
// ---------------------------------------------------------------------

// Two simultaneous parks: exactly one event, exactly one refusal.
//
// This is the test the previous version got wrong. It appended ten
// identical parked events and called all ten surviving a success — which
// proved the lost-update fix while simultaneously demonstrating that the
// transition invariant was NOT enforced at the write boundary. Ten
// consecutive parks is not a valid history; it is the bug.
//
// The lost-update proof lives where it belongs, in the ledger package's
// generic twenty-append test. What activity needs to prove is stricter:
// that concurrent writers cannot both win a transition.
func TestAppendActivity_ConcurrentParksYieldOneEvent(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			results <- appendEventToStore(cfg, featurePath, parser.ActivityEvent{
				Event:  parser.EventParked,
				Reason: "pause " + strconv.Itoa(n),
				At:     "2026-09-01T11:00:00Z",
				By:     "writer-" + strconv.Itoa(n),
			})
		}(i)
	}
	wg.Wait()
	close(results)

	var succeeded, refused int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, errAlreadyParked):
			refused++
		default:
			t.Fatalf("unexpected failure: %v", err)
		}
	}
	if succeeded != 1 || refused != 1 {
		t.Fatalf("want exactly one winner and one already-parked refusal, got %d/%d", succeeded, refused)
	}

	activity, _, err := parser.ParseActivityFile(parser.ActivityPath(featurePath))
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.History) != 1 {
		t.Fatalf("want exactly 1 event, got %d — a repetition reached the history", len(activity.History))
	}
}

// Two simultaneous unparks after a park: one wins, one is told there is
// nothing to unpark.
func TestAppendActivity_ConcurrentUnparksYieldOneEvent(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			results <- appendEventToStore(cfg, featurePath, parser.ActivityEvent{
				Event: parser.EventUnparked,
				At:    "2026-09-01T12:00:00Z",
				By:    "writer-" + strconv.Itoa(n),
			})
		}(i)
	}
	wg.Wait()
	close(results)

	var succeeded, refused int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, errNotParked):
			refused++
		default:
			t.Fatalf("unexpected failure: %v", err)
		}
	}
	if succeeded != 1 || refused != 1 {
		t.Fatalf("want one winner and one not-parked refusal, got %d/%d", succeeded, refused)
	}

	activity, _, err := parser.ParseActivityFile(parser.ActivityPath(featurePath))
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.History) != 2 {
		t.Fatalf("want park + one unpark, got %d events", len(activity.History))
	}
}

// The stale-snapshot case, made deterministic: the declaration is valid
// when the command's advisory read happens and shape-invalid by the time
// the lock is taken. The locked path must refuse it, and must leave the
// bytes alone.
func TestAppendEventToStore_RefusesADeclarationInvalidatedAfterThePreRead(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	path := parser.ActivityPath(featurePath)

	// Valid at pre-read time.
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	advisory := readActivity(featurePath)
	if _, unusable := advisory.Unusable(); unusable {
		t.Fatal("fixture should start usable")
	}

	// Parseable, but shape-invalid: a parking with no reason. Exactly the
	// shape that slips past a re-parse and would be republished by an
	// append that only re-parsed.
	invalid := `schema_version: 1
history:
  - event: parked
    at: 2026-01-01T00:00:00Z
    by: someone
`
	if err := os.WriteFile(path, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}

	err := appendEventToStore(cfg, featurePath, parser.ActivityEvent{
		Event: parser.EventUnparked,
		At:    "2026-09-01T12:00:00Z",
		By:    "dwht",
	})
	if !errors.Is(err, errUnusableDeclaration) {
		t.Fatalf("want errUnusableDeclaration, got %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != invalid {
		t.Errorf("the refused append rewrote the file:\n want %q\n got  %q", invalid, after)
	}
}

// A same-named regular file is not a feature, and should say so rather
// than surfacing an I/O error about an activity path three steps later.
func TestPark_RefusesWhenTheFeaturePathIsAFile(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	other := cfg.FeaturePath("notafeature")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := park(t, cfg, "notafeature", "superseded", "", "dwht")
	if err == nil {
		t.Fatal("a regular file must not be parkable")
	}
	if !strings.Contains(err.Error(), "not a feature directory") {
		t.Errorf("the error should name the real problem: %v", err)
	}
}

// ---------------------------------------------------------------------
// Status and gate rendering.
// ---------------------------------------------------------------------

// Phase and activity are distinct fields, in JSON and in the human
// listing. Collapsing them into one token would destroy exactly the
// distinction this work exists to make: a feature can be at `dialogs` and
// parked, or at `dialogs` and simply undeclared.
func TestStatusJSON_PhaseAndActivityAreDistinctFields(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "waiting on v2", "", "dwht"); err != nil {
		t.Fatal(err)
	}

	entries := featureEntriesFor(cfg, []string{"widget"})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Phase != PhasePlanned {
		t.Errorf("phase should be unaffected by parking: %q", e.Phase)
	}
	if e.Activity != string(parser.ActivityParked) {
		t.Errorf("want activity parked, got %q", e.Activity)
	}
	if !strings.Contains(e.ActivityDetail, "waiting on v2") {
		t.Errorf("detail lost the reason: %q", e.ActivityDetail)
	}
}

// An invalid present declaration renders `unavailable` with its fault,
// and never falls through to active or unclassified.
func TestStatus_InvalidDeclarationRendersUnavailable(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := os.WriteFile(parser.ActivityPath(featurePath), []byte("schema_version: 1\nhistory: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := featureEntriesFor(cfg, []string{"widget"})[0]
	if e.Activity != ActivityUnavailable {
		t.Fatalf("want %q, got %q", ActivityUnavailable, e.Activity)
	}
	if e.ActivityDetail == "" {
		t.Error("unavailable must carry the fault that made it unavailable")
	}
	if cell := activityCell(e); !strings.HasPrefix(cell, "unavailable") {
		t.Errorf("the human cell should lead with the state: %q", cell)
	}
}

// `active` prints as nothing in the human column. It is the ordinary case
// and repeating it forty times buries the lines somebody must act on.
func TestActivityCell_ActiveIsSilent(t *testing.T) {
	if cell := activityCell(featureEntry{Activity: string(parser.ActivityActive)}); cell != "" {
		t.Errorf("active should print as nothing, got %q", cell)
	}
	if cell := activityCell(featureEntry{Activity: ""}); cell != "" {
		t.Errorf("an absent activity should print as nothing, got %q", cell)
	}
}

// A parked feature that has since acquired artifacts is still parked —
// a declaration outranks observation — but the record has stopped being
// true and the listing must say so.
func TestStatus_StaleParkingIsSurfaced(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	// Work resumed: an artifact now exists.
	if err := os.WriteFile(filepath.Join(featurePath, "capabilities.yaml"), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := featureEntriesFor(cfg, []string{"widget"})[0]
	if e.Activity != string(parser.ActivityParked) {
		t.Errorf("the declaration still outranks observation: want parked, got %q", e.Activity)
	}
	if !e.ActivityStale {
		t.Fatal("a parked feature with artifacts must be marked stale")
	}
	if !strings.Contains(activityCell(e), "stale") {
		t.Errorf("the human cell must say stale: %q", activityCell(e))
	}
}

// THE DEAD-CODE CASE.
//
// A stale parking has observed pipeline activity; observed activity is
// what earns a boundary; so a stale parking is ALWAYS on a gated row and
// never on a skipped one. An earlier cut carried activity only on skipped
// rows, which made the staleness field unreachable — gate could not have
// reported the condition it was meant to report. This pins that stale
// parkings appear on gated rows.
func TestGate_StaleParkingAppearsOnAGatedRow(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featurePath, "capabilities.yaml"), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rows := sweepFeatures(cfg, "test", []string{"widget"})
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.Skipped {
		t.Fatal("a feature with artifacts should have a boundary")
	}
	if !r.ActivityStale {
		t.Error("the stale parking must be carried on the gated row")
	}
}

// Gate's skipped bucket splits by disposition, and the underlying
// pass/blocked semantics for gated features are untouched.
func TestGateSummary_SplitsSkippedWithoutChangingBoundarySemantics(t *testing.T) {
	byActivity := map[string]int{
		string(parser.ActivityParked):       14,
		string(parser.ActivityUnclassified): 3,
	}
	got := gateSummaryLine(7, 23, 0, 0, byActivity)
	for _, want := range []string{"7 passed", "23 blocked", "14 parked", "3 unclassified"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %s", want, got)
		}
	}
	// Unclassified last: it is the only bucket that is a call to action.
	if strings.Index(got, "unclassified") < strings.Index(got, "parked") {
		t.Errorf("unclassified should come after parked: %s", got)
	}
	if stale := gateSummaryLine(7, 23, 2, 0, byActivity); !strings.Contains(stale, "2 with a stale parking") {
		t.Errorf("stale parkings must be counted: %s", stale)
	}
}

// The first-run rule from §12: nothing is auto-migrated to parked. A
// project with no declarations reports every ungated feature as
// unclassified, because that is what is true.
func TestGate_FirstRunReportsUnclassifiedNotParked(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	rows := sweepFeatures(cfg, "test", []string{"widget"})
	if rows[0].Activity != string(parser.ActivityUnclassified) {
		t.Errorf("an undeclared feature must be unclassified, got %q", rows[0].Activity)
	}
	if v := gateActivityVerdict(rows[0]); !strings.Contains(v, "no disposition recorded") {
		t.Errorf("the verdict should say what is missing: %q", v)
	}
}

// ---------------------------------------------------------------------
// Activity findings on GATED rows.
//
// The failure these pin: a built feature with a broken activity.yaml
// printed a plain `pass`, contributed to no count, and left the fault
// invisible to CI — a committed declaration parlay itself refuses to
// mutate, passing a gate.
// ---------------------------------------------------------------------

// Gate's activity findings must carry the SAME published codes
// `parlay validate --type activity` produces for the same bytes. Two
// commands publishing different codes for one file is the drift that
// exporting the mapping exists to prevent.
func TestActivityFindings_ParityWithValidateActivity(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"parse refusal", "schema_version: 1\nhistroy: [x]\n"},
		{"empty history", "schema_version: 1\nhistory: []\n"},
		{"parked without reason", "schema_version: 1\nhistory:\n  - event: parked\n    at: 2026-01-01T00:00:00Z\n    by: x\n"},
		{"multi-problem", "schema_version: 1\nhistory:\n  - event: parked\n    at: last tuesday\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, featurePath := parkFixture(t, "widget")
			path := parser.ActivityPath(featurePath)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_ = cfg

			fromGate := map[string]bool{}
			for _, f := range activityFindings(readActivity(featurePath), false) {
				fromGate[f.Code] = true
			}
			fromValidator := map[string]bool{}
			for _, o := range validateActivityContent(path, []byte(tc.content)) {
				fromValidator[o.Code] = true
			}

			if len(fromGate) == 0 {
				t.Fatal("gate produced no findings for an invalid declaration")
			}
			// Fixes travel with codes. A finding that names a problem and
			// not its remedy sends the reader back to the schema to work
			// out what the tool already knew.
			gateFixes := map[string]string{}
			for _, f := range activityFindings(readActivity(featurePath), false) {
				if strings.TrimSpace(f.Fix) == "" {
					t.Errorf("finding %q carries no fix", f.Code)
				}
				gateFixes[f.Code] = f.Fix
			}
			for _, o := range validateActivityContent(path, []byte(tc.content)) {
				if got, ok := gateFixes[o.Code]; ok && got != o.Fix {
					t.Errorf("code %q: gate and validate publish different fixes\n gate:     %q\n validate: %q", o.Code, got, o.Fix)
				}
			}
			for code := range fromValidator {
				if !fromGate[code] {
					t.Errorf("validate publishes %q, gate does not: gate=%v", code, sortedCodes(fromGate))
				}
			}
			for code := range fromGate {
				if !fromValidator[code] {
					t.Errorf("gate publishes %q, validate does not: validate=%v", code, sortedCodes(fromValidator))
				}
			}
		})
	}
}

// An empty history PARSED. Reporting it as not-parseable was the
// collapse that undid the typed routing.
func TestActivityFindings_EmptyHistoryIsNotReportedAsUnparseable(t *testing.T) {
	_, featurePath := parkFixture(t, "widget")
	if err := os.WriteFile(parser.ActivityPath(featurePath), []byte("schema_version: 1\nhistory: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := activityFindings(readActivity(featurePath), false)
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %+v", findings)
	}
	if findings[0].Code == agent.ActivityCodeNotParseable {
		t.Errorf("an empty history parsed fine; it must not be reported as unparseable")
	}
	if findings[0].Code != "activity-declaration-incomplete" {
		t.Errorf("want activity-declaration-incomplete, got %q", findings[0].Code)
	}
}

// A multi-problem file carries every finding, not just the first: a
// reader who fixes what they were shown should not pay the same cost
// again for the next one.
func TestActivityFindings_MultiProblemCarriesEveryFinding(t *testing.T) {
	_, featurePath := parkFixture(t, "widget")
	if err := os.WriteFile(parser.ActivityPath(featurePath),
		[]byte("schema_version: 1\nhistory:\n  - event: parked\n    at: last tuesday\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := activityFindings(readActivity(featurePath), false)
	if len(findings) < 3 {
		t.Fatalf("want every fault (bad timestamp, missing by, missing reason), got %+v", findings)
	}
}

func TestActivityFindings_HealthyDeclarationsProduceNothing(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if f := activityFindings(readActivity(featurePath), false); len(f) != 0 {
		t.Errorf("an undeclared feature has no findings, got %+v", f)
	}
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	if f := activityFindings(readActivity(featurePath), false); len(f) != 0 {
		t.Errorf("an ordinary parking is not a finding, got %+v", f)
	}
	stale := activityFindings(readActivity(featurePath), true)
	if len(stale) != 1 || stale[0].Code != codeParkedFeatureAdvanced {
		t.Fatalf("want %s, got %+v", codeParkedFeatureAdvanced, stale)
	}
	// The remedy must be in Fix, not smuggled into Message: a display
	// showing only Message would otherwise be the sole place a reader
	// could learn what to do.
	if strings.TrimSpace(stale[0].Fix) == "" {
		t.Error("a stale parking must carry an explicit fix")
	}
	if !strings.Contains(stale[0].Fix, "unpark") {
		t.Errorf("the fix should name the action: %q", stale[0].Fix)
	}
}

// Every activity finding, from any shape of broken declaration, carries a
// non-empty fix.
func TestActivityFindings_EveryFindingCarriesAFix(t *testing.T) {
	for _, content := range []string{
		"schema_version: 1\nhistroy: [x]\n",
		"schema_version: 3\nhistory: []\n",
		"schema_version: 1\nhistory: []\n",
		"schema_version: 1\nhistory:\n  - event: parked\n    at: last tuesday\n",
		"schema_version: 1\nhistory:\n  - event: unparked\n    until: someday\n    at: 2026-01-01T00:00:00Z\n    by: x\n",
	} {
		_, featurePath := parkFixture(t, "widget")
		if err := os.WriteFile(parser.ActivityPath(featurePath), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		findings := activityFindings(readActivity(featurePath), false)
		if len(findings) == 0 {
			t.Errorf("no findings for %q", content)
		}
		for _, f := range findings {
			if strings.TrimSpace(f.Fix) == "" {
				t.Errorf("finding %q from %q carries no fix", f.Code, content)
			}
		}
	}
}

func sortedCodes(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// A PASSING gated feature must still surface an unusable declaration, in
// the verdict and in the summary, exactly once.
func TestPrintGateSweep_PassingRowStillShowsAnUnusableDeclaration(t *testing.T) {
	rows := []gateSweepRow{{
		Root: "core", Feature: "widget", Phase: "done", Stage: "done", Passed: true,
		Activity: ActivityUnavailable, ActivityDetail: "history is empty",
		ActivityFindings: []gateBlocker{{Code: "activity-declaration-incomplete", Message: "history is empty"}},
	}}
	cmd := testCommandWithContext(t, testContext(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	printGateSweep(cmd, rows)

	got := out.String()
	if !strings.Contains(got, "pass") {
		t.Error("the boundary verdict must survive")
	}
	if !strings.Contains(got, "activity-declaration-incomplete") {
		t.Errorf("the published code must appear on a passing row: %s", got)
	}
	if !strings.Contains(got, "1 with an unusable declaration") {
		t.Errorf("the summary must count it: %s", got)
	}
	if strings.Count(got, "1 with an unusable declaration") != 1 {
		t.Errorf("counted more than once: %s", got)
	}
}

// A stale parking on a passing row, same contract.
func TestPrintGateSweep_PassingRowStillShowsAStaleParking(t *testing.T) {
	rows := []gateSweepRow{{
		Root: "core", Feature: "widget", Phase: "done", Stage: "done", Passed: true,
		Activity: string(parser.ActivityParked), ActivityDetail: "superseded", ActivityStale: true,
		ActivityFindings: []gateBlocker{{Code: codeParkedFeatureAdvanced, Message: "parked, but the feature has since acquired artifacts"}},
	}}
	cmd := testCommandWithContext(t, testContext(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	printGateSweep(cmd, rows)

	got := out.String()
	if !strings.Contains(got, codeParkedFeatureAdvanced) {
		t.Errorf("the published code must appear: %s", got)
	}
	if !strings.Contains(got, "1 with a stale parking") {
		t.Errorf("the summary must count it: %s", got)
	}
}

// The fault must not hide behind BLOCKED either.
func TestPrintGateSweep_BlockedRowStillShowsTheActivityFault(t *testing.T) {
	rows := []gateSweepRow{{
		Root: "core", Feature: "widget", Phase: "build", Stage: "build",
		Blockers: []gateBlocker{{Code: "some-blocker", Message: "unrelated"}},
		Activity: ActivityUnavailable, ActivityDetail: "history is empty",
		ActivityFindings: []gateBlocker{{Code: "activity-declaration-incomplete", Message: "history is empty"}},
	}}
	cmd := testCommandWithContext(t, testContext(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	printGateSweep(cmd, rows)

	got := out.String()
	if !strings.Contains(got, "BLOCKED (1)") {
		t.Error("the boundary count must not be inflated by activity findings")
	}
	if !strings.Contains(got, "activity-declaration-incomplete") {
		t.Errorf("the activity fault must not hide behind BLOCKED: %s", got)
	}
}

// Skipped rows already say the activity in their verdict; appending would
// repeat it. But they must still be counted.
func TestPrintGateSweep_SkippedRowCountsWithoutRepeating(t *testing.T) {
	rows := []gateSweepRow{{
		Root: "core", Feature: "widget", Phase: "planned", Stage: "—", Skipped: true, Passed: true,
		Activity: ActivityUnavailable, ActivityDetail: "history is empty",
		ActivityFindings: []gateBlocker{{Code: "activity-declaration-incomplete", Message: "history is empty"}},
	}}
	cmd := testCommandWithContext(t, testContext(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	printGateSweep(cmd, rows)

	got := out.String()
	if strings.Count(got, "activity-declaration-incomplete") > 1 {
		t.Errorf("the code should not be repeated on a skipped row: %s", got)
	}
	if n := strings.Count(got, "1 with an unusable declaration"); n != 1 {
		t.Errorf("want the phrase exactly once, got %d: %s", n, got)
	}
}

// Integration: a real feature with a broken declaration produces the
// finding through the real sweep.
func TestSweepFeatures_CarriesActivityFindings(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := os.WriteFile(parser.ActivityPath(featurePath), []byte("schema_version: 1\nhistroy: [x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rows := sweepFeatures(cfg, "test", []string{"widget"})
	if len(rows[0].ActivityFindings) != 1 {
		t.Fatalf("want one finding, got %+v", rows[0].ActivityFindings)
	}
	if rows[0].ActivityFindings[0].Code != agent.ActivityCodeNotParseable {
		t.Errorf("wrong code: %s", rows[0].ActivityFindings[0].Code)
	}
}

// Hand-authored units carry no activity: their code is already written,
// so there is no work to pause. Absence means "not applicable", never
// "active".
func TestFeatureEntries_UnitsCarryNoActivity(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	entries := featureEntriesFor(cfg, []string{"widget"})
	for _, e := range entries {
		if e.Kind == KindHandAuthored && e.Activity != "" {
			t.Errorf("a unit should carry no activity, got %q", e.Activity)
		}
		if e.Kind != KindHandAuthored && e.Activity == "" {
			t.Errorf("an ordinary feature must always carry a computed activity: %+v", e)
		}
	}
}

// The user-visible exit contract, driven through runGateAll rather than
// through printGateSweep.
//
// The loop that decides the exit code is short and looks obviously
// correct, which is exactly why it deserves reachability coverage: a
// finding that renders beautifully and returns zero is a CI gate that
// reports health it never verified.
func TestRunGateAll_ActivityFindingAloneIsNonZero(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	// A declaration that parses and fails shape validation. Nothing else
	// about this feature is wrong — it is too early to have a boundary,
	// so without the activity finding the sweep would be clean.
	if err := os.WriteFile(parser.ActivityPath(featurePath), []byte("schema_version: 1\nhistory: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := gateAllFlag
	gateAllFlag = true
	t.Cleanup(func() { gateAllFlag = prev })

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := runGateAll(cmd, nil)
	if err == nil {
		t.Fatalf("an activity finding alone must make the sweep non-zero\noutput: %s", out.String())
	}
	var exit *ExitCodeError
	if !errors.As(err, &exit) {
		t.Fatalf("want an ExitCodeError, got %T: %v", err, err)
	}
	if !strings.Contains(out.String(), "activity-declaration-incomplete") {
		t.Errorf("the published code must reach the user: %s", out.String())
	}
}

// The complement: a clean project exits zero, so the test above is
// measuring the finding rather than something incidental to the fixture.
func TestRunGateAll_CleanProjectIsZero(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	prev := gateAllFlag
	gateAllFlag = true
	t.Cleanup(func() { gateAllFlag = prev })

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := runGateAll(cmd, nil); err != nil {
		t.Fatalf("a project with no findings must exit zero, got %v\noutput: %s", err, out.String())
	}
}

// The stale-parking remedy must be executable against the real state
// machine, not merely plausible prose.
//
// `park` refuses a repeated park while the feature is still parked, so
// unparking is a prerequisite rather than an alternative; and once build
// outputs exist park refuses even after unparking, because parking is a
// pre-build act. A remedy the tool would reject is worse than none — the
// reader spends the attempt before finding out.
func TestActivityFindings_StaleFixIsExecutable(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	fix := activityFindings(readActivity(featurePath), true)[0].Fix

	if !strings.Contains(fix, "unpark") {
		t.Errorf("the fix must name unparking as the first step: %q", fix)
	}
	// It must not read as an alternative to unparking.
	if strings.Contains(fix, "), or park it again") {
		t.Errorf("re-parking is not an alternative to unparking; park refuses a repeated park: %q", fix)
	}
	if !strings.Contains(fix, "pre-build") {
		t.Errorf("the fix must say re-parking depends on being pre-build: %q", fix)
	}
	if !strings.Contains(fix, "amendment") {
		t.Errorf("the fix must name the built-feature alternative: %q", fix)
	}

	// And the state machine the fix describes is the one the commands
	// actually implement: unpark succeeds, and a re-park after build
	// outputs exist is refused with the amendment pointer.
	if err := unpark(t, cfg, "widget", "dwht"); err != nil {
		t.Fatalf("the fix's first step must work: %v", err)
	}
	buildDir := cfg.BuildPath("widget")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := park(t, cfg, "widget", "still superseded", "", "dwht")
	if err == nil {
		t.Fatal("a built feature must not be re-parkable, which is why the fix names the amendment")
	}
	if !strings.Contains(err.Error(), "retires_feature") {
		t.Errorf("the refusal should point at the same alternative the fix does: %v", err)
	}
}

// ---------------------------------------------------------------------
// activate, and the v1 → v2 upgrade on write.
// ---------------------------------------------------------------------

func activate(t *testing.T, cfg *config.Context, ref, by string) error {
	t.Helper()
	activateBy = by
	t.Cleanup(func() { activateBy = "" })
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	return runActivate(cmd, []string{ref})
}

// THE COMPLETION CASE. Declaring a feature active must actually resolve
// it: an earlier cut offered "leave it undeclared", which left the
// feature unclassified and returned it next sitting. A triage whose only
// options are "park it" and "do nothing" can never reduce its backlog.
func TestActivate_ResolvesAnUndeclaredFeature(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := activate(t, cfg, "widget", "dwht"); err != nil {
		t.Fatal(err)
	}
	if got := readActivity(featurePath).Resolve(false); got != string(parser.ActivityActive) {
		t.Fatalf("want active, got %q", got)
	}

	// It writes `activated`, not `unparked`: the feature was never
	// paused, and a history saying otherwise would be false.
	a, _, err := parser.ParseActivityFile(parser.ActivityPath(featurePath))
	if err != nil {
		t.Fatal(err)
	}
	if a.History[0].Event != parser.EventActivated {
		t.Errorf("want an activated event, got %q", a.History[0].Event)
	}
	if _, ok := a.LatestParking(); ok {
		t.Error("an activated feature must have no parking in force")
	}
}

func TestActivate_RequiresAttribution(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	if err := activate(t, cfg, "widget", ""); err == nil {
		t.Error("activating without --by must be refused")
	}
}

// Transition rules: parked directs to unpark; already-active is a no-op
// and refused.
func TestActivate_TransitionRules(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	err := activate(t, cfg, "widget", "dwht")
	if err == nil {
		t.Fatal("activating a parked feature must be refused")
	}
	if !strings.Contains(err.Error(), "unpark") {
		t.Errorf("the refusal must point at unpark: %v", err)
	}

	if err := unpark(t, cfg, "widget", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := activate(t, cfg, "widget", "dwht"); err == nil {
		t.Error("activating an already-active feature must be refused")
	}
}

// Appending to a v1 declaration upgrades it on save. The upgrade lands on
// a write somebody asked for, never on a read.
func TestAppend_UpgradesAV1DeclarationOnSave(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	path := parser.ActivityPath(featurePath)
	v1 := "schema_version: 1\nhistory:\n  - event: parked\n    reason: superseded\n    at: 2026-04-18T09:12:00Z\n    by: dwht\n"
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	// A read leaves it alone.
	_ = readActivity(featurePath)
	if after, _ := os.ReadFile(path); string(after) != v1 {
		t.Fatal("reading upgraded the file")
	}

	// A write brings it forward.
	if err := unpark(t, cfg, "widget", "dwht"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "schema_version: 2") {
		t.Errorf("the append should have saved v2:\n%s", after)
	}
	if !strings.Contains(string(after), "superseded") {
		t.Error("the v1 history must survive the upgrade")
	}
}

// End to end through the review: both decisions must reduce Remaining and
// stop the subject returning without exclusions.
func TestNextActivityReview_BothDecisionsCompleteTheTriage(t *testing.T) {
	for _, decision := range []string{"park", "activate"} {
		t.Run(decision, func(t *testing.T) {
			cfg, _ := parkFixture(t, "widget")
			before := review(t, cfg)
			if before.Summary.Remaining != 1 || before.Subject == nil {
				t.Fatalf("expected one pending subject, got %+v", before.Summary)
			}

			var err error
			if decision == "park" {
				err = park(t, cfg, "widget", "superseded", "", "dwht")
			} else {
				err = activate(t, cfg, "widget", "dwht")
			}
			if err != nil {
				t.Fatalf("%s: %v", decision, err)
			}

			after := review(t, cfg)
			if after.Summary.Remaining != 0 {
				t.Errorf("%s did not reduce the remaining count: %d", decision, after.Summary.Remaining)
			}
			if after.Subject != nil {
				t.Errorf("%s: the subject returned without exclusions: %+v", decision, after.Subject)
			}
		})
	}
}

// The append-only guard must catch a caller that rewrites LOCKED
// history, not merely one that mishandles its own append.
//
// The snapshot was taken immediately before the append rather than
// immediately after parsing the locked bytes, so anything a future
// caller did in between was inside the baseline and got blessed. The
// schema says this guard protects against future callers; a baseline
// taken after those callers would have run protects against nobody.
func TestPark_RewritingLockedHistoryIsRefused(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	if err := park(t, cfg, "@widget", "waiting on the upstream decision", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	before := readActivity(cfg.FeaturePath("widget"))

	// A future caller that edits an existing event before appending.
	activityMutateBeforeAppend = func(a *parser.Activity) {
		if len(a.History) > 0 {
			a.History[0].Reason = "a reason nobody gave"
		}
	}
	t.Cleanup(func() { activityMutateBeforeAppend = nil })

	err := unpark(t, cfg, "@widget", "dwht")
	if err == nil {
		t.Fatal("a caller rewrote locked history and the guard blessed it")
	}
	if !strings.Contains(err.Error(), agent.CodeActivityHistoryUpdateForbidden) {
		t.Errorf("the refusal does not carry the published code: %v", err)
	}
	after := readActivity(cfg.FeaturePath("widget"))
	if after.Detail() != before.Detail() {
		t.Errorf("the refused mutation still reached disk: %q -> %q", before.Detail(), after.Detail())
	}

	// And an ordinary append still works.
	activityMutateBeforeAppend = nil
	if err := unpark(t, cfg, "@widget", "dwht"); err != nil {
		t.Fatalf("an ordinary append was refused: %v", err)
	}
}
