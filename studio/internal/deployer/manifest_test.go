// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-artifact: test
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

package deployer

import (
	"bytes"
	"path/filepath"
	"testing"
)

// fakeRead returns canned content for a fixed set of skills. The hash of
// the returned content is what the manifest captures.
func fakeRead(t *testing.T) func(string) ([]byte, error) {
	t.Helper()
	contents := map[string][]byte{
		"parlay-design-loop": []byte("---\nname: parlay-design-loop\ndescription: x\n---\nbody-A\n"),
		"parlay-another":     []byte("---\nname: parlay-another\ndescription: y\n---\nbody-B\n"),
	}
	return func(slug string) ([]byte, error) {
		c, ok := contents[slug]
		if !ok {
			t.Fatalf("fakeRead: unexpected slug %q", slug)
		}
		return c, nil
	}
}

func TestDeriveManifestSingleSkillSingleAgent(t *testing.T) {
	agents := []AgentTarget{
		{
			Surface:         AgentClaude,
			SkillTargetPath: func(slug string) string { return filepath.Join(".claude", "skills", slug, "SKILL.md") },
		},
	}
	m, err := DeriveManifest([]string{"parlay-design-loop"}, agents, "/tmp/fixture", fakeRead(t))
	if err != nil {
		t.Fatalf("DeriveManifest: %v", err)
	}
	if len(m) != 1 {
		t.Fatalf("expected 1 entry; got %d", len(m))
	}
	wantPath := filepath.Join("/tmp/fixture", ".claude", "skills", "parlay-design-loop", "SKILL.md")
	if m[0].TargetPath != wantPath {
		t.Fatalf("TargetPath = %q, want %q", m[0].TargetPath, wantPath)
	}
}

func TestDeriveManifestTwoSkillsTwoAgents(t *testing.T) {
	agents := []AgentTarget{
		{Surface: AgentClaude, SkillTargetPath: func(slug string) string { return filepath.Join(".claude", "skills", slug, "SKILL.md") }},
		{Surface: AgentCursor, SkillTargetPath: func(slug string) string { return filepath.Join(".cursor", "agents", slug+".md") }},
	}
	// Skills passed in lexicographic order so the manifest order is
	// "another" entries first, "design-loop" entries second; within each
	// skill, Claude precedes Cursor.
	m, err := DeriveManifest([]string{"parlay-another", "parlay-design-loop"}, agents, "/root", fakeRead(t))
	if err != nil {
		t.Fatalf("DeriveManifest: %v", err)
	}
	if len(m) != 4 {
		t.Fatalf("expected 4 entries; got %d", len(m))
	}
	wantOrder := []string{
		filepath.Join("/root", ".claude", "skills", "parlay-another", "SKILL.md"),
		filepath.Join("/root", ".cursor", "agents", "parlay-another.md"),
		filepath.Join("/root", ".claude", "skills", "parlay-design-loop", "SKILL.md"),
		filepath.Join("/root", ".cursor", "agents", "parlay-design-loop.md"),
	}
	for i, w := range wantOrder {
		if m[i].TargetPath != w {
			t.Fatalf("manifest[%d].TargetPath = %q, want %q", i, m[i].TargetPath, w)
		}
	}
}

func TestDeriveManifestDeterminism(t *testing.T) {
	agents := []AgentTarget{
		{Surface: AgentClaude, SkillTargetPath: func(slug string) string { return filepath.Join(".claude", "skills", slug, "SKILL.md") }},
	}
	m1, err := DeriveManifest([]string{"parlay-design-loop"}, agents, "/root", fakeRead(t))
	if err != nil {
		t.Fatalf("DeriveManifest (first): %v", err)
	}
	m2, err := DeriveManifest([]string{"parlay-design-loop"}, agents, "/root", fakeRead(t))
	if err != nil {
		t.Fatalf("DeriveManifest (second): %v", err)
	}
	if len(m1) != len(m2) {
		t.Fatalf("manifest length differs across derivations: %d vs %d", len(m1), len(m2))
	}
	for i := range m1 {
		if m1[i].SkillSlug != m2[i].SkillSlug ||
			m1[i].Agent != m2[i].Agent ||
			m1[i].TargetPath != m2[i].TargetPath ||
			!bytes.Equal(m1[i].SourceBytes, m2[i].SourceBytes) ||
			m1[i].SourceHash != m2[i].SourceHash {
			t.Fatalf("manifest entry %d differs across derivations", i)
		}
	}
}

func TestManifestPathSet(t *testing.T) {
	agents := []AgentTarget{
		{Surface: AgentClaude, SkillTargetPath: func(slug string) string { return filepath.Join(".claude", "skills", slug, "SKILL.md") }},
	}
	m, err := DeriveManifest([]string{"parlay-design-loop"}, agents, "/root", fakeRead(t))
	if err != nil {
		t.Fatalf("DeriveManifest: %v", err)
	}
	set := m.PathSet()
	if len(set) != 1 {
		t.Fatalf("expected 1 path; got %d", len(set))
	}
	want := filepath.Join("/root", ".claude", "skills", "parlay-design-loop", "SKILL.md")
	if _, ok := set[want]; !ok {
		t.Fatalf("path set missing %q; got %v", want, set)
	}
}
