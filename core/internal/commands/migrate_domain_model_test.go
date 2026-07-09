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
	"bytes"
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

// TestFindDomainModelFilesOutsideRoot_FindsFeatureLevelFiles is the
// regression test for the Phase 4 finding: a legacy project-wide
// domain-model.md was discovered living inside a feature-shaped
// directory (spec/intents/parlay-tool/domain-model/domain-model.md) —
// migrate-domain-model only ever looked at <activeRoot>/domain-model.md
// and was structurally blind to this. The probe must find it (and any
// other feature-level domain-model.md), without claiming to migrate it.
func TestFindDomainModelFilesOutsideRoot_FindsFeatureLevelFiles(t *testing.T) {
	setupTestDir(t)
	cfg := testContext(t)

	featureDir := cfg.FeaturePath("parlay-tool/domain-model")
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(featureDir, "domain-model.md")
	if err := os.WriteFile(stray, []byte("# Whole Project Model\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte("# X\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// A feature with no domain-model.md at all — must not show up.
	other := cfg.FeaturePath("other-feature")
	os.MkdirAll(other, 0755)
	os.WriteFile(filepath.Join(other, "intents.md"), []byte("# Y\n"), 0644)

	found, err := findDomainModelFilesOutsideRoot(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != stray {
		t.Errorf("found = %v, want [%s]", found, stray)
	}
}

// TestRunMigrateDomainModel_ReportsDomainModelFilesElsewhere confirms
// the probe's findings surface in the command's actual output, on the
// greenfield path (no root-level domain-model.md/.yaml at all) — the
// case that would otherwise print only "nothing to migrate" and leave
// the designer unaware a stray domain-model.md exists elsewhere.
func TestRunMigrateDomainModel_ReportsDomainModelFilesElsewhere(t *testing.T) {
	setupTestDir(t)
	cfg := testContext(t)

	featureDir := cfg.FeaturePath("parlay-tool/domain-model")
	os.MkdirAll(featureDir, 0755)
	os.WriteFile(filepath.Join(featureDir, "domain-model.md"), []byte("# Whole Project Model\n"), 0644)
	os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte("# X\n"), 0644)

	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runMigrateDomainModel(cmd, nil); err != nil {
		t.Fatalf("runMigrateDomainModel: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "found 1 domain-model.md file(s)") {
		t.Errorf("expected the probe's finding reported, got: %s", out)
	}
	if !strings.Contains(out, filepath.Join(featureDir, "domain-model.md")) {
		t.Errorf("expected the stray file's path named, got: %s", out)
	}
	// The greenfield behavior itself must be unchanged.
	if !strings.Contains(out, "nothing to migrate") {
		t.Errorf("expected greenfield 'nothing to migrate' still reported, got: %s", out)
	}
}
