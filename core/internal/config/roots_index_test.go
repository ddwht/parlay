package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRootsIndex_Missing(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadRootsIndex(tmp)
	if err != nil {
		t.Fatalf("missing roots.yaml should not error, got: %v", err)
	}
	if idx == nil {
		t.Fatal("nil index")
	}
	if len(idx.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(idx.Children))
	}
	if idx.ParentPath != tmp {
		t.Errorf("parent path: want %s, got %s", tmp, idx.ParentPath)
	}
}

func TestRootsIndex_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	idx := &RootsIndex{
		ParentPath: tmp,
		Children: []Root{
			{Name: "web", RelativePath: "apps/web", Description: "Frontend"},
			{Name: "api", RelativePath: "apps/api"},
		},
	}
	if err := SaveRootsIndex(idx); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadRootsIndex(tmp)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(loaded.Children))
	}
	if loaded.Children[0].Name != "web" || loaded.Children[0].Description != "Frontend" {
		t.Errorf("first child mismatch: %+v", loaded.Children[0])
	}
	if loaded.Children[1].Name != "api" {
		t.Errorf("second child name: %s", loaded.Children[1].Name)
	}
	// Path is computed during load, not stored on disk.
	wantWebPath := filepath.Join(tmp, "apps", "web")
	if loaded.Children[0].Path != wantWebPath {
		t.Errorf("path: want %s, got %s", wantWebPath, loaded.Children[0].Path)
	}
}

func TestAppendRootToIndex_Conflicts(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	idx := &RootsIndex{
		ParentPath: tmp,
		Children:   []Root{{Name: "web", RelativePath: "apps/web"}},
	}
	// Same name → error.
	_, err := AppendRootToIndex(idx, Root{Name: "web", RelativePath: "elsewhere"})
	if err == nil {
		t.Errorf("expected name conflict error")
	}
	// Same path → error.
	_, err = AppendRootToIndex(idx, Root{Name: "different", RelativePath: "apps/web"})
	if err == nil {
		t.Errorf("expected path conflict error")
	}
	// Fresh entry → ok.
	if _, err := AppendRootToIndex(idx, Root{Name: "api", RelativePath: "apps/api"}); err != nil {
		t.Errorf("clean append failed: %v", err)
	}
}

func TestRootsIndex_LookupAndNames(t *testing.T) {
	idx := &RootsIndex{Children: []Root{
		{Name: "web"}, {Name: "api"},
	}}
	if r, ok := idx.Lookup("web"); !ok || r.Name != "web" {
		t.Errorf("lookup miss: %+v ok=%v", r, ok)
	}
	if _, ok := idx.Lookup("missing"); ok {
		t.Errorf("expected miss")
	}
	names := idx.Names()
	if len(names) != 2 || names[0] != "web" || names[1] != "api" {
		t.Errorf("names: %v", names)
	}
}

func TestParentPointerRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	child := filepath.Join(parent, "apps", "web")
	if err := os.MkdirAll(filepath.Join(parent, ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(child, ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ParlayDir, ConfigFile), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, ParlayDir, ConfigFile), []byte("ai-agent: claude\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteParentPointer(child, parent); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readParentPointer(child)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	wantClean := filepath.Clean(parent)
	if got != wantClean {
		t.Errorf("parent: want %s, got %s", wantClean, got)
	}

	// Verify the unrelated field survived.
	data, err := os.ReadFile(filepath.Join(child, ParlayDir, ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(data, "ai-agent: claude") {
		t.Errorf("ai-agent field lost: %s", string(data))
	}

	// Remove and verify gone.
	if err := RemoveParentPointer(child); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, _ = readParentPointer(child)
	if got != "" {
		t.Errorf("parent still present after remove: %s", got)
	}
}

func contains(haystack []byte, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == needle {
			return true
		}
	}
	return false
}
