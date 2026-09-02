// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: activity-declaration

package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ActivityFile is the basename of a feature's activity declaration.
//
// It lives INSIDE the feature directory rather than in a project-level
// index. A central file keyed by qualified feature id goes stale the
// moment `parlay move-feature` moves a feature between initiatives — the
// key changes and the record does not follow it. Colocation moves with
// the directory for free, keeps unrelated parkings out of each other's
// merge conflicts, and makes "why is this parked?" answerable by opening
// the feature.
const ActivityFile = "activity.yaml"

// ActivitySchemaVersion is the version this parser WRITES.
//
// v2 admits the `activated` event. That is a change to the domain of an
// existing field, which `schema-versioning.schema.md` treats as a bump —
// the same class that forced the status JSON envelope to v2 and that the
// house rule records for `.code-hashes.yaml` ("v2 added the
// `hand-authored` provenance, changing the domain of an existing field").
//
// "Nothing else consumes it yet" is not an exception to that rule. This
// repository deployed v1 an hour before v2 was designed, which makes it
// an installed base of exactly one — and the reason for the bump is not
// how many readers exist but what an OLD reader does when handed a new
// file. Under v1 semantics an `activated` event is outside the closed
// vocabulary and the v1 parser refuses it, which is correct. Left at v1,
// that refusal would look like corruption rather than a version it cannot
// read.
const ActivitySchemaVersion = 2

// currentActivityVersion is ActivitySchemaVersion, as a var so a test can
// stand up a synthetic multi-link chain — the real one is one link long
// and a one-link chain cannot demonstrate that the walk loops.
var currentActivityVersion = ActivitySchemaVersion

// ActivityMinSchemaVersion is the oldest version a migrator chain reaches
// from. Everything between it and ActivitySchemaVersion is readable.
const ActivityMinSchemaVersion = 1

// activityMigration is one link in the chain: what it produces, and how.
type activityMigration struct {
	to int
	fn func(*Activity) error
}

// activityMigrations is the REGISTERED chain, keyed by the version each
// link migrates FROM.
//
// A map rather than an if-ladder, and this is not decoration. The earlier
// version of this was `if version == 1 { version = 2 }` under a comment
// calling itself a chain — which works for exactly as long as there are
// two versions. Add a v3 and a readable v2 file falls through
// unmigrated, silently, with nothing to notice that the link nobody wrote
// is missing. The documented policy is a migrator chain; a chain has to
// be a structure that can have a hole in it, so that the hole can be
// reported.
//
// v1 → v2 is an identity: the representation did not change, only what
// the vocabulary admits. Registered anyway, because the registry is the
// thing that must exist.
var activityMigrations = map[int]activityMigration{
	1: {to: 2, fn: func(*Activity) error { return nil }},
}

// migrateActivity walks the chain from the file's version to the current
// one, and refuses rather than guessing when a link is missing.
//
// The file is NOT rewritten on read. A read that silently upgraded every
// declaration it touched would rewrite files nobody asked to change, and
// would do it inside commands that promise not to write. The upgrade
// lands on the next explicit mutation, which is a write somebody asked
// for.
func migrateActivity(a *Activity) error {
	for a.SchemaVersion < currentActivityVersion {
		from := a.SchemaVersion
		link, ok := activityMigrations[from]
		if !ok {
			return fmt.Errorf(
				"no migrator from schema_version %d — the chain to %d is broken, and reading this file without the missing step would apply %d's semantics to %d's data",
				from, currentActivityVersion, currentActivityVersion, from)
		}
		if err := link.fn(a); err != nil {
			return fmt.Errorf("migrating schema_version %d to %d: %w", from, link.to, err)
		}
		// Exactly one step. A PER-VERSION chain that tolerated bigger
		// jumps would not be one: a registered 1→3 link satisfies "the
		// version went up" while silently bypassing whatever v2's
		// migration was supposed to do, and a 1→4 link with a v3 binary
		// hands back an object newer than the code reading it. Both look
		// like success. The step size is the invariant that makes the
		// registry a chain rather than a lookup table of shortcuts.
		if link.to != from+1 {
			return fmt.Errorf(
				"migrator from schema_version %d claims to produce %d — a chain advances one version at a time, and a jump would skip %d's migration entirely",
				from, link.to, from+1)
		}
		a.SchemaVersion = link.to
	}
	// Belt and braces: the loop cannot overshoot given the step check
	// above, but an object at the wrong version is the one failure that
	// would be invisible downstream — everything would parse and mean
	// something slightly different.
	if a.SchemaVersion != currentActivityVersion {
		return fmt.Errorf(
			"migration ended at schema_version %d, not %d", a.SchemaVersion, currentActivityVersion)
	}
	return nil
}

// activityVocabulary reports the event kinds a given version admits.
//
// Version-dependent on purpose: the whole content of the v1→v2 bump is
// that v2 admits one more kind, so a v1 file carrying `activated` is
// invalid rather than merely unusual. Accepting it would make the version
// number decorative.
func activityVocabulary(version int) map[ActivityEventKind]bool {
	v := map[ActivityEventKind]bool{EventParked: true, EventUnparked: true}
	if version >= 2 {
		v[EventActivated] = true
	}
	return v
}

// Activity is a feature's append-only activity declaration.
//
// There is no `state:` field, and that is the design. State is DERIVED
// from history, so a stored status can never disagree with the events
// that produced it — the failure every mutable status field eventually
// has.
type Activity struct {
	SchemaVersion int             `yaml:"schema_version"`
	History       []ActivityEvent `yaml:"history"`
}

// ActivityEvent is one declaration. Written once, never edited: a later
// change of mind is a new event, so two people parking a feature for
// different reasons months apart is legible as two facts rather than one
// overwritten twice.
type ActivityEvent struct {
	Event  ActivityEventKind `yaml:"event"`
	Reason string            `yaml:"reason,omitempty"`
	// Until is free text on purpose — "after adapter-set v2 lands" is the
	// honest answer far more often than a date, and a date field invites
	// a guess that later reads as a commitment nobody made.
	Until string `yaml:"until,omitempty"`
	At    string `yaml:"at"`
	By    string `yaml:"by"`
}

// ActivityEventKind is closed at three values: a pause, its end, and a
// confirmation that there was never a pause at all.
//
// Parking is reversible, which is the whole difference between it and
// retirement — so the vocabulary needs the pause and its undo. The third
// exists because a feature nobody has classified needs a way to be
// declared live, and reusing the undo for it would record an end to a
// pause that never happened.
type ActivityEventKind string

const (
	EventParked   ActivityEventKind = "parked"
	EventUnparked ActivityEventKind = "unparked"
	// EventActivated is somebody looking at an undeclared feature and
	// saying it is live. Distinct from EventUnparked, which ends a pause.
	//
	// The distinction is not pedantry. `unparked` on a feature that was
	// never parked would put a false statement in a history whose whole
	// value is being literally true months later, and LatestParking walks
	// backwards looking for exactly that event as evidence a pause
	// existed. Two different facts, two different records.
	//
	// Added in schema v2.
	EventActivated ActivityEventKind = "activated"
)

// KnownActivityEvent reports whether kind is in the CURRENT vocabulary.
// Version-sensitive checking uses activityVocabulary.
func KnownActivityEvent(kind ActivityEventKind) bool {
	return kind == EventParked || kind == EventUnparked || kind == EventActivated
}

// Activity is what a reader is told about a feature's activity. It is
// three values, and the third is not a hedge.
type ActivityState string

const (
	// ActivityActive — the work is live.
	ActivityActive ActivityState = "active"
	// ActivityParked — the pause was chosen, and the record says by whom
	// and why.
	ActivityParked ActivityState = "parked"
	// ActivityUnclassified — nobody has declared anything, and no
	// pipeline activity has been observed either.
	//
	// This is a statement about the RECORD, not about the work. It does
	// not claim the feature is stalled, abandoned or forgotten; it says
	// the project has not said. That distinction is why nothing here
	// reads mtime or git history: a checkout, a migration or a bulk move
	// all perturb timestamps, and inferring "stalled" from them would be
	// exactly the guess `retires_feature:` refuses to make — a lifecycle
	// transition nobody chose is not one to infer.
	ActivityUnclassified ActivityState = "unclassified"
)

// ParseActivityFile reads a feature's activity.yaml.
//
// A missing file is not an error and not an empty Activity: it is the
// undeclared state, reported by the bool. Callers overwhelmingly want to
// distinguish "no declaration" from "a declaration with no events", and
// collapsing them here would push that distinction onto every one of
// them.
//
// EVERY other failure reports declared=true. A file that exists but
// cannot be read is a fault, and returning "nobody declared anything" for
// it would convert that fault into a quieter, different, and wrong fact —
// the same mistake as reading a malformed file as silence.
func ParseActivityFile(path string) (*Activity, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("read %s: %w", path, err)
	}
	a, err := ParseActivityBytes(path, content)
	if err != nil {
		return nil, true, err
	}
	return a, true, nil
}

// ParseActivityBytes parses activity.yaml content, and REFUSES anything
// whose meaning it cannot be sure of.
//
// Three refusals, all of them about making derivation safe rather than
// about authoring quality (which ValidateActivityShape owns):
//
//  1. Unknown fields. yaml.Unmarshal silently discards them, so `histroy:`
//     parses as an empty history and `reasno:` as a missing reason — a
//     typo becomes a different, well-formed record that no validator can
//     see was ever intended differently.
//  2. A schema_version this binary does not know. Refusing is the entire
//     point of having the field: an old binary cannot know a newer
//     version's semantics, so it must decline rather than guess.
//  3. An event kind outside the closed vocabulary. An earlier cut skipped
//     these and derived from the last kind it recognised, which is worse
//     than failing: a feature parked and later `retired` by a newer
//     parlay would report `parked` — a confident, stale, wrong answer. In
//     a v1 file an unknown kind is simply invalid.
//
// Because parsing enforces all three, a *Activity that exists is one
// whose history Current can walk without hedging.
func ParseActivityBytes(path string, content []byte) (*Activity, error) {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	dec.KnownFields(true)

	var a Activity
	if err := dec.Decode(&a); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("parse %s: the file is empty", path)
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Exactly one document. A second one would be silently ignored, and
	// a declaration nobody reads is indistinguishable from one nobody
	// wrote.
	var extra Activity
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("parse %s: more than one YAML document; an activity declaration is exactly one", path)
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if a.SchemaVersion < ActivityMinSchemaVersion || a.SchemaVersion > ActivitySchemaVersion {
		return nil, fmt.Errorf(
			"parse %s: schema_version %d is outside the range this parlay reads (%d–%d) — a binary that cannot know what a %d declaration means must decline rather than guess at a state nobody declared",
			path, a.SchemaVersion, ActivityMinSchemaVersion, ActivitySchemaVersion, a.SchemaVersion)
	}
	// Vocabulary is checked against the version the FILE declares, before
	// migration. A v1 file carrying `activated` is invalid: v2 is what
	// admits that kind, and accepting it under v1 would make the version
	// number decorative.
	vocab := activityVocabulary(a.SchemaVersion)
	for i, e := range a.History {
		if !vocab[e.Event] {
			return nil, fmt.Errorf(
				"parse %s: history[%d] event %q is not admitted by schema_version %d — refusing to derive a state from a vocabulary that version does not define",
				path, i, e.Event, a.SchemaVersion)
		}
	}
	if err := migrateActivity(&a); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &a, nil
}

// ActivityPath returns a feature directory's activity.yaml path.
func ActivityPath(featurePath string) string {
	return filepath.Join(featurePath, ActivityFile)
}

// Current derives the activity state from the history: the latest event
// wins.
//
// Total, and safely so — ParseActivityBytes refuses unknown event kinds
// and unknown schema versions, so every history reaching here is one this
// binary can read. That is why derivation returns no error: the place to
// refuse an unreadable declaration is where it is read, not at every
// point somebody asks what it means.
//
// An empty history yields unclassified. ValidateActivityShape rejects
// that file, so it should not arrive; the fallthrough is defensive rather
// than a supported input.
func (a *Activity) Current() ActivityState {
	if a == nil {
		return ActivityUnclassified
	}
	for i := len(a.History) - 1; i >= 0; i-- {
		switch a.History[i].Event {
		case EventParked:
			return ActivityParked
		case EventUnparked, EventActivated:
			return ActivityActive
		}
	}
	return ActivityUnclassified
}

// LatestParking returns the parking event in force, if the feature is
// parked. What a reader wants next after "parked" is always why and since
// when, and making them re-walk the history to find it is how a status
// line ends up printing the state without the reason.
func (a *Activity) LatestParking() (ActivityEvent, bool) {
	if a == nil {
		return ActivityEvent{}, false
	}
	for i := len(a.History) - 1; i >= 0; i-- {
		switch a.History[i].Event {
		case EventParked:
			return a.History[i], true
		case EventUnparked, EventActivated:
			return ActivityEvent{}, false
		}
	}
	return ActivityEvent{}, false
}

// ResolveActivityState answers the question status actually asks, which
// is not the same as the question the file answers.
//
// The file can only report `parked`, `active` or "no declaration". Turning
// the third into a finding needs one more fact: whether the pipeline has
// been observed doing anything with this feature. A feature that has
// passed or been blocked at a boundary is demonstrably being worked on,
// and calling that `unclassified` would report a missing disposition for
// work whose activity is already evident — a permanent non-problem, which
// is how a status line stops being read.
//
// So `unclassified` is reserved for the case where the project has said
// nothing AND nothing has been observed. That is the pile a person is
// actually needed for.
func ResolveActivityState(a *Activity, declared bool, observedBoundary bool) ActivityState {
	if declared {
		if state := a.Current(); state != ActivityUnclassified {
			return state
		}
	}
	if observedBoundary {
		return ActivityActive
	}
	return ActivityUnclassified
}

// ActivityProblemKind names a structural fault. It exists so that the
// published diagnostic code and its remedy are chosen by switching on a
// VALUE rather than by matching English inside a message.
//
// Routing on prose was the earlier design and it was quietly fragile:
// both the code and the fix were selected with strings.Contains, so a
// wording-only refactor could reroute a finding to a different published
// code, or attach the wrong remedy, without touching either the schema or
// the router. A test caught the rewordings that broke the substring and
// silently missed the ones that did not — "a reason is required" still
// contains "reason is required". Semantics do not belong in prose.
type ActivityProblemKind string

const (
	ProblemSchemaVersion       ActivityProblemKind = "schema-version"
	ProblemEmptyHistory        ActivityProblemKind = "empty-history"
	ProblemUnknownEvent        ActivityProblemKind = "unknown-event"
	ProblemMissingAt           ActivityProblemKind = "missing-at"
	ProblemTimestampInvalid    ActivityProblemKind = "timestamp-invalid"
	ProblemMissingBy           ActivityProblemKind = "missing-by"
	ProblemParkedWithoutReason ActivityProblemKind = "parked-without-reason"
	ProblemUntilOnUnparked     ActivityProblemKind = "until-on-unparked"
)

// KnownActivityProblemKinds is the REGISTRY of problem kinds, exported so
// a consumer's mapping can be checked against it.
//
// It is a registry, not a proof. Go cannot enumerate the constants of a
// string-backed enum, so nothing mechanically guarantees this list names
// every kind ValidateActivityShape can emit: a future kind declared,
// emitted, and not added here would be invisible to every test that walks
// this list. TestShapeProblems_EmittedKindsMatchTheRegistry closes the gap
// for the kinds emitted today by comparing this list against what the
// validator actually produces — which still cannot see an entirely new
// branch nobody wrote a fixture for.
//
// So: a new kind MUST be registered here. That is a convention this list
// asks for, not an invariant it enforces.
func KnownActivityProblemKinds() []ActivityProblemKind {
	return []ActivityProblemKind{
		ProblemSchemaVersion,
		ProblemEmptyHistory,
		ProblemUnknownEvent,
		ProblemMissingAt,
		ProblemTimestampInvalid,
		ProblemMissingBy,
		ProblemParkedWithoutReason,
		ProblemUntilOnUnparked,
	}
}

// ActivityProblem is one structural fault: what kind it is, and the
// human sentence describing this instance of it. Kind carries the
// meaning; Message is for display only.
type ActivityProblem struct {
	Kind    ActivityProblemKind
	Message string
}

// ValidateActivityShape reports structural problems with a parsed
// declaration. Kept beside the parser because these are facts about the
// file rather than about the project, and the CLI validator wraps it.
//
// This is the AUTHORING pass. ParseActivityBytes has already refused
// anything that makes derivation unsafe, so what remains here is the
// difference between a record that can be read and one that is worth
// having.
func ValidateActivityShape(a *Activity) []ActivityProblem {
	var problems []ActivityProblem
	add := func(kind ActivityProblemKind, format string, args ...any) {
		problems = append(problems, ActivityProblem{
			Kind:    kind,
			Message: fmt.Sprintf(format, args...),
		})
	}
	if a == nil {
		return problems
	}
	// Defensive, and normally unreachable from a file: ParseActivityBytes
	// refuses an unknown schema_version and an unknown event kind before
	// anything reaches this pass. Both branches exist for Activity values
	// built programmatically — a park command assembling a history in
	// memory, or a test — and neither should be described as something
	// ordinary CLI input provokes.
	if a.SchemaVersion != ActivitySchemaVersion {
		add(ProblemSchemaVersion, "schema_version is %d, want %d", a.SchemaVersion, ActivitySchemaVersion)
	}
	// A declaration that declares nothing is the ambiguity this artifact
	// exists to remove, wearing the artifact's own filename. It also makes
	// "a declaration exists" mean nothing to a caller.
	if len(a.History) == 0 {
		add(ProblemEmptyHistory, "history is empty — a declaration with no events declares nothing, which is the state this file exists to replace")
	}
	for i, e := range a.History {
		where := fmt.Sprintf("history[%d]", i)
		if !KnownActivityEvent(e.Event) {
			add(ProblemUnknownEvent, "%s: event %q is not one of parked, unparked, activated", where, e.Event)
		}
		if strings.TrimSpace(e.At) == "" {
			add(ProblemMissingAt, "%s: at is required", where)
		} else if _, err := time.Parse(time.RFC3339, e.At); err != nil {
			// Order is append order, never timestamp order, so nothing
			// here depends on clocks being monotonic or even correct.
			// But an attribution timestamp nobody can parse is not an
			// attribution.
			add(ProblemTimestampInvalid, "%s: at %q is not RFC3339", where, e.At)
		}
		if strings.TrimSpace(e.By) == "" {
			add(ProblemMissingBy, "%s: by is required — a declaration nobody can attribute tells the next reader nothing they did not already know", where)
		}
		// A parking without a reason is the failure this whole record
		// exists to prevent: it converts "we chose to pause this" back
		// into "this stopped", which is the state the project already had
		// and could not act on.
		if e.Event == EventParked && strings.TrimSpace(e.Reason) == "" {
			add(ProblemParkedWithoutReason, "%s: reason is required on a parked event — a pause with no stated reason is indistinguishable from neglect", where)
		}
		if e.Event != EventParked && strings.TrimSpace(e.Until) != "" {
			add(ProblemUntilOnUnparked, "%s: until belongs on a parked event, not a %s one", where, e.Event)
		}
	}
	return problems
}
