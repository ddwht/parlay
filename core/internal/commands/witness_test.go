package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Four conformance tests reported success while covering nothing this session:
// one skipped silently when its subject was absent, one passed on an unrelated
// substring, one used a fixture that could not express the property it claimed
// to test, and a hand check proved a flag existed rather than that a caller
// could populate it.
//
// The shared cause is not carelessness. It is that in each case the WITNESS
// PRECONDITION — the subject exists, is distinguishable, and can express the
// mutation — was never part of the assertion. So the test could pass in a world
// where there was nothing to test.
//
// A witness makes that structural: assert() refuses to run unless at least one
// precondition has been registered, so a test that checks behaviour without
// first establishing its subject fails rather than passes.
type witness struct {
	t          witnessT
	registered int
}

// witnessT is the slice of *testing.T the helper uses, so the helper's own
// guard can be tested. A guard that does not fail when violated is one more
// thing reporting success over nothing.
type witnessT interface {
	Helper()
	Fatalf(format string, args ...any)
}

func newWitness(t witnessT) *witness {
	t.Helper()
	return &witness{t: t}
}

// given registers a precondition. It fails immediately when unmet: a missing
// subject is a failure, never a skip, because "there was nothing to check" and
// "everything checked out" must not report the same way.
func (w *witness) given(desc string, ok bool) *witness {
	w.t.Helper()
	if !ok {
		w.t.Fatalf("precondition not met — %s. The test cannot witness anything in this state, so this is a failure and not a skip.", desc)
	}
	w.registered++
	return w
}

// assert runs the behaviour check. It refuses to run at all if nothing about
// the subject was established first.
func (w *witness) assert(what string, fn func()) {
	w.t.Helper()
	if w.registered == 0 {
		w.t.Fatalf("%s: no precondition registered. Establish that the subject exists and can express this property before asserting it — that omission is what let four tests pass over nothing.", what)
	}
	fn()
}

// repoAnchor resolves the repository root from this source file's own
// compile-time path.
//
// Upward search for a marker directory is what produced the silent skip: it
// found this repo's core/.parlay, which holds no modules, and reported ok with
// four unguided commands passing. An anchor cannot drift into the wrong tree.
func repoAnchor(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve this test's own path")
	}
	// .../core/internal/commands/witness_test.go -> repo root
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
	if _, err := os.Stat(filepath.Join(root, ".parlay", "modules")); err != nil {
		t.Fatalf("deployed modules are not at the anchored root %s: %v. "+
			"Run `make sync-skills`; this is repository-owned input, so its absence is a failure.", root, err)
	}
	return root
}

// The helper's own contract. A guard that does not itself fail when violated is
// another thing reporting success over nothing.
type recordingT struct{ failures []string }

func (r *recordingT) Helper() {}
func (r *recordingT) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
	panic(errStopWitness)
}

var errStopWitness = errors.New("witness stopped")

func runGuarded(fn func()) {
	defer func() {
		if r := recover(); r != nil && r != errStopWitness {
			panic(r)
		}
	}()
	fn()
}

func TestWitness_RefusesToAssertWithoutAPrecondition(t *testing.T) {
	rec := &recordingT{}
	w := newWitness(rec)
	ran := false
	runGuarded(func() { w.assert("something", func() { ran = true }) })

	if ran {
		t.Fatal("assert ran its body with no precondition registered — the guard does not guard")
	}
	if len(rec.failures) != 1 {
		t.Fatalf("asserting without a precondition must fail; got %v", rec.failures)
	}
	if !strings.Contains(rec.failures[0], "no precondition registered") {
		t.Errorf("the failure must say what is missing: %q", rec.failures[0])
	}
}

func TestWitness_FailsRatherThanSkipsOnAnAbsentSubject(t *testing.T) {
	rec := &recordingT{}
	w := newWitness(rec)
	runGuarded(func() { w.given("the deployed module exists", false) })

	if len(rec.failures) != 1 {
		t.Fatalf("an unmet precondition must fail; got %v", rec.failures)
	}
	if !strings.Contains(rec.failures[0], "not a skip") {
		t.Errorf("the whole point is that absence is not a skip: %q", rec.failures[0])
	}
}

func TestWitness_AllowsAssertOnceASubjectIsEstablished(t *testing.T) {
	rec := &recordingT{}
	w := newWitness(rec)
	ran := false
	w.given("a subject exists", true)
	w.assert("its behaviour", func() { ran = true })

	if !ran {
		t.Fatal("a registered precondition must let the assertion run")
	}
	if len(rec.failures) != 0 {
		t.Fatalf("a met precondition must not fail: %v", rec.failures)
	}
}
