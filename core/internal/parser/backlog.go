// parlay-feature: parlay-tool/backlog-and-activity
// parlay-section: cross-cutting
// parlay-source: backlog-item

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

// BacklogDir is the per-root directory holding backlog items.
//
// Under spec/ rather than .parlay/: an item is design intent a person
// reads and decides on, and the .parlay/ zone rule is explicit that it is
// never user-facing.
const BacklogDir = "backlog"

// BacklogSchemaVersion is the version this parser writes.
const BacklogSchemaVersion = 2

// BacklogMinSchemaVersion is the oldest version the migrator chain reaches
// from.
const BacklogMinSchemaVersion = 1

var currentBacklogVersion = BacklogSchemaVersion

// backlogMigrations is the registered chain, keyed by the version each
// link migrates FROM. Empty at v1 — there is nothing behind it yet — and
// the walk below is the same shape activity's uses, so the first real
// link drops in beside a mechanism that already refuses holes.
var backlogMigrations = map[int]backlogMigration{
	// v1 -> v2 admits the `fixed` event, changing the domain of an
	// existing field. The same reason activity went to 2 for
	// `activated`, and the same shape.
	//
	// STRUCTURALLY AN IDENTITY. No existing value changes meaning, so a
	// v1 item is already a valid v2 item and there is nothing to
	// transform. The link exists so the walk has no hole, and so a v1
	// file is upgraded on its next explicit write rather than on read.
	1: {to: 2, fn: func(*BacklogItem) error { return nil }},
}

type backlogMigration struct {
	to int
	fn func(*BacklogItem) error
}

// BacklogItem is one observed piece of undone work.
//
// Mutable, unlike an amendment. Amendment immutability is priced for
// authority mutation; a low-authority inbox note needs typo correction
// and evidence enrichment, and demanding a supersession record for a
// fixed typo is ceremony the act does not warrant.
//
// What is NOT mutable: the Captured block, and History, which is
// append-only.
type BacklogItem struct {
	SchemaVersion int         `yaml:"schema_version"`
	ID            string      `yaml:"id"`
	Kind          BacklogKind `yaml:"kind"`
	// Priority is optional, and absent means UNTRIAGED.
	//
	// This mirrors activity's no-file-means-undeclared exactly: absence
	// is a fact about the record, never a default smuggled in as one. It
	// is what lets a listing put the unranked items first, which is the
	// pile that actually needs a person.
	Priority string   `yaml:"priority,omitempty"`
	Title    string   `yaml:"title"`
	Body     string   `yaml:"body,omitempty"`
	About    []string `yaml:"about,omitempty"`

	Captured BacklogCapture    `yaml:"captured"`
	Evidence []BacklogEvidence `yaml:"evidence,omitempty"`
	History  []BacklogEvent    `yaml:"history,omitempty"`
}

// BacklogCapture is immutable once written: where the observation came
// from. A later reader asking "who saw this, and while doing what" gets
// an answer that no subsequent edit can have moved.
type BacklogCapture struct {
	At      string `yaml:"at"`
	By      string `yaml:"by"`
	Run     string `yaml:"run,omitempty"`
	Feature string `yaml:"feature,omitempty"`
	Phase   string `yaml:"phase,omitempty"`
	// OriginRoot is where the discovery happened, which is not always
	// where the work belongs.
	OriginRoot string `yaml:"origin_root,omitempty"`
}

// BacklogEvidence is a filesystem location. Distinct from About, which
// holds semantic parlay refs: a discovery made in conversation
// legitimately has no file, and a ref like `@feature/operation:x` is not
// a path.
type BacklogEvidence struct {
	Path   string `yaml:"path"`
	Line   int    `yaml:"line,omitempty"`
	Detail string `yaml:"detail,omitempty"`
}

// BacklogEvent is one decision about an item, appended and never edited.
type BacklogEvent struct {
	Event   BacklogEventKind `yaml:"event"`
	Reason  string           `yaml:"reason,omitempty"`
	Becomes string           `yaml:"becomes,omitempty"`
	At      string           `yaml:"at"`
	By      string           `yaml:"by"`
}

// BacklogKind is what sort of undone work this is. Closed at four.
//
// `friction` and `question` were considered and dropped: friction is a
// symptom that resolves into one of these, and an unanswered question
// that BLOCKS authoring is already owned by intent `Questions:`.
type BacklogKind string

const (
	KindDefect BacklogKind = "defect"
	KindGap    BacklogKind = "gap"
	KindDebt   BacklogKind = "debt"
	KindIdea   BacklogKind = "idea"
)

func KnownBacklogKind(k BacklogKind) bool {
	switch k {
	case KindDefect, KindGap, KindDebt, KindIdea:
		return true
	}
	return false
}

// BacklogEventKind is closed at seven values: one nonterminal and six
// terminal.
type BacklogEventKind string

const (
	// EventDeferred is NOT an answer. Lifted from
	// defer-legacy-exemption, including its best property: attempts
	// accumulate, because two people independently unable to decide is a
	// different fact from one attempt overwritten twice.
	EventDeferred BacklogEventKind = "deferred"

	EventPromoted BacklogEventKind = "promoted"
	EventAmended  BacklogEventKind = "amended"
	EventFolded   BacklogEventKind = "folded"
	EventDeclined BacklogEventKind = "declined"
	EventObsolete BacklogEventKind = "obsolete"
	// EventFixed is the third way an item ends WITHOUT becoming a parlay
	// object, beside declined and obsolete.
	//
	// It earns a value by the same recoverability test that keeps
	// declined and obsolete apart: months later, "we chose not to do
	// this", "the condition disappeared", and "somebody changed the
	// system so the condition no longer holds" are three different
	// facts, and prose should not have to reconstruct which one applied.
	//
	// Deliberately NARROW. `resolved` or `completed` would fit more
	// endings and become a catch-all that pulls the model toward generic
	// task tracking, which §16 of the proposal rules out along with
	// owners, estimates and sprints. An `idea` implemented without
	// becoming a feature or an amendment should make somebody ask
	// whether it bypassed the promise model — not be hidden under a
	// broader word.
	EventFixed BacklogEventKind = "fixed"
)

// IsTerminal reports whether an event closes the item.
func (k BacklogEventKind) IsTerminal() bool {
	switch k {
	case EventPromoted, EventAmended, EventFolded, EventDeclined, EventObsolete, EventFixed:
		return true
	}
	return false
}

// backlogEventIntroducedIn is the version an event value first became
// legal. Anything not listed has been there since v1.
func backlogEventIntroducedIn(k BacklogEventKind) int {
	if k == EventFixed {
		return 2
	}
	return 1
}

func KnownBacklogEvent(k BacklogEventKind) bool {
	return k == EventDeferred || k.IsTerminal()
}

// RequiresBecomes reports whether a terminal event must name what the
// work became. `declined` and `obsolete` must NOT — there is nothing it
// became — and keeping them distinct is the point: a reader months later
// cannot recover from silence whether the work moved or stopped
// mattering, and that difference is the whole content of the decision.
func (k BacklogEventKind) RequiresBecomes() bool {
	switch k {
	case EventPromoted, EventAmended, EventFolded:
		return true
	}
	return false
}

// BacklogState is an item's derived lifecycle state. Never stored.
type BacklogState string

const (
	StateOpen BacklogState = "open"
)

// State derives the item's lifecycle from its history.
//
// There is no `state:` field. A stored status can disagree with the
// events that produced it; a derived one cannot.
//
// `deferred` is non-terminal and never changes the answer: an item
// somebody could not decide about is still open, and the tool keeps
// reporting it.
func (i *BacklogItem) State() BacklogState {
	if i == nil {
		return StateOpen
	}
	for _, e := range i.History {
		if e.Event.IsTerminal() {
			return BacklogState(e.Event)
		}
	}
	return StateOpen
}

// Deferrals returns every attempt that reached no decision.
func (i *BacklogItem) Deferrals() []BacklogEvent {
	if i == nil {
		return nil
	}
	var out []BacklogEvent
	for _, e := range i.History {
		if e.Event == EventDeferred {
			out = append(out, e)
		}
	}
	return out
}

// BacklogRoot is the directory holding a root's items.
func BacklogRoot(rootPath string) string {
	return filepath.Join(rootPath, SpecDirName, BacklogDir)
}

// SpecDirName mirrors config.SpecDir without importing it — parser is
// below config in the dependency order, and a cycle here would be paid
// for by every caller.
const SpecDirName = "spec"

// ParseBacklogBytes parses one item, refusing anything whose meaning it
// cannot be sure of: unknown fields, an unreadable version, a kind or
// event outside the closed vocabularies, or more than one document.
//
// Same discipline as activity, and for the same reason: `histroy:` would
// otherwise parse as an empty history and a typo would become a
// different, well-formed record that no validator could see was ever
// meant differently.
func ParseBacklogBytes(path string, content []byte) (*BacklogItem, error) {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	dec.KnownFields(true)

	var item BacklogItem
	if err := dec.Decode(&item); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("parse %s: the file is empty", path)
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra BacklogItem
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("parse %s: more than one YAML document; a backlog item is exactly one", path)
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if item.SchemaVersion < BacklogMinSchemaVersion || item.SchemaVersion > currentBacklogVersion {
		return nil, fmt.Errorf(
			"parse %s: schema_version %d is outside the range this parlay reads (%d–%d)",
			path, item.SchemaVersion, BacklogMinSchemaVersion, currentBacklogVersion)
	}
	if !KnownBacklogKind(item.Kind) {
		return nil, fmt.Errorf(
			"parse %s: kind %q is not one of defect, gap, debt, idea", path, item.Kind)
	}
	for n, e := range item.History {
		if !KnownBacklogEvent(e.Event) {
			return nil, fmt.Errorf(
				"parse %s: history[%d] event %q is not a known disposition", path, n, e.Event)
		}
		// AGAINST THE FILE'S DECLARED VERSION, and before migration.
		//
		// KnownBacklogEvent alone is version-blind, so a v1 file
		// carrying `fixed` would parse clean here and then be migrated
		// to v2 — retroactively legitimising a value that did not exist
		// when the file claimed to have been written. The version field
		// is a claim about which vocabulary the file was written
		// against, and a file that contradicts its own claim is not an
		// old file to upgrade; it is a file whose meaning is not
		// established.
		if intro := backlogEventIntroducedIn(e.Event); intro > item.SchemaVersion {
			return nil, fmt.Errorf(
				"parse %s: history[%d] event %q was introduced at schema_version %d, but this file declares %d",
				path, n, e.Event, intro, item.SchemaVersion)
		}
	}
	if err := migrateBacklog(&item); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &item, nil
}

// migrateBacklog walks the registered chain, refusing a missing or
// malformed link rather than guessing. Same shape as activity's, so the
// first real link drops into a mechanism that already refuses holes.
func migrateBacklog(i *BacklogItem) error {
	for i.SchemaVersion < currentBacklogVersion {
		from := i.SchemaVersion
		link, ok := backlogMigrations[from]
		if !ok {
			return fmt.Errorf("no migrator from schema_version %d — the chain to %d is broken", from, currentBacklogVersion)
		}
		if err := link.fn(i); err != nil {
			return fmt.Errorf("migrating schema_version %d to %d: %w", from, link.to, err)
		}
		if link.to != from+1 {
			return fmt.Errorf(
				"migrator from schema_version %d claims to produce %d — a chain advances one version at a time", from, link.to)
		}
		i.SchemaVersion = link.to
	}
	if i.SchemaVersion != currentBacklogVersion {
		return fmt.Errorf("migration ended at schema_version %d, not %d", i.SchemaVersion, currentBacklogVersion)
	}
	return nil
}

// ParseBacklogFile reads one item from disk.
func ParseBacklogFile(path string) (*BacklogItem, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseBacklogBytes(path, content)
}

// BacklogProblemKind names a structural fault, so the published code and
// remedy are chosen by switching on a value rather than matching English.
type BacklogProblemKind string

const (
	BacklogProblemSchemaVersion    BacklogProblemKind = "schema-version"
	BacklogProblemMissingID        BacklogProblemKind = "missing-id"
	BacklogProblemMissingTitle     BacklogProblemKind = "missing-title"
	BacklogProblemMissingKind      BacklogProblemKind = "missing-kind"
	BacklogProblemCaptureMissing   BacklogProblemKind = "capture-missing"
	BacklogProblemTimestamp        BacklogProblemKind = "timestamp-invalid"
	BacklogProblemPriority         BacklogProblemKind = "priority-invalid"
	BacklogProblemBecomesMissing   BacklogProblemKind = "becomes-missing"
	BacklogProblemBecomesUnwanted  BacklogProblemKind = "becomes-unwanted"
	BacklogProblemReasonMissing    BacklogProblemKind = "reason-missing"
	BacklogProblemTerminalNotLast  BacklogProblemKind = "terminal-not-last"
	BacklogProblemEventAttribution BacklogProblemKind = "event-attribution"
	BacklogProblemAboutRef         BacklogProblemKind = "about-ref-invalid"
)

// KnownBacklogProblemKinds is the REGISTRY, exported so a consumer's
// mapping can be checked against it. A registry, not a proof: Go cannot
// enumerate the constants of a string-backed enum, so a new kind MUST be
// registered here — a convention this list asks for, not an invariant it
// enforces.
func KnownBacklogProblemKinds() []BacklogProblemKind {
	return []BacklogProblemKind{
		BacklogProblemSchemaVersion, BacklogProblemMissingID, BacklogProblemMissingTitle,
		BacklogProblemMissingKind, BacklogProblemCaptureMissing, BacklogProblemTimestamp,
		BacklogProblemPriority, BacklogProblemBecomesMissing, BacklogProblemBecomesUnwanted,
		BacklogProblemReasonMissing, BacklogProblemTerminalNotLast, BacklogProblemEventAttribution,
		BacklogProblemAboutRef,
	}
}

// BacklogProblem is one structural fault: what kind, and the sentence
// describing this instance. Kind carries the meaning; Message displays.
type BacklogProblem struct {
	Kind    BacklogProblemKind
	Message string
}

// ValidateBacklogShape reports what makes a record poor rather than
// unreadable. Parsing has already refused what makes it meaningless.
func ValidateBacklogShape(i *BacklogItem) []BacklogProblem {
	var problems []BacklogProblem
	add := func(k BacklogProblemKind, format string, args ...any) {
		problems = append(problems, BacklogProblem{Kind: k, Message: fmt.Sprintf(format, args...)})
	}
	if i == nil {
		return problems
	}
	if i.SchemaVersion != currentBacklogVersion {
		add(BacklogProblemSchemaVersion, "schema_version is %d, want %d", i.SchemaVersion, currentBacklogVersion)
	}
	if strings.TrimSpace(i.ID) == "" {
		add(BacklogProblemMissingID, "id is required")
	}
	if strings.TrimSpace(i.Title) == "" {
		add(BacklogProblemMissingTitle, "title is required — an item nobody can recognise in a listing is one nobody will triage")
	}
	if strings.TrimSpace(string(i.Kind)) == "" {
		add(BacklogProblemMissingKind, "kind is required")
	}
	if strings.TrimSpace(i.Captured.By) == "" || strings.TrimSpace(i.Captured.At) == "" {
		add(BacklogProblemCaptureMissing, "captured.at and captured.by are required — an observation nobody can attribute is one nobody can follow up")
	} else if _, err := time.Parse(time.RFC3339, i.Captured.At); err != nil {
		add(BacklogProblemTimestamp, "captured.at %q is not RFC3339", i.Captured.At)
	}
	if p := strings.TrimSpace(i.Priority); p != "" && !KnownBacklogPriority(p) {
		add(BacklogProblemPriority, "priority %q is not one of P0, P1, P2", i.Priority)
	}
	// `about` is SEMANTIC parlay refs, and until this check it accepted
	// any string at all. That is not a cosmetic looseness: a scoped read
	// resolves these refs to decide what an item concerns, so a
	// misspelled or invented ref becomes DURABLE state that the read
	// silently misses — the item is stored, listed under no feature, and
	// nothing reports that it went nowhere.
	for n, ref := range i.About {
		if err := ValidateAboutRef(ref); err != nil {
			add(BacklogProblemAboutRef, "about[%d]: %v", n, err)
		}
	}

	for n, e := range i.History {
		where := fmt.Sprintf("history[%d]", n)
		if strings.TrimSpace(e.At) == "" || strings.TrimSpace(e.By) == "" {
			add(BacklogProblemEventAttribution, "%s: at and by are required", where)
		} else if _, err := time.Parse(time.RFC3339, e.At); err != nil {
			add(BacklogProblemTimestamp, "%s: at %q is not RFC3339", where, e.At)
		}
		if strings.TrimSpace(e.Reason) == "" && (e.Event == EventDeferred || e.Event == EventDeclined || e.Event == EventObsolete || e.Event == EventFixed) {
			add(BacklogProblemReasonMissing, "%s: reason is required on %s", where, e.Event)
		}
		if e.Event.RequiresBecomes() && strings.TrimSpace(e.Becomes) == "" {
			add(BacklogProblemBecomesMissing, "%s: %s must name what the work became in becomes:", where, e.Event)
		}
		if !e.Event.RequiresBecomes() && strings.TrimSpace(e.Becomes) != "" {
			add(BacklogProblemBecomesUnwanted, "%s: %s names nothing the work became, so becomes: does not belong on it", where, e.Event)
		}
		// At most one terminal event, and it must be last. Deriving from
		// "the latest terminal event" would quietly admit a deferral
		// recorded after a promotion.
		if e.Event.IsTerminal() && n != len(i.History)-1 {
			add(BacklogProblemTerminalNotLast, "%s: %s closes the item, so no event may follow it", where, e.Event)
		}
	}
	return problems
}

// KnownBacklogPriority reports whether p is in the closed scale.
//
// The SAME scale intents use, with the same meaning: the cost of leaving
// the work undone, not the order to do it in. Reusing the tokens is safe
// only because the meaning transfers — attaching scheduling semantics to
// a scale the project defines as impact is what would break it.
func KnownBacklogPriority(p string) bool {
	return p == "P0" || p == "P1" || p == "P2"
}

// ValidateAboutRef accepts the two ref shapes `about:` may hold.
//
// Either a canonical contract ref — `@feature/<kind>:<name>` with kind
// one of operation|surface|infrastructure|domain, the same grammar
// amendments use — or a BARE FEATURE ref naming a feature and nothing
// inside it: `@widget`, or `@initiative/feature`.
//
// Closed rather than forward-compatible, deliberately. An unknown kind
// is refused instead of being stored for a future reader, because the
// only thing that would actually happen to a stored `@widget/newkind:x`
// is that every scoped read misses it and no one is told. When a fifth
// kind is added to the contract vocabulary, it is added HERE and to the
// canonical parser together; that is a smaller cost than a class of
// silently-invisible items.
func ValidateAboutRef(raw string) error {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return fmt.Errorf("empty ref")
	}
	if !strings.HasPrefix(ref, "@") {
		return fmt.Errorf("%q must start with @ — about holds parlay refs, not free text", raw)
	}
	// A colon means it is claiming to be a contract ref, so hold it to
	// that grammar rather than letting it fall through to the bare form
	// and be silently reinterpreted as a feature named "widget/newkind:x".
	if strings.Contains(ref, ":") {
		if _, err := ParseAmendmentRef(ref); err != nil {
			return err
		}
		return nil
	}
	body := strings.TrimPrefix(ref, "@")
	segs := strings.Split(body, "/")
	if len(segs) > 2 {
		return fmt.Errorf("%q has too many segments — a bare ref is @feature or @initiative/feature", raw)
	}
	for _, seg := range segs {
		if !isRefSegment(seg) {
			return fmt.Errorf("%q: %q is not a valid slug (lowercase letters, digits and dashes)", raw, seg)
		}
	}
	return nil
}

func isRefSegment(seg string) bool {
	if seg == "" {
		return false
	}
	for i, r := range seg {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(seg)-1:
		default:
			return false
		}
	}
	return true
}

// BareAboutFeature returns the feature a VALIDATED about ref names.
//
// It is only correct on input ValidateAboutRef has accepted, and that is
// the point of the pairing: a contract ref goes through the canonical
// parser, and anything else is by then known to be a bare feature ref,
// so stripping the @ is the whole answer rather than a guess dressed up
// as one. An earlier version had no validation behind it and claimed an
// unknown kind "still resolves to its feature" — it did not; it returned
// "widget/newkind:x".
func BareAboutFeature(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.Contains(ref, ":") {
		if parsed, err := ParseAmendmentRef(ref); err == nil {
			return parsed.Feature
		}
		return ""
	}
	return strings.TrimPrefix(ref, "@")
}
