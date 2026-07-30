// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-artifact: test
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/embedded-source-and-deployer-subcommands

package deployer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout swaps the package-level stdout for a buffer and returns
// a restore function the caller defers.
func captureStdout(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	prev := stdout
	stdout = &buf
	return &buf, func() { stdout = prev }
}

func setProjectRootFlag(root string) []string {
	return []string{"--project", root}
}

func TestInitInClaudeProjectWritesSkill(t *testing.T) {
	root := mkProject(t, ".claude")
	buf, restore := captureStdout(t)
	defer restore()
	if err := Init(context.Background(), setProjectRootFlag(root)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	target := filepath.Join(root, ".claude", "skills", "parlay-design-loop", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected skill written to %s; got %v", target, err)
	}
	// Stdout summary contains a "written:" line for the skill.
	if !strings.Contains(buf.String(), "written:") {
		t.Fatalf("expected 'written:' in stdout summary; got %q", buf.String())
	}
}

func TestUpgradeAfterInitIsIdempotent(t *testing.T) {
	root := mkProject(t, ".claude")
	{
		_, restore := captureStdout(t)
		if err := Init(context.Background(), setProjectRootFlag(root)); err != nil {
			restore()
			t.Fatalf("Init: %v", err)
		}
		restore()
	}
	buf, restore := captureStdout(t)
	defer restore()
	if err := Upgrade(context.Background(), setProjectRootFlag(root)); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	// Summary lines should all be "unchanged:" (no orphans in this fixture).
	out := buf.String()
	if strings.Contains(out, "written:") {
		t.Fatalf("expected zero 'written:' lines on idempotent Upgrade; got %q", out)
	}
	if !strings.Contains(out, "unchanged:") {
		t.Fatalf("expected at least one 'unchanged:' line; got %q", out)
	}
}

func TestInitWithoutParlayMarkerFails(t *testing.T) {
	root := t.TempDir()
	// Has .claude/ but NO .parlay/.
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, restore := captureStdout(t)
	defer restore()
	err := Init(context.Background(), setProjectRootFlag(root))
	if err == nil {
		t.Fatalf("expected an error; got nil")
	}
	// Either the project-root validator returns ErrProjectRootInvalid
	// (when --project is used) or our defense-in-depth check returns
	// ErrParlayNotInitialized. Both surface the same operator intent:
	// no .parlay/ at the resolved root.
	msg := err.Error()
	if !strings.Contains(msg, "studio-deployer-parlay-not-initialized") &&
		!strings.Contains(msg, "studio-config-project-root-invalid") &&
		!strings.Contains(msg, ".parlay") {
		t.Fatalf("expected the error to mention the missing .parlay/ marker; got %v", err)
	}
	// Zero files written under .claude/.
	skills := filepath.Join(root, ".claude", "skills")
	if _, err := os.Stat(skills); err == nil {
		entries, _ := os.ReadDir(skills)
		if len(entries) > 0 {
			t.Fatalf("expected zero writes; found %d entries under %s", len(entries), skills)
		}
	}
}

func TestRequireParlayMarkerStableError(t *testing.T) {
	root := t.TempDir()
	err := requireParlayMarker(root)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !errors.Is(err, ErrParlayNotInitialized) {
		t.Fatalf("expected ErrParlayNotInitialized; got %v", err)
	}
	if !strings.Contains(err.Error(), ".parlay") {
		t.Fatalf("expected error message to mention .parlay/; got %q", err.Error())
	}
}

func TestInitHelpTextMentionsRequiredPhrases(t *testing.T) {
	buf, restore := captureStdout(t)
	defer restore()
	if err := Init(context.Background(), []string{"--help"}); err != nil {
		t.Fatalf("Init --help: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Per-agent fan-out") {
		t.Fatalf("init --help missing 'Per-agent fan-out'; got %q", out)
	}
	if !strings.Contains(out, "File ownership") {
		t.Fatalf("init --help missing 'File ownership'; got %q", out)
	}
}

func TestUpgradeHelpTextMentionsRequiredPhrases(t *testing.T) {
	buf, restore := captureStdout(t)
	defer restore()
	if err := Upgrade(context.Background(), []string{"--help"}); err != nil {
		t.Fatalf("Upgrade --help: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Per-agent fan-out") {
		t.Fatalf("upgrade --help missing 'Per-agent fan-out'; got %q", out)
	}
	if !strings.Contains(out, "File ownership") {
		t.Fatalf("upgrade --help missing 'File ownership'; got %q", out)
	}
}

func TestInitHelpDoesNotWrite(t *testing.T) {
	root := mkProject(t, ".claude")
	_, restore := captureStdout(t)
	defer restore()
	if err := Init(context.Background(), []string{"--help", "--project", root}); err != nil {
		t.Fatalf("Init --help: %v", err)
	}
	skillsDir := filepath.Join(root, ".claude", "skills")
	if _, err := os.Stat(skillsDir); err == nil {
		entries, _ := os.ReadDir(skillsDir)
		if len(entries) > 0 {
			t.Fatalf("expected zero writes for --help; found %d entries under %s", len(entries), skillsDir)
		}
	}
}

// TestDeployerPackageHasNoOsExecImport — Init/Upgrade do NOT shell out to
// the parlay binary. The package source MUST NOT import os/exec or
// reference exec.LookPath.
func TestDeployerPackageHasNoOsExecImport(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	forbidden := []string{`"os/exec"`, "exec.LookPath", `exec.Command`}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for _, f := range forbidden {
			if strings.Contains(string(data), f) {
				t.Fatalf("forbidden os/exec usage %q found in %s", f, name)
			}
		}
	}
}
