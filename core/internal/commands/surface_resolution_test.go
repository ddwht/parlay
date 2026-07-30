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

// Both diagnostics were specified in surface.schema.md's File resolution
// section and emitted by nothing — the strings existed only as severity-table
// keys. A designer editing a superseded surface.md saw no effect and no warning.
func TestSurfaceResolutionDiagnostics(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []string
	}{
		{"both present", []string{"surface.yaml", "surface.md"}, []string{"surface-md-superseded"}},
		{"legacy only", []string{"surface.md"}, []string{"surface-md-legacy-format"}},
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

// Neither may block. surface.md is still a supported input, so a project on the
// legacy form must stay buildable — the diagnostics exist to make the migration
// discoverable, not to force it.
func TestSurfaceResolutionNeverBlocks(t *testing.T) {
	for _, files := range [][]string{{"surface.yaml", "surface.md"}, {"surface.md"}} {
		for _, issue := range surfaceResolutionIssues(surfaceFixture(t, files...)) {
			if issue.Severity == "error" {
				t.Errorf("%s is severity error; a supported legacy form must not block the build", issue.Code)
			}
			if issue.Fix == "" {
				t.Errorf("%s carries no fix — the point is pointing at migrate-spec", issue.Code)
			}
		}
	}
}
