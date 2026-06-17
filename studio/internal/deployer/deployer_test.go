// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-artifact: test
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

package deployer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkProject creates a project root containing a .parlay/ marker plus
// every requested agent marker (relative paths under the root).
func mkProject(t *testing.T, agents ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".parlay"), 0o755); err != nil {
		t.Fatalf("mkdir .parlay: %v", err)
	}
	for _, a := range agents {
		if err := os.MkdirAll(filepath.Join(root, a), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", a, err)
		}
	}
	return root
}

func fixtureSkillReader(t *testing.T) func(string) ([]byte, error) {
	t.Helper()
	contents := map[string][]byte{
		"parlay-design-loop": []byte("---\nname: parlay-design-loop\ndescription: x\n---\nbody\n"),
	}
	return func(slug string) ([]byte, error) {
		c, ok := contents[slug]
		if !ok {
			t.Fatalf("fixtureSkillReader: unexpected slug %q", slug)
		}
		return c, nil
	}
}

func fixtureSkillLister() ([]string, error) {
	return []string{"parlay-design-loop"}, nil
}

func newDeployer(t *testing.T, root string) (*Deployer, *bytes.Buffer) {
	t.Helper()
	agents, err := DetectAgentSurfaces(root)
	if err != nil {
		t.Fatalf("DetectAgentSurfaces: %v", err)
	}
	var logBuf bytes.Buffer
	return &Deployer{
		ProjectRoot: root,
		Agents:      agents,
		SkillReader: fixtureSkillReader(t),
		SkillLister: fixtureSkillLister,
		Logger:      log.New(&logBuf, "", 0),
	}, &logBuf
}

func TestRunFreshFixtureAllWritten(t *testing.T) {
	root := mkProject(t, ".claude")
	d, _ := newDeployer(t, root)
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", res.ExitCode)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("expected 1 entry; got %d (%v)", len(res.Entries), res.Entries)
	}
	if res.Entries[0].Status != StatusWritten {
		t.Fatalf("expected StatusWritten; got %v", res.Entries[0].Status)
	}
	target := filepath.Join(root, ".claude", "skills", "parlay-design-loop", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected file %s to exist; got %v", target, err)
	}
}

func TestRunIdempotentSecondRunAllUnchanged(t *testing.T) {
	root := mkProject(t, ".claude")
	d, _ := newDeployer(t, root)
	if _, err := d.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", res.ExitCode)
	}
	for _, e := range res.Entries {
		if e.Status == StatusOrphan {
			continue
		}
		if e.Status != StatusUnchanged {
			t.Fatalf("expected StatusUnchanged for %s; got %v", e.Path, e.Status)
		}
	}
	// No .tmp files left behind.
	target := filepath.Join(root, ".claude", "skills", "parlay-design-loop", "SKILL.md")
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp file unexpectedly present after idempotent second run: %v", err)
	}
}

func TestRunIgnoresNonParlayUserSkill(t *testing.T) {
	root := mkProject(t, ".claude")
	userSkill := filepath.Join(root, ".claude", "skills", "my-custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userSkill), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(userSkill, []byte("USER OWNED"), 0o644); err != nil {
		t.Fatalf("seed user skill: %v", err)
	}
	d, _ := newDeployer(t, root)
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// User skill is preserved untouched.
	got, err := os.ReadFile(userSkill)
	if err != nil {
		t.Fatalf("ReadFile user skill: %v", err)
	}
	if string(got) != "USER OWNED" {
		t.Fatalf("user skill modified: %q", got)
	}
	// User skill does not appear in the summary.
	for _, e := range res.Entries {
		if strings.Contains(e.Path, "my-custom") {
			t.Fatalf("user skill leaked into Run summary: %+v", e)
		}
	}
}

func TestRunIgnoresParlayPrefixedUserSkill(t *testing.T) {
	root := mkProject(t, ".claude")
	userSkill := filepath.Join(root, ".claude", "skills", "parlay-my-custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userSkill), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(userSkill, []byte("USER OWNED PARLAY-PREFIXED"), 0o644); err != nil {
		t.Fatalf("seed user skill: %v", err)
	}
	d, logBuf := newDeployer(t, root)
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(userSkill)
	if err != nil {
		t.Fatalf("ReadFile user skill: %v", err)
	}
	if string(got) != "USER OWNED PARLAY-PREFIXED" {
		t.Fatalf("parlay-prefixed user skill modified: %q", got)
	}
	// Manifest-based ownership: a parlay-prefixed user skill that was
	// NEVER deployed by Studio is not owned by Studio, even though it
	// shares the parlay- naming convention. The deployer leaves it
	// untouched AND does not report it (no orphan entry, no WARN).
	// This prevents Studio from claiming parlay-core's parlay-* skills
	// in a real parlay project.
	for _, e := range res.Entries {
		if e.Path == userSkill {
			t.Fatalf("parlay-prefixed user skill unexpectedly reported: %+v (manifest-based ownership should ignore it)", e)
		}
	}
	if strings.Contains(logBuf.String(), userSkill) {
		t.Fatalf("parlay-prefixed user skill unexpectedly mentioned in logs; got %q", logBuf.String())
	}
}

func TestRunOrphanFromPriorVersionLeftOnDisk(t *testing.T) {
	root := mkProject(t, ".claude")
	orphan := filepath.Join(root, ".claude", "skills", "parlay-old-thing", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(orphan, []byte("STALE"), 0o644); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}
	// Seed the persisted manifest so the deployer knows it owned the
	// orphan in a prior run. Without this, the file is just a
	// user-authored file that happens to be parlay-prefixed and would
	// (correctly) be ignored.
	if err := savePersistedManifest(root, map[string]struct{}{orphan: {}}); err != nil {
		t.Fatalf("seed persisted manifest: %v", err)
	}
	d, logBuf := newDeployer(t, root)
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (orphans do not fail the run)", res.ExitCode)
	}
	foundOrphan := false
	for _, e := range res.Entries {
		if e.Path == orphan && e.Status == StatusOrphan {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Fatalf("expected orphan entry for %s; entries = %+v", orphan, res.Entries)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("orphan file unexpectedly removed: %v", err)
	}
	if !strings.Contains(logBuf.String(), "studio-deployer-orphan-detected") {
		t.Fatalf("expected orphan-detected WARN in logs; got %q", logBuf.String())
	}
}

func TestRunRenameFailureLeavesOriginalIntact(t *testing.T) {
	root := mkProject(t, ".claude")
	target := filepath.Join(root, ".claude", "skills", "parlay-design-loop", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := []byte("ORIGINAL")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatalf("seed original: %v", err)
	}
	d, _ := newDeployer(t, root)
	d.renameFn = func(_, _ string) error { return errors.New("rename injected failure") }
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero ExitCode on injected rename failure")
	}
	// Original target is byte-equal to its pre-call content.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("target modified after failed rename: got %q, want %q", got, original)
	}
	// .tmp is observable mid-run; the next run's cleanup removes it.
	if _, err := os.Stat(target + ".tmp"); err != nil {
		t.Fatalf(".tmp file missing after failed rename: %v", err)
	}
	// Re-run with rename restored — cleanup should remove the .tmp.
	d.renameFn = nil
	if _, err := d.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp file still present after recovery Run: %v", err)
	}
}

func TestRunMultiAgentFailureIsPerAgent(t *testing.T) {
	root := mkProject(t, ".claude", ".cursor")
	d, _ := newDeployer(t, root)
	// Inject a renamer that fails only for Claude targets so Cursor
	// writes proceed cleanly.
	d.renameFn = func(oldPath, newPath string) error {
		if strings.Contains(newPath, ".claude") {
			return errors.New("claude write injected failure")
		}
		return os.Rename(oldPath, newPath)
	}
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero ExitCode (any failed → non-zero)")
	}
	var claudeFailed, cursorWritten bool
	for _, e := range res.Entries {
		if strings.Contains(e.Path, ".claude") && e.Status == StatusFailed {
			claudeFailed = true
		}
		if strings.Contains(e.Path, ".cursor") && e.Status == StatusWritten {
			cursorWritten = true
		}
	}
	if !claudeFailed {
		t.Fatalf("expected at least one Claude entry to be StatusFailed; entries = %+v", res.Entries)
	}
	if !cursorWritten {
		t.Fatalf("expected at least one Cursor entry to be StatusWritten; entries = %+v", res.Entries)
	}
}

func TestRunPartialSurfaceCreatesSubdirOnWrite(t *testing.T) {
	root := mkProject(t, ".claude")
	// Explicitly DO NOT create .claude/skills/.
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills")); !os.IsNotExist(err) {
		t.Fatalf("test setup leaked: .claude/skills already exists")
	}
	d, _ := newDeployer(t, root)
	res, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", res.ExitCode)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatalf("expected .claude/skills/ to be created; got %v", err)
	}
	target := filepath.Join(root, ".claude", "skills", "parlay-design-loop", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected skill file to be written; got %v", err)
	}
}

// TestRunWithNoAgentsErrors verifies the deployer refuses to operate when
// Agents is empty (defensive — subcommands.go should not call Run without
// first detecting agents).
func TestRunWithNoAgentsErrors(t *testing.T) {
	d := &Deployer{
		ProjectRoot: t.TempDir(),
		Agents:      nil,
		SkillReader: fixtureSkillReader(t),
		SkillLister: fixtureSkillLister,
		Logger:      log.New(io.Discard, "", 0),
	}
	_, err := d.Run(context.Background())
	if err == nil {
		t.Fatalf("expected an error when Agents is empty")
	}
}
