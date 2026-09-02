// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package parser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parkedAt(at, reason string) ActivityEvent {
	return ActivityEvent{Event: EventParked, Reason: reason, At: at, By: "dwht"}
}

func unparkedAt(at string) ActivityEvent {
	return ActivityEvent{Event: EventUnparked, At: at, By: "dwht"}
}

// ---------------------------------------------------------------------
// Derivation — the latest event wins.
// ---------------------------------------------------------------------

func TestCurrent_DerivesFromTheLatestEvent(t *testing.T) {
	cases := []struct {
		name    string
		history []ActivityEvent
		want    ActivityState
	}{
		{"no events", nil, ActivityUnclassified},
		{"parked", []ActivityEvent{parkedAt("2026-04-18T09:12:00Z", "superseded")}, ActivityParked},
		{"parked then unparked", []ActivityEvent{
			parkedAt("2026-04-18T09:12:00Z", "superseded"),
			unparkedAt("2026-09-01T11:00:00Z"),
		}, ActivityActive},
		{"re-parked after unparking", []ActivityEvent{
			parkedAt("2026-04-18T09:12:00Z", "superseded"),
			unparkedAt("2026-09-01T11:00:00Z"),
			parkedAt("2026-09-05T08:00:00Z", "blocked on adapter-set v2"),
		}, ActivityParked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Activity{SchemaVersion: ActivitySchemaVersion, History: tc.history}
			if got := a.Current(); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCurrent_NilIsUnclassified(t *testing.T) {
	var a *Activity
	if got := a.Current(); got != ActivityUnclassified {
		t.Errorf("want %q, got %q", ActivityUnclassified, got)
	}
}

// An unknown event kind is REFUSED at parse, not skipped.
//
// An earlier cut skipped it and derived from the last kind it recognised.
// That is worse than failing: a feature parked and later retired by a
// newer parlay would report `parked` — confident, stale and wrong. An old
// binary must decline a vocabulary it cannot know, which is what
// schema_version and a closed enum are for.
func TestParse_RefusesAnUnknownEventKind(t *testing.T) {
	_, err := ParseActivityBytes("activity.yaml", []byte(`schema_version: 1
history:
  - event: parked
    reason: superseded
    at: 2026-04-18T09:12:00Z
    by: dwht
  - event: mothballed
    at: 2026-09-01T11:00:00Z
    by: future-parlay
`))
	if err == nil {
		t.Fatal("expected refusal of an unknown event kind")
	}
	if !strings.Contains(err.Error(), "mothballed") {
		t.Errorf("error should name the offending kind: %v", err)
	}
}

// A newer schema_version is refused before any derivation happens.
func TestParse_RefusesAnUnknownSchemaVersion(t *testing.T) {
	_, err := ParseActivityBytes("activity.yaml", []byte(`schema_version: 3
history:
  - event: parked
    reason: superseded
    at: 2026-04-18T09:12:00Z
    by: dwht
`))
	if err == nil {
		t.Fatal("expected refusal of an unknown schema_version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("error should name the version: %v", err)
	}
}

// A misspelled field must not parse into a different, well-formed record.
// `histroy:` would otherwise yield an empty history and `reasno:` a
// parking with no reason — both valid-looking, neither what was written.
func TestParse_RefusesUnknownFields(t *testing.T) {
	cases := map[string]string{
		"unknown top-level field": `schema_version: 1
histroy:
  - event: parked
    reason: superseded
    at: 2026-04-18T09:12:00Z
    by: dwht
`,
		"unknown event field": `schema_version: 1
history:
  - event: parked
    reasno: superseded
    at: 2026-04-18T09:12:00Z
    by: dwht
`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseActivityBytes("activity.yaml", []byte(content)); err == nil {
				t.Fatal("expected refusal of an unknown field")
			}
		})
	}
}

// A second document would be silently ignored, and a declaration nobody
// reads is indistinguishable from one nobody wrote.
func TestParse_RefusesMoreThanOneDocument(t *testing.T) {
	_, err := ParseActivityBytes("activity.yaml", []byte(`schema_version: 1
history:
  - event: parked
    reason: superseded
    at: 2026-04-18T09:12:00Z
    by: dwht
---
schema_version: 1
history: []
`))
	if err == nil {
		t.Fatal("expected refusal of a multi-document file")
	}
}

// ---------------------------------------------------------------------
// The reason must travel with the state.
// ---------------------------------------------------------------------

func TestLatestParking_CarriesTheReasonInForce(t *testing.T) {
	a := &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{
		parkedAt("2026-04-18T09:12:00Z", "first reason"),
		unparkedAt("2026-06-01T09:00:00Z"),
		parkedAt("2026-09-05T08:00:00Z", "the reason actually in force"),
	}}
	got, ok := a.LatestParking()
	if !ok {
		t.Fatal("expected a parking in force")
	}
	if got.Reason != "the reason actually in force" {
		t.Errorf("got the wrong parking: %q", got.Reason)
	}
}

func TestLatestParking_AbsentOnceUnparked(t *testing.T) {
	a := &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{
		parkedAt("2026-04-18T09:12:00Z", "superseded"),
		unparkedAt("2026-09-01T11:00:00Z"),
	}}
	if _, ok := a.LatestParking(); ok {
		t.Error("an unparked feature has no parking in force")
	}
}

// ---------------------------------------------------------------------
// ResolveActivityState — the contextual reading status actually needs.
// ---------------------------------------------------------------------

func TestResolveActivityState(t *testing.T) {
	parked := &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{parkedAt("2026-04-18T09:12:00Z", "superseded")}}
	unparked := &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{unparkedAt("2026-09-01T11:00:00Z")}}

	cases := []struct {
		name             string
		activity         *Activity
		declared         bool
		observedBoundary bool
		want             ActivityState
	}{
		{"declared parked", parked, true, false, ActivityParked},
		// A declaration outranks observation: a parked feature that still
		// passes a gate is parked, because somebody said so.
		{"declared parked, boundary observed", parked, true, true, ActivityParked},
		{"declared unparked", unparked, true, false, ActivityActive},
		{"no file, no boundary", nil, false, false, ActivityUnclassified},
		// Observed pipeline activity answers the question the missing
		// declaration was going to ask.
		{"no file, boundary observed", nil, false, true, ActivityActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveActivityState(tc.activity, tc.declared, tc.observedBoundary)
			if got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Parsing.
// ---------------------------------------------------------------------

func TestParseActivityFile_MissingIsUndeclaredNotEmpty(t *testing.T) {
	dir := t.TempDir()
	a, declared, err := ParseActivityFile(ActivityPath(dir))
	if err != nil {
		t.Fatalf("a missing declaration is not an error: %v", err)
	}
	if declared {
		t.Error("declared should be false when no file exists")
	}
	if a != nil {
		t.Errorf("activity should be nil, got %+v", a)
	}
}

func TestParseActivityFile_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	content := `schema_version: 1
history:
  - event: parked
    reason: "Superseded by the shipped implementation; keeping intents for reference."
    until: "after adapter-set v2 lands"
    at: 2026-04-18T09:12:00Z
    by: dwht
`
	if err := os.WriteFile(filepath.Join(dir, ActivityFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a, declared, err := ParseActivityFile(ActivityPath(dir))
	if err != nil || !declared {
		t.Fatalf("declared=%v err=%v", declared, err)
	}
	if a.Current() != ActivityParked {
		t.Errorf("want parked, got %q", a.Current())
	}
	p, ok := a.LatestParking()
	if !ok {
		t.Fatal("expected a parking in force")
	}
	if p.Until != "after adapter-set v2 lands" {
		t.Errorf("until did not survive: %q", p.Until)
	}
	if p.By != "dwht" {
		t.Errorf("by did not survive: %q", p.By)
	}
}

// A declaration that exists but cannot be parsed must report the failure,
// not quietly read as undeclared — that would turn a broken file into a
// silent "nobody has said anything", which is a different fact.
// A path that exists but cannot be READ is a fault, not silence. Reading
// it as "nobody declared anything" would convert a fault into a quieter,
// different fact — the same mistake as reading a malformed file as
// silence. A directory in the file's place is the portable way to
// provoke a non-not-exist read error, including under root.
func TestParseActivityFile_UnreadablePathReportsDeclared(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(ActivityPath(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	_, declared, err := ParseActivityFile(ActivityPath(dir))
	if err == nil {
		t.Fatal("expected a read error")
	}
	if !declared {
		t.Error("a path that exists but cannot be read must report declared=true")
	}
}

func TestParseActivityFile_MalformedReportsDeclaredAndErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ActivityFile), []byte("history: [oh dear\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, declared, err := ParseActivityFile(ActivityPath(dir))
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if !declared {
		t.Error("a malformed file still exists; declared must be true")
	}
}

// ---------------------------------------------------------------------
// Shape validation.
// ---------------------------------------------------------------------

func TestValidateActivityShape(t *testing.T) {
	cases := []struct {
		name  string
		in    *Activity
		want  string
		kind  ActivityProblemKind
		clean bool
	}{
		{
			name:  "valid parked",
			in:    &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{parkedAt("2026-04-18T09:12:00Z", "superseded")}},
			clean: true,
		},
		{
			name: "unknown schema version",
			in:   &Activity{SchemaVersion: 3, History: []ActivityEvent{parkedAt("2026-04-18T09:12:00Z", "x")}},
			want: "schema_version",
		},
		{
			name: "empty history declares nothing",
			in:   &Activity{SchemaVersion: 1},
			want: "history is empty",
		},
		{
			name: "kinds are typed, not inferred from prose",
			in:   &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: EventParked, At: "2026-01-01T00:00:00Z", By: "y"}}},
			kind: ProblemParkedWithoutReason,
		},
		{
			name: "at is not RFC3339",
			in:   &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: EventUnparked, At: "last tuesday", By: "y"}}},
			want: "not RFC3339",
		},
		{
			name: "unknown event kind",
			in:   &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: "mothballed", At: "2026-01-01T00:00:00Z", By: "y"}}},
			want: "not one of parked, unparked",
		},
		{
			name: "parked without a reason",
			in:   &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: EventParked, At: "2026-01-01T00:00:00Z", By: "y"}}},
			want: "reason is required",
		},
		{
			name: "missing attribution",
			in:   &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: EventUnparked, At: "2026-01-01T00:00:00Z"}}},
			want: "by is required",
		},
		{
			name: "missing timestamp",
			in:   &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: EventUnparked, By: "y"}}},
			want: "at is required",
		},
		{
			name: "until on an unparked event",
			in:   &Activity{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: EventUnparked, At: "2026-01-01T00:00:00Z", By: "y", Until: "someday"}}},
			want: "until belongs on a parked event",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := ValidateActivityShape(tc.in)
			if tc.clean {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %v", problems)
				}
				return
			}
			if tc.kind != "" {
				var kinds []ActivityProblemKind
				var matched bool
				for _, p := range problems {
					kinds = append(kinds, p.Kind)
					if p.Kind == tc.kind {
						matched = true
					}
				}
				if !matched {
					t.Errorf("want kind %q, got %v", tc.kind, kinds)
				}
				return
			}
			var found bool
			for _, p := range problems {
				if strings.Contains(p.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// The registry must agree with the producer.
//
// KnownActivityProblemKinds is a hand-maintained list, so it can drift
// from what ValidateActivityShape actually emits in either direction:
// a registered kind nothing produces, or — the dangerous one — a kind
// produced and never registered, which every registry-walking test would
// silently skip.
//
// These fixtures between them provoke all eight kinds. Two are needed at
// minimum because EmptyHistory and the per-event faults are mutually
// exclusive by construction: a history with no entries cannot also carry
// a malformed entry.
//
// This still cannot see a branch nobody wrote a fixture for. It makes the
// registry and today's producer agree, which is the strongest statement
// available without compiler support.
func TestShapeProblems_EmittedKindsMatchTheRegistry(t *testing.T) {
	fixtures := []*Activity{
		// SchemaVersion + EmptyHistory.
		{SchemaVersion: 99},
		// UnknownEvent + MissingAt + MissingBy.
		{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: "mothballed"}}},
		// TimestampInvalid + MissingBy + ParkedWithoutReason.
		{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{{Event: EventParked, At: "last tuesday"}}},
		// UntilOnUnparked.
		{SchemaVersion: ActivitySchemaVersion, History: []ActivityEvent{
			{Event: EventUnparked, At: "2026-09-01T11:00:00Z", By: "dwht", Until: "someday"},
		}},
	}

	emitted := map[ActivityProblemKind]bool{}
	for _, f := range fixtures {
		for _, p := range ValidateActivityShape(f) {
			emitted[p.Kind] = true
		}
	}

	registered := map[ActivityProblemKind]bool{}
	for _, k := range KnownActivityProblemKinds() {
		registered[k] = true
	}

	for k := range emitted {
		if !registered[k] {
			t.Errorf("kind %q is emitted but not registered — every registry-walking test would skip it", k)
		}
	}
	for k := range registered {
		if !emitted[k] {
			t.Errorf("kind %q is registered but no fixture produces it — either it is dead, or this test has a gap", k)
		}
	}
}

// ---------------------------------------------------------------------
// Versioning: v1 → v2.
//
// v2 admits `activated`. That is a change to the domain of an existing
// field, which the house rule treats as a bump — and the reason for the
// bump is not how many readers exist but what an OLD reader does when
// handed a new file.
// ---------------------------------------------------------------------

// A v1 declaration reads identically under the v2 binary: same events,
// same derived state. The migration is an identity on representation.
func TestActivityV1_ReadsIdenticallyUnderV2(t *testing.T) {
	v1 := `schema_version: 1
history:
  - event: parked
    reason: superseded
    at: 2026-04-18T09:12:00Z
    by: dwht
  - event: unparked
    at: 2026-09-01T11:00:00Z
    by: dwht
`
	a, err := ParseActivityBytes("activity.yaml", []byte(v1))
	if err != nil {
		t.Fatalf("a v1 declaration must still be readable: %v", err)
	}
	if a.Current() != ActivityActive {
		t.Errorf("derived state changed across the bump: %q", a.Current())
	}
	if len(a.History) != 2 || a.History[0].Reason != "superseded" {
		t.Errorf("history did not survive: %+v", a.History)
	}
	// Migrated in memory...
	if a.SchemaVersion != ActivitySchemaVersion {
		t.Errorf("want in-memory version %d, got %d", ActivitySchemaVersion, a.SchemaVersion)
	}
	if problems := ValidateActivityShape(a); len(problems) != 0 {
		t.Errorf("a migrated v1 declaration must validate: %+v", problems)
	}
}

// WHY THE BUMP MATTERS. Under v1 semantics `activated` is outside the
// closed vocabulary, so a v1 file carrying it is refused. Left at v1,
// that refusal would look like corruption rather than a version the
// reader cannot handle — which is precisely what a version number is for.
func TestActivityV1_RefusesTheV2Vocabulary(t *testing.T) {
	_, err := ParseActivityBytes("activity.yaml", []byte(`schema_version: 1
history:
  - event: activated
    at: 2026-09-01T11:00:00Z
    by: dwht
`))
	if err == nil {
		t.Fatal("a v1 file must not carry an activated event")
	}
	if !strings.Contains(err.Error(), "schema_version 1") {
		t.Errorf("the refusal should name the version that does not admit it: %v", err)
	}
}

func TestActivityV2_AcceptsActivated(t *testing.T) {
	a, err := ParseActivityBytes("activity.yaml", []byte(`schema_version: 2
history:
  - event: activated
    at: 2026-09-01T11:00:00Z
    by: dwht
`))
	if err != nil {
		t.Fatalf("v2 admits activated: %v", err)
	}
	if a.Current() != ActivityActive {
		t.Errorf("activated must derive active, got %q", a.Current())
	}
	// An activated feature was never parked, so there is no parking in
	// force — the distinction the third kind exists to preserve.
	if _, ok := a.LatestParking(); ok {
		t.Error("an activated feature has no parking in force")
	}
}

// A version beyond the chain is refused rather than read leniently.
func TestActivity_RefusesAVersionBeyondTheChain(t *testing.T) {
	if _, err := ParseActivityBytes("activity.yaml", []byte("schema_version: 3\nhistory: []\n")); err == nil {
		t.Fatal("a version past the chain must be refused")
	}
	if _, err := ParseActivityBytes("activity.yaml", []byte("schema_version: 0\nhistory: []\n")); err == nil {
		t.Fatal("a version below the chain must be refused")
	}
}

// Reading does not rewrite. A command that promised not to write must not
// upgrade every v1 file it happens to look at.
func TestActivityV1_ReadDoesNotRewriteTheFile(t *testing.T) {
	dir := t.TempDir()
	v1 := "schema_version: 1\nhistory:\n  - event: parked\n    reason: superseded\n    at: 2026-04-18T09:12:00Z\n    by: dwht\n"
	path := filepath.Join(dir, ActivityFile)
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ParseActivityFile(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != v1 {
		t.Errorf("reading rewrote the file:\n want %q\n got  %q", v1, after)
	}
}

// The chain must be a structure that can have a hole, so the hole can be
// reported. An if-ladder cannot: it just falls through.
func TestActivityMigration_MissingLinkIsRefused(t *testing.T) {
	restore := activityMigrations
	activityMigrations = map[int]activityMigration{} // the v1→v2 link, removed
	defer func() { activityMigrations = restore }()

	_, err := ParseActivityBytes("activity.yaml", []byte("schema_version: 1\nhistory:\n  - event: parked\n    reason: x\n    at: 2026-01-01T00:00:00Z\n    by: y\n"))
	if err == nil {
		t.Fatal("a missing migrator link must be refused, not silently skipped")
	}
	if !strings.Contains(err.Error(), "chain to") {
		t.Errorf("the error should name the broken chain: %v", err)
	}
}

// A multi-step chain runs every link in order. Pinned with a synthetic
// three-version chain, because the real one is currently one link long
// and a one-link chain cannot demonstrate that it loops.
func TestActivityMigration_WalksEveryLink(t *testing.T) {
	restoreChain, restoreCurrent := activityMigrations, currentActivityVersion
	var visited []int
	activityMigrations = map[int]activityMigration{
		1: {to: 2, fn: func(a *Activity) error { visited = append(visited, 1); return nil }},
		2: {to: 3, fn: func(a *Activity) error { visited = append(visited, 2); return nil }},
	}
	currentActivityVersion = 3
	defer func() { activityMigrations, currentActivityVersion = restoreChain, restoreCurrent }()

	a := &Activity{SchemaVersion: 1}
	if err := migrateActivity(a); err != nil {
		t.Fatalf("a complete chain must succeed: %v", err)
	}
	if a.SchemaVersion != 3 {
		t.Errorf("want version 3, got %d", a.SchemaVersion)
	}
	if len(visited) != 2 || visited[0] != 1 || visited[1] != 2 {
		t.Errorf("want both links run in order, got %v", visited)
	}
}

// A link that fails stops the walk and surfaces its cause.
func TestActivityMigration_FailingLinkSurfaces(t *testing.T) {
	restore := activityMigrations
	activityMigrations = map[int]activityMigration{
		1: {to: 2, fn: func(*Activity) error { return errors.New("cannot convert") }},
	}
	defer func() { activityMigrations = restore }()

	a := &Activity{SchemaVersion: 1}
	err := migrateActivity(a)
	if err == nil || !strings.Contains(err.Error(), "cannot convert") {
		t.Fatalf("a failing link must surface its cause, got %v", err)
	}
}

// A chain advances exactly one version at a time. Three ways that can be
// violated, and all three look like success without the check.
func TestActivityMigration_LinkMustAdvanceExactlyOneVersion(t *testing.T) {
	cases := map[string]struct {
		chain   map[int]activityMigration
		current int
	}{
		// Would spin forever.
		"non-advancing": {map[int]activityMigration{1: {to: 1, fn: noopMigration}}, 2},
		// Satisfies "the version went up" while bypassing v2's migration
		// entirely.
		"skipping": {map[int]activityMigration{1: {to: 3, fn: noopMigration}}, 3},
		// Hands back an object newer than the binary reading it.
		"overshooting": {map[int]activityMigration{1: {to: 4, fn: noopMigration}}, 3},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			restoreChain, restoreCurrent := activityMigrations, currentActivityVersion
			activityMigrations, currentActivityVersion = tc.chain, tc.current
			defer func() { activityMigrations, currentActivityVersion = restoreChain, restoreCurrent }()

			a := &Activity{SchemaVersion: 1}
			if err := migrateActivity(a); err == nil {
				t.Fatalf("a %s link must be refused; ended at version %d", name, a.SchemaVersion)
			}
		})
	}
}

func noopMigration(*Activity) error { return nil }
