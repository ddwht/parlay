package commands

import (
	"strings"
	"testing"
)

// Labels must be proven unique in the set the reader is looking at. A fixed
// prefix that usually works and occasionally collides is worse than none: the
// collision surfaces as two entries that look interchangeable at exactly the
// moment somebody is choosing between them.
func TestShortestUniquePrefixes_GrowsUntilUnique(t *testing.T) {
	vals := []string{
		"aaaaaaaaaaaa1111",
		"aaaaaaaaaaaa2222", // shares 12 chars with the first
		"bbbbbbbbcccc",
	}
	got := shortestUniquePrefixes(vals, 8)

	seen := map[string]string{}
	for _, v := range vals {
		p := got[v]
		if !strings.HasPrefix(v, p) {
			t.Fatalf("%q is not a prefix of %q", p, v)
		}
		if len(p) < 8 {
			t.Errorf("%q is shorter than the minimum", p)
		}
		if prev, dup := seen[p]; dup {
			t.Fatalf("label %q is shared by %q and %q", p, prev, v)
		}
		seen[p] = v
	}
	if len(got["bbbbbbbbcccc"]) != 8 {
		t.Errorf("a value that collides with nothing should stay at the minimum; got %q", got["bbbbbbbbcccc"])
	}
	if len(got[vals[0]]) <= 12 {
		t.Errorf("a value sharing 12 characters must grow past them; got %q", got[vals[0]])
	}
}

func TestShortestUniquePrefixes_IdenticalValuesCollapse(t *testing.T) {
	// Byte-identical entries share a fingerprint by design; the copy index is
	// what separates them, and the prefix walk must not spin looking for a
	// difference that is not there.
	got := shortestUniquePrefixes([]string{"same0000", "same0000"}, 8)
	if got["same0000"] != "same0000" {
		t.Fatalf("identical values cannot be told apart by prefix; got %q", got["same0000"])
	}
}

// Counts are current state and mutually exclusive. A single undifferentiated
// pending number erases the distinction doctor exists to surface: untouched
// debt versus questions people examined and could not resolve.
func TestMigrationStatus_CountsAreCurrentStateAndExclusive(t *testing.T) {
	cfg, entries, hash := deferFixture(t)

	// One deferred, one untouched.
	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
	if err := deferRun(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	st, err := CollectCoverageMigrationStatus(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	if st.Answered != 0 || st.PendingDeferred != 1 || st.PendingUnreviewed != 1 {
		t.Fatalf("want 0 answered / 1 deferred / 1 unreviewed; got %+v", st)
	}
	if st.PendingTotal != 2 {
		t.Errorf("pending_total must be the sum; got %d", st.PendingTotal)
	}

	// Now answer the deferred one. Its attempts become history, and it counts
	// ONLY as answered — an occurrence cannot be in two current states.
	resetExcFlags()
	recordExceptionLegacyFP, recordExceptionLegacyHash = entries[0].Fingerprint, hash
	recordExceptionRef, recordExceptionText = entries[0].Ref, entries[0].Text
	recordExceptionReason, recordExceptionBy = "confirmed", "interactive decision"
	recordExceptionFromLegacy = true
	if err := recordExc(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	st, err = CollectCoverageMigrationStatus(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	if st.Answered != 1 || st.PendingDeferred != 0 || st.PendingUnreviewed != 1 {
		t.Fatalf("an answered occurrence with past attempts counts only as answered; got %+v", st)
	}
	if st.DeferralAttempts != 1 {
		t.Errorf("the attempt stays as audit history; got %d", st.DeferralAttempts)
	}
}

// Deferred entries must not trap traversal: putting the question somebody
// already found hard at the front of every sitting stalls the migration exactly
// where it should be making progress.
func TestMigrationStatus_NeverReviewedComesBeforeDeferred(t *testing.T) {
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
	next := st.NextToReview()
	if next == nil {
		t.Fatal("two entries are pending; one must be offered")
	}
	if next.State != "unreviewed" {
		t.Fatalf("the untouched entry must be offered before the deferred one; got %s (%s)", next.State, next.Label)
	}
	if next.Fingerprint != entries[1].Fingerprint {
		t.Errorf("wrong occurrence offered: %s", next.Label)
	}
}

// The label is for selecting; the fingerprint is what a writer takes. A short
// token that sometimes works and sometimes sends you to raw JSON is the split
// this avoids.
func TestMigrationStatus_CarriesFullTokensAlongsideTheLabel(t *testing.T) {
	cfg, entries, _ := deferFixture(t)
	st, err := CollectCoverageMigrationStatus(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range st.Occurrences {
		if o.Fingerprint == "" || len(o.Fingerprint) <= len(o.Label) {
			t.Fatalf("each occurrence must carry the full fingerprint, not the label: %+v", o)
		}
		if o.GrantedBecause == "" {
			t.Errorf("the granting reason is what tells two same-ref entries apart to a person: %+v", o)
		}
	}
	if len(st.Occurrences) != len(entries) {
		t.Fatalf("want %d occurrences, got %d", len(entries), len(st.Occurrences))
	}
	if st.Occurrences[0].Label == st.Occurrences[1].Label {
		t.Fatal("two occurrences sharing a ref and text must still get distinct labels")
	}
}

// The projection's structural invariants, so more consumers can read it safely.
// Both are the kind of property that silently stops holding when somebody adds
// a state and forgets one of the two places that enumerate them.
func TestMigrationStatus_CountsPartitionAndTraversalIsComplete(t *testing.T) {
	cfg, entries, hash := deferFixture(t)

	deferLegacyFP, deferLegacyHash = entries[0].Fingerprint, hash
	deferLegacyBy, deferLegacyReason = "alice", "cannot tell"
	if err := deferRun(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	for _, stage := range []string{"one deferred, one untouched", "one answered, one deferred"} {
		st, err := CollectCoverageMigrationStatus(cfg, "graded")
		if err != nil {
			t.Fatal(err)
		}

		// Counts PARTITION the occurrences: every one falls in exactly one
		// bucket, and the buckets add up to the whole.
		if got := st.Answered + st.PendingUnreviewed + st.PendingDeferred; got != len(st.Occurrences) {
			t.Fatalf("%s: counts (%d) do not partition %d occurrences", stage, got, len(st.Occurrences))
		}
		for _, o := range st.Occurrences {
			switch o.State {
			case "answered", "deferred", "unreviewed":
			default:
				t.Fatalf("%s: occurrence %s has state %q, outside the partition", stage, o.Label, o.State)
			}
		}

		// Traversal reaches every pending occurrence exactly once: walking it
		// to exhaustion must enumerate the pending set with no repeats and no
		// omissions.
		seen := map[string]int{}
		walk := *st
		for i := 0; i < len(st.Occurrences)+1; i++ {
			next := walk.NextToReview()
			if next == nil {
				break
			}
			seen[legacyDispositionKey(next.Fingerprint, next.Duplicate)]++
			// Simulate that occurrence being answered so traversal advances.
			for j := range walk.Occurrences {
				if walk.Occurrences[j].Fingerprint == next.Fingerprint &&
					walk.Occurrences[j].Duplicate == next.Duplicate {
					walk.Occurrences[j].State = "answered"
				}
			}
		}
		if len(seen) != st.PendingTotal {
			t.Fatalf("%s: traversal reached %d occurrences, pending_total is %d", stage, len(seen), st.PendingTotal)
		}
		for k, n := range seen {
			if n != 1 {
				t.Errorf("%s: traversal offered %s %d times", stage, k, n)
			}
		}

		if stage == "one deferred, one untouched" {
			resetExcFlags()
			recordExceptionLegacyFP, recordExceptionLegacyHash = entries[1].Fingerprint, hash
			recordExceptionRef, recordExceptionText = entries[1].Ref, entries[1].Text
			recordExceptionReason, recordExceptionBy = "confirmed", "interactive decision"
			recordExceptionFromLegacy = true
			if err := recordExc(t, cfg, "graded"); err != nil {
				t.Fatal(err)
			}
		}
	}
}
