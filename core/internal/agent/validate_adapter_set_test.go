// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-kind-discriminator
// parlay-artifact: test

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupAdapterSetDir creates a temp .parlay/{adapter-set,adapters/...}.yaml
// layout the validator expects.
func setupAdapterSetDir(t *testing.T, adapterSet string, adapters map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	parlayDir := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(filepath.Join(parlayDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	asPath := filepath.Join(parlayDir, "adapter-set.yaml")
	if err := os.WriteFile(asPath, []byte(adapterSet), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range adapters {
		p := filepath.Join(parlayDir, "adapters", name+".adapter.yaml")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return asPath
}

func TestValidateAdapterSet_UnknownKind(t *testing.T) {
	asPath := setupAdapterSetDir(t,
		`name: my-app
targets:
  storage: { adapter: foo, root: src }
`, nil)
	content, _ := os.ReadFile(asPath)
	outcomes := ValidateAdapterSet(ModeBuild, asPath, content)
	if !findCode(outcomes, "adapter-kind-unknown") {
		t.Errorf("missing adapter-kind-unknown; got %+v", outcomes)
	}
}

func TestValidateAdapterSet_AdapterMissing(t *testing.T) {
	asPath := setupAdapterSetDir(t,
		`name: my-app
targets:
  presentation: { adapter: nonexistent, root: src }
`, nil)
	content, _ := os.ReadFile(asPath)
	outcomes := ValidateAdapterSet(ModeBuild, asPath, content)
	if !findCode(outcomes, "adapter-set-adapter-missing") {
		t.Errorf("missing adapter-set-adapter-missing; got %+v", outcomes)
	}
}

func TestValidateAdapterSet_KindMismatch(t *testing.T) {
	asPath := setupAdapterSetDir(t,
		`name: my-app
targets:
  application: { adapter: prisma-postgres, root: src }
`,
		map[string]string{
			"prisma-postgres": "name: prisma-postgres\nkind: persistence\n",
		})
	content, _ := os.ReadFile(asPath)
	outcomes := ValidateAdapterSet(ModeBuild, asPath, content)
	if !findCode(outcomes, "adapter-set-kind-mismatch") {
		t.Errorf("missing adapter-set-kind-mismatch; got %+v", outcomes)
	}
}

func TestValidateAdapterSet_AbsentKindDefaultsToPresentation(t *testing.T) {
	asPath := setupAdapterSetDir(t,
		`name: my-app
targets:
  presentation: { adapter: legacy, root: src }
`,
		map[string]string{
			// No kind: declared.
			"legacy": "name: legacy\nframework: \"Some Framework\"\n",
		})
	content, _ := os.ReadFile(asPath)
	outcomes := ValidateAdapterSet(ModeBuild, asPath, content)
	if findCode(outcomes, "adapter-set-kind-mismatch") {
		t.Errorf("legacy adapter without kind should default to presentation; got %+v", outcomes)
	}
}

func findCode(outcomes []ValidationOutcome, code string) bool {
	for _, o := range outcomes {
		if o.Code == code {
			return true
		}
	}
	return false
}

// findMessage returns true if any outcome's Message contains substr.
func findMessage(outcomes []ValidationOutcome, substr string) bool {
	for _, o := range outcomes {
		if strings.Contains(o.Message, substr) {
			return true
		}
	}
	return false
}
