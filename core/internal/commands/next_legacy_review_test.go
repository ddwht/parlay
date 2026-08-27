package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// The defect this closes: the skill was told to build the command from packet
// fields that are deliberately absent from the JSON, to always pass a
// --criterion that entry-wide occurrences must omit, and to exclude by a bare
// fingerprint that is wrong when copies exist. Every leaf test passed and the
// walkthrough was not walkable.
//
// So the actions are EXECUTED here, exactly as emitted, with only the authority
// values appended — which is what the skill does.
func TestNextLegacyReview_EmittedActionsActuallyRun(t *testing.T) {
	for _, tc := range []struct{ name, outcome string }{
		{"reconfirm", OutcomeReconfirm},
		{"drop", OutcomeDrop},
		{"defer", OutcomeDefer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, _ := deferFixture(t)
			out := nextReview(t, cfg)
			if out.Packet == nil {
				t.Fatal("expected an occurrence")
			}
			action, ok := out.Actions[tc.outcome]
			if !ok {
				t.Fatalf("no action for %q; got %v", tc.outcome, out.Actions)
			}
			for _, want := range []string{"--reason", "--by"} {
				if !hasArg(action.Requires, want) {
					t.Errorf("%s must be listed as caller-supplied, not baked in", want)
				}
				if hasArg(action.Args, want) {
					t.Errorf("%s must NOT be pre-filled: a literal here would put the same attribution on every judgment", want)
				}
			}
			runEmitted(t, cfg, action, "the reviewer's own words", "reviewer@example.test")
		})
	}
}

// Entry-wide occurrences must omit --criterion. Passing one narrows a decision
// the reviewer was explicitly asked to make over every requirement.
func TestNextLegacyReview_EntryWideActionOmitsTheCriterion(t *testing.T) {
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
    steps:
      - { type: validate-input }
`
	if err := os.WriteFile(filepath.Join(cfg.FeaturePath("graded"), "capabilities.yaml"), []byte(caps), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyFixture(t, cfg, "schema_version: 1\nfeature: graded\nexemptions:\n"+
		"    - suite: s\n      item: \"@graded/operation:customer.archive\"\n      reason: stubbed for the beta\n")

	out := nextReview(t, cfg)
	action := out.Actions[OutcomeReconfirm]
	if hasArg(action.Args, "--criterion") {
		t.Fatalf("an entry-wide decision must not be recorded against one bullet: %v", action.Args)
	}
	runEmitted(t, cfg, action, "still stubbed", "reviewer@example.test")

	// And it really did answer the entry, so the sitting can move on.
	st, _ := CollectCoverageMigrationStatus(cfg, "graded")
	if st.PendingTotal != 0 {
		t.Fatalf("the only entry was answered; got %d pending", st.PendingTotal)
	}
}

// Copies share a fingerprint by design, so the exclusion token must carry the
// copy index or excluding one fails to exclude it.
func TestNextLegacyReview_ExcludeTokenCarriesTheCopyIndex(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	identical := "    - suite: s\n      item: \"@graded/operation:customer.archive\"\n" +
		"      criterion_text: archiving a customer with unpaid invoices is rejected\n      reason: same in every field\n"
	legacyFixture(t, cfg, "schema_version: 1\nfeature: graded\nexemptions:\n"+identical+identical)

	first := nextReview(t, cfg)
	if first.ExcludeToken == "" {
		t.Fatal("an exclusion token must be emitted ready-made")
	}
	second := nextReview(t, cfg, first.ExcludeToken)
	if second.Packet == nil {
		t.Fatal("the second copy is a separate judgment and must be offered")
	}
	if second.ExcludeToken == first.ExcludeToken {
		t.Fatalf("the sitting re-offered the same copy: both tokens are %q", first.ExcludeToken)
	}
	if !strings.Contains(second.ExcludeToken, "#1") {
		t.Errorf("the second copy's token must carry its index; got %q", second.ExcludeToken)
	}
}

func hasArg(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// runEmitted executes an action exactly as emitted, appending only the values a
// person supplies.
func runEmitted(t *testing.T, cfg *config.Context, a ReviewAction, reason, by string) {
	t.Helper()
	argv := append(append([]string{}, a.Args...), "--reason", reason, "--by", by)

	var run func(*cobra.Command, []string) error
	var flags *pflag.FlagSet
	switch a.Command {
	case "record-exception":
		run, flags = runRecordException, recordExceptionCmd.Flags()
	case "drop-legacy-exemption":
		run, flags = runDropLegacyExemption, dropLegacyCmd.Flags()
	case "defer-legacy-exemption":
		run, flags = runDeferLegacyExemption, deferLegacyCmd.Flags()
	default:
		t.Fatalf("action names an unknown command %q", a.Command)
	}

	resetFlagsAfterTest(t, flags)
	if err := flags.Parse(argv[1:]); err != nil {
		t.Fatalf("%s: emitted args do not parse: %v\nargv: %v", a.Command, err, argv)
	}
	cmd := testCommandWithContext(t, cfg)
	cmd.Args = cobra.ExactArgs(1)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := run(cmd, []string{strings.TrimPrefix(argv[0], "@")}); err != nil {
		t.Fatalf("%s refused its own emitted arguments: %v\nargv: %v", a.Command, err, argv)
	}
}
