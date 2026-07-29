// parlay-feature: parlay-tool/multi-adapter
// parlay-component: config-migration-result
// parlay-artifact: test

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateConfig_WritesPresentationOnlyAdapterSet(t *testing.T) {
	dir := setupTestDir(t)
	parlayDir := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(parlayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parlayDir, "config.yaml"),
		[]byte("sdd-framework: GitHub SpecKit\nprototype-framework: Go CLI\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := testCommandWithContext(t, testContext(t))
	if err := runMigrateConfig(cmd, nil); err != nil {
		t.Fatalf("runMigrateConfig: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(parlayDir, "adapter-set.yaml"))
	if err != nil {
		t.Fatalf("read adapter-set: %v", err)
	}
	if !strings.Contains(string(content), "presentation:") {
		t.Errorf("missing presentation slot; got:\n%s", content)
	}
	if !strings.Contains(string(content), "go-cli") {
		t.Errorf("missing go-cli adapter slug; got:\n%s", content)
	}
}

func TestMigrateConfig_NoLegacyFieldIsNoOp(t *testing.T) {
	dir := setupTestDir(t)
	parlayDir := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(parlayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parlayDir, "config.yaml"),
		[]byte("sdd-framework: GitHub SpecKit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := testCommandWithContext(t, testContext(t))
	if err := runMigrateConfig(cmd, nil); err != nil {
		t.Fatalf("runMigrateConfig: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parlayDir, "adapter-set.yaml")); !os.IsNotExist(err) {
		t.Error("adapter-set.yaml should not be written when no legacy field exists")
	}
}

func TestMigrateConfig_Idempotent(t *testing.T) {
	dir := setupTestDir(t)
	parlayDir := filepath.Join(dir, ".parlay")
	os.MkdirAll(parlayDir, 0o755)
	os.WriteFile(filepath.Join(parlayDir, "config.yaml"),
		[]byte("prototype-framework: Go CLI\n"), 0o644)
	existing := []byte("name: existing\n")
	os.WriteFile(filepath.Join(parlayDir, "adapter-set.yaml"), existing, 0o644)

	cmd := testCommandWithContext(t, testContext(t))
	if err := runMigrateConfig(cmd, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(parlayDir, "adapter-set.yaml"))
	if string(got) != string(existing) {
		t.Errorf("adapter-set.yaml was overwritten on second run; got %q", got)
	}
}

func TestSlugifyFramework(t *testing.T) {
	cases := map[string]string{
		"Go CLI":                "go-cli",
		"React + Ant Design":    "react-antd",
		"Angular + Clarity":     "angular-clarity",
		"Some Custom Framework": "some-custom-framework",
	}
	for input, want := range cases {
		if got := slugifyFramework(input); got != want {
			t.Errorf("slugifyFramework(%q): got %q, want %q", input, got, want)
		}
	}
}
