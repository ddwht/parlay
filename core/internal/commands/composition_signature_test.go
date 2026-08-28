package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func specDirOf(root string) string { return filepath.Join(root, config.SpecDir) }

func sigOrFail(t *testing.T, root, feature string) string {
	t.Helper()
	s, err := compositionSignature(specDirOf(root), feature)
	if err != nil {
		t.Fatalf("compositionSignature(%s): %v", feature, err)
	}
	return s
}

// The lifecycle case the whole signature exists for.
//
// Feature A ships. Feature B is then added declaring `supersedes: @a/panel`,
// and NOTHING inside A's directory is touched. Every feature-local signature
// field — intents, dialogs, surface, capabilities, infrastructure, layout,
// authored, domain — is byte-identical, so before this term existed A's
// buildfile read fresh and A's component kept being emitted and routed while
// the composed page no longer contained it.
func TestCompositionSignature_SiblingSupersessionMarksOwnerStale(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")

	before := sigOrFail(t, root, "alpha")

	// B arrives. A is not edited.
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Panel V2\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 1\n    supersedes: \"@alpha/panel\"\n")

	after := sigOrFail(t, root, "alpha")

	if before == after {
		t.Fatal("alpha's composition signature did not move when beta retired its fragment — the stale-output hole is back")
	}

	// And the reverse: removing the supersession must make alpha active again
	// and restore the original value, or a repaired tree would stay
	// permanently stale.
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Panel V2\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: sidebar\n    order: 1\n")
	restored := sigOrFail(t, root, "alpha")
	if restored == after {
		t.Fatal("removing the supersedes: edge left the signature unchanged")
	}
}

// A sibling appearing on a shared page changes what this feature's output is
// assembled beside, so it must move the value even with no supersession.
func TestCompositionSignature_SiblingOnSharedPageMovesValue(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	before := sigOrFail(t, root, "alpha")

	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Other\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: sidebar\n    order: 1\n")
	if after := sigOrFail(t, root, "alpha"); after == before {
		t.Fatal("a sibling joining the same page left alpha's composition signature unchanged")
	}
}

// A feature on an unrelated page must NOT move the value. A signature that
// moved on every unrelated edit would mark everything stale forever, and
// staleness nobody can act on is staleness everybody ignores.
func TestCompositionSignature_UnrelatedPageDoesNotMoveValue(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	before := sigOrFail(t, root, "alpha")

	writeFeatureSurface(t, root, "gamma",
		"fragments:\n  - name: Far Away\n    shows: read-collection\n    source: \"@gamma/intent\"\n    page: settings\n    region: main\n    order: 1\n")
	if after := sigOrFail(t, root, "alpha"); after != before {
		t.Fatal("an unrelated page moved alpha's composition signature")
	}
}

// Locking a page manifest re-scopes or reorders the composed view without any
// surface changing, so it is part of the composed answer.
func TestCompositionSignature_ManifestMovesValue(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	before := sigOrFail(t, root, "alpha")

	pagesDir := filepath.Join(specDirOf(root), "pages")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pagesDir, "dashboard.page.md"),
		[]byte("---\nname: dashboard\n---\n\n# dashboard\n\n## main\n\n1. @alpha/panel\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if after := sigOrFail(t, root, "alpha"); after == before {
		t.Fatal("locking a page manifest left the composition signature unchanged")
	}
}

// The value feeds a staleness gate, so it must be stable across runs on an
// unchanged tree — otherwise every build is stale and the gate is noise.
func TestCompositionSignature_StableAcrossRuns(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Other\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: sidebar\n    order: 2\n")

	first := sigOrFail(t, root, "alpha")
	for i := 0; i < 5; i++ {
		if again := sigOrFail(t, root, "alpha"); again != first {
			t.Fatalf("signature flapped between runs: %s vs %s", first, again)
		}
	}
}

// A backend-only feature contributes no fragments. It must get a stable value
// rather than an error, or a feature with no pages fails the gate forever.
func TestCompositionSignature_NoPagesIsStable(t *testing.T) {
	root := t.TempDir()
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")

	a := sigOrFail(t, root, "backend-only")
	b := sigOrFail(t, root, "backend-only")
	if a != b || a == "" {
		t.Fatalf("no-page feature signature unstable or empty: %q vs %q", a, b)
	}
}
