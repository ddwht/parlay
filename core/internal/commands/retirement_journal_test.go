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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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

// --- Suite 8 (continued): a journal is an instruction to delete --------
//
// A resumed run reads a root name and a path out of a journal and
// removes both the directory and the registration they name — with no
// preflight, no sweep and no disposition record, because the run that
// wrote the journal performed all of those. That is the right ordering
// and it is also a delegation of authority to a file. So the file has to
// be shown to be this operation's own record, and not something dropped
// into the journal location that borrows its standing.

// interruptedRetirement leaves a genuine part-finished retirement: a
// promoted archive with a verifying manifest, an outstanding journal,
// the child directory still in place and the root still registered.
// Tampering starts from here, so a refusal is attributable to what the
// test changed rather than to the state being unfinished.
func interruptedRetirement(t *testing.T) string {
	t.Helper()
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	failAt("write-record", 1)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}
	if !journalPresent(parent) || !childDirPresent(parent) || !rootRegistered(t, parent, "old") {
		t.Fatal("the fixture needs a genuine part-finished retirement")
	}
	retirementHook = nil
	return parent
}

func writeJournalRaw(t *testing.T, parent, filename, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(retiredRootsDir(parent), filename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertNothingResumed requires the run to have refused and the two
// things a resume would have destroyed to still be there.
func assertNothingResumed(t *testing.T, parent string, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("a journal that cannot be shown to be this operation's own record must refuse the run")
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Errorf("the refusal must say what failed (want %q); got: %v", want, err)
	}
	if !childDirPresent(parent) {
		t.Error("nothing may be deleted on the authority of a journal that was refused")
	}
	if !rootRegistered(t, parent, "old") {
		t.Error("nothing may be deregistered on the authority of a journal that was refused")
	}
}

func TestJournal_ForgedJournalsAreRefusedWithNothingMutated(t *testing.T) {
	genuine := "root: old\nrelative-path: old\noutstanding:\n    - write-record\n    - deregister-root\n"

	cases := []struct {
		name string
		// tamper rewrites the journal location; it returns the argument
		// the operator passes to retire-root.
		tamper func(t *testing.T, parent string) string
		want   string
	}{
		{
			name: "filename-does-not-name-the-root-it-claims",
			tamper: func(t *testing.T, parent string) string {
				if err := os.Remove(retirementJournalPath(parent, "old")); err != nil {
					t.Fatal(err)
				}
				writeJournalRaw(t, parent, "evil"+journalFileSuffix, genuine)
				return "old"
			},
			want: "is filed under",
		},
		{
			name: "root-carries-traversal",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					"root: ../../evil\nrelative-path: old\noutstanding:\n    - deregister-root\n")
				return "old"
			},
			want: "plain slug",
		},
		{
			name: "unknown-fields",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					genuine+"command: rm -rf /\n")
				return "old"
			},
			want: "command",
		},
		{
			name: "steps-shuffled-out-of-execution-order",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					"root: old\nrelative-path: old\noutstanding:\n    - deregister-root\n    - write-record\n")
				return "old"
			},
			want: "tail of the order",
		},
		{
			name: "steps-duplicated",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					"root: old\nrelative-path: old\noutstanding:\n    - deregister-root\n    - deregister-root\n")
				return "old"
			},
			want: "tail of the order",
		},
		{
			name: "steps-empty",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					"root: old\nrelative-path: old\noutstanding: []\n")
				return "old"
			},
			want: "names no outstanding steps",
		},
		{
			name: "unknown-step",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					"root: old\nrelative-path: old\noutstanding:\n    - nuke-everything\n")
				return "old"
			},
			want: "unknown step",
		},
		{
			name: "path-leaves-the-project",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					"root: old\nrelative-path: ../escape\noutstanding:\n    - deregister-root\n")
				return "old"
			},
			want: "not inside the project",
		},
		{
			name: "no-archive-stands-behind-it",
			tamper: func(t *testing.T, parent string) string {
				// The journal is genuine in every other respect; the
				// archive it is supposed to be the tail of is gone.
				if err := os.RemoveAll(retirementDestination(parent, "old")); err != nil {
					t.Fatal(err)
				}
				return "old"
			},
			want: "no archive stands at",
		},
		{
			name: "archive-has-no-verifying-manifest",
			tamper: func(t *testing.T, parent string) string {
				if err := os.Remove(filepath.Join(retirementDestination(parent, "old"), "manifest.yaml")); err != nil {
					t.Fatal(err)
				}
				return "old"
			},
			want: "no manifest that reads back and verifies",
		},
		{
			name: "second-yaml-document",
			tamper: func(t *testing.T, parent string) string {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					genuine+"---\nroot: old\nrelative-path: lib\noutstanding:\n    - deregister-root\n")
				return "old"
			},
			want: "more than one YAML document",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := interruptedRetirement(t)
			arg := tc.tamper(t, parent)
			cmd, _ := retireCmd(t, parent, "y\n")
			assertNothingResumed(t, parent, runRetireRoot(cmd, []string{arg}), tc.want)
		})
	}
}

// A hand-written journal naming a live root that no retirement ever
// started is the plainest forgery there is: writing the file is trivial,
// and it would otherwise deregister and delete that root on sight.
func TestJournal_AHandWrittenJournalCannotRetireALiveRoot(t *testing.T) {
	parent, _ := archiveFixture(t)
	lib := addRetirementChild(t, parent, "lib", "lib", "helper")
	interactiveTTY(t)
	if err := os.MkdirAll(retiredRootsDir(parent), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJournalRaw(t, parent, "lib"+journalFileSuffix,
		"root: lib\nrelative-path: lib\noutstanding:\n    - deregister-root\n")

	before := treeSnapshot(t, lib.Path)
	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"lib"})
	if err == nil {
		t.Fatal("a journal nobody's retirement wrote must not be able to delete a live root")
	}
	if !strings.Contains(err.Error(), "no archive stands at") {
		t.Errorf("the refusal must name the missing archive; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, lib.Path))
	if !rootRegistered(t, parent, "lib") {
		t.Error("the live root must still be registered")
	}
}

// A forgery in the journal location must not be reachable by retiring
// some OTHER root either: the scan reads every journal it finds, so an
// unauthenticatable one refuses the whole run rather than being skipped
// on its way to a legitimate target.
func TestJournal_AForgedJournalRefusesEvenARunAimedElsewhere(t *testing.T) {
	parent, _ := archiveFixture(t)
	addRetirementChild(t, parent, "lib", "lib", "helper")
	interactiveTTY(t)
	if err := os.MkdirAll(retiredRootsDir(parent), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJournalRaw(t, parent, "evil"+journalFileSuffix,
		"root: old\nrelative-path: old\noutstanding:\n    - deregister-root\n")

	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	before := treeSnapshot(t, parent)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"lib"}); err == nil {
		t.Fatal("an unauthenticatable journal must refuse the run rather than be passed over")
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

// A file in the journal location whose name is not a root name was not
// filed by this operation and is refused before its contents are read.
func TestJournal_AFileNotNamedForARootIsRefusedInTheJournalLocation(t *testing.T) {
	parent, _ := archiveFixture(t)
	interactiveTTY(t)
	if err := os.MkdirAll(retiredRootsDir(parent), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJournalRaw(t, parent, "Not A Root"+journalFileSuffix,
		"root: old\nrelative-path: old\noutstanding:\n    - deregister-root\n")

	_, err := FindRetirementJournal(parent, "old")
	if err == nil {
		t.Fatal("a file in the journal location whose name is not a root name must refuse")
	}
	if !strings.Contains(err.Error(), "not named for a root") {
		t.Errorf("the refusal must say the filename is not a root name; got: %v", err)
	}
}

// The authentication must not refuse the real thing: a genuine
// part-finished retirement still resumes.
func TestJournal_AGenuineJournalStillAuthenticatesAndResumes(t *testing.T) {
	parent := interruptedRetirement(t)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("a journal this operation actually wrote must authenticate and resume: %v", err)
	}
	assertRetirementComplete(t, parent, "old")
}

// --- Suite 8 (continued): provenance, not just plausibility ------------
//
// Shape checks establish that a journal is well formed. They do not
// establish that it and the archive beside it came from a run that
// actually archived this root — and a manifest's self-covering hash
// proves only that its own member list is internally consistent, which
// a list invented from nothing satisfies perfectly. These cases pin the
// three layers that turn plausibility into provenance: the archived
// bytes are re-hashed, the journal names its archive by digest, and the
// progress the journal claims is cross-checked against the filesystem.

// forgedArchive builds a destination that passes every shape check:
// contents that exist, a manifest whose members hash correctly and
// whose self-covering hash verifies, and a retirement record naming the
// root. It returns the digest of the manifest it wrote, so a forged
// journal can name it correctly too.
func forgedArchive(t *testing.T, parent, rootName, relPath string, members map[string]string) string {
	t.Helper()
	dest := retirementDestination(parent, rootName)
	contents := filepath.Join(dest, "contents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &ArchiveManifest{}
	for rel, body := range members {
		path := filepath.Join(contents, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest.Members = append(manifest.Members, ManifestMember{Path: rel, SHA256: hashBytes([]byte(body))})
	}
	manifestPath := filepath.Join(dest, "manifest.yaml")
	if err := WriteManifest(manifest, manifestPath); err != nil {
		t.Fatal(err)
	}
	// The record a journal claiming a finished write-record step needs.
	record := &retirementRecord{
		Root:         rootName,
		RelativePath: relPath,
		RetiredAt:    "2026-01-01T00:00:00Z",
		Archive:      "contents/",
		Manifest:     "manifest.yaml",
		Dispositions: "dispositions.yaml",
	}
	data, err := yaml.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, retirementRecordFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := manifestDigest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// TestJournal_AWellFormedForgeryCannotRetireALiveRoot is the decisive
// case. Nothing here is malformed: the archive's contents exist and
// hash to what its manifest says, the manifest covers its own member
// list, a retirement record sits beside it naming the same root, and
// the journal is a slug-valid, correctly-filed, correctly-ordered tail
// that names the manifest by its true digest. Every shape check passes.
//
// What the forgery cannot do — without genuinely archiving the root,
// which is the thing a retirement is — is account for the files the
// live root still holds. The retirement is about to delete those files
// in favour of this copy, so the copy has to be a copy of them.
func TestJournal_AWellFormedForgeryCannotRetireALiveRoot(t *testing.T) {
	parent, _ := archiveFixture(t)
	lib := addRetirementChild(t, parent, "lib", "lib", "helper")
	if err := os.WriteFile(filepath.Join(lib.Path, "important.go"), []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	interactiveTTY(t)

	// A complete, self-consistent archive — of something that is not lib.
	digest := forgedArchive(t, parent, "lib", "lib", map[string]string{
		"README.md": "plausible\n",
	})
	writeJournalRaw(t, parent, "lib"+journalFileSuffix,
		"root: lib\nrelative-path: lib\noutstanding:\n    - deregister-root\nmanifest-digest: "+digest+"\n")

	before := treeSnapshot(t, lib.Path)
	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"lib"})
	if err == nil {
		t.Fatal("a well-formed forged archive and journal must not be able to delete and deregister a live root")
	}
	if !strings.Contains(err.Error(), "does not preserve") {
		t.Errorf("the refusal must name what the archive fails to preserve; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, lib.Path))
	if !rootRegistered(t, parent, "lib") {
		t.Error("the live root must still be registered")
	}
}

func TestJournal_AnEmptyManifestIsNotAnArchive(t *testing.T) {
	// The cheapest forgery of all: a self-covering hash over an empty
	// member list verifies perfectly, and describes nothing.
	parent, _ := archiveFixture(t)
	lib := addRetirementChild(t, parent, "lib", "lib", "helper")
	interactiveTTY(t)

	digest := forgedArchive(t, parent, "lib", "lib", nil)
	writeJournalRaw(t, parent, "lib"+journalFileSuffix,
		"root: lib\nrelative-path: lib\noutstanding:\n    - deregister-root\nmanifest-digest: "+digest+"\n")

	before := treeSnapshot(t, lib.Path)
	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"lib"})
	if err == nil || !strings.Contains(err.Error(), "names no members") {
		t.Fatalf("a manifest naming nothing must be refused as not being that root's archive; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, lib.Path))
	if !rootRegistered(t, parent, "lib") {
		t.Error("the live root must still be registered")
	}
}

func TestJournal_ProvenanceLayersEachRefuseOnTheirOwn(t *testing.T) {
	// Each layer, exercised from a genuine part-finished retirement so
	// the tamper is the only thing wrong.
	cases := []struct {
		name   string
		tamper func(t *testing.T, parent string)
		want   string
	}{
		{
			name: "journal-records-no-manifest-digest",
			tamper: func(t *testing.T, parent string) {
				writeJournalRaw(t, parent, "old"+journalFileSuffix,
					"root: old\nrelative-path: old\noutstanding:\n    - write-record\n    - deregister-root\n")
			},
			want: "records no manifest digest",
		},
		{
			name: "manifest-rewritten-consistently-under-the-journal",
			tamper: func(t *testing.T, parent string) {
				// Self-consistent, and not the manifest the journal was
				// written beside: the member list re-covers itself.
				dest := retirementDestination(parent, "old")
				m, err := ReadManifest(filepath.Join(dest, "manifest.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				m.Members = m.Members[:len(m.Members)-1]
				if err := WriteManifest(m, filepath.Join(dest, "manifest.yaml")); err != nil {
					t.Fatal(err)
				}
			},
			want: "not from the same run",
		},
		{
			name: "archived-bytes-do-not-match-the-manifest",
			tamper: func(t *testing.T, parent string) {
				// The manifest is untouched, so its digest still matches
				// the journal; only the archived file changed. Nothing
				// but re-hashing the members catches this.
				corrupt := filepath.Join(retirementDestination(parent, "old"), "contents", "internal", "alpha.go")
				if err := os.WriteFile(corrupt, []byte("package tampered\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "does not hold what its manifest describes",
		},
		{
			name: "claims-the-record-step-finished-with-no-record",
			tamper: func(t *testing.T, parent string) {
				j, err := LoadRetirementJournal(parent, "old")
				if err != nil {
					t.Fatal(err)
				}
				j.Outstanding = []string{journalStepDeregisterRoot}
				if err := WriteRetirementJournal(parent, j); err != nil {
					t.Fatal(err)
				}
			},
			want: "claims progress the archive does not show",
		},
		{
			name: "record-beside-the-archive-describes-another-retirement",
			tamper: func(t *testing.T, parent string) {
				j, err := LoadRetirementJournal(parent, "old")
				if err != nil {
					t.Fatal(err)
				}
				j.Outstanding = []string{journalStepDeregisterRoot}
				if err := WriteRetirementJournal(parent, j); err != nil {
					t.Fatal(err)
				}
				dest := retirementDestination(parent, "old")
				data, err := yaml.Marshal(&retirementRecord{Root: "old", RelativePath: "somewhere-else"})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dest, retirementRecordFile), data, 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "describe different retirements",
		},
		{
			name: "archive-does-not-preserve-a-file-the-root-still-holds",
			tamper: func(t *testing.T, parent string) {
				// A file appears in the root after the archive was made.
				// The retirement would delete it in favour of a copy
				// that never held it.
				if err := os.WriteFile(filepath.Join(parent, "old", "unarchived.go"),
					[]byte("package old\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "not named by the archive at all",
		},
		{
			name: "a-live-file-edited-during-the-interruption-is-not-in-the-archive",
			tamper: func(t *testing.T, parent string) {
				// The honest case, and it refuses for the same reason:
				// the archive is stale, the newer bytes are not in it,
				// and completing the run would destroy them.
				if err := os.WriteFile(filepath.Join(parent, "old", "internal", "alpha.go"),
					[]byte("package alpha // edited while the run was interrupted\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "holds different bytes from the archived copy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := interruptedRetirement(t)
			tc.tamper(t, parent)
			cmd, _ := retireCmd(t, parent, "y\n")
			assertNothingResumed(t, parent, runRetireRoot(cmd, []string{"old"}), tc.want)
		})
	}
}

func TestJournal_AGenuineJournalCarriesItsManifestDigest(t *testing.T) {
	parent := interruptedRetirement(t)
	j, err := LoadRetirementJournal(parent, "old")
	if err != nil || j == nil {
		t.Fatalf("the genuine journal must authenticate: %v", err)
	}
	if j.ManifestDigest == "" {
		t.Fatal("every journal this operation writes must name the archive it belongs to")
	}
	want, err := manifestDigest(filepath.Join(retirementDestination(parent, "old"), "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if j.ManifestDigest != want {
		t.Errorf("the recorded digest must be the one a resumed run re-derives: got %s, want %s", j.ManifestDigest, want)
	}
	// And the digest survives a completed step, so a journal shrunk
	// mid-run still names its archive. The step is completed the way the
	// run completes it — record first, then shrink — because the
	// progress cross-check requires exactly that correspondence.
	if err := writeRetirementRecord(parent, "old", &retirementRecord{
		Root: "old", RelativePath: "old", Archive: "contents/", Manifest: "manifest.yaml",
	}); err != nil {
		t.Fatal(err)
	}
	if err := completeJournalStep(parent, j); err != nil {
		t.Fatal(err)
	}
	shrunk, err := LoadRetirementJournal(parent, "old")
	if err != nil {
		t.Fatalf("the shrunk journal must still authenticate: %v", err)
	}
	if shrunk.ManifestDigest != want {
		t.Error("the digest must survive a completed step")
	}
}

// --- Suite 8 (continued): what the chain can and cannot establish ------
//
// Every artifact in the chain — journal, manifest, record, digest — is
// writable by whoever runs this tool. A party who can forge one can
// forge the set, consistently, so no comparison among them establishes
// origin; that needs a trust anchor outside the repository, and there
// is none. These cases pin what IS true instead, which is the property
// that actually protects the operator: nothing is destroyed unless it
// is provably preserved, and every destructive run — resumed as much as
// fresh — passes a person.

// forgedChainAt builds a complete, mutually consistent chain against an
// existing root: an archive whose contents hash to its manifest, a
// retirement record naming the root, and a journal carrying the true
// manifest digest and a valid step tail. bodies decides what the
// archived copy actually holds — which is the whole question.
func forgedChainAt(t *testing.T, parent, rootName, relPath string, bodies map[string]string) {
	t.Helper()
	digest := forgedArchive(t, parent, rootName, relPath, bodies)
	writeJournalRaw(t, parent, rootName+journalFileSuffix,
		"root: "+rootName+"\nrelative-path: "+relPath+
			"\noutstanding:\n    - deregister-root\nmanifest-digest: "+digest+"\n")
}

// liveContents reads every file under a root, so a forgery can be built
// with exactly the right paths — the case a filename-only check misses.
func liveContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestJournal_ForgedChainWithTheRightPathsAndWrongBytesCannotDelete is
// the decisive case, and the one a filename-only comparison lets
// through. The forged archive names EVERY path the live root holds —
// so coverage by name is complete — and holds different bytes under
// each, with the manifest, its self-covering hash, the digest, the
// record and the journal all recomputed to match. Nothing in the chain
// is inconsistent.
//
// It is refused anyway, because the check that matters is not whether
// the archive looks right but whether the bytes about to be destroyed
// are in it.
func TestJournal_ForgedChainWithTheRightPathsAndWrongBytesCannotDelete(t *testing.T) {
	parent, _ := archiveFixture(t)
	lib := addRetirementChild(t, parent, "lib", "lib", "helper")
	if err := os.WriteFile(filepath.Join(lib.Path, "important.go"),
		[]byte("package lib // the real bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	interactiveTTY(t)

	// Same paths, different bytes, everything else recomputed.
	bodies := map[string]string{}
	for rel := range liveContents(t, lib.Path) {
		bodies[rel] = "forged stand-in for " + rel + "\n"
	}
	forgedChainAt(t, parent, "lib", "lib", bodies)

	before := treeSnapshot(t, lib.Path)
	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"lib"})
	if err == nil {
		t.Fatal("an archive holding the right paths and the wrong bytes must not authorize destroying the real ones")
	}
	if !strings.Contains(err.Error(), "holds different bytes from the archived copy") {
		t.Errorf("the refusal must say the live bytes are not the archived ones; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, lib.Path))
	if !rootRegistered(t, parent, "lib") {
		t.Error("the live root must still be registered")
	}
}

// TestJournal_AByteIdenticalForgedChainPassesAndIsHarmless states the
// residual plainly rather than pretending it away. A chain forged by
// someone with write access, whose archive holds the root's actual
// bytes, DOES satisfy every check — because it has, by construction,
// preserved everything it proposes to remove. That is not a hole in the
// integrity property; it is the integrity property holding.
//
// What stands between such a chain and a deletion is the next test: a
// person is asked.
func TestJournal_AByteIdenticalForgedChainPassesAndIsHarmless(t *testing.T) {
	parent, _ := archiveFixture(t)
	lib := addRetirementChild(t, parent, "lib", "lib", "helper")
	if err := os.WriteFile(filepath.Join(lib.Path, "important.go"),
		[]byte("package lib // the real bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	interactiveTTY(t)

	live := liveContents(t, lib.Path)
	forgedChainAt(t, parent, "lib", "lib", live)

	// Authentication passes: nothing is inconsistent, and nothing that
	// would be destroyed is unpreserved.
	j, err := FindRetirementJournal(parent, "lib")
	if err != nil {
		t.Fatalf("a chain whose archive holds the root's actual bytes satisfies every check: %v", err)
	}
	if j == nil {
		t.Fatal("the journal should have been found")
	}

	// Confirmed by a person, the run completes — and every byte it
	// destroyed is readable in the archive, which is what makes the
	// residual harmless rather than merely accepted.
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"lib"}); err != nil {
		t.Fatalf("the confirmed resume should complete: %v", err)
	}
	preserved := filepath.Join(retirementDestination(parent, "lib"), "contents")
	for rel, want := range live {
		got, readErr := os.ReadFile(filepath.Join(preserved, filepath.FromSlash(rel)))
		if readErr != nil || string(got) != want {
			t.Errorf("every destroyed byte must be recoverable from the archive: %s (%v)", rel, readErr)
		}
	}
}

// --- Resume passes the same person a fresh execution does -------------
//
// The residual above reduces to "a human explicitly confirms destroying
// bytes that are verifiably archived" only if the resume path really
// does ask. These pin that it does, on the same terms as a fresh run.

func TestJournal_ResumeRefusesWithNobodyToAuthorizeIt(t *testing.T) {
	for _, mode := range []string{"non-interactive", "no-terminal"} {
		t.Run(mode, func(t *testing.T) {
			parent := interruptedRetirement(t)
			switch mode {
			case "non-interactive":
				retireRootNonInteractive = true
			case "no-terminal":
				no := false
				retireRootTTYOverride = &no
			}
			before := treeSnapshot(t, parent)
			// Stdin says yes. A run that merely failed to get an answer
			// would proceed on it; a run that refuses because there is
			// nobody to ask must not ask at all.
			cmd, out := retireCmd(t, parent, "y\n")
			err := runRetireRoot(cmd, []string{"old"})
			if err == nil {
				t.Fatal("a resume with nobody to authorize it must refuse — resuming is executing, and a destructive execution has no safe default")
			}
			if !strings.Contains(err.Error(), "no person to ask") &&
				!strings.Contains(err.Error(), "without a person to authorize") {
				t.Errorf("the refusal must say the absence of a person is the reason; got: %v", err)
			}
			if strings.Contains(out.String(), "Resume and complete the outstanding steps?") {
				t.Errorf("a headless run must refuse rather than ask a question nobody can answer; got:\n%s", out.String())
			}
			assertTreeUnchanged(t, before, treeSnapshot(t, parent))
			if !rootRegistered(t, parent, "old") {
				t.Error("a refused resume must change nothing")
			}
		})
	}
}

func TestJournal_ResumeRequiresAnExplicitYes(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", "no\n", "maybe\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			parent := interruptedRetirement(t)
			before := treeSnapshot(t, parent)
			cmd, out := retireCmd(t, parent, answer)
			err := runRetireRoot(cmd, []string{"old"})
			if err == nil {
				t.Fatal("a resume that was not explicitly confirmed must not proceed")
			}
			if !strings.Contains(out.String(), "Resume and complete the outstanding steps?") {
				t.Errorf("the resume must ask before it acts; got:\n%s", out.String())
			}
			assertTreeUnchanged(t, before, treeSnapshot(t, parent))
			if !rootRegistered(t, parent, "old") {
				t.Error("an unconfirmed resume must change nothing")
			}
		})
	}
}

func TestJournal_ResumePreviewCommitsToNothing(t *testing.T) {
	parent := interruptedRetirement(t)
	retireRootPreview = true
	before := treeSnapshot(t, parent)
	cmd, out := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("previewing a part-finished run must be available unattended: %v", err)
	}
	if !strings.Contains(out.String(), "Outstanding, in order:") {
		t.Errorf("the preview must report what remains; got:\n%s", out.String())
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}
