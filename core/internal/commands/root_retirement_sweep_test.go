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

// --- Suite 3 (continued): the grammar features are actually written in
//
// Markers, `@feature` refs and `--root <name>` are the DECORATED ways of
// naming a feature. The ordinary way is to write the group-qualified
// slug down: `design-loop/design-loop` in a Go comment, a YAML value
// naming `studio-foundation/studio-deployer`, prose in a shipped skill
// naming a component under it. Every one of those keeps pointing at the
// retiring root after it is gone, so every one of them is a finding.

// groupSweepFixture builds a retiring root whose features are
// group-qualified, the way real features are named.
func groupSweepFixture(t *testing.T) (parent string, retiring config.Root) {
	t.Helper()
	resetRetirementState(t)
	parent = makeRetirementParent(t)
	retiring = addRetirementChild(t, parent, "old", "old",
		"design-loop/design-loop", "studio-foundation/studio-deployer")
	addRetirementChild(t, parent, "lib", "lib", "helper")
	return parent, retiring
}

// assertGroupQualifiedHit requires a blocking group-qualified finding in
// the named artifact, carrying the reference as written.
func assertGroupQualifiedHit(t *testing.T, result RootSweepResult, artifact, ref string) {
	t.Helper()
	for _, f := range result.Findings {
		if !strings.Contains(f.Path, artifact) {
			continue
		}
		if f.Kind != sweepKindGroupQualifiedReference {
			continue
		}
		if f.Ref != ref {
			t.Errorf("%s: the finding must carry the reference as written (got %q, want %q)", artifact, f.Ref, ref)
			return
		}
		if !f.Blocking {
			t.Errorf("%s: a live reference into the retiring root must block: %+v", artifact, f)
		}
		if f.Feature == "" {
			t.Errorf("%s: the finding must name the retiring feature it found: %+v", artifact, f)
		}
		return
	}
	t.Errorf("%s: a plain group-qualified reference (%q) must be found; findings: %+v", artifact, ref, result.Findings)
}

func TestSweep_PlainGroupQualifiedReferencesAreFoundInEveryCorpus(t *testing.T) {
	parent, retiring := groupSweepFixture(t)

	// A Go comment in surviving source.
	writeParentFile(t, parent, "internal/shared/loop.go",
		"// The phase order here mirrors design-loop/design-loop.\npackage shared\n")
	// A YAML value in surviving build state.
	writeParentFile(t, parent, "lib/.parlay/build/helper/buildfile.yaml",
		"feature: helper\nderived-from: design-loop/design-loop\n")
	// Markdown prose in a document nobody generated.
	writeParentFile(t, parent, "docs/architecture.md",
		"Phase ordering is owned by design-loop/design-loop and nothing else.\n")

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "loop.go", "design-loop/design-loop")
	assertGroupQualifiedHit(t, result, "buildfile.yaml", "design-loop/design-loop")
	assertGroupQualifiedHit(t, result, "architecture.md", "design-loop/design-loop")
}

func TestSweep_ComponentQualifiedReferencesAreFound(t *testing.T) {
	parent, retiring := groupSweepFixture(t)
	writeParentFile(t, parent, "internal/deploy/runner.go",
		"// Delegates to studio-foundation/studio-deployer/cross-cutting/deploy-step.\npackage deploy\n")
	writeParentFile(t, parent, "docs/deploy.md",
		"See studio-foundation/studio-deployer/cross-cutting/deploy-step/notes for the sequence.\n")

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "runner.go",
		"studio-foundation/studio-deployer/cross-cutting/deploy-step")
	assertGroupQualifiedHit(t, result, "deploy.md",
		"studio-foundation/studio-deployer/cross-cutting/deploy-step/notes")
}

func TestSweep_GroupQualifiedReferenceInGeneratedAndDeployedCopies(t *testing.T) {
	parent, retiring := groupSweepFixture(t)
	instruction := "Rebuild design-loop/design-loop before shipping.\n"
	// The deployed copy, the module it was deployed from, and the
	// embedded authoring source are three artifacts and three findings.
	writeParentFile(t, parent, ".claude/skills/parlay-loop/SKILL.md", instruction)
	writeParentFile(t, parent, ".parlay/modules/loop.md", instruction)
	writeParentFile(t, parent, "core/internal/embedded/skills/loop.skill.md", instruction)

	result := runSweep(t, parent, retiring, nil)
	var hits int
	for _, f := range result.Findings {
		if f.Kind == sweepKindGroupQualifiedReference && strings.Contains(f.Ref, "design-loop/design-loop") {
			hits++
		}
	}
	if hits != 3 {
		t.Errorf("a generated copy, the module it came from and the embedded source are three findings; got %d: %+v", hits, result.Findings)
	}
}

func TestSweep_GroupQualifiedReferenceHeldInASiblingRootSpec(t *testing.T) {
	parent, retiring := groupSweepFixture(t)
	// A sibling root's own specification naming the retiring root's
	// feature: neither root is the active one, and a single-root sweep
	// would never look here.
	writeParentFile(t, parent, "lib/spec/intents/helper/infrastructure.md",
		"**Behavior**: The helper defers phase ordering to design-loop/design-loop.\n")

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "lib/spec/intents/helper/infrastructure.md",
		"design-loop/design-loop")

	// And the retirement refuses while it stands.
	retireRootDispositions = writeDispositionsFile(t,
		deliveredDispositions("design-loop/design-loop", "studio-foundation/studio-deployer"))
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "infrastructure.md") {
		t.Errorf("the retirement must refuse naming the sibling-root reference; got: %v", err)
	}
}

func TestSweep_GroupQualifiedMatchingIsWordBounded(t *testing.T) {
	parent, retiring := groupSweepFixture(t)
	// None of these name the retiring root's features. A match here
	// would be a false positive produced by substring matching.
	writeParentFile(t, parent, "docs/nearby.md",
		"The codesign-loop/design-loop-notes file is unrelated.\n"+
			"So is redesign-loops and the design-loop idea in general.\n"+
			"predesign-loop/design-loopy names nothing here.\n")

	result := runSweep(t, parent, retiring, nil)
	for _, f := range result.Findings {
		if strings.Contains(f.Path, "nearby.md") {
			t.Errorf("word boundaries must keep a longer identifier from matching a feature slug: %+v", f)
		}
	}
}

func TestSweep_BareWordFeatureNeedsAPathIshContext(t *testing.T) {
	// A single-segment feature slug is an ordinary English word as often
	// as it is a reference, so it counts only in a path-ish context.
	// This is the guard that keeps the wider grammar usable; it never
	// applies to the group-qualified slugs the corpus actually uses.
	parent, retiring := sweepFixture(t) // features: alpha, beta
	writeParentFile(t, parent, "docs/prose.md",
		"The alpha release was fine and beta went out on time.\n")
	writeParentFile(t, parent, "docs/pathish.md",
		"Regenerate alpha/handbook before the release.\n")

	result := runSweep(t, parent, retiring, nil)
	for _, f := range result.Findings {
		if strings.Contains(f.Path, "prose.md") {
			t.Errorf("a bare word matching a single-segment slug in prose must not block: %+v", f)
		}
	}
	if hits := findingsMatching(result, "pathish.md"); len(hits) == 0 {
		t.Errorf("the same slug in a path-ish context must be found; findings: %+v", result.Findings)
	}
}

func TestSweep_IsALineBasedLexicalScanThatRefusesWhatItCannotRead(t *testing.T) {
	// What the sweep detects is exactly what it says it detects: written
	// references, found by reading lines. Nothing is parsed, so nothing
	// is claimed about structure — but a file that cannot be READ at all
	// refuses the retirement, because a check whose purpose is to
	// establish that nothing points here cannot report a clean result
	// over something it did not read.
	parent, retiring := groupSweepFixture(t)

	// A file whose contents are structurally broken YAML is still
	// scanned line by line, and the reference in it is still found.
	writeParentFile(t, parent, "lib/broken.yaml",
		"feature: [unclosed\n  derived-from: design-loop/design-loop\n")
	// A binary file carries no textual reference and is passed over
	// without becoming a failure.
	writeParentFile(t, parent, "lib/blob.bin", "\x00\x01\x02binary design-loop/design-loop\x00")
	// A file that cannot be read is a scan failure.
	sealed := writeParentFile(t, parent, "lib/sealed.go", "package lib\n")
	if err := os.Chmod(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sealed, 0o644) })

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "broken.yaml", "design-loop/design-loop")
	for _, f := range result.Failures {
		if strings.Contains(f.Path, "broken.yaml") {
			t.Errorf("a lexical scan has no parse step and so no parse failure: %+v", f)
		}
		if strings.Contains(f.Path, "blob.bin") {
			t.Errorf("binary content carries no textual reference and is not a failure: %+v", f)
		}
	}
	sealedFailure := false
	for _, f := range result.Failures {
		if strings.Contains(f.Path, "sealed.go") {
			sealedFailure = true
		}
	}
	if !sealedFailure {
		t.Fatalf("a file that cannot be read must be a scan failure naming it; failures: %+v", result.Failures)
	}

	retireRootDispositions = writeDispositionsFile(t,
		deliveredDispositions("design-loop/design-loop", "studio-foundation/studio-deployer"))
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "cannot tell is not none") {
		t.Errorf("an unreadable file must refuse the retirement; got: %v", err)
	}
}
