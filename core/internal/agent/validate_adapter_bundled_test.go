// parlay-feature: parlay-tool/adapter-authoring
// parlay-artifact: test
//
// The guard that keeps strict validation honest: every adapter parlay ships or
// dogfoods must pass the complete validator. Without this, tightening a rule
// silently breaks `parlay init` for the next user.

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundledAdaptersValidateClean(t *testing.T) {
	patterns := []string{
		"../embedded/adapters/*.adapter.yaml",
		"../../../studio/.parlay/adapters/*.adapter.yaml",
		"../../.parlay/adapters/*.adapter.yaml",
		"../commands/testdata/multitarget/.parlay/adapters/*.adapter.yaml",
	}
	var checked int
	for _, pat := range patterns {
		files, err := filepath.Glob(pat)
		if err != nil {
			t.Fatalf("glob %s: %v", pat, err)
		}
		for _, f := range files {
			content, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			checked++
			for _, o := range ValidateAdapter(ModeBuild, f, content) {
				if o.Severity == SeverityError {
					t.Errorf("%s\n    [%s] %s", filepath.Base(f), o.Code, o.Message)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no adapters were checked — the glob patterns are wrong")
	}
	t.Logf("validated %d adapter files", checked)
}
