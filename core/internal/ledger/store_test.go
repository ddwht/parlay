package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	target := filepath.Join(root, "spec", "intents", "widget", "activity.yaml")
	return New(root, target), root
}

// append is the shape every consumer uses: read, add, publish.
func appendLine(s *Store, line string) error {
	return s.Update(func(current []byte, _ bool) ([]byte, bool, error) {
		return append(current, []byte(line+"\n")...), true, nil
	})
}

func TestRead_MissingFileIsNotAnError(t *testing.T) {
	s, _ := newStore(t)
	data, exists, err := s.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if exists {
		t.Error("exists should be false for a file never written")
	}
	if data != nil {
		t.Errorf("data should be nil, got %q", data)
	}
}

func TestUpdate_RoundTrips(t *testing.T) {
	s, _ := newStore(t)
	want := "schema_version: 1\nhistory: []\n"

	if err := s.Update(func([]byte, bool) ([]byte, bool, error) {
		return []byte(want), true, nil
	}); err != nil {
		t.Fatal(err)
	}
	got, exists, err := s.Read()
	if err != nil || !exists {
		t.Fatalf("read back: exists=%v err=%v", exists, err)
	}
	if string(got) != want {
		t.Errorf("round trip: want %q, got %q", want, got)
	}
}

// Returning write=false is the honest way to say "nothing to change"
// without rewriting identical bytes and touching the mtime.
func TestUpdate_NoWriteLeavesTheFileAlone(t *testing.T) {
	s, _ := newStore(t)
	if err := appendLine(s, "first"); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(s.Target())
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Update(func([]byte, bool) ([]byte, bool, error) {
		return []byte("ignored"), false, nil
	}); err != nil {
		t.Fatal(err)
	}

	got, _, _ := s.Read()
	if string(got) != "first\n" {
		t.Errorf("file changed despite write=false: %q", got)
	}
	after, _ := os.Stat(s.Target())
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("mtime moved despite write=false")
	}
}

func TestUpdate_SeesCurrentBytesAndExistence(t *testing.T) {
	s, _ := newStore(t)
	var sawExists bool
	var sawBytes string
	if err := s.Update(func(current []byte, exists bool) ([]byte, bool, error) {
		sawExists, sawBytes = exists, string(current)
		return []byte("v1"), true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if sawExists || sawBytes != "" {
		t.Errorf("first update: want exists=false bytes=\"\", got %v %q", sawExists, sawBytes)
	}
	if err := s.Update(func(current []byte, exists bool) ([]byte, bool, error) {
		sawExists, sawBytes = exists, string(current)
		return current, false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !sawExists || sawBytes != "v1" {
		t.Errorf("second update: want exists=true bytes=\"v1\", got %v %q", sawExists, sawBytes)
	}
}

// A body that fails must not publish anything.
func TestUpdate_FailedBodyWritesNothing(t *testing.T) {
	s, _ := newStore(t)
	sentinel := errors.New("body failed")
	if err := s.Update(func([]byte, bool) ([]byte, bool, error) {
		return []byte("should not land"), true, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("want the body's error back, got %v", err)
	}
	if _, err := os.Stat(s.Target()); !os.IsNotExist(err) {
		t.Error("target exists after a failed body")
	}
}

// The lock file belongs in .parlay/, not beside the target. A .lock
// appearing in spec/ would eventually be committed, which is what the
// zone rules exist to prevent.
func TestLockFileLivesUnderParlayNotBesideTheTarget(t *testing.T) {
	s, root := newStore(t)
	if err := appendLine(s, "x"); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Dir(s.Target()))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "lock") {
			t.Errorf("lock artifact %q left in the spec zone", e.Name())
		}
	}
	locks, err := os.ReadDir(filepath.Join(root, ".parlay", "locks"))
	if err != nil {
		t.Fatalf("no .parlay/locks directory: %v", err)
	}
	if len(locks) == 0 {
		t.Error("expected a lock file under .parlay/locks")
	}
}

// Two targets must not contend.
func TestDistinctTargetsGetDistinctLocks(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	a := New(root, filepath.Join(root, "spec", "intents", "alpha", "activity.yaml"))
	b := New(root, filepath.Join(root, "spec", "intents", "beta", "activity.yaml"))
	if a.lockPath == b.lockPath {
		t.Fatalf("two targets share one lock: %s", a.lockPath)
	}
}

// The path used for locking and the path used for I/O must be the same
// identity. New resolves target once and keeps that; an earlier cut
// hashed the absolute path but stored the caller's relative one, so a cwd
// change between New and the write moved the I/O and left the lock
// guarding a file nobody was touching.
func TestNew_ResolvesTargetSoLockAndIOAgree(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := filepath.Join(root, "spec", "intents", "widget")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	s := New(root, "activity.yaml")
	if !filepath.IsAbs(s.Target()) {
		t.Fatalf("target not resolved to absolute: %q", s.Target())
	}
	// Moving the working directory must not move where writes land.
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	if err := appendLine(s, "recorded"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "activity.yaml")); err != nil {
		t.Fatalf("write did not land at the resolved target: %v", err)
	}
}

// Two lexical spellings of one real file must not acquire two locks —
// both writers would believe they held it.
func TestNew_SymlinkedParentResolvesToOneLock(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	a := New(root, filepath.Join(real, "activity.yaml"))
	b := New(root, filepath.Join(link, "activity.yaml"))
	if a.lockPath != b.lockPath {
		t.Errorf("two spellings of one file got two locks:\n  %s\n  %s", a.lockPath, b.lockPath)
	}
}

func TestCreate_RefusesAnExistingFile(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Create([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := s.Create([]byte("second")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("want os.ErrExist, got %v", err)
	}
	got, _ := os.ReadFile(s.Target())
	if string(got) != "first" {
		t.Errorf("the existing record was overwritten: %q", got)
	}
}

func TestCreate_LeavesNoTempBehind(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	assertNoTemps(t, filepath.Dir(s.Target()))
}

// ---------------------------------------------------------------------
// Failure injection — the crash guarantee.
//
// Create publishes by hard link from a fully-synced temp precisely so a
// failure before publication leaves NO file at the canonical path. Opening
// the final path directly (the earlier cut) left a truncated record there
// on any mid-write failure, and every retry afterwards saw ErrExist and
// refused to repair it.
// ---------------------------------------------------------------------

func TestCreate_FsyncFailureLeavesNoTarget(t *testing.T) {
	s, _ := newStore(t)
	restore := syncFile
	syncFile = func(*os.File) error { return errors.New("disk gave up") }
	defer func() { syncFile = restore }()

	if err := s.Create([]byte("never lands")); err == nil {
		t.Fatal("expected the fsync failure to surface")
	}
	if _, err := os.Stat(s.Target()); !os.IsNotExist(err) {
		t.Error("a partial record was left at the canonical path")
	}
	assertNoTemps(t, filepath.Dir(s.Target()))
}

func TestCreate_PublishFailureLeavesNoTarget(t *testing.T) {
	s, _ := newStore(t)
	restore := publishLink
	publishLink = func(string, string) error { return errors.New("link unsupported") }
	defer func() { publishLink = restore }()

	if err := s.Create([]byte("never lands")); err == nil {
		t.Fatal("expected the publish failure to surface")
	}
	if _, err := os.Stat(s.Target()); !os.IsNotExist(err) {
		t.Error("a record was left at the canonical path despite publish failing")
	}
	assertNoTemps(t, filepath.Dir(s.Target()))
}

// A failed publish over an EXISTING record must leave that record exactly
// as it was — the failure mode that would otherwise destroy somebody's
// data while reporting an error about something else.
func TestCreate_PublishFailureLeavesAnExistingRecordIntact(t *testing.T) {
	s, _ := newStore(t)
	if err := s.Create([]byte("original")); err != nil {
		t.Fatal(err)
	}
	restore := publishLink
	publishLink = func(string, string) error { return errors.New("link unsupported") }
	defer func() { publishLink = restore }()

	if err := s.Create([]byte("intruder")); err == nil {
		t.Fatal("expected the publish failure to surface")
	}
	got, err := os.ReadFile(s.Target())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Errorf("existing record was disturbed: %q", got)
	}
	assertNoTemps(t, filepath.Dir(s.Target()))
}

func TestUpdate_FsyncFailureLeavesNoTarget(t *testing.T) {
	s, _ := newStore(t)
	restore := syncFile
	syncFile = func(*os.File) error { return errors.New("disk gave up") }
	defer func() { syncFile = restore }()

	if err := appendLine(s, "never lands"); err == nil {
		t.Fatal("expected the fsync failure to surface")
	}
	if _, err := os.Stat(s.Target()); !os.IsNotExist(err) {
		t.Error("a partial record was left at the canonical path")
	}
	assertNoTemps(t, filepath.Dir(s.Target()))
}

// ---------------------------------------------------------------------

// THE POINT OF THIS PACKAGE.
//
// Twenty concurrent read-append-publish transactions. Every event must
// survive.
//
// Through atomicfile this loses records, and not marginally: the same
// twenty-writer shape run against atomicfile.WriteAtomic landed 1 of 20
// events. Each writer reads the same bytes, appends its own line and
// replaces the whole file, so the writers race to overwrite one another
// and the last one home wins carrying a single new line.
//
// That failure corrupts nothing, which is what makes it dangerous: the
// file still parses, every field is well-formed, and nothing anywhere
// reports that nineteen records were dropped.
func TestConcurrentAppends_LoseNothing(t *testing.T) {
	s, _ := newStore(t)
	const writers = 20

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			errs <- appendLine(s, "event-"+strconv.Itoa(n))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a writer failed: %v", err)
		}
	}

	final, err := os.ReadFile(s.Target())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(final)), "\n")
	if len(lines) != writers {
		t.Fatalf("lost records: want %d lines, got %d", writers, len(lines))
	}
	seen := map[string]bool{}
	for _, l := range lines {
		seen[l] = true
	}
	for i := range writers {
		if !seen["event-"+strconv.Itoa(i)] {
			t.Errorf("event-%d was lost", i)
		}
	}
}

// The typed, bounded contention contract. Pinned with a short wait rather
// than the production five seconds — the behaviour under test is the
// error type and the bound, not the specific duration.
func TestBusyError_WhenTheLockIsHeld(t *testing.T) {
	s, _ := newStore(t)
	restoreWait, restoreRetry := lockWait, lockRetry
	lockWait, lockRetry = 120*time.Millisecond, 10*time.Millisecond
	defer func() { lockWait, lockRetry = restoreWait, restoreRetry }()

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Update(func(current []byte, _ bool) ([]byte, bool, error) {
			close(held)
			<-release
			return current, false, nil
		})
	}()
	<-held

	start := time.Now()
	_, _, err := s.Read()
	elapsed := time.Since(start)
	close(release)
	// Join before the deferred restore writes lockWait, so the timing
	// vars are never mutated while a holder could still read them.
	<-done

	var busy *BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("want *BusyError, got %v", err)
	}
	if busy.Path != s.Target() {
		t.Errorf("BusyError names %q, want the target %q", busy.Path, s.Target())
	}
	if elapsed > time.Second {
		t.Errorf("waited %s — the bound is not being honoured", elapsed)
	}
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// The temp must be gone AND its removal must be durable before Create
// reports success. An unchecked deferred remove reports success while
// leaving a .ledger-*.tmp in a directory a person reads, and leaves the
// removal unfsynced so a crash can resurrect the entry after a clean
// return. The successful-path no-temp test cannot catch either, because
// nothing fails in it.
func TestCreate_TempRemovalFailureIsReportedNotSwallowed(t *testing.T) {
	s, _ := newStore(t)
	restore := removeFile
	removeFile = func(string) error { return errors.New("cannot unlink") }
	defer func() { removeFile = restore }()

	err := s.Create([]byte("published"))
	if err == nil {
		t.Fatal("temp removal failed but Create reported success")
	}
	// The record IS published; the error must say so rather than reading
	// as a failed create and sending a caller to retry it.
	if !strings.Contains(err.Error(), "was published") {
		t.Errorf("error does not say the record landed: %v", err)
	}
	got, readErr := os.ReadFile(s.Target())
	if readErr != nil {
		t.Fatalf("the record should exist despite the cleanup failure: %v", readErr)
	}
	if string(got) != "published" {
		t.Errorf("published bytes wrong: %q", got)
	}
}

// Update's publish step, injected. A failed rename must leave an existing
// record exactly as it was — the failure that would otherwise destroy
// data while reporting an error about something else — and must not
// strand a temp in the spec zone.
func TestUpdate_RenameFailureLeavesExistingRecordIntact(t *testing.T) {
	s, _ := newStore(t)
	if err := appendLine(s, "original"); err != nil {
		t.Fatal(err)
	}
	restore := publishRename
	publishRename = func(string, string) error { return errors.New("rename refused") }
	defer func() { publishRename = restore }()

	if err := appendLine(s, "intruder"); err == nil {
		t.Fatal("expected the rename failure to surface")
	}
	got, err := os.ReadFile(s.Target())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original\n" {
		t.Errorf("existing record was disturbed: %q", got)
	}
	assertNoTemps(t, filepath.Dir(s.Target()))
}

// When rename fails AND the temp cannot be removed, the residue is named
// in the error rather than left for the caller to trip over later.
func TestUpdate_RenameAndCleanupBothFailingNamesTheResidue(t *testing.T) {
	s, _ := newStore(t)
	restoreRename, restoreRemove := publishRename, removeFile
	publishRename = func(string, string) error { return errors.New("rename refused") }
	removeFile = func(string) error { return errors.New("cannot unlink") }
	defer func() { publishRename, removeFile = restoreRename, restoreRemove }()

	err := appendLine(s, "x")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), ".ledger-") {
		t.Errorf("error does not name the stranded temp: %v", err)
	}
}

// Past the link the record exists. A directory-fsync failure means only
// that durability is unconfirmed — it must not read as "creation failed",
// which would send the caller to retry straight into ErrExist.
func TestCreate_DirFsyncFailureSaysThePublicationHappened(t *testing.T) {
	s, _ := newStore(t)
	restore := syncDir
	syncDir = func(string) error { return errors.New("dir fsync refused") }
	defer func() { syncDir = restore }()

	err := s.Create([]byte("published"))
	if err == nil {
		t.Fatal("expected the fsync failure to surface")
	}
	if !strings.Contains(err.Error(), "was published") || !strings.Contains(err.Error(), "do not retry") {
		t.Errorf("error does not distinguish unconfirmed durability from a failed create: %v", err)
	}
	got, readErr := os.ReadFile(s.Target())
	if readErr != nil {
		t.Fatalf("the record should be readable despite the fsync failure: %v", readErr)
	}
	if string(got) != "published" {
		t.Errorf("published bytes wrong: %q", got)
	}
	assertNoTemps(t, filepath.Dir(s.Target()))
}

// A temp that cannot be cleaned up after a failed write is named in the
// error. The guarantee is that a residue is always named, never that it
// cannot happen.
func TestWriteTemp_StrandedTempIsNamedNotSwallowed(t *testing.T) {
	s, _ := newStore(t)
	restoreSync, restoreRemove := syncFile, removeFile
	syncFile = func(*os.File) error { return errors.New("disk gave up") }
	removeFile = func(string) error { return errors.New("cannot unlink") }
	defer func() { syncFile, removeFile = restoreSync, restoreRemove }()

	err := s.Create([]byte("x"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), ".ledger-") {
		t.Errorf("error does not name the stranded temp: %v", err)
	}
	if _, statErr := os.Stat(s.Target()); !os.IsNotExist(statErr) {
		t.Error("nothing should have been published")
	}
}
