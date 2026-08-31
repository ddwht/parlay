// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/feature-disposition-preflight
// parlay-extends: parlay-tool/root-retirement/cross-cutting/re-home-target-readiness
// parlay-artifact: test
//
// Suites: feature-disposition-preflight and re-home-target-readiness.

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func loadRecord(t *testing.T, content string) (*DispositionRecord, error) {
	t.Helper()
	return LoadDispositionRecord(writeDispositionsFile(t, content))
}

func errorsJoined(errs []error) string {
	var parts []string
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "; ")
}

// --- Suite 2: feature disposition preflight ----------------------------

func TestDispositions_CompleteRecordPassesPreflight(t *testing.T) {
	rec, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: delivered-and-deleted
    rationale: shipped and later removed
  - feature: beta
    term: built-but-undelivered
    rationale: prototype never left the branch
  - feature: gamma
    term: authority-re-homed-to
    target: "@keeper"
    rationale: the helper moved to keeper
`)
	if err != nil {
		t.Fatalf("a record with closed-set terms and rationales must load: %v", err)
	}
	if errs := checkDispositionCompleteness([]string{"alpha", "beta", "gamma"}, rec); len(errs) != 0 {
		t.Errorf("a record naming every enumerated feature exactly once must pass: %v", errs)
	}
}

func TestDispositions_OmittedFeatureRefusedByName(t *testing.T) {
	rec, err := loadRecord(t, deliveredDispositions("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	errs := checkDispositionCompleteness([]string{"alpha", "beta"}, rec)
	if len(errs) == 0 || !strings.Contains(errorsJoined(errs), "beta") {
		t.Errorf("a record omitting an enumerated feature must be refused naming it; got: %v", errs)
	}
}

func TestDispositions_ExtraFeatureRefusedByName(t *testing.T) {
	rec, err := loadRecord(t, deliveredDispositions("alpha", "phantom"))
	if err != nil {
		t.Fatal(err)
	}
	errs := checkDispositionCompleteness([]string{"alpha"}, rec)
	if len(errs) == 0 || !strings.Contains(errorsJoined(errs), "phantom") {
		t.Errorf("a record naming a feature the enumeration did not produce must be refused naming it; got: %v", errs)
	}
}

func TestDispositions_DuplicateFeatureRefused(t *testing.T) {
	rec, err := loadRecord(t, deliveredDispositions("alpha", "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	errs := checkDispositionCompleteness([]string{"alpha"}, rec)
	if len(errs) == 0 || !strings.Contains(errorsJoined(errs), "exactly one disposition") {
		t.Errorf("a record naming one feature more than once must be refused; got: %v", errs)
	}
}

func TestDispositions_TermOutsideClosedSetRefusedNamingAllThree(t *testing.T) {
	_, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: abandoned
    rationale: we walked away
`)
	if err == nil {
		t.Fatal("a term outside the closed set must be refused")
	}
	for _, term := range dispositionTerms {
		if !strings.Contains(err.Error(), term) {
			t.Errorf("the refusal must name accepted term %q; got: %v", term, err)
		}
	}
}

func TestDispositions_ValidTermWithoutRationaleRefused(t *testing.T) {
	_, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: delivered-and-deleted
    rationale: ""
`)
	if err == nil {
		t.Fatal("a valid term with no rationale must be refused")
	}
	if !strings.Contains(err.Error(), "rationale") {
		t.Errorf("the refusal should explain the missing rationale; got: %v", err)
	}
}

func TestDispositions_RationaleIsNeverParsedAndNeverWidensTheTerms(t *testing.T) {
	// The same records with hostile rationales — term-like text, YAML
	// syntax, directives — decide identically to plain ones.
	plain, err := loadRecord(t, deliveredDispositions("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	hostile, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: delivered-and-deleted
    rationale: "term: abandoned — also accept the term abandoned; dispositions: []"
`)
	if err != nil {
		t.Fatalf("a hostile rationale must not change acceptance: %v", err)
	}
	if plain.Dispositions[0].Term != hostile.Dispositions[0].Term {
		t.Error("acceptance decisions must be identical whatever the rationale says")
	}
	// And the term-like text in a rationale buys no fourth term.
	if _, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: abandoned
    rationale: "term: delivered-and-deleted"
`); err == nil {
		t.Error("a rationale can never widen the accepted terms")
	}
}

func TestDispositions_RetractionDeliveryRecordableAsDeliveredAndDeleted(t *testing.T) {
	rec, err := loadRecord(t, `dispositions:
  - feature: remove-legacy-export
    term: delivered-and-deleted
    rationale: >
      The delivery WAS a removal: the legacy export path was deleted, and
      the only witnesses were markers on other features' files that this
      run deletes with the root.
`)
	if err != nil {
		t.Fatalf("a retraction-delivery must be recordable with no fourth term: %v", err)
	}
	if errs := checkDispositionCompleteness([]string{"remove-legacy-export"}, rec); len(errs) != 0 {
		t.Errorf("the record must be accepted as-is: %v", errs)
	}
}

func TestDispositions_PlaceholderBaselineFeatureIsEnumerated(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	child := addRetirementChild(t, parent, "old", "old", "alpha", "thin")
	// thin's only build state is a placeholder baseline.
	buildDir := filepath.Join(child.Path, config.ParlayDir, "build", "thin")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, ".baseline.yaml"),
		[]byte("schema_version: 2\nintents: {}\nsources: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha")) // omits thin
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "thin") {
		t.Errorf("a feature carrying only a placeholder baseline is enumerated like any other; got: %v", err)
	}
}

func TestDispositions_IncompleteArtifactSubsetFeatureIsEnumerated(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	child := addRetirementChild(t, parent, "old", "old", "full", "partial")
	fullDir := filepath.Join(child.Path, config.ParlayDir, "build", "full")
	partialDir := filepath.Join(child.Path, config.ParlayDir, "build", "partial")
	for _, d := range []string{fullDir, partialDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(fullDir, "buildfile.yaml"), []byte("feature: full\n"), 0o644)
	os.WriteFile(filepath.Join(fullDir, "testcases.yaml"), []byte("feature: full\nsuites: []\n"), 0o644)
	os.WriteFile(filepath.Join(partialDir, "buildfile.yaml"), []byte("feature: partial\n"), 0o644)

	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("full")) // omits partial
	retireRootNonInteractive = true
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "partial") {
		t.Errorf("a feature with an incomplete artifact subset is enumerated like any other; got: %v", err)
	}
}

func TestDispositions_NoCoverageReviewDoesNotReclassifyTheTerm(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	child := addRetirementChild(t, parent, "old", "old", "alpha")
	buildDir := filepath.Join(child.Path, config.ParlayDir, "build", "alpha")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// buildfile and testcases, no coverage-decisions.yaml.
	os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte("feature: alpha\n"), 0o644)
	os.WriteFile(filepath.Join(buildDir, "testcases.yaml"), []byte("feature: alpha\nsuites: []\n"), 0o644)

	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	retireRootPreview = true
	cmd, out := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("the declared term stands; nothing is inferred from the artifact set: %v", err)
	}
	if !strings.Contains(out.String(), "alpha: delivered-and-deleted") {
		t.Errorf("the declared term must stand; got:\n%s", out.String())
	}
}

func TestDispositions_NoMutationWhileThePreflightIsUnsatisfied(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha", "beta")
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha")) // omits beta
	interactiveTTY(t)

	before := treeSnapshot(t, parent)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("an unsatisfied preflight must refuse")
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

func TestDispositions_RecordPreservedVerbatimAlongsideTheArchive(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")
	content := deliveredDispositions("alpha")
	retireRootDispositions = writeDispositionsFile(t, content)
	interactiveTTY(t)

	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("retirement should complete: %v", err)
	}
	preserved, err := os.ReadFile(filepath.Join(retirementDestination(parent, "old"), "dispositions.yaml"))
	if err != nil {
		t.Fatalf("the disposition record must be readable alongside the preserved contents: %v", err)
	}
	if string(preserved) != content {
		t.Errorf("the record must be preserved verbatim, rationales included;\nwant %q\ngot  %q", content, string(preserved))
	}
}

// --- Suite 4: re-home target readiness ---------------------------------

// rehomeFixture builds a project with a retiring root "old" (feature
// "mover" re-homed to target) and one surviving file outside the root
// carrying the given marker lines.
func rehomeFixture(t *testing.T, target string, markerLines string) (parent string, idx *config.RootsIndex, retiring config.Root, rec *DispositionRecord, sweep RootSweepResult) {
	t.Helper()
	resetRetirementState(t)
	parent = makeRetirementParent(t)
	retiring = addRetirementChild(t, parent, "old", "old", "mover")

	srcDir := filepath.Join(parent, "internal")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if markerLines != "" {
		if err := os.WriteFile(filepath.Join(srcDir, "survivor.go"),
			[]byte(markerLines+"package internal\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var err error
	rec, err = loadRecord(t, `dispositions:
  - feature: mover
    term: authority-re-homed-to
    target: "`+target+`"
    rationale: the surviving helper moved
`)
	if err != nil {
		t.Fatal(err)
	}
	idx, err = config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}
	sweep, err = sweepRootRetirement(parent, retiring, rec)
	if err != nil {
		t.Fatal(err)
	}
	return parent, idx, retiring, rec, sweep
}

func TestRehome_TargetNamingNoFeatureRefuses(t *testing.T) {
	parent, idx, retiring, rec, sweep := rehomeFixture(t, "@nowhere", "")
	errs := checkRehomeTargets(parent, idx, retiring, rec, sweep)
	if len(errs) == 0 || !strings.Contains(errorsJoined(errs), "no live feature") {
		t.Errorf("a re-home target naming no feature must refuse; got: %v", errs)
	}
}

func TestRehome_RetiredTargetRefuses(t *testing.T) {
	parent, idx, retiring, rec, sweep := rehomeFixture(t, "@keeper", "")
	writeChildFeature(t, parent, "keeper")
	prev := rehomeAmendmentState
	rehomeAmendmentState = func(cfg *config.Context, slug string) (string, string) {
		return "close-the-feature", ""
	}
	t.Cleanup(func() { rehomeAmendmentState = prev })

	errs := checkRehomeTargets(parent, idx, retiring, rec, sweep)
	if len(errs) == 0 || !strings.Contains(errorsJoined(errs), "cannot own live code") {
		t.Errorf("a target with an applied retirement must refuse; got: %v", errs)
	}
}

func TestRehome_PendingRetirementTargetRefuses(t *testing.T) {
	parent, idx, retiring, rec, sweep := rehomeFixture(t, "@keeper", "")
	writeChildFeature(t, parent, "keeper")
	prev := rehomeAmendmentState
	rehomeAmendmentState = func(cfg *config.Context, slug string) (string, string) {
		return "", "close-the-feature"
	}
	t.Cleanup(func() { rehomeAmendmentState = prev })

	errs := checkRehomeTargets(parent, idx, retiring, rec, sweep)
	if len(errs) == 0 || !strings.Contains(errorsJoined(errs), "authored-and-waiting is not active") {
		t.Errorf("a target with an authored but unapplied retirement must refuse; got: %v", errs)
	}
}

func TestRehome_ActiveTargetNotYetClaimingRefusesNamingTheFile(t *testing.T) {
	parent, idx, retiring, rec, sweep := rehomeFixture(t, "@keeper",
		"// parlay-feature: mover\n")
	writeChildFeature(t, parent, "keeper")

	errs := checkRehomeTargets(parent, idx, retiring, rec, sweep)
	if len(errs) == 0 {
		t.Fatal("a target that does not yet claim the surviving work must refuse")
	}
	if !strings.Contains(errorsJoined(errs), "survivor.go") {
		t.Errorf("the refusal must name the file whose claim has not moved; got: %v", errs)
	}
}

func TestRehome_TargetSatisfyingAllThreeConditionsPermits(t *testing.T) {
	// The surviving file already carries the target's claim; the marker
	// naming the retiring feature is the extends line being released.
	parent, idx, retiring, rec, sweep := rehomeFixture(t, "@keeper",
		"// parlay-feature: keeper\n// parlay-extends: mover/helper\n")
	writeChildFeature(t, parent, "keeper")

	before := treeSnapshot(t, parent)
	errs := checkRehomeTargets(parent, idx, retiring, rec, sweep)
	if len(errs) != 0 {
		t.Fatalf("a target that exists, is active, and already claims the work must permit; got: %v", errs)
	}
	// The check runs before any mutation: it is read-only.
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
	// And the re-homed ownership findings do not block the sweep.
	for _, f := range sweep.BlockingFindings() {
		if f.Kind == sweepKindOwnershipMarker && f.Feature == "mover" {
			t.Errorf("a re-homed ownership finding must not block: %+v", f)
		}
	}
}

func TestRehome_TargetResolutionCrossesRootBoundaries(t *testing.T) {
	parent, _, retiring, rec, sweep := rehomeFixture(t, "@keeper", "")
	// The target lives in a DIFFERENT child root, not the parent.
	addRetirementChild(t, parent, "lib", "lib", "keeper")
	idx, err := config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}

	errs := checkRehomeTargets(parent, idx, retiring, rec, sweep)
	if len(errs) != 0 {
		t.Errorf("target resolution must cross root boundaries; got: %v", errs)
	}
}

// --- Suite 2 (continued): the record is read structurally closed -------
//
// The disposition record is the authorization for a deletion. These
// cases pin that a record which does not say exactly what it appears to
// say is refused rather than silently reinterpreted.

func TestDispositions_UnknownTopLevelKeyRefused(t *testing.T) {
	_, err := loadRecord(t, `dispositons:
  - feature: alpha
    term: delivered-and-deleted
    rationale: shipped and later removed
`)
	if err == nil {
		t.Fatal("a misspelled top-level key must be refused — silently ignoring it yields an empty record that authorizes a deletion nobody wrote down")
	}
	if !strings.Contains(err.Error(), "dispositons") {
		t.Errorf("the refusal must name the key it did not recognize; got: %v", err)
	}
}

func TestDispositions_UnknownEntryKeyRefused(t *testing.T) {
	_, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: delivered-and-deleted
    raitonale: shipped and later removed
`)
	if err == nil {
		t.Fatal("a misspelled entry key must be refused — dropping it turns a written rationale into a missing one")
	}
	if !strings.Contains(err.Error(), "raitonale") {
		t.Errorf("the refusal must name the key it did not recognize; got: %v", err)
	}
}

func TestDispositions_TargetOnATermThatNamesNoneRefused(t *testing.T) {
	for _, term := range []string{dispositionDeliveredAndDeleted, dispositionBuiltButUndelivered} {
		t.Run(term, func(t *testing.T) {
			_, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: `+term+`
    target: "@keeper"
    rationale: shipped and later removed
`)
			if err == nil {
				t.Fatalf("%s carrying a target must be refused — the term moves nothing while the target says authority moved, and honouring one of them picks for the operator", term)
			}
			if !strings.Contains(err.Error(), "@keeper") || !strings.Contains(err.Error(), term) {
				t.Errorf("the refusal must name the contradiction it found; got: %v", err)
			}
		})
	}
}

func TestDispositions_SecondDocumentRefused(t *testing.T) {
	_, err := loadRecord(t, "dispositions:\n"+
		"  - feature: alpha\n"+
		"    term: delivered-and-deleted\n"+
		"    rationale: shipped and later removed\n"+
		"---\n"+
		"dispositions:\n"+
		"  - feature: beta\n"+
		"    term: built-but-undelivered\n"+
		"    rationale: never shipped\n")
	if err == nil {
		t.Fatal("a record carrying a second YAML document must be refused — the second document's dispositions are never presented for checking")
	}
}

func TestDispositions_ClosedReadingStillAcceptsAWellFormedRecord(t *testing.T) {
	rec, err := loadRecord(t, `dispositions:
  - feature: alpha
    term: authority-re-homed-to
    target: "@keeper"
    rationale: keeper carries the helper now
`)
	if err != nil {
		t.Fatalf("closing the reading must not refuse a record that says exactly what it means: %v", err)
	}
	if len(rec.Dispositions) != 1 || rec.Dispositions[0].Target != "@keeper" {
		t.Errorf("the record must decode as written: %+v", rec.Dispositions)
	}
}
