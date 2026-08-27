package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

func nextReview(t *testing.T, cfg *config.Context, exclude ...string) nextReviewOutput {
	t.Helper()
	nextReviewExclude = exclude
	t.Cleanup(func() { nextReviewExclude = nil })
	cmd := testCommandWithContext(t, cfg)
	cmd.Args = cobra.ExactArgs(1)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runNextLegacyReview(cmd, []string{"graded"}); err != nil {
		t.Fatalf("next-legacy-review: %v", err)
	}
	var out nextReviewOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	return out
}

// Two projections over one semantic object, never two sources of truth. If a
// later presenter starts deriving its own counts, this is what catches it.
func TestNextLegacyReview_SummaryMatchesTheListing(t *testing.T) {
	cfg, entries, hash := deferFixture(t)
	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
	if err := deferRun(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	listing, err := CollectCoverageMigrationStatus(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	got := nextReview(t, cfg).Summary

	want, _ := json.Marshal(listing)
	have, _ := json.Marshal(got)
	if string(want) != string(have) {
		t.Fatalf("the summary must equal the listing exactly:\nlisting: %s\nnext:    %s", want, have)
	}
}

// The loop bug this exists to prevent. Deferring does not answer an entry, so
// without exclusions the sitting eventually hands back the question it just
// asked — and once no never-reviewed entries remain, forever.
func TestNextLegacyReview_AdvancesAfterADeferralRatherThanRepeating(t *testing.T) {
	cfg, _, _ := deferFixture(t)

	deferNext := func(prev ...string) nextReviewOutput {
		t.Helper()
		out := nextReview(t, cfg, prev...)
		if out.Packet == nil {
			t.Fatalf("expected an occurrence to review; got %+v", out)
		}
		deferLegacyFP, deferLegacyHash = out.Tokens.Fingerprint, out.Tokens.LegacyFileHash
		deferLegacyDup = out.Tokens.Duplicate
		deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
		if err := deferRun(t, cfg, "graded"); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// Defer everything. While never-reviewed entries remain, traversal moves on
	// by itself — the trap only springs once every pending entry is deferred
	// and there is nothing fresh to offer instead.
	first := deferNext()
	second := deferNext(first.Tokens.Fingerprint)
	if second.Tokens.Fingerprint == first.Tokens.Fingerprint {
		t.Fatal("an untouched entry should have been offered before revisiting a deferred one")
	}

	// Now a bare call has only deferred entries to choose from, and hands back
	// the first one again. Without exclusions the sitting loops here forever.
	repeat := nextReview(t, cfg)
	if repeat.Packet == nil || repeat.Tokens.Fingerprint != first.Tokens.Fingerprint {
		t.Fatalf("with everything deferred, a bare call re-offers the first — that is the loop exclusions exist to break; got %+v", repeat.Tokens)
	}

	// With both excluded, the sitting ends — and says plainly that deferring
	// left work behind rather than reporting completion.
	done := nextReview(t, cfg, first.Tokens.Fingerprint, second.Tokens.Fingerprint)
	if !done.Done || done.Packet != nil {
		t.Fatalf("nothing remains for this sitting; got %+v", done)
	}
	if done.Exhausted {
		t.Fatal("both entries were deferred, not answered — this is not completion")
	}
	if !strings.Contains(done.Note, "still unanswered") {
		t.Errorf("a finished sitting must not read as a finished migration: %q", done.Note)
	}

	// A later sitting starts fresh and revisits the deferred work, since no
	// session state was written anywhere.
	fresh := nextReview(t, cfg)
	if fresh.Packet == nil {
		t.Fatal("a new sitting must offer the deferred entries again")
	}
}

// A stale exclusion is refused, not ignored. Accepting one would let a caller
// carrying identities from an older version of the legacy file skip
// occurrences that are not the ones it believes it handled.
func TestNextLegacyReview_RefusesAnExclusionItCannotPlace(t *testing.T) {
	cfg, _, _ := deferFixture(t)
	nextReviewExclude = []string{"0000000000000000000000000000000000000000000000000000000000000000"}
	t.Cleanup(func() { nextReviewExclude = nil })
	cmd := testCommandWithContext(t, cfg)
	cmd.Args = cobra.ExactArgs(1)
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := runNextLegacyReview(cmd, []string{"graded"})
	if err == nil {
		t.Fatal("an exclusion naming nothing in the current status must be refused")
	}
	if !strings.Contains(err.Error(), "start the sitting again") {
		t.Errorf("the refusal must tell the caller what to do: %v", err)
	}
}

// When everything really is answered, the difference matters: a migration that
// is finished reads differently from a sitting that is.
func TestNextLegacyReview_ReportsRealCompletionDistinctly(t *testing.T) {
	cfg, entries, hash := deferFixture(t)
	for _, e := range entries {
		resetExcFlags()
		recordExceptionLegacyFP, recordExceptionLegacyHash = e.Fingerprint, hash
		recordExceptionRef, recordExceptionText = e.Ref, e.Text
		recordExceptionReason, recordExceptionBy = "still holds", "interactive decision"
		recordExceptionFromLegacy = true
		if err := recordExc(t, cfg, "graded"); err != nil {
			t.Fatal(err)
		}
	}
	out := nextReview(t, cfg)
	if !out.Done || !out.Exhausted {
		t.Fatalf("every entry is answered; want done and all_answered: %+v", out)
	}
	if !strings.Contains(out.Note, "stop reporting") {
		t.Errorf("completion should say the boundary goes quiet: %q", out.Note)
	}
}

// Tokens are for the skill to forward, never for a person to read or retype.
func TestNextLegacyReview_KeepsTokensOutOfTheDisplay(t *testing.T) {
	cfg, _, _ := deferFixture(t)
	out := nextReview(t, cfg)
	if out.Tokens == nil || out.Tokens.Fingerprint == "" || out.Tokens.LegacyFileHash == "" {
		t.Fatalf("the envelope must carry both writer tokens: %+v", out.Tokens)
	}
	if strings.Contains(out.Packet.Display, out.Tokens.Fingerprint) {
		t.Error("the full fingerprint must not appear in the evidence a person reads")
	}
	if strings.Contains(out.Packet.Display, out.Tokens.LegacyFileHash) {
		t.Error("the file hash must not appear in the evidence a person reads")
	}
	// The packet and the envelope must agree about which occurrence this is.
	if out.Packet.Fingerprint != out.Tokens.Fingerprint {
		t.Error("packet and tokens name different occurrences")
	}
}
