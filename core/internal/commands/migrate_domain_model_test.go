// parlay-feature: studio-support/domain-model-yaml-migration
// parlay-component: md-deprecation-header
// parlay-artifact: test
//
// Tests for the deprecation-header helpers used by the
// migrate-domain-model AI skill. These are the testable surface of the
// markdown side of the migration (the YAML-emission side is exercised
// by the validator tests in agent/validate_test.go).

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasDeprecationHeader(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "empty file",
			content: "",
			want:    false,
		},
		{
			name:    "fresh markdown without header",
			content: "# Domain Model\n\nOrders have items.\n",
			want:    false,
		},
		{
			name:    "header at top",
			content: DeprecationHeader + "# Domain Model\n",
			want:    true,
		},
		{
			name:    "header after some other content (defensive)",
			content: "# Domain Model\n\n" + DeprecationHeader,
			want:    true,
		},
		{
			name:    "different blockquote not the header",
			content: "> Not the deprecation marker.\n\n# Domain Model\n",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasDeprecationHeader([]byte(tc.content)); got != tc.want {
				t.Errorf("HasDeprecationHeader(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}
}

func TestPrependDeprecationHeader(t *testing.T) {
	t.Run("adds header to fresh file", func(t *testing.T) {
		dir := t.TempDir()
		mdPath := filepath.Join(dir, "domain-model.md")
		original := "# Domain Model\n\nOrders have items.\n"
		if err := os.WriteFile(mdPath, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}

		added, err := PrependDeprecationHeader(mdPath)
		if err != nil {
			t.Fatalf("PrependDeprecationHeader: %v", err)
		}
		if !added {
			t.Fatal("expected added=true on fresh file")
		}

		got, err := os.ReadFile(mdPath)
		if err != nil {
			t.Fatal(err)
		}
		gotStr := string(got)
		if !strings.HasPrefix(gotStr, "> **Deprecated** — see [`./domain-model.yaml`]") {
			t.Errorf("file should start with the deprecation header, got: %q", gotStr[:min(len(gotStr), 80)])
		}
		if !strings.Contains(gotStr, original) {
			t.Errorf("file should preserve original content verbatim, got: %q", gotStr)
		}
	})

	t.Run("idempotent — second run leaves file untouched", func(t *testing.T) {
		dir := t.TempDir()
		mdPath := filepath.Join(dir, "domain-model.md")
		original := "# Domain Model\n\nOrders have items.\n"
		if err := os.WriteFile(mdPath, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}

		// First run: adds the header.
		if _, err := PrependDeprecationHeader(mdPath); err != nil {
			t.Fatal(err)
		}
		afterFirst, err := os.ReadFile(mdPath)
		if err != nil {
			t.Fatal(err)
		}

		// Second run: no-op.
		added, err := PrependDeprecationHeader(mdPath)
		if err != nil {
			t.Fatalf("PrependDeprecationHeader (second run): %v", err)
		}
		if added {
			t.Fatal("expected added=false on second run (header already present)")
		}

		afterSecond, err := os.ReadFile(mdPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(afterFirst) != string(afterSecond) {
			t.Errorf("second run should not modify file; before=%q after=%q",
				string(afterFirst), string(afterSecond))
		}

		// Exactly one header — testcases.yaml fixture
		// "exactly-one-deprecation-header".
		count := strings.Count(string(afterSecond), "**Deprecated** — see [`./domain-model.yaml`]")
		if count != 1 {
			t.Errorf("expected exactly one deprecation header, got %d", count)
		}
	})

	t.Run("missing source file errors cleanly", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.md")
		_, err := PrependDeprecationHeader(missing)
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
		if !strings.Contains(err.Error(), "read domain-model.md") {
			t.Errorf("error should be wrapped with verb-phrase prefix, got: %v", err)
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
