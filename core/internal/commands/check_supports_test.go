// parlay-feature: parlay-tool/multi-adapter
// parlay-component: pre-codegen-support-gate-failure
// parlay-artifact: test

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSupports_PresentationOnlyShortCircuits(t *testing.T) {
	dir := setupTestDir(t)
	parlay := filepath.Join(dir, ".parlay")
	os.MkdirAll(parlay, 0o755)
	os.WriteFile(filepath.Join(parlay, "config.yaml"), []byte("sdd-framework: GitHub SpecKit\n"), 0o644)

	cmd := testCommandWithContext(t, testContext(t))
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runCheckSupports(cmd, []string{"@foo"}); err != nil {
		t.Fatalf("runCheckSupports: %v", err)
	}
	if !strings.Contains(buf.String(), "\"ready\": true") {
		t.Errorf("expected ready: true; got %s", buf.String())
	}
}

func TestCheckSupports_NoCapabilitiesFileIsNoOp(t *testing.T) {
	dir := setupTestDir(t)
	parlay := filepath.Join(dir, ".parlay")
	adapters := filepath.Join(parlay, "adapters")
	os.MkdirAll(adapters, 0o755)
	os.WriteFile(filepath.Join(parlay, "config.yaml"), []byte("sdd-framework: x\n"), 0o644)
	os.WriteFile(filepath.Join(parlay, "adapter-set.yaml"), []byte(`name: full
targets:
  presentation: { adapter: react-antd, root: src }
  application:  { adapter: nestjs-application, root: src }
links:
  - { from: presentation, relation: calls, to: application }
`), 0o644)
	os.WriteFile(filepath.Join(adapters, "react-antd.adapter.yaml"), []byte("name: react-antd\nkind: presentation\n"), 0o644)
	os.WriteFile(filepath.Join(adapters, "nestjs-application.adapter.yaml"), []byte("name: nestjs-application\nkind: application\nsupports:\n  operation_kinds: []\n  steps: []\n  policies: []\n  errors: []\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "spec", "intents", "task-list"), 0o755)

	cmd := testCommandWithContext(t, testContext(t))
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runCheckSupports(cmd, []string{"@task-list"}); err != nil {
		t.Fatalf("runCheckSupports: %v", err)
	}
	if !strings.Contains(buf.String(), "\"ready\": true") {
		t.Errorf("expected ready: true (no capabilities); got %s", buf.String())
	}
}
