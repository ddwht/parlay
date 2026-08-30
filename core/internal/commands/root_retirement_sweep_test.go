// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/project-wide-source-aware-inbound-sweep
// parlay-artifact: test
//
// Suite: project-wide-source-aware-inbound-sweep.

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// sweepFixture builds a parent with a retiring child "old" (features
// alpha, beta) and a sibling child "lib", returning what
// sweepRootRetirement needs.
func sweepFixture(t *testing.T) (parent string, retiring config.Root) {
	t.Helper()
	resetRetirementState(t)
	parent = makeRetirementParent(t)
	retiring = addRetirementChild(t, parent, "old", "old", "alpha", "beta")
	addRetirementChild(t, parent, "lib", "lib", "helper")
	return parent, retiring
}

func runSweep(t *testing.T, parent string, retiring config.Root, rec *DispositionRecord) RootSweepResult {
	t.Helper()
	result, err := sweepRootRetirement(parent, retiring, rec)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	return result
}

func writeParentFile(t *testing.T, parent, rel, content string) string {
	t.Helper()
	path := filepath.Join(parent, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func findingsMatching(result RootSweepResult, substr string) []RootSweepFinding {
	var out []RootSweepFinding
	for _, f := range result.Findings {
		if strings.Contains(f.Path, substr) || strings.Contains(f.Ref, substr) {
			out = append(out, f)
		}
	}
	return out
}

func TestSweep_FindsReferenceHeldInAThirdRoot(t *testing.T) {
	parent, retiring := sweepFixture(t)
	// The reference lives in a sibling root that is neither retiring nor
	// the active one — a single-root sweep would never see it.
	writeParentFile(t, parent, "lib/notes/design.md",
		"The helper reads old/spec/intents/alpha for its vocabulary.\n")

	result := runSweep(t, parent, retiring, nil)
	hits := findingsMatching(result, "design.md")
	if len(hits) == 0 {
		t.Fatalf("a reference held in a third root must be found; findings: %+v", result.Findings)
	}
}

func TestSweep_OwnershipMarkerOnSurvivingSourceBlocks(t *testing.T) {
	parent, retiring := sweepFixture(t)
	writeParentFile(t, parent, "internal/shared/util.go",
		"// parlay-feature: alpha\n// parlay-component: util\npackage shared\n")

	result := runSweep(t, parent, retiring, nil)
	hits := findingsMatching(result, "util.go")
	if len(hits) == 0 {
		t.Fatal("an ownership marker naming a retiring-root feature must be a finding")
	}
	if hits[0].Kind != sweepKindOwnershipMarker || !hits[0].Blocking {
		t.Errorf("the marker finding must block while it stands: %+v", hits[0])
	}

	// And the retirement refuses while the finding stands.
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha", "beta"))
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "util.go") {
		t.Errorf("the retirement must refuse naming the finding; got: %v", err)
	}
}

func TestSweep_PathReferenceMatchingNoEnumeratedFeatureBlocks(t *testing.T) {
	parent, retiring := sweepFixture(t)
	// "old/tools/migrate.sh" corresponds to no feature directory —
	// path-space matching catches what feature enumeration cannot.
	writeParentFile(t, parent, "docs/runbook.md",
		"Run old/tools/migrate.sh before every release.\n")

	result := runSweep(t, parent, retiring, nil)
	hits := findingsMatching(result, "runbook.md")
	if len(hits) == 0 {
		t.Fatal("a reference into the root's namespace matching no enumerated feature must block")
	}
	if hits[0].Kind != sweepKindPathReference {
		t.Errorf("expected a path-reference finding; got %+v", hits[0])
	}
}

func TestSweep_GuidanceInstructionNamingRootProvidedCommandBlocks(t *testing.T) {
	parent, retiring := sweepFixture(t)
	writeParentFile(t, parent, ".claude/skills/do-thing/SKILL.md",
		"# Do the thing\n\nRun `parlay build-feature @thing --root old` to rebuild.\n")

	result := runSweep(t, parent, retiring, nil)
	hits := findingsMatching(result, "SKILL.md")
	if len(hits) == 0 {
		t.Fatal("a shipped-guidance instruction naming something only the retiring root provides must block")
	}
}

func TestSweep_GeneratedDocumentAndItsSourceAreTwoFindings(t *testing.T) {
	parent, retiring := sweepFixture(t)
	instruction := "Consult old/spec/intents/alpha before changing this.\n"
	writeParentFile(t, parent, ".parlay/modules/generate-thing.md", instruction)
	writeParentFile(t, parent, "internal/embedded/skills/generate-thing.skill.md", instruction)

	result := runSweep(t, parent, retiring, nil)
	if got := len(findingsMatching(result, "generate-thing")); got != 2 {
		t.Errorf("the same instruction in a generated document and its source is two findings, one per file; got %d: %+v",
			got, findingsMatching(result, "generate-thing"))
	}
}

func TestSweep_OutOfRootOwnershipBlocksUnlessRehomed(t *testing.T) {
	parent, retiring := sweepFixture(t)
	writeParentFile(t, parent, "internal/shared/kept.go",
		"// parlay-feature: keeper\n// parlay-extends: alpha/helper\npackage shared\n")

	// Without a re-homing disposition, the ownership blocks.
	noRehome := runSweep(t, parent, retiring, nil)
	blocked := false
	for _, f := range noRehome.BlockingFindings() {
		if strings.Contains(f.Path, "kept.go") {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("out-of-root ownership must block without a re-homing disposition")
	}

	// With an authority-re-homed-to disposition covering the feature,
	// the ownership finding no longer blocks (readiness owns the rest).
	rec, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: authority-re-homed-to
    target: "@keeper"
    rationale: keeper carries the helper now
  - feature: beta
    term: delivered-and-deleted
    rationale: gone
`)
	if err != nil {
		t.Fatal(err)
	}
	rehomed := runSweep(t, parent, retiring, rec)
	for _, f := range rehomed.BlockingFindings() {
		if strings.Contains(f.Path, "kept.go") {
			t.Errorf("a re-homed ownership finding must no longer block: %+v", f)
		}
	}
}

func TestSweep_UnreadableFileIsAScanFailureThatRefuses(t *testing.T) {
	parent, retiring := sweepFixture(t)
	sealed := writeParentFile(t, parent, "lib/internal/sealed.go", "package internal\n")
	if err := os.Chmod(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sealed, 0o644) })

	result := runSweep(t, parent, retiring, nil)
	found := false
	for _, f := range result.Failures {
		if strings.Contains(f.Path, "sealed.go") {
			found = true
		}
	}
	if !found {
		t.Fatalf("an unreadable file must be recorded as a scan failure naming it; failures: %+v", result.Failures)
	}

	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha", "beta"))
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "cannot tell is not none") {
		t.Errorf("the retirement must refuse — cannot tell is not none; got: %v", err)
	}
}

func TestSweep_NarrativeProseMentioningTheRootDoesNotBlock(t *testing.T) {
	parent, retiring := sweepFixture(t)
	writeParentFile(t, parent, "docs/history.md",
		"The old root used to hold this work before the split.\n"+
			"// The old subproject handled exports back then.\n")

	result := runSweep(t, parent, retiring, nil)
	if hits := findingsMatching(result, "history.md"); len(hits) != 0 {
		t.Errorf("prose mentioning the root's name with no instruction and no marker must not block: %+v", hits)
	}
}

func TestSweep_EveryFindingCarriesArtifactPositionAndReference(t *testing.T) {
	parent, retiring := sweepFixture(t)
	writeParentFile(t, parent, "internal/shared/util.go", "// parlay-feature: alpha\npackage shared\n")
	writeParentFile(t, parent, "docs/runbook.md", "Run old/tools/migrate.sh weekly.\n")
	writeParentFile(t, parent, "docs/spec-notes.md", "See @alpha for details.\n")
	writeParentFile(t, parent, ".claude/skills/x/SKILL.md", "Use --root old here.\n")

	result := runSweep(t, parent, retiring, nil)
	if len(result.Findings) < 4 {
		t.Fatalf("expected findings of each kind; got: %+v", result.Findings)
	}
	for _, f := range result.Findings {
		if f.Path == "" || f.Position == "" || f.Ref == "" {
			t.Errorf("every finding must carry owning artifact, position, and the reference as written: %+v", f)
		}
		if !strings.HasPrefix(f.Position, "line ") {
			t.Errorf("position should locate the reference within the artifact: %+v", f)
		}
	}
}

func TestSweep_CompletesBeforeAnyMutation(t *testing.T) {
	parent, retiring := sweepFixture(t)
	_ = retiring
	// A blocking finding stands, so the run refuses — and the recorded
	// event order must show the sweep ran with no mutation event at all.
	writeParentFile(t, parent, "internal/shared/util.go", "// parlay-feature: alpha\npackage shared\n")
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha", "beta"))
	interactiveTTY(t)

	events := recordEvents(t)
	before := treeSnapshot(t, parent)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the run must refuse while the finding stands")
	}
	sweepSeen := false
	for _, e := range *events {
		switch e {
		case "sweep":
			sweepSeen = true
		case "stage-archive", "archive-copy", "promote", "write-journal", "write-record", "remove-contents", "deregister-index":
			t.Errorf("no write of any kind may precede sweep completion (or happen at all here); events: %v", *events)
		}
	}
	if !sweepSeen {
		t.Errorf("the sweep must have run: %v", *events)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}
