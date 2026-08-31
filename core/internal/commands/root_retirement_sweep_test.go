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
// slug down: `demo-group/demo-feature` in a Go comment, a YAML value
// naming `demo-group/other-feature`, prose in a shipped skill
// naming a component under it. Every one of those keeps pointing at the
// retiring root after it is gone, so every one of them is a finding.

// groupSweepFixture builds a retiring root whose features are
// group-qualified, the way real features are named.
func groupSweepFixture(t *testing.T) (parent string, retiring config.Root) {
	t.Helper()
	resetRetirementState(t)
	parent = makeRetirementParent(t)
	retiring = addRetirementChild(t, parent, "old", "old",
		"demo-group/demo-feature", "demo-group/other-feature")
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
		"// The phase order here mirrors demo-group/demo-feature.\npackage shared\n")
	// A YAML value in surviving build state.
	writeParentFile(t, parent, "lib/.parlay/build/helper/buildfile.yaml",
		"feature: helper\nderived-from: demo-group/demo-feature\n")
	// Markdown prose in a document nobody generated.
	writeParentFile(t, parent, "docs/architecture.md",
		"Phase ordering is owned by demo-group/demo-feature and nothing else.\n")

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "loop.go", "demo-group/demo-feature")
	assertGroupQualifiedHit(t, result, "buildfile.yaml", "demo-group/demo-feature")
	assertGroupQualifiedHit(t, result, "architecture.md", "demo-group/demo-feature")
}

func TestSweep_ComponentQualifiedReferencesAreFound(t *testing.T) {
	parent, retiring := groupSweepFixture(t)
	writeParentFile(t, parent, "internal/deploy/runner.go",
		"// Delegates to demo-group/other-feature/cross-cutting/deploy-step.\npackage deploy\n")
	writeParentFile(t, parent, "docs/deploy.md",
		"See demo-group/other-feature/cross-cutting/deploy-step/notes for the sequence.\n")

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "runner.go",
		"demo-group/other-feature/cross-cutting/deploy-step")
	assertGroupQualifiedHit(t, result, "deploy.md",
		"demo-group/other-feature/cross-cutting/deploy-step/notes")
}

func TestSweep_GroupQualifiedReferenceInGeneratedAndDeployedCopies(t *testing.T) {
	parent, retiring := groupSweepFixture(t)
	instruction := "Rebuild demo-group/demo-feature before shipping.\n"
	// The deployed copy, the module it was deployed from, and the
	// embedded authoring source are three artifacts and three findings.
	writeParentFile(t, parent, ".claude/skills/parlay-loop/SKILL.md", instruction)
	writeParentFile(t, parent, ".parlay/modules/loop.md", instruction)
	writeParentFile(t, parent, "core/internal/embedded/skills/loop.skill.md", instruction)

	result := runSweep(t, parent, retiring, nil)
	var hits int
	for _, f := range result.Findings {
		if f.Kind == sweepKindGroupQualifiedReference && strings.Contains(f.Ref, "demo-group/demo-feature") {
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
		"**Behavior**: The helper defers phase ordering to demo-group/demo-feature.\n")

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "lib/spec/intents/helper/infrastructure.md",
		"demo-group/demo-feature")

	// And the retirement refuses while it stands.
	retireRootDispositions = writeDispositionsFile(t,
		deliveredDispositions("demo-group/demo-feature", "demo-group/other-feature"))
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
		"The codemo-group/demo-feature-notes file is unrelated.\n"+
			"So is redesign-loops and the demo-group idea in general.\n"+
			"predemo-group/demo-featurey names nothing here.\n")

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
		"feature: [unclosed\n  derived-from: demo-group/demo-feature\n")
	// A binary file carries no textual reference and is passed over
	// without becoming a failure.
	writeParentFile(t, parent, "lib/blob.bin", "\x00\x01\x02binary demo-group/demo-feature\x00")
	// A file that cannot be read is a scan failure.
	sealed := writeParentFile(t, parent, "lib/sealed.go", "package lib\n")
	if err := os.Chmod(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sealed, 0o644) })

	result := runSweep(t, parent, retiring, nil)
	assertGroupQualifiedHit(t, result, "broken.yaml", "demo-group/demo-feature")
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
		deliveredDispositions("demo-group/demo-feature", "demo-group/other-feature"))
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "cannot tell is not none") {
		t.Errorf("an unreadable file must refuse the retirement; got: %v", err)
	}
}

// --- Suite 3 (continued): what is project content, and what is not -----
//
// Both of these came out of a real preview run against this repository,
// where the sweep reported findings the operator could not act on: one
// set from a checkout of an old commit that happened to sit inside the
// tree, and one from the operator's own disposition record being read
// back at them as evidence.

// plantNestedCheckout creates a directory that git would recognize as a
// checkout of its own — a linked worktree marks itself with a .git FILE
// rather than a directory — holding content that names the retiring
// root exactly as live content would.
func plantNestedCheckout(t *testing.T, parent, rel, gitEntry string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(parent, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	switch gitEntry {
	case "file":
		if err := os.WriteFile(filepath.Join(dir, ".git"),
			[]byte("gitdir: /somewhere/.git/worktrees/old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	case "dir":
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSweep_ANestedCheckoutIsNotProjectContent(t *testing.T) {
	// A worktree marks itself with a .git file; a submodule or vendored
	// clone with a .git directory. Both are another commit's copy of the
	// project, holding whatever the features were called then.
	for _, gitEntry := range []string{"file", "dir"} {
		t.Run(gitEntry, func(t *testing.T) {
			parent, retiring := groupSweepFixture(t)
			plantNestedCheckout(t, parent, ".claude/worktrees/agent-stale", gitEntry, map[string]string{
				"internal/old.go": "// parlay-feature: demo-group/demo-feature\n" +
					"// The stale checkout still calls it demo-group/demo-feature.\n" +
					"package old\n",
				"docs/guide.md": "Run parlay build-feature --root old to rebuild.\n",
			})

			result := runSweep(t, parent, retiring, nil)
			for _, f := range result.Findings {
				if strings.Contains(f.Path, "agent-stale") {
					t.Errorf("a checkout of another commit is not project content and must not be swept: %+v", f)
				}
			}
			for _, f := range result.Failures {
				if strings.Contains(f.Path, "agent-stale") {
					t.Errorf("a skipped checkout must not be a scan failure either: %+v", f)
				}
			}
		})
	}
}

func TestSweep_ANestedCheckoutDoesNotBlockOrFailReHomeReadiness(t *testing.T) {
	// The readiness check asks whether the surviving files already carry
	// the target's marker, and it asks it of the findings the sweep
	// produced — so a stale checkout that still names the retiring
	// feature would fail readiness against a file nobody can fix.
	parent, retiring := groupSweepFixture(t)
	writeParentFile(t, parent, "internal/shared/kept.go",
		"// parlay-feature: keeper\n// parlay-extends: demo-group/demo-feature/helper\npackage shared\n")
	plantNestedCheckout(t, parent, ".claude/worktrees/agent-stale", "file", map[string]string{
		"internal/shared/kept.go": "// parlay-feature: demo-group/demo-feature\npackage shared\n",
	})

	rec, err := loadRecord(t, `dispositions:
  - feature: demo-group/demo-feature
    term: authority-re-homed-to
    target: "@keeper"
    rationale: keeper carries the helper now
  - feature: demo-group/other-feature
    term: delivered-and-deleted
    rationale: gone
`)
	if err != nil {
		t.Fatal(err)
	}
	result := runSweep(t, parent, retiring, rec)
	for _, f := range result.Findings {
		if strings.Contains(f.Path, "agent-stale") {
			t.Fatalf("the stale checkout must not reach readiness at all: %+v", f)
		}
	}
	idx, err := config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range checkRehomeTargets(parent, idx, retiring, rec, result) {
		if strings.Contains(e.Error(), "agent-stale") {
			t.Errorf("re-home readiness must not be judged against a checkout of another commit: %v", e)
		}
	}
}

func TestSweep_TheDispositionRecordItselfIsExempt(t *testing.T) {
	// The record necessarily names every feature in the retiring root —
	// that is what it is for — so scanning it reports the operator's own
	// answers back as evidence against them.
	parent, retiring := groupSweepFixture(t)
	// Inside the project tree, so the walk reaches it.
	recPath := writeParentFile(t, parent, "docs/plans/retirement-dispositions.yaml",
		deliveredDispositions("demo-group/demo-feature", "demo-group/other-feature"))
	rec, err := LoadDispositionRecord(recPath)
	if err != nil {
		t.Fatal(err)
	}

	result := runSweep(t, parent, retiring, rec)
	for _, f := range result.Findings {
		if strings.Contains(f.Path, "retirement-dispositions.yaml") {
			t.Errorf("the disposition record in use must not be swept: %+v", f)
		}
	}

	// Exactly that file, and nothing broader: another file that merely
	// looks like a disposition record is scanned like anything else.
	writeParentFile(t, parent, "docs/plans/other-dispositions.yaml",
		deliveredDispositions("demo-group/demo-feature"))
	again := runSweep(t, parent, retiring, rec)
	found := false
	for _, f := range again.Findings {
		if strings.Contains(f.Path, "other-dispositions.yaml") {
			found = true
		}
	}
	if !found {
		t.Error("the exemption must cover exactly the record in use, not every file shaped like one")
	}
}

// --- Suite 2 (continued): the operator's dismissal, written down -------

func TestRetireRoot_AnAcknowledgedReferenceNoLongerBlocksAndIsListed(t *testing.T) {
	parent, _ := groupSweepFixture(t)
	writeParentFile(t, parent, "docs/history.md",
		"Phase ordering used to live in demo-group/demo-feature before the split.\n")

	dispositions := `dispositions:
  - feature: demo-group/demo-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
  - feature: demo-group/other-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
acknowledged-references:
  - path: docs/history.md
    reference: demo-group/demo-feature
    rationale: a sentence about what used to be, not an instruction that reads the root
`
	retireRootDispositions = writeDispositionsFile(t, dispositions)
	retireRootPreview = true
	cmd, out := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("the preview should run: %v", err)
	}
	if !strings.Contains(out.String(), "Acknowledged (accepted by the disposition record") {
		t.Errorf("an acknowledged finding must be listed under its own heading, not silently dropped; got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "not an instruction that reads the root") {
		t.Errorf("the preview must show why it was accepted; got:\n%s", out.String())
	}

	// And execution is no longer blocked by it.
	retireRootPreview = false
	retireRootNonInteractive = true
	cmd, _ = retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil {
		t.Fatal("the unattended refusal should still apply")
	}
	if strings.Contains(err.Error(), "history.md") {
		t.Errorf("an acknowledged finding must not block execution; got: %v", err)
	}
}

func TestRetireRoot_AnUnacknowledgedReferenceStillRefuses(t *testing.T) {
	parent, _ := groupSweepFixture(t)
	writeParentFile(t, parent, "docs/history.md",
		"Phase ordering used to live in demo-group/demo-feature before the split.\n")
	writeParentFile(t, parent, "docs/runbook.md",
		"Rebuild demo-group/demo-feature before shipping.\n")

	retireRootDispositions = writeDispositionsFile(t, `dispositions:
  - feature: demo-group/demo-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
  - feature: demo-group/other-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
acknowledged-references:
  - path: docs/history.md
    reference: demo-group/demo-feature
    rationale: prose about what used to be
`)
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "runbook.md") {
		t.Fatalf("a finding nobody acknowledged must still refuse — fail-closed is not traded for the dismissal; got: %v", err)
	}
	if strings.Contains(err.Error(), "history.md") {
		t.Errorf("the acknowledged one must not still be listed as blocking; got: %v", err)
	}
}

func TestRetireRoot_AnAcknowledgmentMustMatchExactly(t *testing.T) {
	// Neither half may be approximate: an acknowledgment that covers
	// findings the operator has not read is the check switched off.
	cases := []struct {
		name string
		ack  string
	}{
		{
			name: "wrong-path",
			ack: `  - path: docs/other.md
    reference: demo-group/demo-feature
    rationale: assessed`,
		},
		{
			name: "wrong-reference",
			ack: `  - path: docs/history.md
    reference: demo-group/demo-feature/phases
    rationale: assessed`,
		},
		{
			name: "path-prefix-is-not-a-match",
			ack: `  - path: docs
    reference: demo-group/demo-feature
    rationale: assessed`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent, _ := groupSweepFixture(t)
			writeParentFile(t, parent, "docs/history.md",
				"Phase ordering used to live in demo-group/demo-feature before the split.\n")
			retireRootDispositions = writeDispositionsFile(t, `dispositions:
  - feature: demo-group/demo-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
  - feature: demo-group/other-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
acknowledged-references:
`+tc.ack+"\n")
			retireRootNonInteractive = true
			cmd, _ := retireCmd(t, parent, "")
			err := runRetireRoot(cmd, []string{"old"})
			if err == nil || !strings.Contains(err.Error(), "history.md") {
				t.Fatalf("an acknowledgment that does not name the finding exactly must not dismiss it; got: %v", err)
			}
		})
	}
}

func TestRetireRoot_AnAcknowledgmentMatchingNothingIsReported(t *testing.T) {
	parent, _ := groupSweepFixture(t)
	retireRootDispositions = writeDispositionsFile(t, `dispositions:
  - feature: demo-group/demo-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
  - feature: demo-group/other-feature
    term: delivered-and-deleted
    rationale: shipped and later removed
acknowledged-references:
  - path: docs/typo.md
    reference: demo-group/demo-feature
    rationale: assessed
`)
	retireRootPreview = true
	cmd, out := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Acknowledgment matched no finding") {
		t.Errorf("an acknowledgment that dismissed nothing must say so, or a misspelling looks like a refusal nobody can explain; got:\n%s", out.String())
	}
}

func TestDispositions_AcknowledgmentsAreReadStructurallyClosed(t *testing.T) {
	base := `dispositions:
  - feature: alpha
    term: delivered-and-deleted
    rationale: shipped and later removed
acknowledged-references:
`
	for _, tc := range []struct{ name, entry, want string }{
		{"unknown-field", "  - path: docs/x.md\n    reference: a/b\n    rationale: ok\n    reson: typo\n", "reson"},
		{"no-path", "  - reference: a/b\n    rationale: ok\n", "names no path"},
		{"no-reference", "  - path: docs/x.md\n    rationale: ok\n", "names no reference"},
		{"no-rationale", "  - path: docs/x.md\n    reference: a/b\n", "no rationale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadRecord(t, base+tc.entry)
			if err == nil {
				t.Fatalf("an acknowledgment that is not fully written down must be refused")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal must say what is wrong (want %q); got: %v", tc.want, err)
			}
		})
	}

	rec, err := loadRecord(t, base+"  - path: docs/x.md\n    reference: a/b\n    rationale: assessed as prose\n")
	if err != nil {
		t.Fatalf("a well-formed acknowledgment must be accepted: %v", err)
	}
	if len(rec.Acknowledged) != 1 || rec.Acknowledged[0].Reference != "a/b" {
		t.Errorf("the acknowledgment must decode as written: %+v", rec.Acknowledged)
	}
	if rec.Path == "" {
		t.Error("the record must remember where it was read from, so the sweep can exempt exactly that file")
	}
}
