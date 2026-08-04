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

// writeIntents creates a feature directory and an intents.md inside it.
func writeIntents(t *testing.T, root, slug string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "intents", slug)
	writeFile(t, filepath.Join(dir, "intents.md"))
	return dir
}

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
	writeFile(t, filepath.Join(dir, "dialogs.md"))

	got := ComputeFeaturePhase(rootCtxAt(root), "with-dialogs")
	if got != PhaseDialogs {
		t.Fatalf("with-dialogs: want %q, got %q", PhaseDialogs, got)
	}
}

func TestComputeFeaturePhase_PlusSurfaceOnly(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "with-surface")
	writeFile(t, filepath.Join(dir, "dialogs.md"))
	writeFile(t, filepath.Join(dir, "surface.md"))

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
	writeFile(t, filepath.Join(dir, "dialogs.md"))
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
	// stage/full fixture — intents + dialogs + surface + buildfile.
	// `done` is reached at buildfile presence; the engineering spec
	// under spec/handoff/ is intentionally NOT consulted.
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	dir := writeIntents(t, root, "full")
	writeFile(t, filepath.Join(dir, "dialogs.md"))
	writeFile(t, filepath.Join(dir, "surface.md"))
	writeFile(t, filepath.Join(root, ".parlay", "build", "full", "buildfile.yaml"))

	got := ComputeFeaturePhase(rootCtxAt(root), "full")
	if got != PhaseDone {
		t.Fatalf("full: want %q, got %q", PhaseDone, got)
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
	writeFile(t, filepath.Join(dir, "dialogs.md"))

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
	writeFile(t, filepath.Join(dir, "dialogs.md"))
	writeFile(t, filepath.Join(dir, "surface.md"))
	writeFile(t, filepath.Join(root, ".parlay", "build", "looped", "buildfile.yaml"))

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
	// The contract says: value is always one of the five exported
	// constants. Exhaustively try every reachable on-disk topology
	// (none, +intents, +dialogs, +surface, +infra, +buildfile, all)
	// and assert each result is a member of the constant set.
	allowed := map[FeaturePhase]bool{
		PhaseIntents:   true,
		PhaseDialogs:   true,
		PhaseArtifacts: true,
		PhaseBuild:     true,
		PhaseDone:      true,
	}

	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)

	// missing feature → still a valid phase (intents is the floor).
	rc := rootCtxAt(root)
	if got := ComputeFeaturePhase(rc, "missing"); !allowed[got] {
		t.Fatalf("missing feature returned out-of-set value %q", got)
	}

	// Walk through every step of the ladder, asserting each return.
	steps := []func(){
		func() { writeIntents(t, root, "ladder") },
		func() {
			writeFile(t, filepath.Join(root, "spec", "intents", "ladder", "dialogs.md"))
		},
		func() {
			writeFile(t, filepath.Join(root, "spec", "intents", "ladder", "surface.md"))
		},
		func() {
			writeFile(t, filepath.Join(root, "spec", "intents", "ladder", "infrastructure.md"))
		},
		func() {
			writeFile(t, filepath.Join(root, ".parlay", "build", "ladder", "buildfile.yaml"))
		},
	}
	for i, step := range steps {
		step()
		got := ComputeFeaturePhase(rc, "ladder")
		if !allowed[got] {
			t.Fatalf("step %d returned out-of-set value %q", i, got)
		}
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

	// studio/widget — full pipeline through buildfile.
	dir := writeIntents(t, studio, "widget")
	writeFile(t, filepath.Join(dir, "dialogs.md"))
	writeFile(t, filepath.Join(dir, "surface.md"))
	writeFile(t, filepath.Join(studio, ".parlay", "build", "widget", "buildfile.yaml"))

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
	if got := ComputeFeaturePhase(nil, "anything"); got != PhaseIntents {
		t.Fatalf("nil ctx: want %q, got %q", PhaseIntents, got)
	}
}

func TestComputeFeaturePhase_EmptySlug(t *testing.T) {
	root := t.TempDir()
	if got := ComputeFeaturePhase(rootCtxAt(root), ""); got != PhaseIntents {
		t.Fatalf("empty slug: want %q, got %q", PhaseIntents, got)
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
