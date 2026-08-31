// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/compare-and-swap-save
// parlay-artifact: test

package domainmodel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

// TestSaveMatchingEtagWrites asserts a save presenting the load-time etag
// writes the file and the edit lands on disk.
func TestSaveMatchingEtagWrites(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	model.Entities = append(model.Entities, Entity{
		Name:   "Widget",
		Fields: []Field{{Name: "id", Type: "uuid", Required: true}},
	})

	newEtag, err := Save(context.Background(), root, model, etag)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if newEtag == etag {
		t.Fatal("etag did not advance after a content-changing save")
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !strings.Contains(string(after), "Widget") {
		t.Fatalf("edit not reflected on disk:\n%s", after)
	}
}

// TestSaveStaleEtagConflictWritesNothing asserts a save presenting a stale etag
// returns the conflict envelope carrying both etags and leaves the on-disk file
// unchanged.
func TestSaveStaleEtagConflictWritesNothing(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, _, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = Save(context.Background(), root, model, Etag("sha256:9f2c-stale"))
	var cerr *ConflictError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected *ConflictError, got %v", err)
	}
	if cerr.AttemptedETag != "sha256:9f2c-stale" {
		t.Fatalf("attempted etag = %q, want the presented stale token", cerr.AttemptedETag)
	}
	if cerr.CurrentETag == "" || cerr.CurrentETag == cerr.AttemptedETag {
		t.Fatalf("conflict must carry the differing current etag, got %q", cerr.CurrentETag)
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != testPopulatedYAML {
		t.Fatal("on-disk file changed despite a stale-etag conflict")
	}
}

// TestSaveEmptyModelSentinelCreatesFile asserts a first save presenting the
// empty sentinel creates domain-model.yaml, and a non-sentinel token against a
// missing file is a conflict.
func TestSaveEmptyModelSentinelCreatesFile(t *testing.T) {
	root := t.TempDir()
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if etag != SentinelEmpty {
		t.Fatalf("bootstrap etag = %q, want %q", etag, SentinelEmpty)
	}

	model.Entities = append(model.Entities, Entity{
		Name:   "Customer",
		Fields: []Field{{Name: "id", Type: "uuid", Required: true}},
	})
	if _, err := Save(context.Background(), root, model, etag); err != nil {
		t.Fatalf("first save against empty sentinel: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, modelFileName)); err != nil {
		t.Fatalf("first save did not create the file: %v", err)
	}

	// A non-sentinel token against a still-missing file is a conflict.
	root2 := t.TempDir()
	_, err = Save(context.Background(), root2, model, Etag("sha256:not-empty"))
	var cerr *ConflictError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected conflict for a stale token against a missing file, got %v", err)
	}
}

// TestSavePersistsCurrentSchemaVersion asserts a saved model carries the
// binary's current schema_version on disk.
func TestSavePersistsCurrentSchemaVersion(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	model.SchemaVersion = CurrentSchemaVersion
	if _, err := Save(context.Background(), root, model, etag); err != nil {
		t.Fatalf("Save: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !strings.Contains(string(after), "schema_version: 1") {
		t.Fatalf("saved file missing current schema_version:\n%s", after)
	}
}

// TestSaveConcurrentWritersExactlyOneWins asserts the model lock makes the
// compare and the write one step: two savers presenting the SAME valid etag
// race, exactly one lands, the other gets a conflict rather than clobbering,
// and the file that remains is a whole model rather than a splice of both.
//
// Without the lock both writers read identical current bytes, both compare
// equal, and both rename — the second silently erases the first. That is the
// failure this test exists to keep out.
func TestSaveConcurrentWritersExactlyOneWins(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	base, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	names := []string{"Widget", "Gadget"}
	errs := make([]error, len(names))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			model := base
			model.Entities = append(append([]Entity(nil), base.Entities...), Entity{
				Name:   name,
				Fields: []Field{{Name: "id", Type: "uuid", Required: true}},
			})
			<-start
			_, errs[i] = Save(context.Background(), root, model, etag)
		}()
	}
	close(start)
	wg.Wait()

	var winners, conflicts int
	for i, err := range errs {
		var cerr *ConflictError
		switch {
		case err == nil:
			winners++
		case errors.As(err, &cerr):
			conflicts++
			if cerr.AttemptedETag != string(etag) {
				t.Fatalf("%s: conflict attempted etag = %q, want the presented token %q", names[i], cerr.AttemptedETag, etag)
			}
			if cerr.CurrentETag == cerr.AttemptedETag {
				t.Fatalf("%s: conflict must name the etag it collided with", names[i])
			}
		default:
			t.Fatalf("%s: unexpected error: %v", names[i], err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners = %d, conflicts = %d; want exactly one of each", winners, conflicts)
	}

	// The survivor must be one intact model carrying exactly one of the two
	// added entities — never both, never a truncated file.
	after, _, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("load after the race: %v", err)
	}
	var added []string
	for _, e := range after.Entities {
		if e.Name == "Widget" || e.Name == "Gadget" {
			added = append(added, e.Name)
		}
	}
	if len(added) != 1 {
		t.Fatalf("post-race model carries %v; want exactly one winner's entity", added)
	}
	if len(after.Entities) != len(base.Entities)+1 {
		t.Fatalf("post-race entity count = %d, want %d", len(after.Entities), len(base.Entities)+1)
	}
}

// TestWriteFileAtomicParallelWritersUseDistinctTemps asserts parallel atomic
// writes to one path never share a temp file: every writer succeeds, the
// published bytes are exactly one writer's payload, and nothing is left behind.
//
// A fixed path+".tmp" fails this — writers overwrite each other's temp file,
// the first rename pulls it out from under the rest, and what lands can be a
// splice of two payloads.
func TestWriteFileAtomicParallelWritersUseDistinctTemps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, modelFileName)

	const writers = 8
	payloads := make([][]byte, writers)
	for i := range payloads {
		// Distinct contents AND distinct lengths, so a spliced result cannot
		// pass for a whole one.
		payloads[i] = []byte(strings.Repeat("writer-"+strconv.Itoa(i)+"\n", i+1))
	}

	errs := make([]error, writers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = writeFileAtomic(path, payloads[i])
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read published file: %v", err)
	}
	matched := -1
	for i, p := range payloads {
		if string(got) == string(p) {
			matched = i
			break
		}
	}
	if matched < 0 {
		t.Fatalf("published bytes match no single writer's payload:\n%q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file %s survived a successful write", e.Name())
		}
	}
}

// TestSaveLockBusyTimesOutWithoutTouchingTheFile asserts a save that cannot
// take the lock within the bounded wait gives up with a LockBusyError, having
// read and written nothing — and that the error is NOT a ConflictError, since
// there are no etags to reload against.
func TestSaveLockBusyTimesOutWithoutTouchingTheFile(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Collapse the wait; a real 5s hold would only make the test slow.
	prevWait, prevRetry := modelLockWait, modelLockRetry
	modelLockWait, modelLockRetry = 120*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { modelLockWait, modelLockRetry = prevWait, prevRetry })

	lockDir := filepath.Join(root, ".parlay")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	holder := flock.New(filepath.Join(lockDir, modelLockName))
	if err := holder.Lock(); err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	defer func() { _ = holder.Unlock() }()

	model.Entities = append(model.Entities, Entity{
		Name:   "Widget",
		Fields: []Field{{Name: "id", Type: "uuid", Required: true}},
	})
	_, err = Save(context.Background(), root, model, etag)

	var busy *LockBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("expected *LockBusyError while the lock is held, got %v", err)
	}
	var cerr *ConflictError
	if errors.As(err, &cerr) {
		t.Fatal("lock contention must not masquerade as a compare-and-swap conflict")
	}
	if busy.Waited != modelLockWait {
		t.Fatalf("LockBusyError.Waited = %s, want the bounded wait %s", busy.Waited, modelLockWait)
	}

	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != testPopulatedYAML {
		t.Fatal("a save that never got the lock changed the file")
	}
}

// TestSaveSucceedsOnceTheLockIsReleased asserts the contention path is a wait,
// not a poisoning: the same save that timed out succeeds once the holder lets
// go, so a LockBusyError really does mean "retry this", not "rebase it".
func TestSaveSucceedsOnceTheLockIsReleased(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	model.Entities = append(model.Entities, Entity{
		Name:   "Widget",
		Fields: []Field{{Name: "id", Type: "uuid", Required: true}},
	})

	lockDir := filepath.Join(root, ".parlay")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	holder := flock.New(filepath.Join(lockDir, modelLockName))
	if err := holder.Lock(); err != nil {
		t.Fatalf("hold the lock: %v", err)
	}

	released := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = holder.Unlock()
		close(released)
	}()

	if _, err := Save(context.Background(), root, model, etag); err != nil {
		t.Fatalf("Save should wait for the lock and then succeed, got %v", err)
	}
	<-released
	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !strings.Contains(string(after), "Widget") {
		t.Fatalf("save after release did not land:\n%s", after)
	}
}
