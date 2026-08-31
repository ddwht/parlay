// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/complete-directory-archive-with-manifest
// parlay-extends: parlay-tool/root-retirement/cross-cutting/escaping-paths-unreadable-members-fail-closed
// parlay-artifact: test
//
// Suites: complete-directory-archive-with-manifest and
// escaping-paths-and-unreadable-members-fail-closed.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// archiveFixture builds a parent with a retiring child "old" whose tree
// carries configuration, adapters, and build state.
func archiveFixture(t *testing.T) (parent string, child config.Root) {
	t.Helper()
	resetRetirementState(t)
	parent = makeRetirementParent(t)
	child = addRetirementChild(t, parent, "old", "old", "alpha")
	for rel, body := range map[string]string{
		".parlay/adapters/go-cli.adapter.yaml": "name: go-cli\n",
		".parlay/build/alpha/buildfile.yaml":   "feature: alpha\n",
		".parlay/build/alpha/.baseline.yaml":   "schema_version: 2\nintents: {}\nsources: {}\n",
		"internal/alpha.go":                    "package alpha\n",
	} {
		path := filepath.Join(child.Path, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return parent, child
}

// runAuthorizedRetirement executes a full, confirmed retirement of "old".
func runAuthorizedRetirement(t *testing.T, parent string) error {
	t.Helper()
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	cmd, _ := retireCmd(t, parent, "y\n")
	return runRetireRoot(cmd, []string{"old"})
}

// --- Suite 5: complete-directory archive with manifest -----------------

func TestArchive_EveryFilePreservedByteIdentical(t *testing.T) {
	parent, child := archiveFixture(t)
	childSnap := treeSnapshot(t, child.Path)

	if err := runAuthorizedRetirement(t, parent); err != nil {
		t.Fatalf("retirement should complete: %v", err)
	}
	contents := filepath.Join(retirementDestination(parent, "old"), "contents")
	preserved := treeSnapshot(t, contents)
	for rel, want := range childSnap {
		if got, ok := preserved[rel]; !ok || got != want {
			t.Errorf("member %s must be present in the preserved copy with identical bytes (got %q, want %q)", rel, got, want)
		}
	}
}

func TestArchive_ManifestNamesEveryMemberAndCoversItself(t *testing.T) {
	parent, _ := archiveFixture(t)
	if err := runAuthorizedRetirement(t, parent); err != nil {
		t.Fatalf("retirement should complete: %v", err)
	}
	dest := retirementDestination(parent, "old")
	manifest, err := ReadManifest(filepath.Join(dest, "manifest.yaml"))
	if err != nil {
		t.Fatalf("the manifest must read back and verify: %v", err)
	}
	listed := map[string]bool{}
	for _, m := range manifest.Members {
		if m.SHA256 == "" {
			t.Errorf("member %s carries no content hash", m.Path)
		}
		listed[m.Path] = true
	}
	for _, rel := range []string{
		".parlay/adapters/go-cli.adapter.yaml",
		".parlay/build/alpha/.baseline.yaml",
		"internal/alpha.go",
		"spec/intents/alpha/intents.md",
	} {
		if !listed[rel] {
			t.Errorf("manifest must name preserved member %s", rel)
		}
	}
	if manifest.ManifestHash == "" {
		t.Fatal("the manifest must cover itself with a hash over its member list")
	}
	// Tampering with the member list breaks the self-covering hash.
	data, err := os.ReadFile(filepath.Join(dest, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "internal/alpha.go", "internal/omega.go", 1)
	tamperedPath := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(tamperedPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(tamperedPath); err == nil {
		t.Error("a manifest whose member list changed must fail its own hash")
	}
}

func TestArchive_FailureDuringArchivingLeavesProjectAsItWas(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)

	copies := 0
	retirementHook = func(event string) error {
		if event == "archive-copy" {
			copies++
			if copies == 2 {
				return fmt.Errorf("injected: interrupted mid-copy")
			}
		}
		return nil
	}
	before := treeSnapshot(t, parent)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("the interrupted run must fail")
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
	if _, err := os.Stat(retirementDestination(parent, "old")); !os.IsNotExist(err) {
		t.Error("no destination directory may be promoted after a mid-copy failure")
	}
}

func TestArchive_CompleteBeforeRegistrationChanges(t *testing.T) {
	parent, _ := archiveFixture(t)
	events := *(&[]string{})
	retirementHook = func(event string) error {
		events = append(events, event)
		return nil
	}
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("retirement should complete: %v", err)
	}
	promote, deregister := -1, -1
	for i, e := range events {
		if e == "promote" && promote == -1 {
			promote = i
		}
		if e == "deregister-index" {
			deregister = i
		}
	}
	if promote == -1 || deregister == -1 || promote > deregister {
		t.Errorf("archive and manifest writes must strictly precede the roots-index write; events: %v", events)
	}
}

// --- Suite 6: escaping paths and unreadable members fail closed --------

func TestArchiveWalk_EscapingSymlinkAbortsBeforeAnythingIsWritten(t *testing.T) {
	parent, child := archiveFixture(t)
	outside := filepath.Join(filepath.Dir(parent), "outside.txt")
	if err := os.WriteFile(outside, []byte("not the root's\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(child.Path, "notes")); err != nil {
		t.Fatal(err)
	}

	before := treeSnapshot(t, parent)
	err := runAuthorizedRetirement(t, parent)
	if err == nil {
		t.Fatal("a symlink resolving outside the child directory must abort the run")
	}
	if !strings.Contains(err.Error(), "notes") {
		t.Errorf("the refusal must name the member that caused it; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

func TestArchiveWalk_InternalSymlinkIsPreserved(t *testing.T) {
	parent, child := archiveFixture(t)
	if err := os.Symlink("internal/alpha.go", filepath.Join(child.Path, "alias.go")); err != nil {
		t.Fatal(err)
	}

	if err := runAuthorizedRetirement(t, parent); err != nil {
		t.Fatalf("an internal symlink is preserved like any other member: %v", err)
	}
	preserved := filepath.Join(retirementDestination(parent, "old"), "contents", "alias.go")
	target, err := os.Readlink(preserved)
	if err != nil {
		t.Fatalf("the internal link must be preserved: %v", err)
	}
	if target != "internal/alpha.go" {
		t.Errorf("the link's target must be preserved as written; got %q", target)
	}
}

func TestArchiveWalk_TraversalSegmentEscapeAborts(t *testing.T) {
	parent, child := archiveFixture(t)
	// A member whose resolved path lands outside through ../ segments.
	if err := os.Symlink("../../outside-by-traversal.txt", filepath.Join(child.Path, "sneaky")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(parent), "outside-by-traversal.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := treeSnapshot(t, parent)
	err := runAuthorizedRetirement(t, parent)
	if err == nil {
		t.Fatal("a traversal-segment escape must abort before anything is written")
	}
	if !strings.Contains(err.Error(), "sneaky") {
		t.Errorf("the refusal must name the member; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

func TestArchiveWalk_EscapeIsJudgedOnResolvedPathNotTheName(t *testing.T) {
	parent, child := archiveFixture(t)
	// Ordinary-looking name resolving outside: aborts.
	outside := filepath.Join(filepath.Dir(parent), "elsewhere.txt")
	if err := os.WriteFile(outside, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(child.Path, "readme.txt")); err != nil {
		t.Fatal(err)
	}
	_, _, err := validateArchiveWalk(child.Path)
	if err == nil || !strings.Contains(err.Error(), "readme.txt") {
		t.Fatalf("the ordinary-looking escape must abort naming the member; got: %v", err)
	}
	if err := os.Remove(filepath.Join(child.Path, "readme.txt")); err != nil {
		t.Fatal(err)
	}

	// Traversal-laden name resolving INSIDE: preserved.
	if err := os.Symlink("internal/../internal/alpha.go", filepath.Join(child.Path, "docs")); err != nil {
		t.Fatal(err)
	}
	members, _, err := validateArchiveWalk(child.Path)
	if err != nil {
		t.Fatalf("a traversal-laden name resolving inside must be preserved: %v", err)
	}
	found := false
	for _, m := range members {
		if m.RelPath == "docs" && m.IsSymlink {
			found = true
		}
	}
	if !found {
		t.Errorf("the inside-resolving member must be among the preserved members: %+v", members)
	}
}

func TestArchiveWalk_UnreadableMemberAbortsBeforeAnythingIsWritten(t *testing.T) {
	parent, child := archiveFixture(t)
	sealed := filepath.Join(child.Path, "internal", "sealed.go")
	if err := os.WriteFile(sealed, []byte("package alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sealed, 0o644) })

	before := treeSnapshot(t, parent)
	err := runAuthorizedRetirement(t, parent)
	if err == nil {
		t.Fatal("an unreadable member must abort the run")
	}
	if !strings.Contains(err.Error(), "sealed.go") {
		t.Errorf("the refusal must name the unreadable member; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

func TestArchiveWalk_NeitherConditionIsResolvedBySkipping(t *testing.T) {
	parent, child := archiveFixture(t)
	outside := filepath.Join(filepath.Dir(parent), "somewhere.txt")
	if err := os.WriteFile(outside, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(child.Path, "escaper")); err != nil {
		t.Fatal(err)
	}
	sealed := filepath.Join(child.Path, "internal", "sealed.go")
	if err := os.WriteFile(sealed, []byte("package alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sealed, 0o644) })

	if err := runAuthorizedRetirement(t, parent); err == nil {
		t.Fatal("the walk must abort, never skip and continue")
	}
	// No archive claiming completeness exists with either member absent.
	if _, err := os.Stat(retirementDestination(parent, "old")); !os.IsNotExist(err) {
		t.Error("no archive may exist after an aborted walk")
	}
	if entries, err := os.ReadDir(retiredRootsDir(parent)); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".staging-") {
				t.Errorf("no staged partial archive may remain: %s", e.Name())
			}
		}
	}
}

func TestArchiveWalk_AbortedRunLeavesProjectExactlyAsItWas(t *testing.T) {
	// Both abort kinds, each followed by a full tree comparison.
	for _, kind := range []string{"escape", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			parent, child := archiveFixture(t)
			if kind == "escape" {
				outside := filepath.Join(filepath.Dir(parent), "out.txt")
				if err := os.WriteFile(outside, []byte("x\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(child.Path, "leak")); err != nil {
					t.Fatal(err)
				}
			} else {
				sealed := filepath.Join(child.Path, "internal", "sealed.go")
				if err := os.WriteFile(sealed, []byte("package alpha\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(sealed, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(sealed, 0o644) })
			}

			before := treeSnapshot(t, parent)
			idxBefore, err := os.ReadFile(filepath.Join(parent, config.ParlayDir, config.RootsIndexFile))
			if err != nil {
				t.Fatal(err)
			}
			runErr := runAuthorizedRetirement(t, parent)
			if runErr == nil {
				t.Fatal("the run must abort")
			}
			// The refusal names the member that caused it.
			if kind == "escape" && !strings.Contains(runErr.Error(), "leak") {
				t.Errorf("the refusal must name the offending member's path; got: %v", runErr)
			}
			if kind == "unreadable" && !strings.Contains(runErr.Error(), "sealed.go") {
				t.Errorf("the refusal must name the offending member's path; got: %v", runErr)
			}
			// No archive, no record, no change to the root registration.
			assertTreeUnchanged(t, before, treeSnapshot(t, parent))
			idxAfter, err := os.ReadFile(filepath.Join(parent, config.ParlayDir, config.RootsIndexFile))
			if err != nil {
				t.Fatal(err)
			}
			if string(idxBefore) != string(idxAfter) {
				t.Error("the root registration must be unchanged")
			}
		})
	}
}

// --- Suite 5 (continued): the manifest describes the ARCHIVED bytes ---
//
// The member hashes are computed on the SOURCE, during the pre-copy walk
// — the only moment escape and readability can be judged before anything
// is written. That leaves a window between the walk and the copy. A
// manifest nobody checked against the staged copy is a claim about the
// source, not about the archive, and an archive that fails its own
// integrity check the moment it is written preserves nothing verifiable.
// So the staged bytes are re-hashed before the archive is promoted.

// stagingContentsPath is where a run's copy lives before promotion.
func stagingContentsPath(parent, name string) string {
	return filepath.Join(retiredRootsDir(parent), ".staging-"+name, "contents")
}

func TestArchive_StagedBytesDisagreeingWithTheManifestAbortTheRun(t *testing.T) {
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	before := treeSnapshot(t, parent)

	// The seam: corrupt a staged member after the copy and before the
	// verification, standing in for every way a copy can come out wrong.
	corrupted := filepath.Join(stagingContentsPath(parent, "old"), "internal", "alpha.go")
	retirementHook = func(event string) error {
		if event == "verify-archive" {
			return os.WriteFile(corrupted, []byte("package tampered\n"), 0o644)
		}
		return nil
	}

	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil {
		t.Fatal("archived bytes that do not hash to what the manifest records must abort the run")
	}
	if !strings.Contains(err.Error(), "internal/alpha.go") {
		t.Errorf("the refusal must name the member whose bytes disagreed; got: %v", err)
	}
	if !strings.Contains(err.Error(), "does not match what it claims to preserve") {
		t.Errorf("the refusal must say the archive does not match its own manifest; got: %v", err)
	}

	// Aborting before any mutation of the live tree is the point: the
	// archive is complete-or-absent, and this one is absent.
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
	if rootRegistered(t, parent, "old") == false {
		t.Error("a failed integrity check must leave the root registered")
	}
}

func TestArchive_SourceChangingBetweenWalkAndCopyIsCaught(t *testing.T) {
	// The concurrent-change case the hashes exist to detect. Hashing the
	// source and then counting members would let this through: the count
	// is right and the bytes are not.
	parent, child := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	before := treeSnapshot(t, parent)

	source := filepath.Join(child.Path, "internal", "alpha.go")
	retirementHook = func(event string) error {
		if event == "archive-copy" {
			// Rewrite a source member that the walk has already hashed
			// and the copy has not yet reached.
			return os.WriteFile(source, []byte("package changed_after_the_walk\n"), 0o644)
		}
		return nil
	}

	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil {
		t.Fatal("a source that changed between the walk and the copy must abort the run rather than yield an archive that fails its own integrity check")
	}
	if !strings.Contains(err.Error(), "internal/alpha.go") {
		t.Errorf("the refusal must name the member that changed; got: %v", err)
	}
	// The source file itself changed (the test changed it), so the
	// comparison is against the snapshot taken after that change.
	after := treeSnapshot(t, parent)
	before["old/internal/alpha.go"] = after["old/internal/alpha.go"]
	assertTreeUnchanged(t, before, after)
}

func TestArchive_VerificationHashesMembersRatherThanCountingThem(t *testing.T) {
	// A member list of the right length can still hold the wrong bytes.
	// Swapping two members' contents preserves every count and every
	// path, and changes only what the hashes are for.
	parent, _ := archiveFixture(t)
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)

	retirementHook = func(event string) error {
		if event != "verify-archive" {
			return nil
		}
		contents := stagingContentsPath(parent, "old")
		a := filepath.Join(contents, "internal", "alpha.go")
		b := filepath.Join(contents, ".parlay", "adapters", "go-cli.adapter.yaml")
		aData, err := os.ReadFile(a)
		if err != nil {
			return err
		}
		bData, err := os.ReadFile(b)
		if err != nil {
			return err
		}
		if err := os.WriteFile(a, bData, 0o644); err != nil {
			return err
		}
		return os.WriteFile(b, aData, 0o644)
	}

	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("an archive whose member count and paths are intact but whose bytes are swapped must still fail verification — counting members is not verifying them")
	}
}

func TestArchive_HonestArchiveStillCompletesAndVerifies(t *testing.T) {
	// The verification is a gate, not a wall: an archive that does match
	// its manifest promotes, and the preserved copy verifies afterwards
	// against the same hashes.
	parent, child := archiveFixture(t)
	childSnap := treeSnapshot(t, child.Path)
	if err := runAuthorizedRetirement(t, parent); err != nil {
		t.Fatalf("an archive matching its manifest must complete: %v", err)
	}
	dest := retirementDestination(parent, "old")
	manifest, err := ReadManifest(filepath.Join(dest, "manifest.yaml"))
	if err != nil {
		t.Fatalf("the manifest must read back and verify: %v", err)
	}
	contents := filepath.Join(dest, "contents")
	for _, m := range manifest.Members {
		sum, hashErr := archiveHashFile(filepath.Join(contents, filepath.FromSlash(m.Path)))
		if hashErr != nil {
			continue // symlink members are covered by the snapshot compare below
		}
		if sum != m.SHA256 {
			t.Errorf("preserved member %s must hash to what the manifest records", m.Path)
		}
	}
	preserved := treeSnapshot(t, contents)
	for rel, want := range childSnap {
		if got, ok := preserved[rel]; !ok || got != want {
			t.Errorf("member %s must be preserved byte-identically (got %q, want %q)", rel, got, want)
		}
	}
}
