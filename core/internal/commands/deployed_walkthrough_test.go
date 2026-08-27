package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Phase 7. The seam, not the two halves.
//
// Everything this session went wrong in the same place: leaf code passed, its
// guidance passed, and the join between them was never exercised. The skill
// named a field that had been removed from the JSON, always passed a flag that
// must be omitted, and excluded by a token that was wrong for duplicates —
// while every unit test was green and a hand check of the flags reported
// success.
//
// So this starts from the DEPLOYED module text, extracts the protocol it tells
// a reader to follow, and proves the production emitter and writers implement
// that same protocol. It cannot prove an agent obeys prose. It can prove the
// prose describes something that exists and works.

func deployedModule(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoAnchor(t), ".parlay", "modules", "migrate-coverage.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the deployed walkthrough is missing at %s: %v — this is repository-owned input, so its absence fails rather than skips", path, err)
	}
	return string(body)
}

var moduleCommandRe = regexp.MustCompile(`parlay internal ([a-z-]+)`)

// The module must name commands that exist, and every command it names must be
// one this classification calls skill-required or probe — never something a
// person is told to run that the CLI does not have.
func TestDeployedWalkthrough_NamesOnlyRealCommands(t *testing.T) {
	w := newWitness(t)
	body := deployedModule(t)

	named := map[string]bool{}
	for _, m := range moduleCommandRe.FindAllStringSubmatch(body, -1) {
		named[m[1]] = true
	}
	w.given("the deployed module names at least one command", len(named) > 0)

	registered := map[string]*cobra.Command{}
	for _, c := range internalCommands(t) {
		registered[c.Name()] = c
	}
	w.given("the internal command surface is registered", len(registered) > 0)

	w.assert("every named command exists and is reachable by design", func() {
		for name := range named {
			c, ok := registered[name]
			if !ok {
				t.Errorf("the module tells a reader to run `parlay internal %s`, which does not exist", name)
				continue
			}
			switch ReachabilityClass(c) {
			case ClassSkillRequired, ClassProbe:
			default:
				t.Errorf("the module names %s, classified %q — a walkthrough should only drive skill-required or probe commands",
					name, ReachabilityClass(c))
			}
		}
	})
}

// The module tells the reader to run actions[choice] with its args, appending
// only what `requires` names, and to exclude by exclude_token. This proves the
// emitter actually provides those fields under those names — the failure that
// made the previous version unwalkable was a field the prose named and the JSON
// did not carry.
func TestDeployedWalkthrough_ProtocolFieldsExist(t *testing.T) {
	w := newWitness(t)
	body := deployedModule(t)
	cfg, _, _ := deferFixture(t)

	for _, field := range []string{"actions[", "requires", "exclude_token", "packet.display", "packet.decision"} {
		w.given("the module references "+field, strings.Contains(body, field))
	}

	out := nextReview(t, cfg)
	w.given("the emitter produced an occurrence to review", out.Packet != nil)

	w.assert("every field the module names is present in the envelope", func() {
		var envelope map[string]any
		cmd := testCommandWithContext(t, cfg)
		cmd.Args = cobra.ExactArgs(1)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		nextReviewExclude = nil
		if err := runNextLegacyReview(cmd, []string{"graded"}); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"packet", "actions", "exclude_token", "tokens", "summary"} {
			if _, ok := envelope[key]; !ok {
				t.Errorf("the module reads %q from the envelope, which does not emit it", key)
			}
		}
		packet, _ := envelope["packet"].(map[string]any)
		for _, key := range []string{"display", "decision"} {
			if _, ok := packet[key]; !ok {
				t.Errorf("the module reads packet.%s, which is not emitted", key)
			}
		}
	})
}

// The whole protocol, driven as the module describes it, for each outcome.
// This is the check that would have caught all three unwalkable defects.
func TestDeployedWalkthrough_DrivesEachOutcomeEndToEnd(t *testing.T) {
	body := deployedModule(t)

	for _, outcome := range []string{OutcomeReconfirm, OutcomeDrop, OutcomeDefer} {
		t.Run(outcome, func(t *testing.T) {
			w := newWitness(t)
			w.given("the module documents the "+outcome+" outcome", strings.Contains(body, outcome))

			cfg, _, _ := deferFixture(t)
			before, err := CollectCoverageMigrationStatus(cfg, "graded")
			if err != nil {
				t.Fatal(err)
			}
			w.given("the fixture has pending work to decide", before.PendingTotal > 0)

			// Step 2 of the module: ask for the next one.
			out := nextReview(t, cfg)
			w.given("the walkthrough was handed an occurrence", out.Packet != nil)
			w.given("it was handed a plan for this outcome", func() bool {
				_, ok := out.Actions[outcome]
				return ok
			}())

			// Step 3: the display is what a person reads, and carries no tokens.
			w.given("the display is non-empty", strings.TrimSpace(out.Packet.Display) != "")

			// Step 4: the chooser is built from decision, which offers exactly
			// the three documented outcomes.
			w.given("the decision offers three options", len(out.Packet.Decision.Options) == 3)

			// Steps 5 and 6: supply the authority values and run the plan.
			w.assert("running the emitted plan records the decision", func() {
				action := out.Actions[outcome]
				runEmitted(t, cfg, action, "a reason in the reviewer's own words", "reviewer@example.test")

				after, err := CollectCoverageMigrationStatus(cfg, "graded")
				if err != nil {
					t.Fatal(err)
				}
				switch outcome {
				case OutcomeReconfirm, OutcomeDrop:
					if after.Answered != before.Answered+1 {
						t.Fatalf("%s must answer exactly one occurrence: %d -> %d", outcome, before.Answered, after.Answered)
					}
					if after.PendingTotal != before.PendingTotal-1 {
						t.Errorf("pending must fall by one: %d -> %d", before.PendingTotal, after.PendingTotal)
					}
				case OutcomeDefer:
					if after.Answered != before.Answered {
						t.Fatalf("deferring must answer nothing: %d -> %d", before.Answered, after.Answered)
					}
					if after.PendingTotal != before.PendingTotal {
						t.Fatalf("deferring must not reduce pending work: %d -> %d", before.PendingTotal, after.PendingTotal)
					}
					if after.PendingDeferred != before.PendingDeferred+1 {
						t.Errorf("the attempt must be visible: %d -> %d", before.PendingDeferred, after.PendingDeferred)
					}
				}
			})

			// Step 6's last instruction: the exclusion token advances the sitting.
			w.assert("the emitted exclusion token advances the sitting", func() {
				next := nextReview(t, cfg, out.ExcludeToken)
				if next.Packet != nil && next.ExcludeToken == out.ExcludeToken {
					t.Fatal("the sitting re-offered the occurrence just handled")
				}
			})
		})
	}
}

// The module tells the reader to restart from the listing when a write is
// refused for a changed file. That refusal has to actually happen.
func TestDeployedWalkthrough_StaleTokensAreRefusedAsDocumented(t *testing.T) {
	w := newWitness(t)
	body := deployedModule(t)
	w.given("the module documents the stale-file restart", strings.Contains(body, "changed after it was listed"))

	cfg, _, _ := deferFixture(t)
	out := nextReview(t, cfg)
	w.given("a plan was emitted against the current file", out.Packet != nil)

	// The retired review changes after the reviewer was shown it.
	legacyFixture(t, cfg, twoJudgmentsOnOneBullet+
		"    - suite: s\n      item: \"@graded/fragment:Customer Detail\"\n      reason: added after the listing\n")

	w.assert("the writer refuses a plan built against the older file", func() {
		action := out.Actions[OutcomeReconfirm]
		argv := append(append([]string{}, action.Args...), "--reason", "r", "--by", "reviewer@example.test")
		resetFlagsAfterTest(t, recordExceptionCmd.Flags())
		if err := recordExceptionCmd.Flags().Parse(argv[1:]); err != nil {
			t.Fatal(err)
		}
		cmd := testCommandWithContext(t, cfg)
		cmd.Args = cobra.ExactArgs(1)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		err := runRecordException(cmd, []string{"graded"})
		if err == nil {
			t.Fatal("a decision approved against a stale listing must be refused")
		}
		if !strings.Contains(err.Error(), "changed after it was listed") {
			t.Fatalf("the refusal must be the one the module tells the reader to expect; got: %v", err)
		}
	})
}

// Phase 6. The lifecycle witness, driven through the same route a person takes.
//
// My first draft of this test asserted that walking all three outcomes leaves
// the gate passing. Codex caught that it contradicts the design: if "I cannot
// say" correctly does not answer an entry, a run containing one MUST still
// block. The test would have encoded the bug as the requirement — the same
// shape as the audit test I had to correct earlier this session.
//
// It uses the emitted plans rather than calling the writers directly, because a
// second direct path could pass while the route a person actually takes stayed
// broken. That is the failure this release exists to eliminate.
func TestDeployedWalkthrough_DeferBlocksUntilTheExactEntryIsResolved(t *testing.T) {
	w := newWitness(t)
	cfg, _, _ := deferFixture(t)

	start, err := CollectCoverageMigrationStatus(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	w.given("the fixture holds two distinct stranded judgments", start.PendingTotal == 2)
	w.given("they are distinguishable", start.Occurrences[0].Fingerprint != start.Occurrences[1].Fingerprint)

	blockerNames := func(rec *CoverageExceptions) []string {
		t.Helper()
		stranded, unreadable := strandedLegacyExemptions(cfg, "graded", rec)
		if unreadable != "" {
			t.Fatal(unreadable)
		}
		return stranded
	}
	w.given("the boundary reports both before anything is decided", len(blockerNames(nil)) == 2)

	// Sitting one: answer the first, defer the second.
	first := nextReview(t, cfg)
	runEmitted(t, cfg, first.Actions[OutcomeReconfirm], "the constraint still enforces it", "reviewer@example.test")

	second := nextReview(t, cfg, first.ExcludeToken)
	if second.Packet == nil {
		t.Fatal("a second judgment is pending and must be offered")
	}
	deferredToken := second.ExcludeToken
	deferredLabel := second.Packet.Label
	runEmitted(t, cfg, second.Actions[OutcomeDefer], "cannot find whether that path still exists", "reviewer@example.test")

	w.assert("a deferral leaves the boundary blocking, naming that exact entry", func() {
		rec := mustLoadExc(t, cfg)
		stranded := blockerNames(rec)
		if len(stranded) != 1 {
			t.Fatalf("one entry was answered and one deferred, so exactly one must still block; got %d: %v", len(stranded), stranded)
		}
		if !strings.Contains(stranded[0], deferredLabel) {
			t.Errorf("the blocker must name the deferred occurrence specifically: %q", stranded[0])
		}
		if !strings.Contains(stranded[0], "none conclusive") {
			t.Errorf("it must not read as progress: %q", stranded[0])
		}

		st, _ := CollectCoverageMigrationStatus(cfg, "graded")
		if st.PendingTotal != 1 || st.PendingDeferred != 1 {
			t.Errorf("the deferred entry is still pending work: %+v", st)
		}
		// The sitting is over; the migration is not. Those must not report the
		// same way.
		done := nextReview(t, cfg, first.ExcludeToken, deferredToken)
		if !done.Done {
			t.Fatal("both entries were handled this sitting")
		}
		if done.Exhausted {
			t.Fatal("one was deferred, not answered — the migration is not finished")
		}
	})

	// Sitting two: a later reviewer resolves THAT entry.
	w.assert("resolving the exact deferred entry is what finally clears it", func() {
		revisit := nextReview(t, cfg)
		if revisit.Packet == nil {
			t.Fatal("a new sitting must offer the deferred entry again")
		}
		if revisit.ExcludeToken != deferredToken {
			t.Fatalf("the new sitting must offer the entry left unresolved; got %q want %q", revisit.ExcludeToken, deferredToken)
		}
		// The earlier attempt is carried, so this reviewer starts from what the
		// last one could not settle rather than from nothing.
		if !strings.Contains(revisit.Packet.Display, "cannot find whether that path still exists") {
			t.Errorf("the prior attempt must be shown to this reviewer, so they start from what the last one could not settle:\n%s", revisit.Packet.Display)
		}

		runEmitted(t, cfg, revisit.Actions[OutcomeDrop], "that path was removed in the redesign", "second.reviewer@example.test")

		rec := mustLoadExc(t, cfg)
		if stranded := blockerNames(rec); len(stranded) != 0 {
			t.Fatalf("every judgment is answered, so nothing may still block: %v", stranded)
		}
		st, _ := CollectCoverageMigrationStatus(cfg, "graded")
		if st.PendingTotal != 0 || st.Answered != 2 {
			t.Fatalf("both entries must now be answered: %+v", st)
		}
		if st.DeferralAttempts != 1 {
			t.Errorf("the attempt stays as history after the entry is decided; got %d", st.DeferralAttempts)
		}

		final := nextReview(t, cfg)
		if !final.Done || !final.Exhausted {
			t.Fatalf("the migration is finished and must say so: %+v", final)
		}
	})
}
