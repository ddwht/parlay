// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/archive-invisibility-and-integrity
// parlay-artifact: test
//
// Suite: archive-invisibility-to-discovery-and-archive-integrity.
// The invisibility cases are regression pins: preserved paths live under
// the .parlay dot-directory that every live-work walk skips, and these
// tests are what stops a future walker from silently starting to read
// archives.

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// retiredFixture completes a retirement of child "old" (feature alpha)
// and returns the parent path.
func retiredFixture(t *testing.T) string {
	t.Helper()
	parent, _ := archiveFixture(t)
	if err := runAuthorizedRetirement(t, parent); err != nil {
		t.Fatalf("retirement should complete: %v", err)
	}
	return parent
}

func runRetiredRootsCheck(t *testing.T, parent string) (string, error) {
	t.Helper()
	retiredRootsCheck = true
	idx, err := config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}
	root := config.Root{Name: filepath.Base(parent), Path: parent, Kind: config.RootKindParent}
	cmd := withCtx(t, root, idx)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	runErr := runRetiredRoots(cmd, nil)
	return buf.String(), runErr
}

func TestRetiredRoots_PreservedFeaturesAppearInNoLiveEnumeration(t *testing.T) {
	parent := retiredFixture(t)

	// The registration no longer names the root, so every root-anchored
	// enumeration starts elsewhere.
	idx, err := config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Lookup("old"); ok {
		t.Fatal("the retired root must not be registered")
	}

	// Discovery below the parent finds no root for the preserved copy —
	// it lives under the .parlay dot-directory discovery skips.
	for _, cand := range config.DiscoverRootsBelow(parent, 6) {
		if strings.Contains(cand.RelativePath, ".parlay") || cand.Name == "old" {
			t.Errorf("discovery must not surface the preserved copy: %+v", cand)
		}
	}

	// The parent's own feature enumeration sees none of the preserved
	// features.
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: filepath.Base(parent), Path: parent, Kind: config.RootKindParent},
	}, idx)
	features, err := cfg.AllFeatures()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range features {
		if f == "alpha" {
			t.Errorf("a preserved feature must not appear in any live enumeration; got %v", features)
		}
	}
}

func TestRetiredRoots_PreservedBuildStateAppearsInNoLiveBuildWalk(t *testing.T) {
	parent := retiredFixture(t)

	// The preserved build state exists — under the archive.
	preserved := filepath.Join(retirementDestination(parent, "old"), "contents", ".parlay", "build", "alpha", "buildfile.yaml")
	if _, err := os.Stat(preserved); err != nil {
		t.Fatalf("the build state must be preserved: %v", err)
	}

	// Live build-state walks are anchored on registered roots: the
	// parent's own build tree holds nothing of the retired root, and no
	// registered root's path reaches into the archive.
	idx, _ := config.LoadRootsIndex(parent)
	roots := []string{parent}
	for _, c := range idx.Children {
		roots = append(roots, c.Path)
	}
	for _, rootPath := range roots {
		buildRoot := filepath.Join(rootPath, config.ParlayDir, "build")
		if _, err := os.Stat(filepath.Join(buildRoot, "alpha")); err == nil {
			t.Errorf("preserved build state must not appear under a live root's build tree: %s", buildRoot)
		}
	}
}

func TestRetiredRoots_IntegrityCheckPassesImmediatelyAfterRetirement(t *testing.T) {
	parent := retiredFixture(t)
	out, err := runRetiredRootsCheck(t, parent)
	if err != nil {
		t.Fatalf("the integrity check must pass immediately after a completed retirement: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[OK]") {
		t.Errorf("the check should report the archive clean; got:\n%s", out)
	}
}

func TestRetiredRoots_IntegrityCheckReportsChangedMember(t *testing.T) {
	parent := retiredFixture(t)
	member := filepath.Join(retirementDestination(parent, "old"), "contents", "internal", "alpha.go")
	if err := os.WriteFile(member, []byte("package tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runRetiredRootsCheck(t, parent)
	if err == nil {
		t.Fatal("a changed member must fail the check")
	}
	if !strings.Contains(out, "changed:") || !strings.Contains(out, "internal/alpha.go") {
		t.Errorf("the changed member must be reported; got:\n%s", out)
	}
}

func TestRetiredRoots_IntegrityCheckReportsMissingMember(t *testing.T) {
	parent := retiredFixture(t)
	member := filepath.Join(retirementDestination(parent, "old"), "contents", "internal", "alpha.go")
	if err := os.Remove(member); err != nil {
		t.Fatal(err)
	}
	out, err := runRetiredRootsCheck(t, parent)
	if err == nil {
		t.Fatal("a missing member must fail the check")
	}
	if !strings.Contains(out, "missing:") || !strings.Contains(out, "internal/alpha.go") {
		t.Errorf("the missing member must be reported; got:\n%s", out)
	}
}

func TestRetiredRoots_IntegrityCheckReportsUnlistedMember(t *testing.T) {
	parent := retiredFixture(t)
	planted := filepath.Join(retirementDestination(parent, "old"), "contents", "internal", "planted.go")
	if err := os.WriteFile(planted, []byte("package planted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runRetiredRootsCheck(t, parent)
	if err == nil {
		t.Fatal("a member present in the archive but absent from the manifest must fail the check")
	}
	if !strings.Contains(out, "unlisted:") || !strings.Contains(out, "internal/planted.go") {
		t.Errorf("the unlisted member must be reported; got:\n%s", out)
	}
}

func TestRetiredRoots_PlaceholderBaselinePassesTheCheckUnchanged(t *testing.T) {
	// The archive fixture preserves a placeholder baseline naming no
	// intents and no sources — verification compares bytes against
	// recorded hashes and raises nothing on emptiness.
	parent := retiredFixture(t)
	preserved := filepath.Join(retirementDestination(parent, "old"), "contents", ".parlay", "build", "alpha", ".baseline.yaml")
	if _, err := os.Stat(preserved); err != nil {
		t.Fatalf("the placeholder baseline must be preserved: %v", err)
	}
	out, err := runRetiredRootsCheck(t, parent)
	if err != nil {
		t.Fatalf("a preserved placeholder baseline passes the check unchanged — history, not corruption: %v\n%s", err, out)
	}
}

func TestRetiredRoots_EmptinessNeverRaisesAnIntegrityFinding(t *testing.T) {
	// A root whose build state is mostly empty files verifies clean:
	// zero findings whose reason is emptiness or thinness.
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	child := addRetirementChild(t, parent, "old", "old", "alpha")
	for _, rel := range []string{
		".parlay/build/alpha/.baseline.yaml",
		".parlay/build/alpha/coverage-decisions.yaml",
	} {
		path := filepath.Join(child.Path, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := runAuthorizedRetirement(t, parent); err != nil {
		t.Fatalf("retirement should complete: %v", err)
	}
	report, err := verifyArchiveIntegrity(retirementDestination(parent, "old"))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Clean() {
		t.Errorf("empty preserved build state must verify clean; got %+v", report)
	}
}
