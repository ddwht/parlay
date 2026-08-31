// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/mutation-order-rollback-resumable-journal
// parlay-artifact: test
//
// Suite: mutation-order-rollback-and-the-resumable-journal. Faults are
// injected at the retirement's mutation boundaries through the
// retirementHook seam; each interruption is followed by the invariant
// pair the fragment pins: either the prior state is restored exactly,
// or an explicit resumable journal stands.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// failAt makes the hook abort at the nth occurrence of the named event.
func failAt(event string, occurrence int) {
	seen := 0
	retirementHook = func(e string) error {
		if e == event {
			seen++
			if seen == occurrence {
				return fmt.Errorf("injected: interrupted at %s", e)
			}
		}
		return nil
	}
}

func rootRegistered(t *testing.T, parent, name string) bool {
	t.Helper()
	idx, err := config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := idx.Lookup(name)
	return ok
}

func childDirPresent(parent string) bool {
	_, err := os.Stat(filepath.Join(parent, "old"))
	return err == nil
}

func journalPresent(parent string) bool {
	_, err := os.Stat(retirementJournalPath(parent, "old"))
	return err == nil
}

func TestJournal_InterruptionBeforeArchiveCompleteLeavesPriorState(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("archive-copy", 1)

	before := treeSnapshot(t, parent)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

func TestJournal_InterruptionAfterArchiveLeavesJournalAndRegistration(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("write-record", 1)

	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}
	if !journalPresent(parent) {
		t.Fatal("an interruption after the archive and before the registration change must leave a resumable journal")
	}
	if !rootRegistered(t, parent, "old") {
		t.Error("the registration must still name the root while the journal is outstanding")
	}
}

// The two structural invariants, checked with interruptions injected at
// every mutation boundary:
//   - at no point is the root deregistered while its directory is still
//     in place;
//   - whenever the contents have moved and the root is still registered,
//     the journal exists.
func TestJournal_NoInterruptionLeavesAContradictoryState(t *testing.T) {
	for _, boundary := range retirementBoundaries {
		t.Run(boundary, func(t *testing.T) {
			parent, _ := archiveFixture(t)
			retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
			interactiveTTY(t)
			failAt(boundary, 1)

			cmd, _ := retireCmd(t, parent, "y\n")
			if err := runRetireRoot(cmd, []string{"old"}); err == nil {
				t.Fatalf("the run must fail when interrupted at %s", boundary)
			}

			registered := rootRegistered(t, parent, "old")
			dirPresent := childDirPresent(parent)
			journaled := journalPresent(parent)

			if !registered && dirPresent {
				t.Errorf("at %s: the root is deregistered while its directory is still in place", boundary)
			}
			if !dirPresent && registered && !journaled {
				t.Errorf("at %s: the contents have moved, the root is still registered, and no journal exists", boundary)
			}
		})
	}
}

func TestJournal_ListsOutstandingStepsInExecutionOrder(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("write-record", 1)

	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}
	j, err := LoadRetirementJournal(parent, "old")
	if err != nil || j == nil {
		t.Fatalf("the journal must be readable: %v", err)
	}
	want := []string{journalStepWriteRecord, journalStepDeregisterRoot}
	if len(j.Outstanding) != len(want) {
		t.Fatalf("outstanding steps: got %v, want %v", j.Outstanding, want)
	}
	for i := range want {
		if j.Outstanding[i] != want[i] {
			t.Errorf("outstanding steps must be in the order they must happen: got %v, want %v", j.Outstanding, want)
		}
	}
}

func TestJournal_ResumedRunPerformsOutstandingStepsOnly(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("write-record", 1)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}

	// Resume: the outstanding steps run; the completed archive is not
	// re-attempted.
	events := recordEvents(t)
	cmd, out := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("the resumed run must complete: %v", err)
	}
	for _, e := range *events {
		switch e {
		case "stage-archive", "archive-copy", "promote", "archive-walk", "enumerate-features", "sweep":
			t.Errorf("a resumed run repeats no completed step; saw %q in %v", e, *events)
		}
	}
	if !strings.Contains(out.String(), "Outstanding") {
		t.Errorf("the resumed run should report what is already done and what remains; got:\n%s", out.String())
	}
	if rootRegistered(t, parent, "old") {
		t.Error("the resumed run must deregister the root")
	}
	if journalPresent(parent) {
		t.Error("the journal must be removed in the same final step")
	}
	if _, err := os.Stat(filepath.Join(retirementDestination(parent, "old"), "retirement-record.yaml")); err != nil {
		t.Errorf("the resumed run must have written the retirement record: %v", err)
	}
}

func TestJournal_SecondRetirementRefusesWhileJournalOutstanding(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("write-record", 1)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}

	// A fresh retirement of the same root refuses, naming the
	// part-finished run — declining the resume prompt starts nothing.
	retirementHook = nil
	cmd, _ = retireCmd(t, parent, "n\n")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "part-finished") {
		t.Errorf("a fresh retirement must refuse naming the part-finished run; got: %v", err)
	}

	// And so does an unattended one.
	retireRootNonInteractive = true
	cmd, _ = retireCmd(t, parent, "")
	err = runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "part-finished") {
		t.Errorf("an unattended run must refuse naming the part-finished run; got: %v", err)
	}
}

func TestJournal_CompletedRunLeavesRegistrationWithoutTheRoot(t *testing.T) {
	parent, _ := archiveFixture(t)
	if err := runAuthorizedRetirement(t, parent); err != nil {
		t.Fatalf("retirement should complete: %v", err)
	}
	if rootRegistered(t, parent, "old") {
		t.Error("after a completed run the registration must not name the root")
	}
	if journalPresent(parent) {
		t.Error("no journal may outlive a completed run")
	}
}

// retirementBoundaries lists every mutation boundary of a retirement, in
// the order a full run fires them. Interruption tests iterate it rather
// than sampling it: a boundary nobody injects at is a boundary nobody
// knows the behavior of.
var retirementBoundaries = []string{
	"stage-archive", "archive-copy", "verify-archive", "promote", "write-journal",
	"write-record", "remove-contents", "deregister-index", "remove-journal",
}

// assertRetirementComplete pins what "the retirement finished" means, so
// a resumability test cannot pass on a weaker predicate: the
// registration no longer names the root, the root's directory is gone,
// no journal outlives the run, and the archive holds the complete
// evidence set.
func assertRetirementComplete(t *testing.T, parent, name string) {
	t.Helper()
	if rootRegistered(t, parent, name) {
		t.Error("a completed retirement leaves a registration that does not name the root")
	}
	if childDirPresent(parent) {
		t.Error("a completed retirement leaves no directory behind for the retired root")
	}
	if journalPresent(parent) {
		t.Error("a completed retirement leaves no journal")
	}
	dest := retirementDestination(parent, name)
	for _, rel := range []string{"contents", "manifest.yaml", "dispositions.yaml", "retirement-record.yaml"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Errorf("a completed retirement preserves %s: %v", rel, err)
		}
	}
	manifest, err := ReadManifest(filepath.Join(dest, "manifest.yaml"))
	if err != nil {
		t.Fatalf("the preserved manifest must read back and verify: %v", err)
	}
	if len(manifest.Members) == 0 {
		t.Error("the preserved manifest must name the members it preserved")
	}
}

// TestJournal_EveryBoundaryFailureIsResumableToCompletion is the real
// resumability obligation: not that an interrupted run leaves a
// consistent-looking state, but that a SUBSEQUENT run finishes the
// retirement. A state that satisfies every structural predicate and
// still cannot be completed is exactly the failure this catches — the
// last steps deregister the root and then remove the journal, so a run
// interrupted between them must be resumable without the registration
// that a resume once needed in order to find it.
func TestJournal_EveryBoundaryFailureIsResumableToCompletion(t *testing.T) {
	for _, boundary := range retirementBoundaries {
		t.Run(boundary, func(t *testing.T) {
			parent, _ := archiveFixture(t)
			retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
			interactiveTTY(t)
			failAt(boundary, 1)

			cmd, _ := retireCmd(t, parent, "y\n")
			if err := runRetireRoot(cmd, []string{"old"}); err == nil {
				t.Fatalf("the run must fail when interrupted at %s", boundary)
			}

			// The next invocation — a resume where a journal stands, a
			// fresh run where the interruption restored the prior state
			// — completes the retirement. Which of the two it is, is the
			// operation's business, not the operator's.
			retirementHook = nil
			cmd, _ = retireCmd(t, parent, "y\n")
			if err := runRetireRoot(cmd, []string{"old"}); err != nil {
				t.Fatalf("after an interruption at %s the next run must complete the retirement: %v", boundary, err)
			}
			assertRetirementComplete(t, parent, "old")
		})
	}
}

// TestJournal_ResumesARunWhoseRootIsAlreadyDeregistered pins the exact
// state the ordering creates and nothing else reaches: the registration
// has already been removed and only the journal is left. A resume that
// resolved its target through the registration could never see this
// run, so the journal location is scanned first, by filename.
func TestJournal_ResumesARunWhoseRootIsAlreadyDeregistered(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("remove-journal", 1)

	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}
	// The state under test: deregistered, contents moved, journal left.
	if rootRegistered(t, parent, "old") {
		t.Fatal("this test needs the state where the registration has already gone")
	}
	if !journalPresent(parent) {
		t.Fatal("this test needs the state where the journal is still outstanding")
	}

	// The journal must be findable without the registration.
	found, err := FindRetirementJournal(parent, "old")
	if err != nil {
		t.Fatalf("scanning for an in-flight retirement must not fail: %v", err)
	}
	if found == nil {
		t.Fatal("an in-flight retirement must be found by scanning the journal location, not by resolving a registration that has already been removed")
	}

	retirementHook = nil
	cmd, out := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("a run whose root is already deregistered must still complete its retirement: %v", err)
	}
	if !strings.Contains(out.String(), "part-finished") {
		t.Errorf("the run should say it is finishing a part-finished retirement; got:\n%s", out.String())
	}
	assertRetirementComplete(t, parent, "old")
}

// The journal is looked for before the registration is consulted, so a
// retirement named by its registered PATH resumes exactly as one named
// by its registered name does.
func TestJournal_InFlightRunResumesWhenNamedByItsRegisteredPath(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("remove-journal", 1)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}

	retirementHook = nil
	cmd, _ = retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old/"}); err != nil {
		t.Fatalf("naming the in-flight retirement by its registered path must resume it: %v", err)
	}
	assertRetirementComplete(t, parent, "old")
}

// A journal that cannot be read is not "nothing in flight": starting a
// fresh destructive run over a part-finished one is the failure the
// scan refuses.
func TestJournal_UnreadableJournalRefusesRatherThanStartingOver(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("write-record", 1)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}

	if err := os.WriteFile(retirementJournalPath(parent, "old"), []byte("outstanding: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	retirementHook = nil
	cmd, _ = retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil {
		t.Fatal("a journal that cannot be parsed must refuse the run rather than be read as nothing in flight")
	}
	if !strings.Contains(err.Error(), "journal") {
		t.Errorf("the refusal must name the journal it could not read; got: %v", err)
	}
}
