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

	raw, err := json.Marshal(packetFor(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	subject, history := strings.Index(body, `"subject"`), strings.Index(body, `"history"`)
	if subject < 0 || history < 0 {
		t.Fatalf("packet must carry both blocks: %s", body)
	}
	if subject > history {
		t.Fatal("the subject must be serialised before the historical context")
	}
	if !strings.Contains(body, historyLabel) {
		t.Error("the historical block must carry its fixed label, not caller-chosen phrasing")
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
	if !p.Subject.NothingCovers {
		t.Fatal("with no cases, the packet must say so explicitly")
	}
	if len(p.Subject.Cases) != 0 {
		t.Errorf("no cases exist; got %v", p.Subject.Cases)
	}
	if len(p.Subject.Requirements) == 0 {
		t.Error("the requirement still exists and must be shown even when nothing observes it")
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
		if p.Subject.NothingCovers {
			continue
		}
		found = true
		if len(p.Subject.Cases) == 0 {
			t.Fatal("nothing_covers is false, so cases must be listed")
		}
		if !strings.Contains(p.Subject.Cases[0], "state-only") {
			t.Errorf("the packet must say HOW the case observes, since a weak observation changes the answer: %q", p.Subject.Cases[0])
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
	if !p.Subject.EntryWide {
		t.Fatal("an exemption naming no criterion text is entry-wide")
	}
	if len(p.Subject.Requirements) != 3 {
		t.Fatalf("re-confirming this grants a waiver over every requirement now on the operation, so all three must be shown; got %d: %v",
			len(p.Subject.Requirements), p.Subject.Requirements)
	}
	if !strings.Contains(p.Question.Prompt, "ALL 3") {
		t.Errorf("breadth must be stated in the question, not left inferable from a list nobody counts; got %q", p.Question.Prompt)
	}
}

// The decision schema is the part that IS structurally checkable, so it is
// checked: exactly three outcomes, no default, rationale required.
func TestReviewPacket_OffersThreeOutcomesAndNoDefault(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	legacyFixture(t, testContext(t), twoJudgmentsOnOneBullet)

	p := packetFor(t, "")
	if len(p.Question.Outcomes) != 3 {
		t.Fatalf("exactly three outcomes; got %v", p.Question.Outcomes)
	}
	want := map[string]bool{"still-holds": true, "no-longer-holds": true, "cannot-say": true}
	for _, o := range p.Question.Outcomes {
		if !want[o] {
			t.Errorf("unexpected outcome %q", o)
		}
		delete(want, o)
	}
	if len(want) != 0 {
		t.Errorf("missing outcomes: %v", want)
	}
	if !p.Question.NoDefault || !p.Question.RationaleRequired {
		t.Error("the packet must state that there is no default and a rationale is required")
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
		if len(p.History.Attempts) != 1 {
			t.Fatalf("the prior attempt must be carried; got %v", p.History.Attempts)
		}
		if !strings.Contains(p.History.Attempts[0], "alice") ||
			!strings.Contains(p.History.Attempts[0], "could not find the constraint") {
			t.Errorf("the attempt must name who and why: %q", p.History.Attempts[0])
		}
		return
	}
	t.Fatal("no deferred occurrence found")
}
