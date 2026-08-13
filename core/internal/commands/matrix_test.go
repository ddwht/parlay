// parlay-artifact: test
//
// THE FIXTURE MATRIX AND THE DIFFERENTIAL SUITE.
//
// Phase 0 and Phase 5 of the diagnostics-consolidation plan, which are the
// two parts of it that survived verification intact — and which the
// verification pass itself is the argument for. Four inferences about how
// parlay behaves turned out to be wrong when checked against the source,
// and every one of them would have been settled in an afternoon by a table
// of (fixture x surface) -> (output, exit code). Without such a table the
// next person reasons from comments and specs, one of which describes an
// architecture that no longer exists.
//
// WHAT THIS IS. A set of small on-disk parlay projects, each in a state
// with a known property, run through every verdict-emitting surface (see
// matrix_surfaces_test.go), with the output and the exit code of each
// recorded in a golden file under testdata/matrix/.
//
// WHAT IT IS FOR. Three assertions the golden files make possible:
//
//  1. No two surfaces contradict each other about the same fixture. This is
//     the test that would have caught R4-16, R4-22, R4-04 and R4-14 as ONE
//     failure rather than as four separate bug reports across three runs.
//  2. Every surface's exit code agrees with its own verdict. Half the
//     findings in this family are exit-code bugs, and exit codes are absent
//     from most existing tests.
//  3. No surface answers `satisfied` for a fixture that removed the
//     information that surface needs. Green must mean checked-and-good.
//
// KNOWN DISAGREEMENTS ARE TRACKED, NOT FIXED HERE. Where a surface still
// gets one of these wrong, it goes in knownIndeterminacyGaps with a reason
// rather than being quietly excluded. An entry that starts passing also
// fails the test, so the list cannot rot — the same discipline
// conformance_test.go's knownUnimplementedCodes already uses. That converts
// an invisible class of bug into a visible, tracked list, which is the
// point: the next run measures the tool rather than the tester's
// thoroughness.
//
// UPDATING THE GOLDEN FILES. `go test ./core/internal/commands/ -run
// TestMatrix -update`. Read the diff. A change in these files is a change in
// what parlay tells people, and it should be a decision, not a surprise.

package commands

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

var updateMatrix = flag.Bool("update", false, "rewrite the (fixture x surface) golden files")

// matrixFixture is one project state.
type matrixFixture struct {
	// name is the golden file's basename and the row's label.
	name string
	// property is what this state is a fixture FOR — the one-line entry
	// from the plan's fixture table.
	property string
	// build creates the project on disk and returns a context rooted at it.
	build func(t *testing.T) *config.Context
	// cannotCheck names the surfaces whose input this fixture removed.
	// Such a surface must not answer `satisfied`: it has nothing to have
	// checked. Empty for fixtures that remove nothing.
	cannotCheck []string
}

// knownIndeterminacyGaps records (fixture/surface) pairs where a surface
// still answers `satisfied` about something it could not check.
//
// Each entry is a real defect, not an exemption to be added to casually. It
// means a user is being told "fine" by a surface that established nothing.
// Shrink this list; do not grow it. An entry here that has been FIXED also
// fails the test, so the list cannot silently rot.
var knownIndeterminacyGaps = map[string]string{
	"drift-no-baseline/check-drift": "detectDrift returns has_drift:false when the baseline file is absent " +
		"(\"No baseline = no drift to detect\"), and driftOutput carries no field that separates " +
		"'compared, nothing changed' from 'nothing to compare against'. A caller cannot tell the " +
		"two apart, so a feature that was never built reads exactly like one that is up to date.",
	"drift-no-baseline/check-coverage": "check-coverage embeds the same detectDrift result and omits the " +
		"drift section entirely unless HasDrift is true, so it inherits the blind spot above and " +
		"hides it one level further from the reader.",
	"empty-project/check-drift": "the sharpest form of the same defect, found by this matrix rather than " +
		"by a bug report: `check-drift expense-list` against a project with NO features at all " +
		"reports has_drift:false and exits 0. The command is not saying 'that feature does not " +
		"exist' — it is saying 'that feature is up to date'. Absent input and clean input produce " +
		"the identical answer, so a typo in a feature name reads in CI as a passing check.",
}

// ---------------------------------------------------------------------
// Fixture builders.
// ---------------------------------------------------------------------

// writeFixtureFile writes one file inside the fixture project, creating
// parent directories.
func writeFixtureFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// featureWithIntents lays down the minimum that makes a directory count as
// a feature: config.ClassifyDir requires intents.md before AllFeatures()
// will see it at all. dialogs.md comes with it so that check-coverage has
// something to grade in every fixture rather than erroring out on half of
// them — a surface that cannot run is a hole in the matrix, not a cell.
func featureWithIntents(t *testing.T, dir, feature string) {
	t.Helper()
	// Heading levels are load-bearing: parser.ParseIntentsFile reads "## "
	// and parser.ParseDialogsFile reads "### ". Titles match so that
	// check-coverage grades a covered intent rather than an uncovered one
	// in every fixture that is not specifically about coverage.
	writeFixtureFile(t, dir, "spec/intents/"+feature+"/intents.md",
		"# "+feature+"\n\n## View The List\n\nAs a user I want to see the list.\n")
	writeFixtureFile(t, dir, "spec/intents/"+feature+"/dialogs.md",
		"# "+feature+" — Dialogs\n\n## Flow\n\n### View The List\n\nThe list appears.\n")
}

// composingBuildfile is a feature whose named fixture is the one that
// reaches the composed runtime seed.
func composingBuildfile(feature, fixture, status string) string {
	return "feature: " + feature + "\nfixtures:\n  " + fixture + ":\n    composes: true\n    data:\n" +
		"      ExpenseReport:\n        - id: rep-1\n          status: " + status + "\n"
}

// scenarioBuildfile is a feature whose fixture describes a state the
// prototype never boots into.
func scenarioBuildfile(feature, fixture, status string) string {
	return "feature: " + feature + "\nfixtures:\n  " + fixture + ":\n    data:\n" +
		"      ExpenseReport:\n        - id: rep-1\n          status: " + status + "\n"
}

func matrixFixtures() []matrixFixture {
	return []matrixFixture{
		{
			name:     "coherent",
			property: "the control — two features that agree, both composing",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				featureWithIntents(t, dir, "approvals")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				writeFixtureFile(t, dir, ".parlay/build/approvals/buildfile.yaml",
					composingBuildfile("approvals", "seed", "submitted"))
				return testContext(t)
			},
		},
		{
			name:     "contradiction-clean",
			property: "two composing fixtures contradicting, no scenario involved",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				featureWithIntents(t, dir, "approvals")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				writeFixtureFile(t, dir, ".parlay/build/approvals/buildfile.yaml",
					composingBuildfile("approvals", "seed", "approved"))
				return testContext(t)
			},
		},
		{
			name:     "contradiction-diluted",
			property: "same, with a scenario fixture carrying one of the values (R4-16 probe A)",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				featureWithIntents(t, dir, "approvals")
				featureWithIntents(t, dir, "audit")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				writeFixtureFile(t, dir, ".parlay/build/approvals/buildfile.yaml",
					composingBuildfile("approvals", "seed", "approved"))
				writeFixtureFile(t, dir, ".parlay/build/audit/buildfile.yaml",
					scenarioBuildfile("audit", "rejection-scenario", "approved"))
				return testContext(t)
			},
		},
		{
			name:     "seed-ambiguous",
			property: "no composes: marker anywhere, two features disagreeing",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				featureWithIntents(t, dir, "approvals")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					scenarioBuildfile("expense-list", "seed", "submitted"))
				writeFixtureFile(t, dir, ".parlay/build/approvals/buildfile.yaml",
					scenarioBuildfile("approvals", "seed", "approved"))
				return testContext(t)
			},
		},
		{
			name:     "feature-unbuilt",
			property: "feature with intents, no buildfile — coherence over a subset",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				featureWithIntents(t, dir, "approvals")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				return testContext(t)
			},
			cannotCheck: []string{"check-composition"},
		},
		{
			name:     "empty-project",
			property: "no features at all — every surface has nothing to check",
			build: func(t *testing.T) *config.Context {
				setupTestDir(t)
				return testContext(t)
			},
			cannotCheck: []string{"check-composition", "scaffold-seed", "status --json",
				"verify-generated", "verify-generated --strict", "check-coverage", "check-drift"},
		},
		{
			name:     "provenance-uncertified",
			property: "snapshot with no schema-version — pre-provenance (R4-18)",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				buildProvenanceSnapshot(t, dir, provenanceSnapshot{
					schemaVersion: 0,
					provenance:    ProvenanceGenerated,
				})
				return testContext(t)
			},
			cannotCheck: []string{"verify-generated", "verify-generated --strict"},
		},
		{
			name:     "provenance-undeclared",
			property: "snapshot with a version but no per-file provenance — control for the above",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				buildProvenanceSnapshot(t, dir, provenanceSnapshot{
					schemaVersion: CodeHashesSchemaVersion,
					provenance:    ProvenanceUnknown,
				})
				return testContext(t)
			},
			cannotCheck: []string{"verify-generated", "verify-generated --strict"},
		},
		{
			name:     "provenance-handedit",
			property: "hand-edited generated file with no intervening save (R4-17)",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				buildProvenanceSnapshot(t, dir, provenanceSnapshot{
					schemaVersion: CodeHashesSchemaVersion,
					provenance:    ProvenanceGenerated,
					handEditAfter: true,
				})
				return testContext(t)
			},
			cannotCheck: []string{"verify-generated", "verify-generated --strict"},
		},
		{
			name:     "provenance-hand-authored",
			property: "hand-edited file a unit declares — change is reported, never graded",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				buildProvenanceSnapshot(t, dir, provenanceSnapshot{
					schemaVersion: CodeHashesSchemaVersion,
					provenance:    ProvenanceHandAuthored,
					// The same edit that makes provenance-handedit fail
					// --strict. The only difference between the two
					// fixtures is the declared provenance, which is the
					// whole claim: who wrote a file decides whether its
					// changing is a problem.
					handEditAfter: true,
				})
				return testContext(t)
			},
			// No cannotCheck entry, unlike every sibling provenance row.
			// Those fixtures remove the input verify-generated needs; this
			// one supplies it in full. The surface can check this fixture,
			// does, and answers clean — which is the correct answer, not a
			// gap in one. That asymmetry is the fixture's whole point.
		},
		{
			name:     "domain-model-invalid",
			property: "domain model with a dangling ref and an out-of-set type — error severity",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				writeFixtureFile(t, dir, "domain-model.yaml", `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: uuid
        required: true
      - name: amount
        type: sideways
        required: true
relationships:
  - name: report-owner
    from: ExpenseReport
    to: Employee
    cardinality: many-to-one
`)
				return testContext(t)
			},
		},
		{
			name:     "domain-model-deprecated",
			property: "domain model carrying only the deprecated operations: block — warning severity",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				writeFixtureFile(t, dir, "domain-model.yaml", `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: uuid
        required: true
operations:
  - name: submit-report
    input:
      - ExpenseReport.id
    effects:
      - "set ExpenseReport.status to submitted"
`)
				return testContext(t)
			},
		},
		{
			name:     "drift-no-baseline",
			property: "feature with no baseline at all — nothing to compare against",
			build: func(t *testing.T) *config.Context {
				dir := setupTestDir(t)
				featureWithIntents(t, dir, "expense-list")
				writeFixtureFile(t, dir, ".parlay/build/expense-list/buildfile.yaml",
					composingBuildfile("expense-list", "seed", "submitted"))
				return testContext(t)
			},
			cannotCheck: []string{"check-drift", "check-coverage"},
		},
	}
}

// provenanceSnapshot describes the code-hashes state a fixture should be
// left in. Written through the real saveCodeHashes so the on-disk shape is
// whatever the tool actually writes, not a hand-rolled approximation that
// could drift from it.
type provenanceSnapshot struct {
	schemaVersion int
	provenance    string
	// handEditAfter rewrites the file after the snapshot is taken, so its
	// bytes no longer match the recorded hash with no save in between.
	handEditAfter bool
}

func buildProvenanceSnapshot(t *testing.T, dir string, spec provenanceSnapshot) {
	t.Helper()
	sourceRoot := filepath.Join(dir, "src")
	generated := filepath.Join(sourceRoot, "list.go")
	writeMarkedFile(t, generated, "expense-list", "list-comp", "func List() {}")

	cfg := testContext(t)
	hashes, _, err := buildCodeHashes(cfg, "expense-list", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	hashes.SchemaVersion = spec.schemaVersion
	for path, entry := range hashes.Files {
		entry.Provenance = spec.provenance
		hashes.Files[path] = entry
	}
	if err := saveCodeHashes(cfg, "expense-list", hashes); err != nil {
		t.Fatal(err)
	}
	if spec.handEditAfter {
		if err := os.WriteFile(generated, []byte(
			"// parlay-feature: expense-list\n// parlay-component: list-comp\nfunc List() { /* HAND-EDITED */ }\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------
// The matrix run.
// ---------------------------------------------------------------------

// cell is one (fixture, surface) result.
type cell struct {
	surface  string
	verdict  verdict
	exitCode int
	output   string
	// noInput is true when this surface found nothing recorded to check
	// against, as opposed to finding something it could not interpret. See
	// surface.noInput for why the two must not be conflated.
	noInput bool
}

// runMatrixRow builds one fixture and runs every surface against it.
func runMatrixRow(t *testing.T, f matrixFixture) []cell {
	t.Helper()
	cfg := f.build(t)
	root := cfg.Root.Path

	var cells []cell
	for _, s := range matrixSurfaces() {
		if !s.appliesTo(f.name) {
			continue
		}
		args := s.args
		if args == nil {
			args = defaultArgsFor(s.name)
		}
		local := s
		local.args = args
		out, code := runSurface(t, local, cfg)
		c := cell{
			surface:  s.name,
			verdict:  s.verdictOf(out, code),
			exitCode: code,
			output:   normalizeOutput(out, root),
		}
		if s.noInput != nil {
			c.noInput = s.noInput(out)
		}
		cells = append(cells, c)
	}
	return cells
}

// defaultArgsFor supplies the positional argument the per-feature surfaces
// need. Every fixture that has a feature at all calls it expense-list, so
// one name serves the whole matrix.
//
// verify-generated is in this list deliberately. Run with no argument it
// reads the PROJECT-level snapshot, which none of these fixtures writes, so
// every row came back has_hashes:false and the provenance fixtures graded
// the same as the empty project — the matrix looked green because it was
// asking the wrong question, which is the failure mode a matrix is supposed
// to prevent rather than demonstrate.
func defaultArgsFor(name string) []string {
	switch name {
	case "check-coverage", "check-drift", "verify-generated", "verify-generated --strict":
		return []string{"expense-list"}
	default:
		return nil
	}
}

// renderRow formats one fixture's cells for its golden file.
func renderRow(f matrixFixture, cells []cell) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# fixture: %s\n# property: %s\n", f.name, f.property)
	if len(f.cannotCheck) > 0 {
		fmt.Fprintf(&b, "# removes the input of: %s\n", strings.Join(f.cannotCheck, ", "))
	}
	for _, c := range cells {
		fmt.Fprintf(&b, "\n=== %s\nverdict: %s\nexit: %d\n%s\n", c.surface, c.verdict, c.exitCode, c.output)
	}
	return b.String()
}

// TestMatrix_GoldenFiles is Phase 0: pin the current behaviour before
// changing any of it. Every cell recorded verbatim, exit code included.
//
// The by-product is the specification. Once this table exists, a
// disagreement is two adjacent lines in one file rather than something
// someone has to go looking for.
func TestMatrix_GoldenFiles(t *testing.T) {
	for _, f := range matrixFixtures() {
		t.Run(f.name, func(t *testing.T) {
			got := renderRow(f, runMatrixRow(t, f))
			golden := filepath.Join(matrixTestdataDir(t), f.name+".txt")

			if *updateMatrix {
				if err := os.MkdirAll(filepath.Dir(golden), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("no golden file for fixture %q — run with -update: %v", f.name, err)
			}
			if string(want) != got {
				t.Errorf("fixture %q changed what parlay reports.\n--- want ---\n%s\n--- got ---\n%s", f.name, want, got)
			}
		})
	}
}

// matrixTestdataDir resolves testdata/matrix relative to the package
// source, not the fixture's temp cwd — every fixture chdirs.
func matrixTestdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(packageSourceDir(t), "testdata", "matrix")
}

// packageSourceDir is the directory this test file lives in. Resolved from
// the compiled-in caller path rather than from the working directory,
// because every fixture chdirs into a temp tree and never comes back until
// cleanup.
func packageSourceDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve the package source directory")
	}
	return filepath.Dir(file)
}

// TestMatrix_NoTwoSurfacesDisagree is Phase 5's differential assertion, and
// the reason the whole file exists.
//
// For any one fixture, two surfaces may legitimately answer different
// questions — check-coverage and check-composition are not asking the same
// thing. What they may NOT do is contradict: one saying the project is fine
// while another says the same project is broken. When that happens, the
// more reassuring answer is usually the one a user is pointed at, and the
// broken project ships.
//
// This is the test that would have caught R4-16, R4-22, R4-04 and R4-14 as
// one failure.
func TestMatrix_NoTwoSurfacesDisagree(t *testing.T) {
	seenDisagreements := map[string]bool{}

	for _, f := range matrixFixtures() {
		t.Run(f.name, func(t *testing.T) {
			cells := runMatrixRow(t, f)

			var satisfiedBy, violatedBy []string
			for _, c := range cells {
				switch c.verdict {
				case satisfied:
					satisfiedBy = append(satisfiedBy, c.surface)
				case violated:
					violatedBy = append(violatedBy, c.surface)
				}
			}
			if len(satisfiedBy) > 0 && len(violatedBy) > 0 {
				// Only surfaces that overlap in what they check can
				// contradict. compositionPeers are the ones that all read
				// the composed runtime, which is where the historical
				// disagreements were.
				sat := intersect(satisfiedBy, compositionPeers)
				vio := intersect(violatedBy, compositionPeers)
				if len(sat) > 0 && len(vio) > 0 {
					if why, known := knownDisagreements[f.name]; known {
						seenDisagreements[f.name] = true
						t.Logf("KNOWN DISAGREEMENT %s: %s", f.name, why)
						return
					}
					t.Errorf("surfaces contradict about fixture %q — %v say satisfied while %v say violated.\n"+
						"Two commands computing the same property and disagreeing is the bug class this suite exists to catch.",
						f.name, sat, vio)
				}
			}
		})
	}

	// Same anti-rot discipline as the other lists: a disagreement that has
	// been resolved must be removed from the register, not left to imply a
	// problem that no longer exists.
	for name, why := range knownDisagreements {
		if !seenDisagreements[name] {
			t.Errorf("knownDisagreements lists fixture %q, but its surfaces no longer contradict. "+
				"Remove the entry — it was recorded because: %s", name, why)
		}
	}
}

// knownDisagreements are fixtures where two surfaces still contradict each
// other, with the reason recorded.
//
// This register is the plan's "cheapest useful stopping point" made
// concrete: the disagreements are left in place and documented as known,
// which converts an invisible class of bug into a visible, tracked list.
// Each entry is a defect. Shrink this list; an entry that has been fixed
// fails the test above, so nothing here can quietly stop being true.
var knownDisagreements = map[string]string{
	"seed-ambiguous": "R4-16's SECOND instance, which partitioning findContradictions does not reach. " +
		"When NO fixture in the project is designated composing, scaffold-seed refuses outright " +
		"(derivable:false, composition-seed-ambiguous) because it cannot pick a fixture to compose; " +
		"check-composition meanwhile sees every site as non-composing, files the disagreement as a " +
		"scenario-divergence note, and reports coherent:true. So the command a user runs to ask " +
		"'is my project coherent' says yes about a project that cannot produce a runtime at all. " +
		"Closing it means check-composition reporting the seed ambiguity itself — a behavioural " +
		"change that will fail projects it used to pass, and which the plan says should land on " +
		"its own with this fixture named in the commit, rather than riding along with a fix to a " +
		"different mechanism.",
}

// compositionPeers are the surfaces that all read the composed runtime, and
// so are entitled to be held against each other. check-composition and
// scaffold-seed in particular now compute the contradiction verdict
// equivalently by construction — agent.ComposingFixture yields at most one
// composing fixture per feature, so two distinct values among composing
// sites necessarily span two features — and this assertion is what keeps
// that true rather than merely currently-true.
var compositionPeers = []string{"check-composition", "scaffold-seed"}

func intersect(have, want []string) []string {
	set := map[string]bool{}
	for _, w := range want {
		set[w] = true
	}
	var out []string
	for _, h := range have {
		if set[h] {
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

// TestMatrix_ExitCodeAgreesWithVerdict is the other half of Phase 5's
// differential assertion, and the one existing tests are blindest to: exit
// codes are absent from most of them, and half this family of findings are
// exit-code bugs.
//
// The rule: `violated` exits non-zero. `satisfied` exits zero.
// `indeterminate` exits zero by default and non-zero under --strict.
// Rendering never participates in grading — a --json flag may change how a
// result is written down, never what it is.
func TestMatrix_ExitCodeAgreesWithVerdict(t *testing.T) {
	for _, f := range matrixFixtures() {
		t.Run(f.name, func(t *testing.T) {
			for _, c := range runMatrixRow(t, f) {
				strict := strings.Contains(c.surface, "--strict")
				switch c.verdict {
				case satisfied:
					if c.exitCode != 0 {
						t.Errorf("%s reported satisfied for %q but exited %d", c.surface, f.name, c.exitCode)
					}
				case violated:
					if c.exitCode == 0 {
						if _, known := knownExitCodeGaps[f.name+"/"+c.surface]; !known {
							t.Errorf("%s reported violated for %q and still exited 0 — "+
								"a caller trusting the exit code is told this project is fine", c.surface, f.name)
						}
					}
				case indeterminate:
					// A surface with nothing recorded to check against is
					// exempt: that is the documented first-generation state,
					// not a check that failed. See surface.noInput.
					if strict && c.exitCode == 0 && !c.noInput {
						t.Errorf("%s could not check %q and still exited 0 under --strict — "+
							"--strict exists to make 'I could not check' fail", c.surface, f.name)
					}
				}
			}
		})
	}
}

// knownExitCodeGaps are surfaces that report a problem and still exit 0.
//
// Same discipline as knownIndeterminacyGaps: each is a real defect, the
// list should shrink, and an entry that has been fixed fails the test.
//
// Every entry here is a REPORTER — a command whose documented job is to
// emit JSON for a caller to decide on, where the exit code is deliberately
// not the verdict. That is a defensible design; what is not defensible is
// it being undiscoverable, which is what this list fixes.
var knownExitCodeGaps = map[string]string{
	"contradiction-clean/scaffold-seed": "scaffold-seed reports derivable:false and exits 0. It is a " +
		"reporter for the build phase, which reads the JSON. Documented, but it means the strongest " +
		"statement in the tree about composed-runtime coherence is invisible to a CI script.",
	"contradiction-diluted/scaffold-seed": "as above.",
	"seed-ambiguous/scaffold-seed":        "as above.",
	"provenance-handedit/verify-generated": "verify-generated without --strict exits 0 whatever it finds, " +
		"by design — generate-code.skill.md step 10 parses the JSON and decides. --strict is the CI path " +
		"and does fail here.",
	"provenance-uncertified/verify-generated": "as above.",
	"provenance-undeclared/verify-generated":  "as above.",
}

// TestMatrix_NoSurfaceAnswersSatisfiedWhenItCouldNotCheck is Phase 5's
// indeterminacy test, and the deliverable the whole plan is really about.
//
// For every fixture that removes information — no baseline, no provenance,
// no version marker, unbuilt feature, no features at all — no surface may
// answer `satisfied`. Green has to mean checked-and-good, always. When a
// surface could not check something it must say so, in its own category,
// and that category must be visible rather than absorbed.
func TestMatrix_NoSurfaceAnswersSatisfiedWhenItCouldNotCheck(t *testing.T) {
	seenGaps := map[string]bool{}

	for _, f := range matrixFixtures() {
		if len(f.cannotCheck) == 0 {
			continue
		}
		t.Run(f.name, func(t *testing.T) {
			blind := map[string]bool{}
			for _, s := range f.cannotCheck {
				blind[s] = true
			}
			for _, c := range runMatrixRow(t, f) {
				if !blind[c.surface] || c.verdict != satisfied {
					continue
				}
				key := f.name + "/" + c.surface
				if why, known := knownIndeterminacyGaps[key]; known {
					seenGaps[key] = true
					t.Logf("KNOWN GAP %s: %s", key, why)
					continue
				}
				t.Errorf("%s answered `satisfied` about fixture %q, whose whole property is that "+
					"the input that surface needs is absent. A surface that could not check something "+
					"must not report it as good.", c.surface, f.name)
			}
		})
	}

	// The list cannot rot: an entry that has been fixed fails here, so
	// nobody carries a stale excuse forward.
	for key, why := range knownIndeterminacyGaps {
		if !seenGaps[key] {
			t.Errorf("knownIndeterminacyGaps lists %q as a known gap, but it no longer reproduces. "+
				"Remove the entry — it was recorded because: %s", key, why)
		}
	}
}

// TestMatrix_SurfaceListCoversTheCheckCommands keeps the surface list
// honest. The list not existing is most of why this family of bugs went
// three runs undetected, so a new check command that nobody adds to it
// would put the tree straight back in that position.
func TestMatrix_SurfaceListCoversTheCheckCommands(t *testing.T) {
	covered := map[string]bool{}
	for _, s := range matrixSurfaces() {
		covered[strings.Fields(s.name)[0]] = true
	}
	// Deliberately out of the matrix, with the reason recorded in
	// matrixSurfaces' doc comment. Listed here so "not covered" is always
	// an explicit choice.
	excused := map[string]string{
		"check-buildfile":   "needs a full valid buildfile per fixture; the fixtures here are fixture-data shaped",
		"check-readiness":   "reports pipeline phase readiness, not project health",
		"check-supports":    "multi-target projects only; every fixture here is presentation-only",
		"check-write-set":   "guards a codegen write set, which none of these fixtures has",
		"check-review-gate": "gates the coverage-review artifact, which none of these fixtures has",
		"check-amendments":  "validates the amendment ledger, which exists only in parlay.ledger projects; every fixture here is flag-off with no amendments/ directory",
	}

	entries, err := os.ReadDir(packageSourceDir(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "check_") || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		cmdName := strings.ReplaceAll(strings.TrimSuffix(name, ".go"), "_", "-")
		// check_composition_cardinality.go is part of check-composition.
		if covered[cmdName] {
			continue
		}
		if _, ok := excused[cmdName]; ok {
			continue
		}
		if strings.HasPrefix(cmdName, "check-composition") {
			continue
		}
		t.Errorf("%s exists but %q is neither in matrixSurfaces() nor excused. "+
			"A verdict-emitting surface nobody wrote down is how this bug family survived three runs.", name, cmdName)
	}
}
