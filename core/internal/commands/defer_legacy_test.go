package commands

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

func deferRun(t *testing.T, cfg *config.Context, feature string) error {
	t.Helper()
	cmd := testCommandWithContext(t, cfg)
	cmd.Args = cobra.ExactArgs(1)
	var out strings.Builder
	cmd.SetOut(&out)
	return runDeferLegacyExemption(cmd, []string{feature})
}

func deferFixture(t *testing.T) (*config.Context, []legacyEntry, string) {
	t.Helper()
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	legacyFixture(t, cfg, twoJudgmentsOnOneBullet)
	entries, hash, err := loadLegacyEntries(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deferLegacyFP, deferLegacyDup, deferLegacyHash = "", 0, ""
		deferLegacyReason, deferLegacyBy = "", ""
	})
	return cfg, entries, hash
}

// The load-bearing property. A deferral must never unblock, no matter how many
// accumulate: treating uncertainty as a completed outcome silently withdraws a
// possibly load-bearing waiver, which is the failure this reconciliation exists
// to prevent.
func TestDeferLegacy_NeverAnswersTheEntry(t *testing.T) {
	cfg, entries, hash := deferFixture(t)

	for _, who := range []string{"first reviewer", "second reviewer"} {
		deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
		deferLegacyBy = who
		deferLegacyReason = "cannot tell whether the constraint still exists"
		if err := deferRun(t, cfg, "graded"); err != nil {
			t.Fatal(err)
		}
	}

	stranded, unreadable := strandedLegacyExemptions(cfg, "graded", mustLoadExc(t, cfg))
	if unreadable != "" {
		t.Fatal(unreadable)
	}
	if len(stranded) != 2 {
		t.Fatalf("both entries are still unanswered — deferring must not reduce the count; got %d: %v", len(stranded), stranded)
	}
	joined := strings.Join(stranded, "\n")
	if !strings.Contains(joined, "2 reviews") {
		t.Errorf("the report should surface that people already looked: %s", joined)
	}
	if !strings.Contains(joined, "none conclusive") {
		t.Errorf("the report must not let attempts read as progress: %s", joined)
	}
}

// Two people independently unable to decide is a materially different fact from
// one attempt overwritten twice.
func TestDeferLegacy_AttemptsAppendRatherThanReplace(t *testing.T) {
	cfg, entries, hash := deferFixture(t)
	for _, who := range []string{"alice", "bob"} {
		deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
		deferLegacyBy, deferLegacyReason = who, "unclear to me"
		if err := deferRun(t, cfg, "graded"); err != nil {
			t.Fatal(err)
		}
	}
	rec := mustLoadExc(t, cfg)
	if len(rec.DeferredLegacy) != 2 {
		t.Fatalf("both attempts must survive; got %d", len(rec.DeferredLegacy))
	}
	if rec.DeferredLegacy[0].By != "alice" {
		t.Errorf("the earlier attempt must not be overwritten; got %q", rec.DeferredLegacy[0].By)
	}
}

// A write can succeed while its response is lost. The retry that follows is the
// same act of review and must not become a second attempt — nor an error the
// operator has to reason about.
func TestDeferLegacy_AnExactRetryIsIdempotent(t *testing.T) {
	cfg, entries, hash := deferFixture(t)
	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"

	for i := 0; i < 3; i++ {
		if err := deferRun(t, cfg, "graded"); err != nil {
			t.Fatalf("a retry must not error (attempt %d): %v", i+1, err)
		}
	}
	if n := len(mustLoadExc(t, cfg).DeferredLegacy); n != 1 {
		t.Fatalf("three identical submissions are one act of review; got %d entries", n)
	}

	// A different reason is a genuinely different attempt.
	deferLegacyReason = "checked the schema, still cannot tell"
	if err := deferRun(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}
	if n := len(mustLoadExc(t, cfg).DeferredLegacy); n != 2 {
		t.Fatalf("a distinct attempt must append; got %d", n)
	}
}

// Deferring is not a way around the token discipline: an attempt recorded
// against an entry the reviewer never saw is as misleading as a decision.
func TestDeferLegacy_RequiresTheVersionItWasShown(t *testing.T) {
	cfg, entries, _ := deferFixture(t)
	deferLegacyFP = entries[0].Fingerprint
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
	deferLegacyHash = ""

	if err := deferRun(t, cfg, "graded"); err == nil || !strings.Contains(err.Error(), "--legacy-file-hash is required") {
		t.Fatalf("want a refusal naming the missing token; got: %v", err)
	}
}

// Deferring an entry somebody already decided files an attempt against a closed
// question. Saying so is more useful than accepting it.
func TestDeferLegacy_RefusesAnAlreadyAnsweredEntry(t *testing.T) {
	cfg, entries, hash := deferFixture(t)

	resetExcFlags()
	recordExceptionLegacyFP, recordExceptionLegacyHash = entries[0].Fingerprint, hash
	recordExceptionRef, recordExceptionText = entries[0].Ref, entries[0].Text
	recordExceptionReason, recordExceptionBy = "still enforced", "interactive decision"
	recordExceptionFromLegacy = true
	if err := recordExc(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
	err := deferRun(t, cfg, "graded")
	if err == nil || !strings.Contains(err.Error(), "already answered") {
		t.Fatalf("want a refusal naming the existing decision; got: %v", err)
	}
}

// A deferred entry is still open, so it must remain answerable.
func TestDeferLegacy_ADeferredEntryCanStillBeDecided(t *testing.T) {
	cfg, entries, hash := deferFixture(t)
	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
	if err := deferRun(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	resetExcFlags()
	recordExceptionLegacyFP, recordExceptionLegacyHash = entries[0].Fingerprint, hash
	recordExceptionRef, recordExceptionText = entries[0].Ref, entries[0].Text
	recordExceptionReason, recordExceptionBy = "confirmed the constraint exists", "interactive decision"
	recordExceptionFromLegacy = true
	if err := recordExc(t, cfg, "graded"); err != nil {
		t.Fatalf("a deferred entry must remain answerable: %v", err)
	}

	rec := mustLoadExc(t, cfg)
	if len(rec.DeferredLegacy) != 1 {
		t.Error("the earlier attempt must survive the decision — it is why the entry took two passes")
	}
	// The two fixture entries share a ref AND criterion text, differing only in
	// why they were granted — so the report must disambiguate them, and the
	// assertion must key on the occurrence rather than the pair.
	stranded, _ := strandedLegacyExemptions(cfg, "graded", rec)
	for _, s := range stranded {
		if strings.Contains(s, entries[0].Fingerprint[:8]) {
			t.Fatalf("the answered occurrence must no longer be stranded: %s", s)
		}
	}
	if len(stranded) != 1 {
		t.Fatalf("its sibling is untouched and must still be reported; got %v", stranded)
	}
	if !strings.Contains(stranded[0], entries[1].Fingerprint[:8]) {
		t.Errorf("the report must identify WHICH occurrence remains: %s", stranded[0])
	}
}
