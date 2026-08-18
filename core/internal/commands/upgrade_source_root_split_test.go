package commands

import "testing"

func TestHasFileConventionKey(t *testing.T) {
	split := []byte("kind: presentation\nfile-conventions:\n  project-root: \".\"\n  source-root: \"src/\"\n")
	legacy := []byte("kind: presentation\nfile-conventions:\n  source-root: \"src/\"\n  naming: PascalCase\n")
	// A top-level project-root, or one nested under some other block, must not
	// count: the field only means anything inside file-conventions.
	elsewhere := []byte("file-conventions:\n  source-root: \"src/\"\npackages:\n  project-root: nope\n")
	commented := []byte("file-conventions:\n  # project-root: \".\"\n  source-root: \"src/\"\n")

	for _, c := range []struct {
		name string
		in   []byte
		key  string
		want bool
	}{
		{"split declares project-root", split, "project-root", true},
		{"legacy does not", legacy, "project-root", false},
		{"both declare source-root", legacy, "source-root", true},
		{"other block does not count", elsewhere, "project-root", false},
		{"a comment does not count", commented, "project-root", false},
	} {
		if got := hasFileConventionKey(c.in, c.key); got != c.want {
			t.Errorf("%s: hasFileConventionKey(%q) = %v, want %v", c.name, c.key, got, c.want)
		}
	}
}
