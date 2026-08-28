package commands

import (
	"os"
	"path/filepath"
	"testing"
)

// The strongest property this design can have: what the emitter WRITES, the
// validator ACCEPTS. If these two ever disagree the tool refuses its own
// output, which is the failure mode the shared-derivation design exists to
// make impossible — so it is asserted rather than assumed.
func TestEmitAssemblyWriteRoundTripsThroughReadiness(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)

	path := filepath.Join(cfg.BuildPath("graded"), "testcases.yaml")

	// Strip the derived suite, then regenerate it with the writer.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, blockers := expectedAssemblySuites(cfg, "graded")
	if len(blockers) > 0 {
		t.Fatalf("derivation blocked on a clean fixture: %v", blockers)
	}
	var suites []emittedSuite
	for _, page := range sortedAssemblyPages(expected) {
		suites = append(suites, buildEmittedSuite(page, expected[page]))
	}
	if len(suites) == 0 {
		t.Fatal("clean fixture derives no assembly suite; the round trip would prove nothing")
	}
	if err := writeAssemblySuites(path, suites); err != nil {
		t.Fatalf("write: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) == string(before) {
		t.Log("note: writer produced byte-identical output")
	}

	b, w := checkAssemblyReadiness(cfg, "graded")
	if len(b) > 0 {
		t.Fatalf("the validator rejected the emitter's own output:\n%v", b)
	}
	_ = w
}
