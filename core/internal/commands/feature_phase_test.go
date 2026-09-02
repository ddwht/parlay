// parlay-feature: parlay-tool/status-feature-phases
// parlay-section: cross-cutting
// parlay-source: shared-feature-phase-helper
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// rootCtxAt builds a *config.Context with the given path as a standalone
// active root. Used by every test in this file to materialize a per-root
// context for ComputeFeaturePhase.
func rootCtxAt(path string) *config.Context {
	return config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{
			Name: filepath.Base(path),
			Path: path,
			Kind: config.RootKindStandalone,
		},
	}, nil)
}

// writeFile is a tiny helper that creates parent dirs and writes a file
// in one call — most fixtures here are touch-style.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeIntents creates a feature directory and an intents.md carrying one
// real, parseable intent. Content, not just presence: the ladder's bottom
// two rungs read these files, so a touch-style fixture now means
// PhasePlanned rather than PhaseIntents.
func writeIntents(t *testing.T, root, slug string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "intents", slug)
	writeContent(t, filepath.Join(dir, "intents.md"), oneIntent)
	return dir
}

// writeScaffoldIntents creates the feature directory with an intents.md
// that parses to ZERO intents — what `parlay add-feature` actually writes.
func writeScaffoldIntents(t *testing.T, root, slug string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "intents", slug)
	writeContent(t, filepath.Join(dir, "intents.md"), scaffoldIntents)
	return dir
}

// writeDialogs writes a dialogs.md carrying one real, parseable dialog.
func writeDialogs(t *testing.T, dir string) {
	t.Helper()
	writeContent(t, filepath.Join(dir, "dialogs.md"), oneDialog)
}

// writeStubDialogs writes the header-only dialogs.md that `add-feature`
// creates: present on disk, zero parsed dialogs.
func writeStubDialogs(t *testing.T, dir string) {
	t.Helper()
	writeContent(t, filepath.Join(dir, "dialogs.md"), stubDialogs)
}

func writeContent(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	oneIntent = `# Widget

---

## Add A Thing

**Goal**: Capture a thing so it can be reviewed later.
**Persona**: CLI User
`
	// Mirrors what add-feature writes: the example intent is commented
	// out precisely so the file parses as zero intents.
	scaffoldIntents = `# Widget

<!--
## Example Intent

**Goal**: something
**Persona**: someone
-->
`
	oneDialog = `# Widget — Dialogs

---

### Add A Thing

**Trigger**: The user runs the add command.

User: Runs ` + "`widget add \"x\"`" + `
System: Records it and confirms with the assigned id.
`
	stubDialogs = `# Widget — Dialogs

---

`
)

// ---------------------------------------------------------------------
// Phase ladder — one fixture per rung.
// ---------------------------------------------------------------------

func TestComputeFeaturePhase_IntentsOnly(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	writeIntents(t, root, "intents-only")

	got := ComputeFeaturePhase(rootCtxAt(root), "intents-only")
	if got != PhaseIntents {
		t.Fatalf("intents-only: want %q, got %q", PhaseIntents, got)
	}
}

func TestComputeFeaturePhase_PlusDialogs(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "with-dialogs")
	writeDialogs(t, dir)

	got := ComputeFeaturePhase(rootCtxAt(root), "with-dialogs")
	if got != PhaseDialogs {
		t.Fatalf("with-dialogs: want %q, got %q", PhaseDialogs, got)
	}
}

func TestComputeFeaturePhase_PlusSurfaceOnly(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "with-surface")
	writeDialogs(t, dir)
	writeFile(t, filepath.Join(dir, "surface.yaml"))

	got := ComputeFeaturePhase(rootCtxAt(root), "with-surface")
	if got != PhaseArtifacts {
		t.Fatalf("with-surface: want %q, got %q", PhaseArtifacts, got)
	}
}

func TestComputeFeaturePhase_PlusInfrastructureOnly(t *testing.T) {
	// Constraint pin from intents/dialogs: presence of EITHER artifact
	// is sufficient. infrastructure.md alone reaches PhaseArtifacts.
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "with-infra")
	writeDialogs(t, dir)
	writeFile(t, filepath.Join(dir, "infrastructure.md"))

	got := ComputeFeaturePhase(rootCtxAt(root), "with-infra")
	if got != PhaseArtifacts {
		t.Fatalf("with-infra: want %q, got %q", PhaseArtifacts, got)
	}
}

func TestComputeFeaturePhase_PlusBuildfileNoArtifacts(t *testing.T) {
	// stage/with-buildfile fixture — buildfile present, but artifacts
	// missing, so the phase ladder pins at PhaseBuild (not PhaseDone).
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	writeIntents(t, root, "with-buildfile")
	writeFile(t, filepath.Join(root, ".parlay", "build", "with-buildfile", "buildfile.yaml"))

	got := ComputeFeaturePhase(rootCtxAt(root), "with-buildfile")
	if got != PhaseBuild {
		t.Fatalf("with-buildfile: want %q, got %q", PhaseBuild, got)
	}
}

func TestComputeFeaturePhase_AllFourPreTerminal(t *testing.T) {
	// stage/full fixture — intents + dialogs + surface + both build files.
	// `done` is reached when the build phase is COMPLETE, which means
	// testcases.yaml as well as buildfile.yaml; the engineering spec under
	// spec/handoff/ is intentionally NOT consulted.
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "full")
	writeDialogs(t, dir)
	writeFile(t, filepath.Join(dir, "surface.yaml"))
	writeFile(t, filepath.Join(root, ".parlay", "build", "full", "buildfile.yaml"))
	writeFile(t, filepath.Join(root, ".parlay", "build", "full", "testcases.yaml"))

	got := ComputeFeaturePhase(rootCtxAt(root), "full")
	if got != PhaseDone {
		t.Fatalf("full: want %q, got %q", PhaseDone, got)
	}
}

// TestComputeFeaturePhase_BuildfileWithoutTestcasesIsNotDone is the
// regression test for a run that stopped mid-build and was reported as
// finished.
//
// A headless single-turn driver ends its turn to wait for a phase subagent
// that never reports back. What lands on disk is a valid buildfile.yaml, no
// testcases.yaml and no generated code — and the run exits 0. `parlay status`
// was the one thing positioned to contradict that exit code, and it said
// `done`, because this rung asked only for the buildfile. Two independent
// signals agreeing that a broken run succeeded is how the failure stayed
// invisible.
func TestComputeFeaturePhase_BuildfileWithoutTestcasesIsNotDone(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "halfbuilt")
	writeDialogs(t, dir)
	writeFile(t, filepath.Join(dir, "surface.md"))
	writeFile(t, filepath.Join(root, ".parlay", "build", "halfbuilt", "buildfile.yaml"))

	got := ComputeFeaturePhase(rootCtxAt(root), "halfbuilt")
	if got != PhaseBuild {
		t.Fatalf("buildfile without testcases: want %q, got %q", PhaseBuild, got)
	}
}

// ---------------------------------------------------------------------
// Purity invariants — no exits, no stdout writes, idempotent, total.
// ---------------------------------------------------------------------

func TestComputeFeaturePhase_Idempotent(t *testing.T) {
	// Same input twice → same enum value, no observable change.
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "idem")
	writeDialogs(t, dir)

	rc := rootCtxAt(root)
	first := ComputeFeaturePhase(rc, "idem")
	second := ComputeFeaturePhase(rc, "idem")
	if first != second {
		t.Fatalf("idempotency violated: first=%q second=%q", first, second)
	}
}

func TestComputeFeaturePhase_NoSideEffectsAcrossManyCalls(t *testing.T) {
	// Loop the helper many times. We can't observe os.Exit (it would
	// crash the test binary), but we can observe that stdout stays
	// empty and that the result is invariant — the two checks the
	// purity contract makes externally inspectable.
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "looped")
	writeDialogs(t, dir)
	writeFile(t, filepath.Join(dir, "surface.yaml"))
	writeFile(t, filepath.Join(root, ".parlay", "build", "looped", "buildfile.yaml"))
	writeFile(t, filepath.Join(root, ".parlay", "build", "looped", "testcases.yaml"))

	rc := rootCtxAt(root)
	out := captureStdout(t, func() {
		for i := 0; i < 1000; i++ {
			if got := ComputeFeaturePhase(rc, "looped"); got != PhaseDone {
				t.Fatalf("iteration %d: want %q, got %q", i, PhaseDone, got)
			}
		}
	})
	if out != "" {
		t.Fatalf("ComputeFeaturePhase wrote to stdout: %q", out)
	}
}

func TestComputeFeaturePhase_ReturnsOnlyExportedConstants(t *testing.T) {
	// The contract says: value is always one of the exported
	// constants. Exhaustively try every reachable on-disk topology
	// (none, +intents, +dialogs, +surface, +infra, +buildfile, all)
	// and assert each result is a member of the constant set.
	allowed := map[FeaturePhase]bool{
		PhasePlanned:   true,
		PhaseIntents:   true,
		PhaseDialogs:   true,
		PhaseArtifacts: true,
		PhaseBuild:     true,
		PhaseDone:      true,
	}

	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)

	// missing feature → still a valid phase (planned is the floor).
	rc := rootCtxAt(root)
	if got := ComputeFeaturePhase(rc, "missing"); !allowed[got] {
		t.Fatalf("missing feature returned out-of-set value %q", got)
	}

	// Walk through every step of the ladder, asserting each return.
	steps := []func(){
		func() { writeIntents(t, root, "ladder") },
		func() {
			writeDialogs(t, filepath.Join(root, "spec", "intents", "ladder"))
		},
		func() {
			writeFile(t, filepath.Join(root, "spec", "intents", "ladder", "surface.yaml"))
		},
		func() {
			writeFile(t, filepath.Join(root, "spec", "intents", "ladder", "infrastructure.md"))
		},
		func() {
			writeFile(t, filepath.Join(root, ".parlay", "build", "ladder", "buildfile.yaml"))
		},
		// The terminal rung needs testcases.yaml too. Without this step the
		// walk stops at PhaseBuild and the exhaustive check quietly stops
		// covering PhaseDone — still passing, since membership is all it
		// asserts, while testing one case fewer than it claims to.
		func() {
			writeFile(t, filepath.Join(root, ".parlay", "build", "ladder", "testcases.yaml"))
		},
	}
	var last FeaturePhase
	for i, step := range steps {
		step()
		got := ComputeFeaturePhase(rc, "ladder")
		if !allowed[got] {
			t.Fatalf("step %d returned out-of-set value %q", i, got)
		}
		last = got
	}
	if last != PhaseDone {
		t.Fatalf("the full ladder should end at %q, got %q — the walk no longer reaches the terminal rung", PhaseDone, last)
	}
}

// ---------------------------------------------------------------------
// Per-root invariant — no cross-contamination between sibling roots.
// ---------------------------------------------------------------------

func TestComputeFeaturePhase_PerRoot_SameNameTwoRoots(t *testing.T) {
	// `widget` exists in both core and studio with different on-disk
	// pipeline depths. The two contexts must never cross-contaminate.
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	core := filepath.Join(parent, "core")
	studio := filepath.Join(parent, "studio")
	if err := os.MkdirAll(core, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(studio, 0o755); err != nil {
		t.Fatal(err)
	}

	// core/widget — intents only.
	writeIntents(t, core, "widget")

	// studio/widget — full pipeline through a COMPLETE build.
	dir := writeIntents(t, studio, "widget")
	writeDialogs(t, dir)
	writeFile(t, filepath.Join(dir, "surface.yaml"))
	writeFile(t, filepath.Join(studio, ".parlay", "build", "widget", "buildfile.yaml"))
	writeFile(t, filepath.Join(studio, ".parlay", "build", "widget", "testcases.yaml"))

	if got := ComputeFeaturePhase(rootCtxAt(core), "widget"); got != PhaseIntents {
		t.Fatalf("core/widget: want %q, got %q", PhaseIntents, got)
	}
	if got := ComputeFeaturePhase(rootCtxAt(studio), "widget"); got != PhaseDone {
		t.Fatalf("studio/widget: want %q, got %q", PhaseDone, got)
	}
}

func TestComputeFeaturePhase_PerRoot_BuildStateDoesNotLeakAcrossRoots(t *testing.T) {
	// Critical per-root property. Two roots, both have a
	// .parlay/build/widget/buildfile.yaml, but only one of them has
	// the corresponding spec/intents/widget/intents.md. A naive
	// implementation that consults a project-wide build singleton
	// would mistakenly report PhaseBuild for the root WITHOUT intents.
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	withIntents := filepath.Join(parent, "with-intents")
	withoutIntents := filepath.Join(parent, "without-intents")
	if err := os.MkdirAll(withIntents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(withoutIntents, 0o755); err != nil {
		t.Fatal(err)
	}

	writeIntents(t, withIntents, "widget")
	writeFile(t, filepath.Join(withIntents, ".parlay", "build", "widget", "buildfile.yaml"))

	// withoutIntents has the buildfile but NO spec/intents/widget/.
	writeFile(t, filepath.Join(withoutIntents, ".parlay", "build", "widget", "buildfile.yaml"))

	// withIntents → buildfile is visible → PhaseBuild.
	if got := ComputeFeaturePhase(rootCtxAt(withIntents), "widget"); got != PhaseBuild {
		t.Fatalf("with-intents/widget: want %q, got %q", PhaseBuild, got)
	}
	// withoutIntents → its own buildfile is visible too, but no
	// dialogs/surface, so the phase still pins at build (not done).
	// The point of this test is that the OTHER root's buildfile does
	// not leak in — we verify that by testing that the result for
	// withoutIntents matches what its own on-disk topology dictates,
	// not what withIntents has.
	if got := ComputeFeaturePhase(rootCtxAt(withoutIntents), "widget"); got != PhaseBuild {
		t.Fatalf("without-intents/widget: want %q, got %q", PhaseBuild, got)
	}
}

// ---------------------------------------------------------------------
// Defensive / boundary inputs.
// ---------------------------------------------------------------------

func TestComputeFeaturePhase_NilContext(t *testing.T) {
	// Defensive: passing nil shouldn't panic — a missing rootCtx is a
	// programmer error but the helper should fail closed at the floor.
	if got := ComputeFeaturePhase(nil, "anything"); got != PhasePlanned {
		t.Fatalf("nil ctx: want %q, got %q", PhasePlanned, got)
	}
}

func TestComputeFeaturePhase_EmptySlug(t *testing.T) {
	root := t.TempDir()
	if got := ComputeFeaturePhase(rootCtxAt(root), ""); got != PhasePlanned {
		t.Fatalf("empty slug: want %q, got %q", PhasePlanned, got)
	}
}

// A unit is not on the ladder at all. It has intents.md and nothing else
// the phases measure, so walking the rungs pins it at "intents" forever
// and reports a permanent non-problem.
func TestComputeFeaturePhase_HandAuthoredUnitIsOffTheLadder(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "geometry-engine")
	if err := os.WriteFile(filepath.Join(dir, config.AuthoredFile),
		[]byte("schema_version: 1\nunit: geometry-engine\nsummary: s\nsources: [\"src/**\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := ComputeFeaturePhase(rootCtxAt(root), "geometry-engine"); got != PhaseHandAuthored {
		t.Errorf("want %q, got %q", PhaseHandAuthored, got)
	}
}

// And the declaration is what decides — the same directory without it is
// an ordinary feature sitting at the first rung.
func TestComputeFeaturePhase_WithoutDeclarationIsAnOrdinaryFeature(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	writeIntents(t, root, "geometry-engine")

	if got := ComputeFeaturePhase(rootCtxAt(root), "geometry-engine"); got != PhaseIntents {
		t.Errorf("want %q, got %q", PhaseIntents, got)
	}
}

// ---------------------------------------------------------------------
// The planned rung — the bug this ladder change exists to fix.
// ---------------------------------------------------------------------

// The regression guard that matters most: run the REAL add-feature and
// ask the ladder what it made. Before the content-aware rungs this
// answered "dialogs" — a folder with two empty founding files claiming
// authored dialogs — because add-feature writes dialogs.md eagerly and
// the ladder asked only whether the file existed. Asserting against the
// real command rather than a hand-built fixture is the point: the bug
// lived in the disagreement between the two.
func TestComputeFeaturePhase_FreshlyAddedFeatureIsPlanned(t *testing.T) {
	setupTestDir(t)
	if err := os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"fleet", "overview"}); err != nil {
		t.Fatal(err)
	}

	got := ComputeFeaturePhase(testContext(t), "fleet-overview")
	if got != PhasePlanned {
		t.Fatalf("a brand-new feature should be %q, got %q", PhasePlanned, got)
	}
}

// A scaffolded intents.md parses to zero intents because its example is
// commented out. That is what makes `planned` detectable at all.
func TestComputeFeaturePhase_ScaffoldIntentsAreZeroIntents(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeScaffoldIntents(t, root, "scaffolded")
	writeStubDialogs(t, dir)

	if got := ComputeFeaturePhase(rootCtxAt(root), "scaffolded"); got != PhasePlanned {
		t.Fatalf("scaffold: want %q, got %q", PhasePlanned, got)
	}
}

// Presence is not authorship. A real intents.md plus the header-only
// dialogs.md add-feature writes is an intents-phase feature, not a
// dialogs-phase one.
func TestComputeFeaturePhase_StubDialogsDoNotReachDialogsPhase(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "stubbed")
	writeStubDialogs(t, dir)

	if got := ComputeFeaturePhase(rootCtxAt(root), "stubbed"); got != PhaseIntents {
		t.Fatalf("stub dialogs: want %q, got %q", PhaseIntents, got)
	}
}

// An unreadable founding document must not demote the feature. Reporting
// a feature as emptier than it is, on the evidence of a file the tool
// could not read, is the one failure this fallback exists to prevent.
func TestComputeFeaturePhase_UnparseableIntentsDoNotDemote(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := filepath.Join(root, "spec", "intents", "broken")
	// A heading with no Goal/Persona still parses as an intent; to force
	// the fallback we need the read itself to fail, so use a directory
	// where the file should be.
	if err := os.MkdirAll(filepath.Join(dir, "intents.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := ComputeFeaturePhase(rootCtxAt(root), "broken"); got == PhasePlanned {
		t.Fatalf("unreadable intents.md demoted the feature to %q", got)
	}
}

// A backend feature legitimately has no dialog turns, and says so in
// dialogs.md. Once it has artifacts, a buildfile and testcases it is
// finished — reading dialog CONTENT at the terminal rung would demote it
// to `build` for having correctly declared that it has none. Drawn from
// a real feature in this repo (parlay-tool/structured-domain-model-validation),
// which the first cut of the content-aware ladder demoted.
func TestComputeFeaturePhase_BuiltBackendFeatureWithNoDialogTurnsIsDone(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "backend")
	writeContent(t, filepath.Join(dir, "dialogs.md"),
		"# Backend — Dialogs\n\n_CLI/backend feature — no interactive dialog turns._\n\n---\n")
	writeFile(t, filepath.Join(dir, "capabilities.yaml"))
	build := filepath.Join(root, ".parlay", "build", "backend")
	writeFile(t, filepath.Join(build, "buildfile.yaml"))
	writeFile(t, filepath.Join(build, "testcases.yaml"))

	if got := ComputeFeaturePhase(rootCtxAt(root), "backend"); got != PhaseDone {
		t.Fatalf("built backend feature with no dialog turns: want %q, got %q", PhaseDone, got)
	}
}

// The parse-failure fallback is claimed for BOTH founding documents, so
// both are pinned. An unreadable dialogs.md promotes an authored-intent
// feature to the dialogs rung — presence is the conservative answer, and
// half a rule with a guard is a rule that drifts.
func TestComputeFeaturePhase_UnparseableDialogsDoNotDemote(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "brokendialogs")
	if err := os.MkdirAll(filepath.Join(dir, "dialogs.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := ComputeFeaturePhase(rootCtxAt(root), "brokendialogs"); got != PhaseDialogs {
		t.Fatalf("unreadable dialogs.md: want %q (presence fallback), got %q", PhaseDialogs, got)
	}
}

// The terminal rung claims structural integrity of BOTH founding
// documents. A feature carrying dialogs, an artifact and both build
// outputs but no intents.md promises nothing, so there is nothing for
// those build outputs to be the completion of — it must not report done.
// Status enumeration hides this because features are discovered through
// intents.md, but an invariant that holds only via the caller's
// discovery order is not an invariant.
func TestComputeFeaturePhase_MissingIntentsIsNotDone(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := filepath.Join(root, "spec", "intents", "nointents")
	writeDialogs(t, dir)
	writeFile(t, filepath.Join(dir, "capabilities.yaml"))
	build := filepath.Join(root, ".parlay", "build", "nointents")
	writeFile(t, filepath.Join(build, "buildfile.yaml"))
	writeFile(t, filepath.Join(build, "testcases.yaml"))

	if got := ComputeFeaturePhase(rootCtxAt(root), "nointents"); got == PhaseDone {
		t.Fatalf("feature with no intents.md reported %q", got)
	}
}
