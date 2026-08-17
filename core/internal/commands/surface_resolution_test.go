package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func surfaceFixture(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("pages: []\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func codesOf(issues []readinessIssue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

// Since v0.3 surface.md is not a surface artifact: any presence of it is a
// hard error pointing at the migration out. yaml-only and empty directories
// stay clean.
func TestSurfaceResolutionDiagnostics(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []string
	}{
		{"both present", []string{"surface.yaml", "surface.md"}, []string{"surface-md-unsupported"}},
		{"legacy only", []string{"surface.md"}, []string{"surface-md-unsupported"}},
		{"yaml only", []string{"surface.yaml"}, nil},
		{"neither", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := codesOf(surfaceResolutionIssues(surfaceFixture(t, tc.files...)))
			if len(got) != len(tc.want) {
				t.Fatalf("codes = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("codes = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// The error must block AND carry the way out: an unreadable legacy form with
// no fix text would strand exactly the projects the migration exists for.
func TestSurfaceResolutionBlocksWithFix(t *testing.T) {
	for _, files := range [][]string{{"surface.yaml", "surface.md"}, {"surface.md"}} {
		for _, issue := range surfaceResolutionIssues(surfaceFixture(t, files...)) {
			if issue.Severity != "error" {
				t.Errorf("%s severity = %q; surface.md stopped being readable in v0.3, so this must block", issue.Code, issue.Severity)
			}
			if issue.Fix == "" {
				t.Errorf("%s carries no fix — the point is pointing at migrate-spec", issue.Code)
			}
		}
	}
}
