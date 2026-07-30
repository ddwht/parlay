// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-artifact: test
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/multi-agent-target-resolution

package deployer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkProjectWithMarkers creates a tempdir-rooted project containing the
// named marker directories. Each name is a relative path that is mkdir-p
// created under the root. Returns the absolute root.
func mkProjectWithMarkers(t *testing.T, markers ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, m := range markers {
		if err := os.MkdirAll(filepath.Join(root, m), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", m, err)
		}
	}
	return root
}

func TestDetectAgentSurfacesClaudeOnly(t *testing.T) {
	root := mkProjectWithMarkers(t, ".claude")
	got, err := DetectAgentSurfaces(root)
	if err != nil {
		t.Fatalf("DetectAgentSurfaces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 detected agent; got %d (%v)", len(got), got)
	}
	if got[0].Surface != AgentClaude {
		t.Fatalf("expected AgentClaude; got %v", got[0].Surface)
	}
	want := filepath.Join(".claude", "skills", "parlay-design-loop", "SKILL.md")
	if p := got[0].SkillTargetPath("parlay-design-loop"); p != want {
		t.Fatalf("SkillTargetPath = %q, want %q", p, want)
	}
}

func TestDetectAgentSurfacesCursorOnly(t *testing.T) {
	root := mkProjectWithMarkers(t, ".cursor")
	got, err := DetectAgentSurfaces(root)
	if err != nil {
		t.Fatalf("DetectAgentSurfaces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 detected agent; got %d (%v)", len(got), got)
	}
	if got[0].Surface != AgentCursor {
		t.Fatalf("expected AgentCursor; got %v", got[0].Surface)
	}
	want := filepath.Join(".cursor", "agents", "parlay-design-loop.md")
	if p := got[0].SkillTargetPath("parlay-design-loop"); p != want {
		t.Fatalf("SkillTargetPath = %q, want %q", p, want)
	}
}

func TestDetectAgentSurfacesBothClaudeAndCursor(t *testing.T) {
	root := mkProjectWithMarkers(t, ".claude", ".cursor")
	got, err := DetectAgentSurfaces(root)
	if err != nil {
		t.Fatalf("DetectAgentSurfaces: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 detected agents; got %d (%v)", len(got), got)
	}
	if got[0].Surface != AgentClaude {
		t.Fatalf("expected Claude first; got %v", got[0].Surface)
	}
	if got[1].Surface != AgentCursor {
		t.Fatalf("expected Cursor second; got %v", got[1].Surface)
	}
}

func TestDetectAgentSurfacesGenericCLIOnly(t *testing.T) {
	root := mkProjectWithMarkers(t, ".parlay/cli")
	got, err := DetectAgentSurfaces(root)
	if err != nil {
		t.Fatalf("DetectAgentSurfaces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 detected agent; got %d (%v)", len(got), got)
	}
	if got[0].Surface != AgentGenericCLI {
		t.Fatalf("expected AgentGenericCLI; got %v", got[0].Surface)
	}
	want := filepath.Join(".parlay", "cli", "skills", "parlay-design-loop.md")
	if p := got[0].SkillTargetPath("parlay-design-loop"); p != want {
		t.Fatalf("SkillTargetPath = %q, want %q", p, want)
	}
}

func TestDetectAgentSurfacesNoSurfaceReturnsStableError(t *testing.T) {
	root := mkProjectWithMarkers(t /* no markers */)
	_, err := DetectAgentSurfaces(root)
	if err == nil {
		t.Fatalf("expected an error; got nil")
	}
	if !errors.Is(err, ErrNoAgentDetected) {
		t.Fatalf("expected ErrNoAgentDetected; got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "studio-deployer-no-agent-detected") {
		t.Fatalf("error message missing stable code; got %q", msg)
	}
	for _, surface := range []string{"Claude Code", "Cursor", "Generic CLI"} {
		if !strings.Contains(msg, surface) {
			t.Fatalf("error message missing %q; got %q", surface, msg)
		}
	}
}

// TestDetectAgentSurfacesPartialSurface — .claude/ exists but .claude/skills/
// does not. The agent is still detected; the deployer's per-agent step
// creates the subdirectory at write time.
func TestDetectAgentSurfacesPartialSurface(t *testing.T) {
	root := mkProjectWithMarkers(t, ".claude") // skills subdir intentionally absent
	got, err := DetectAgentSurfaces(root)
	if err != nil {
		t.Fatalf("DetectAgentSurfaces: %v", err)
	}
	if len(got) != 1 || got[0].Surface != AgentClaude {
		t.Fatalf("expected [AgentClaude]; got %v", got)
	}
}

// The duplicate-locally invariant that used to live here — agents.go must not
// import any core/* package — is gone with the module merge. Two independent
// reasons: the boundary only existed because Studio shipped as its own Go
// module and had to stay binary-independent of Core's API, and the guard matched
// the pre-merge module path, so after the rename it could not have caught an
// offending import even if the boundary still applied.
//
// Its sibling in domain/no_core_import_test.go went for the same reason.

func TestAgentSurfaceStrings(t *testing.T) {
	cases := map[AgentSurface]string{
		AgentClaude:     "claude",
		AgentCursor:     "cursor",
		AgentGenericCLI: "generic-cli",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Fatalf("AgentSurface(%d).String() = %q, want %q", s, got, want)
		}
	}
}
