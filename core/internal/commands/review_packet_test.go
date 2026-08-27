package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func packetFor(t *testing.T, label string) *ReviewPacket {
	t.Helper()
	cfg := testContext(t)
	st, err := CollectCoverageMigrationStatus(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range st.Occurrences {
		if label == "" || o.Label == label {
			p, err := BuildReviewPacket(cfg, "graded", o)
			if err != nil {
				t.Fatal(err)
			}
			return p
		}
	}
	t.Fatalf("no occurrence %q", label)
	return nil
}

// The reason this is a formatter and not a prose instruction: the order is a
// property of the artifact, testable, rather than a promise a lint can only
// approximate by checking which phrase appears first in an instruction file.
func TestReviewPacket_SubjectIsSerialisedBeforeHistory(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	legacyFixture(t, testContext(t), twoJudgmentsOnOneBullet)

	d := packetFor(t, "").Display
	subject := strings.Index(d, "What you are deciding about")
	history := strings.Index(d, historyLabel)
	if subject < 0 || history < 0 {
		t.Fatalf("the display must carry both blocks:\n%s", d)
	}
	if subject > history {
		t.Fatalf("the subject must be rendered before the historical context:\n%s", d)
	}
	// The options must NOT appear in the evidence. They belong to the chooser,
	// and stating them twice makes the formatter responsible for duplication
	// while inviting a caller to parse choices back out of prose.
	for _, label := range []string{"Still holds", "No longer holds", "I cannot say"} {
		if strings.Contains(d, label) {
			t.Errorf("option %q leaked into the rendered evidence", label)
		}
	}
	if !strings.Contains(historyLabel, "NOT WHAT YOU ARE DECIDING") {
		t.Error("the label must say plainly that history is not the subject")
	}
}

// A waiver frequently exists PRECISELY because nothing covers the requirement.
// An empty list reads as "no information"; this is the most important fact on
// the page and has to be stated.
func TestReviewPacket_SaysNothingCoversItRatherThanShowingAnEmptyList(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	legacyFixture(t, cfg, twoJudgmentsOnOneBullet)
	// No testcases.yaml at all — the shape a pre-testcases waiver is in.

	p := packetFor(t, "")
	if !p.subject.NothingCovers {
		t.Fatal("with no cases, the packet must record that explicitly")
	}
	if len(p.subject.Cases) != 0 {
		t.Errorf("no cases exist; got %v", p.subject.Cases)
	}
	if len(p.subject.Requirements) == 0 {
		t.Error("the requirement still exists and must be shown even when nothing observes it")
	}
	// And it must reach the REVIEWER, not merely the struct. Asserting the
	// field alone leaves the rendering free to omit it — the same split-path
	// blind spot as testing a command and its guidance separately.
	if !strings.Contains(p.Display, "NOTHING covers this") {
		t.Fatalf("the reviewer must be told nothing covers it, not shown an empty list:\n%s", p.Display)
	}
	if !strings.Contains(p.Display, "it stays untested") {
		t.Errorf("the consequence of re-confirming must be stated:\n%s", p.Display)
	}
}

func TestReviewPacket_ShowsWhatCurrentlyObservesTheRequirement(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	legacyFixture(t, cfg, twoJudgmentsOnOneBullet)

	current, err := CurrentCriteria(cfg, "graded")
	if err != nil || len(current) == 0 {
		t.Fatalf("fixture must declare requirements: %v", err)
	}
	var target AuthorizedCriterion
	for _, c := range current {
		if c.Ref == "@graded/operation:customer.archive" {
			target = c
			break
		}
	}
	if target.Ref == "" {
		t.Skip("fixture has no capability requirement to observe")
	}

	cases := "schema_version: 3\nsuites:\n  - name: Archive\n    cases:\n      - name: rejects unpaid\n        coverage: state-only\n        criterion: {ref: \"" + target.Ref + "\", text: \"" + target.Text + "\"}\n        steps: [\"try it\"]\n"
	if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "testcases.yaml"), []byte(cases), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg2 := testContext(t)
	st, err := CollectCoverageMigrationStatus(cfg2, "graded")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, o := range st.Occurrences {
		if o.Ref != target.Ref {
			continue
		}
		p, err := BuildReviewPacket(cfg2, "graded", o)
		if err != nil {
			t.Fatal(err)
		}
		if p.subject.NothingCovers {
			continue
		}
		found = true
		if len(p.subject.Cases) == 0 {
			t.Fatal("nothing_covers is false, so cases must be listed")
		}
		if !strings.Contains(p.subject.Cases[0], "state-only") {
			t.Errorf("the packet must say HOW the case observes, since a weak observation changes the answer: %q", p.subject.Cases[0])
		}
	}
	if !found {
		t.Error("the occurrence on this requirement should have found the case")
	}
}

// The failure this closes: an old entry naming only an operation may now cover
// several requirements, and re-confirming it would grant a waiver over all of
// them. The breadth must be visible in the question, not just inferable from a
// list nobody counts.
func TestReviewPacket_EntryWideBreadthIsStatedInTheQuestion(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	// The shared fixture declares ONE requirement per operation, which cannot
	// witness breadth at all: entry-wide and bullet-specific resolve to the
	// same single item, so both the "show every requirement" logic and the
	// "name the breadth" logic could be deleted and this would still pass.
	// Three requirements is the smallest fixture where the property is real.
	caps := `schema_version: 1
feature: graded
operations:
  - id: customer.archive
    source: '@graded/archive-a-customer'
    kind: command
    subject:
      entity: Customer
    verify:
      - archiving a customer with unpaid invoices is rejected
      - archiving twice is idempotent
      - an archived customer stops appearing in search
    steps:
      - { type: validate-input }
`
	if err := os.WriteFile(filepath.Join(cfg.FeaturePath("graded"), "capabilities.yaml"), []byte(caps), 0o644); err != nil {
		t.Fatal(err)
	}

	entryWide := "schema_version: 1\nfeature: graded\nexemptions:\n" +
		"    - suite: s\n      item: \"@graded/operation:customer.archive\"\n      reason: the whole operation was stubbed\n"
	legacyFixture(t, cfg, entryWide)

	p := packetFor(t, "")
	if !p.subject.EntryWide {
		t.Fatal("an exemption naming no criterion text is entry-wide")
	}
	if len(p.subject.Requirements) != 3 {
		t.Fatalf("re-confirming this grants a waiver over every requirement now on the operation, so all three must be shown; got %d: %v",
			len(p.subject.Requirements), p.subject.Requirements)
	}
	if !strings.Contains(p.Decision.Question, "ALL 3") {
		t.Errorf("breadth must be stated in the question, not left inferable from a list nobody counts; got %q", p.Decision.Question)
	}
}

// The decision schema is the part that IS structurally checkable, so it is
// checked: exactly three outcomes, no default, rationale required.
func TestReviewPacket_OffersThreeOutcomesAndNoDefault(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	legacyFixture(t, testContext(t), twoJudgmentsOnOneBullet)

	p := packetFor(t, "")
	if len(p.Decision.Options) != 3 {
		t.Fatalf("exactly three outcomes; got %+v", p.Decision.Options)
	}
	want := map[string]bool{OutcomeReconfirm: true, OutcomeDrop: true, OutcomeDefer: true}
	for _, o := range p.Decision.Options {
		if !want[o.ID] {
			t.Errorf("unexpected outcome id %q", o.ID)
		}
		delete(want, o.ID)
		if strings.TrimSpace(o.Label) == "" || strings.TrimSpace(o.Description) == "" {
			t.Errorf("option %q needs a label and a description a person can act on", o.ID)
		}
		for _, hint := range []string{"recommend", "(default)", "suggested"} {
			if strings.Contains(strings.ToLower(o.Label+o.Description), hint) {
				t.Errorf("option %q steers the reviewer with %q", o.ID, hint)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing outcomes: %v", want)
	}
	if !p.Decision.NoDefault || !p.Decision.RationaleRequired {
		t.Error("the contract must state that there is no default and a rationale is required")
	}
	// The old reason must never be positioned as a starting point for the new
	// one. It appears in History and nowhere else.
	raw, _ := json.Marshal(p)
	if strings.Count(string(raw), "enforced by a database constraint") > 1 {
		t.Error("the prior reason must appear only as history, never seeded into the answer")
	}
}

// Earlier attempts belong in history, so the next reviewer knows the question
// already defeated somebody.
func TestReviewPacket_CarriesPriorAttempts(t *testing.T) {
	cfg, entries, hash := deferFixture(t)
	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "could not find the constraint"
	if err := deferRun(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	st, _ := CollectCoverageMigrationStatus(cfg, "graded")
	for _, o := range st.Occurrences {
		if o.State != "deferred" {
			continue
		}
		p, err := BuildReviewPacket(cfg, "graded", o)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.history.Attempts) != 1 {
			t.Fatalf("the prior attempt must be carried; got %v", p.history.Attempts)
		}
		if !strings.Contains(p.history.Attempts[0], "alice") ||
			!strings.Contains(p.history.Attempts[0], "could not find the constraint") {
			t.Errorf("the attempt must name who and why: %q", p.history.Attempts[0])
		}
		return
	}
	t.Fatal("no deferred occurrence found")
}

// Every pending occurrence must produce a usable packet. An entry that cannot
// be rendered is an entry that cannot be answered, and it would block the
// migration with no way forward.
func TestReviewPacket_EveryPendingOccurrenceRenders(t *testing.T) {
	cfg, entries, hash := deferFixture(t)
	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
	if err := deferRun(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	st, err := CollectCoverageMigrationStatus(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, o := range st.Occurrences {
		if o.State == "answered" {
			continue
		}
		p, err := BuildReviewPacket(cfg, "graded", o)
		if err != nil {
			t.Fatalf("%s (%s): %v", o.Label, o.State, err)
		}
		if strings.TrimSpace(p.Display) == "" {
			t.Errorf("%s renders an empty display", o.Label)
		}
		if strings.TrimSpace(p.Decision.Question) == "" {
			t.Errorf("%s asks nothing", o.Label)
		}
		if p.Fingerprint == "" {
			t.Errorf("%s carries no writable token", o.Label)
		}
		seen++
	}
	if seen != st.PendingTotal {
		t.Fatalf("checked %d packets for %d pending occurrences", seen, st.PendingTotal)
	}
}

// Breadth must reach the chooser, not just the evidence. A reviewer skimming
// straight to the options should still see that "still holds" means all of
// them.
func TestReviewPacket_BreadthReachesTheDecisionNotJustTheDisplay(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	caps := `schema_version: 1
feature: graded
operations:
  - id: customer.archive
    source: '@graded/archive-a-customer'
    kind: command
    subject:
      entity: Customer
    verify:
      - archiving a customer with unpaid invoices is rejected
      - archiving twice is idempotent
      - an archived customer stops appearing in search
    steps:
      - { type: validate-input }
`
	if err := os.WriteFile(filepath.Join(cfg.FeaturePath("graded"), "capabilities.yaml"), []byte(caps), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyFixture(t, cfg, "schema_version: 1\nfeature: graded\nexemptions:\n"+
		"    - suite: s\n      item: \"@graded/operation:customer.archive\"\n      reason: the whole operation was stubbed\n")

	p := packetFor(t, "")
	var reconfirm ReviewOption
	for _, o := range p.Decision.Options {
		if o.ID == OutcomeReconfirm {
			reconfirm = o
		}
	}
	if !strings.Contains(reconfirm.Description, "ALL 3") {
		t.Errorf("the re-confirm option must say it waives all three: %q", reconfirm.Description)
	}
}
